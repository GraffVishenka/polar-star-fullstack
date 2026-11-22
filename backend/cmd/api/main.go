// Команда api — точка входа backend'а: конфигурация, миграции, сидинг,
// HTTP-сервер и фоновые воркеры (NFR-4, NFR-5).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// Встроенная база IANA-поясов: CAL-2 не должен зависеть от наличия tzdata
	// в контейнере (образ alpine ставит tzdata, но это гарантия и для distroless).
	_ "time/tzdata"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/buildings"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/db"
	"github.com/graffvishenka/polar-star-service/backend/internal/dictionaries"
	"github.com/graffvishenka/polar-star-service/backend/internal/equipment"
	"github.com/graffvishenka/polar-star-service/backend/internal/files"
	"github.com/graffvishenka/polar-star-service/backend/internal/httpserver"
	"github.com/graffvishenka/polar-star-service/backend/internal/kb"
	"github.com/graffvishenka/polar-star-service/backend/internal/notifications"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/settings"
	"github.com/graffvishenka/polar-star-service/backend/internal/tickets"
	"github.com/graffvishenka/polar-star-service/backend/internal/users"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// run — общий сценарий запуска с поддержкой подкоманд migrate/seed/version.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("конфигурация: %w", err)
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)
	logger.Info("api: запуск",
		"env", cfg.AppEnv, "version", cfg.Version,
		"timezone", cfg.Timezone, "addr", cfg.HTTPAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := "serve"
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		command = strings.ToLower(strings.TrimSpace(os.Args[1]))
	}

	switch command {
	case "serve":
		return runServer(ctx, cfg, logger)
	case "migrate":
		return db.Migrate(ctx, cfg.DSN(true), logger)
	case "seed":
		return runMigrateAndSeed(ctx, cfg, logger)
	case "version":
		fmt.Println(cfg.Version)
		return nil
	default:
		return fmt.Errorf("неизвестная команда %q (ожидается serve|migrate|seed|version)", command)
	}
}

// runMigrateAndSeed применяет миграции и идемпотентный сидинг (make seed).
func runMigrateAndSeed(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	if err := db.Migrate(ctx, cfg.DSN(true), logger); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DSN(false), cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	if err != nil {
		return err
	}
	defer func() { _ = pool.Close() }()

	return db.Seed(ctx, pool, logger, db.SeedOptions{
		AdminLogin:    cfg.SeedAdminLogin,
		AdminFullName: cfg.SeedAdminFullName,
		AdminPassword: cfg.SeedAdminPassword,
	})
}

// runServer поднимает API и воркеры, применяет миграции/сидинг при старте.
func runServer(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	if err := cfg.ValidateAddr(); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o750); err != nil {
		return fmt.Errorf("каталог загрузок %q: %w", cfg.UploadDir, err)
	}

	pool, err := db.Open(ctx, cfg.DSN(false), cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	if err != nil {
		return err
	}
	defer func() { _ = pool.Close() }()

	// MySQL в docker-compose может стартовать дольше API.
	if err := db.WaitReady(ctx, pool, 90*time.Second, logger); err != nil {
		return err
	}
	if err := db.Migrate(ctx, cfg.DSN(true), logger); err != nil {
		return err
	}
	if err := db.Seed(ctx, pool, logger, db.SeedOptions{
		AdminLogin:    cfg.SeedAdminLogin,
		AdminFullName: cfg.SeedAdminFullName,
		AdminPassword: cfg.SeedAdminPassword,
	}); err != nil {
		return err
	}

	// Платформенные зависимости: реальные часы. Календарь и push-настройки
	// берутся из Б7 (settings.CalendarProvider) ниже.
	clock := platform.RealClock{}

	// Б7: настройки, праздники и рабочий календарь берутся из БД. Ошибка чтения
	// не блокирует старт API: остаются значения по умолчанию (Пн–Сб,
	// Asia/Yekaterinburg, 10:00), а календарь пересоберётся при следующем
	// изменении настроек (CAL-5).
	settingsService := settings.NewService(
		settings.NewMySQLRepository(pool), clock, logger, settings.WithLocation(cfg.Location))
	if err := settingsService.Reload(ctx); err != nil {
		logger.Warn("settings: настройки недоступны, используются значения по умолчанию", "error", err)
	}
	calendar := settingsService.Provider()

	// Б8: push-уведомления. Без VAPID-ключей транспорт переходит в no-op
	// (NOTIF-1): API и подписки работают, но события не создаются. Настройки
	// push (push_enabled, deadline_reminder_time, timezone) — из Б7 через
	// pushSettingsAdapter; до готовности БД провайдер отдаёт дефолты.
	notificationsRepository := notifications.NewMySQLRepository(pool)
	pushSender := notifications.NewWebPushSender(cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey, cfg.VAPIDSubject)
	if !pushSender.Enabled() {
		logger.Warn("push: VAPID_PUBLIC_KEY/VAPID_PRIVATE_KEY не заданы — уведомления отключены")
	}
	pushSettings := pushSettingsAdapter{provider: settingsService.Provider()}
	notifier := notifications.NewDispatcher(notificationsRepository, pushSettings, clock, logger, pushSender.Enabled())
	notificationsService := notifications.NewService(notificationsRepository, pushSender.PublicKey(),
		pushSender.Enabled(), clock, logger)
	notificationsHandler := notifications.NewHandler(notificationsService, logger)

	repository := auth.NewMySQLRepository(pool)
	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL, clock)
	authService := auth.NewService(repository, tokenManager, clock, logger, auth.Options{
		RefreshTTL:  cfg.RefreshTokenTTL,
		LoginLimit:  10, // AUTH-3: 10 неудачных входов на login+IP за 10 минут
		LoginWindow: 10 * time.Minute,
	})
	authHandler := auth.NewHandler(authService, logger, cfg.IsProduction())
	authenticator := auth.NewAuthenticator(repository, tokenManager)

	// Б2: пользователи, роли и здание пользователя. Отзыв сессий при
	// блокировке/сбросе пароля использует тот же auth.Repository.
	usersRepository := users.NewMySQLRepository(pool)
	usersService := users.NewService(usersRepository, repository, users.NewMySQLTicketResolver(pool), logger)
	usersHandler := users.NewHandler(usersService, logger)

	// Б3: файлы и медиа — локальный диск (UPLOAD_DIR), метаданные в MySQL,
	// определение MIME по сигнатуре и лимиты 10 МБ/200 МБ.
	filesRepo := files.NewMySQLRepository(pool)
	filesStore := files.NewStorage(cfg.UploadDir, cfg.Location, clock)
	filesService := files.NewService(filesRepo, filesStore, files.Options{
		Clock:          clock,
		Logger:         logger,
		MaxUploadBytes: cfg.MaxUploadBytes,
	})
	filesHandler := files.NewHandler(filesService, logger)

	// Б4: здания, склады, оборудование и журнал перемещений (EQ-1..EQ-7).
	buildingsHandler := buildings.NewHandler(
		buildings.NewService(buildings.NewMySQLRepository(pool)), logger)
	equipmentHandler := equipment.NewHandler(
		equipment.NewService(equipment.NewMySQLRepository(pool)), logger)

	// Б7: настройки/праздники (CAL-1..CAL-3) и справочники видов проблем и
	// типов оборудования.
	settingsHandler := settings.NewHandler(settingsService, logger)
	dictionariesHandler := dictionaries.NewHandler(
		dictionaries.NewService(dictionaries.NewMySQLRepository(pool)), logger)

	// Б5: заявки. Настройки заявок берутся из settings-сервиса (Б7): срок
	// дедлайна, календарь и названия/цвета статусов (TICKET-1, TICKET-3).
	// Адаптер объявлен ниже, чтобы домены settings и tickets не зависели
	// друг от друга.
	ticketsRepository := tickets.NewMySQLRepository(pool)
	ticketsService := tickets.NewService(ticketsRepository, usersRepository,
		ticketSettingsAdapter{provider: settingsService.Provider()},
		clock, notifier, tickets.NewFilesAttacher(filesService), logger)
	ticketsHandler := tickets.NewHandler(ticketsService, logger)

	// Б5 подключает проверки доступа к файлам заявок (ТЗ TICKET-7/TICKET-13):
	// публичные вложения — по токену заявки, авторизованные — по её зданию.
	// Без этих авторизаторов файлы заявок были бы доступны/недоступны неверно.
	filesService.SetTicketAuthorizer(tickets.NewTicketTokenAuthorizer(ticketsRepository))
	filesService.SetFileAccessAuthorizer(tickets.NewFileAccessAuthorizer(ticketsRepository, ticketsRepository))

	// Б6: база знаний — дерево, санитизация HTML, медиа статей и публичная
	// выдача медиа (KB-1..KB-8). kbService также реализует
	// files.KBMediaAuthorizer для `GET /api/public/files/{id}` без токена.
	kbService := kb.NewService(kb.NewMySQLRepository(pool),
		kb.NewFilesMediaValidator(filesService), kb.Options{Clock: clock, Logger: logger})
	kbHandler := kb.NewHandler(kbService, logger)
	filesService.SetKBMediaAuthorizer(kbService)

	httpServer := httpserver.New(httpserver.Deps{
		Config:               cfg,
		Logger:               logger,
		DB:                   pool,
		Clock:                clock,
		Calendar:             calendar,
		Notifier:             notifier,
		AuthService:          authService,
		AuthHandler:          authHandler,
		Authenticator:        authenticator,
		UsersHandler:         usersHandler,
		FilesHandler:         filesHandler,
		BuildingsHandler:     buildingsHandler,
		EquipmentHandler:     equipmentHandler,
		TicketsHandler:       ticketsHandler,
		PublicTicketsHandler: ticketsHandler,
		KBHandler:            kbHandler,
		SettingsHandler:      settingsHandler,
		DictionariesHandler:  dictionariesHandler,
		PushHandler:          notificationsHandler,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// ReadTimeout/WriteTimeout не задаются: выгрузка видео до 200 МБ (Б3).
		IdleTimeout: 120 * time.Second,
	}

	// Воркеры-заглушки для ещё не реализованных блоков (корзина — Б10);
	// outbox и напоминания Б8 запускаются отдельным worker'ом ниже.
	workerCtx, stopWorkers := context.WithCancel(ctx)
	workers := startWorkers(workerCtx, logger)
	defer func() {
		stopWorkers()
		workers.Wait()
	}()

	// Б3: раз в час удаляются файлы без привязки (linked_at IS NULL) старше
	// 24 часов; ошибки очистки только логируются и не влияют на API.
	go files.NewCleaner(filesRepo, filesStore, logger, clock, files.CleanerOptions{}).Run(workerCtx)

	// Б8: каждые 30 секунд отправляется outbox и проверяется время ежедневных
	// напоминаний; повторный запуск за день не дублирует доставку (NOTIF-7).
	notificationWorker := notifications.NewWorker(notificationsRepository, pushSender, pushSettings,
		notifier, clock, logger)
	go notificationWorker.Run(workerCtx)

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("api: HTTP-сервер слушает", "addr", cfg.HTTPAddr)
		serveErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("api: получен сигнал остановки")
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("api: HTTP-сервер: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("api: graceful shutdown: %w", err)
	}
	logger.Info("api: остановлен")
	return nil
}

// newLogger — JSON-логи в stdout с уровнем из LOG_LEVEL (NFR-4).
func newLogger(cfg *config.Config) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	return slog.New(handler).With("service", "polar-star-api", "env", cfg.AppEnv)
}

// ticketSettingsAdapter связывает настройки Б7 с доменом заявок (Б5):
// срок дедлайна и календарь берутся из CalendarProvider (актуальный снапшот
// настроек), названия и цвета статусов — из тех же настроек (TICKET-1).
// Адаптер объявлен в cmd/api, чтобы домены settings и tickets не зависели
// друг от друга; при недоступной БД провайдер отдаёт значения по умолчанию.
type ticketSettingsAdapter struct {
	provider *settings.CalendarProvider
}

// DeadlineDays — N рабочих дней для дедлайна (TICKET-3).
func (a ticketSettingsAdapter) DeadlineDays() int { return a.provider.DeadlineDays() }

// Calendar — рабочий календарь с маской и праздниками из настроек (CAL-1).
func (a ticketSettingsAdapter) Calendar() platform.Calendar { return a.provider.Calendar() }

// StatusStyle — настраиваемые название и цвет статуса; пустые значения
// означают «использовать дефолт домена заявок».
func (a ticketSettingsAdapter) StatusStyle(status tickets.Status) (string, string) {
	return a.provider.StatusLabel(string(status)), a.provider.StatusColor(string(status))
}

// pushSettingsAdapter связывает настройки Б7 с уведомлениями Б8: push_enabled,
// deadline_reminder_time и timezone берутся из актуального снапшота
// CalendarProvider. Адаптер объявлен в cmd/api, чтобы домены settings и
// notifications не зависели друг от друга.
type pushSettingsAdapter struct {
	provider *settings.CalendarProvider
}

// Enabled — push_enabled (NOTIF-6).
func (a pushSettingsAdapter) Enabled() bool { return a.provider.PushEnabled() }

// ReminderTime — deadline_reminder_time в формате HH:MM (CAL-3).
func (a pushSettingsAdapter) ReminderTime() string { return a.provider.ReminderTime() }

// Timezone — часовой пояс приложения (CAL-2).
func (a pushSettingsAdapter) Timezone() *time.Location { return a.provider.Location() }

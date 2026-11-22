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
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/db"
	"github.com/graffvishenka/polar-star-service/backend/internal/httpserver"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
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

	// Платформенные зависимости: реальные часы, календарь по умолчанию (маска
	// Пн–Сб, без праздников), заглушка Notifier до Б8. Б7 заменит календарь на
	// настройки из БД через тот же интерфейс.
	clock := platform.RealClock{}
	calendar := platform.NewDefaultCalendar(cfg.Location, platform.DefaultWorkdays(), nil, clock)
	notifier := &platform.LogNotifier{Logger: logger}

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

	httpServer := httpserver.New(httpserver.Deps{
		Config:        cfg,
		Logger:        logger,
		DB:            pool,
		Clock:         clock,
		Calendar:      calendar,
		Notifier:      notifier,
		AuthService:   authService,
		AuthHandler:   authHandler,
		Authenticator: authenticator,
		UsersHandler:  usersHandler,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// ReadTimeout/WriteTimeout не задаются: выгрузка видео до 200 МБ (Б3).
		IdleTimeout: 120 * time.Second,
	}

	// Воркеры-заглушки (outbox, напоминания, автоочистка) отключаются вместе с
	// контекстом; реальные реализации появятся в Б8/Б10.
	workerCtx, stopWorkers := context.WithCancel(ctx)
	workers := startWorkers(workerCtx, logger, calendar)
	defer func() {
		stopWorkers()
		workers.Wait()
	}()

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

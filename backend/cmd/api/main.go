// РљРѕРјР°РЅРґР° api вЂ” С‚РѕС‡РєР° РІС…РѕРґР° backend'Р°: РєРѕРЅС„РёРіСѓСЂР°С†РёСЏ, РјРёРіСЂР°С†РёРё, СЃРёРґРёРЅРі,
// HTTP-СЃРµСЂРІРµСЂ Рё С„РѕРЅРѕРІС‹Рµ РІРѕСЂРєРµСЂС‹ (NFR-4, NFR-5).
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

	// Р’СЃС‚СЂРѕРµРЅРЅР°СЏ Р±Р°Р·Р° IANA-РїРѕСЏСЃРѕРІ: CAL-2 РЅРµ РґРѕР»Р¶РµРЅ Р·Р°РІРёСЃРµС‚СЊ РѕС‚ РЅР°Р»РёС‡РёСЏ tzdata
	// РІ РєРѕРЅС‚РµР№РЅРµСЂРµ (РѕР±СЂР°Р· alpine СЃС‚Р°РІРёС‚ tzdata, РЅРѕ СЌС‚Рѕ РіР°СЂР°РЅС‚РёСЏ Рё РґР»СЏ distroless).
	_ "time/tzdata"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/buildings"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/db"
	"github.com/graffvishenka/polar-star-service/backend/internal/equipment"
	"github.com/graffvishenka/polar-star-service/backend/internal/files"
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

// run вЂ” РѕР±С‰РёР№ СЃС†РµРЅР°СЂРёР№ Р·Р°РїСѓСЃРєР° СЃ РїРѕРґРґРµСЂР¶РєРѕР№ РїРѕРґРєРѕРјР°РЅРґ migrate/seed/version.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("РєРѕРЅС„РёРіСѓСЂР°С†РёСЏ: %w", err)
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)
	logger.Info("api: Р·Р°РїСѓСЃРє",
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
		return fmt.Errorf("РЅРµРёР·РІРµСЃС‚РЅР°СЏ РєРѕРјР°РЅРґР° %q (РѕР¶РёРґР°РµС‚СЃСЏ serve|migrate|seed|version)", command)
	}
}

// runMigrateAndSeed РїСЂРёРјРµРЅСЏРµС‚ РјРёРіСЂР°С†РёРё Рё РёРґРµРјРїРѕС‚РµРЅС‚РЅС‹Р№ СЃРёРґРёРЅРі (make seed).
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

// runServer РїРѕРґРЅРёРјР°РµС‚ API Рё РІРѕСЂРєРµСЂС‹, РїСЂРёРјРµРЅСЏРµС‚ РјРёРіСЂР°С†РёРё/СЃРёРґРёРЅРі РїСЂРё СЃС‚Р°СЂС‚Рµ.
func runServer(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	if err := cfg.ValidateAddr(); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o750); err != nil {
		return fmt.Errorf("РєР°С‚Р°Р»РѕРі Р·Р°РіСЂСѓР·РѕРє %q: %w", cfg.UploadDir, err)
	}

	pool, err := db.Open(ctx, cfg.DSN(false), cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	if err != nil {
		return err
	}
	defer func() { _ = pool.Close() }()

	// MySQL РІ docker-compose РјРѕР¶РµС‚ СЃС‚Р°СЂС‚РѕРІР°С‚СЊ РґРѕР»СЊС€Рµ API.
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

	// РџР»Р°С‚С„РѕСЂРјРµРЅРЅС‹Рµ Р·Р°РІРёСЃРёРјРѕСЃС‚Рё: СЂРµР°Р»СЊРЅС‹Рµ С‡Р°СЃС‹, РєР°Р»РµРЅРґР°СЂСЊ РїРѕ СѓРјРѕР»С‡Р°РЅРёСЋ (РјР°СЃРєР°
	// РџРЅвЂ“РЎР±, Р±РµР· РїСЂР°Р·РґРЅРёРєРѕРІ), Р·Р°РіР»СѓС€РєР° Notifier РґРѕ Р‘8. Р‘7 Р·Р°РјРµРЅРёС‚ РєР°Р»РµРЅРґР°СЂСЊ РЅР°
	// РЅР°СЃС‚СЂРѕР№РєРё РёР· Р‘Р” С‡РµСЂРµР· С‚РѕС‚ Р¶Рµ РёРЅС‚РµСЂС„РµР№СЃ.
	clock := platform.RealClock{}
	calendar := platform.NewDefaultCalendar(cfg.Location, platform.DefaultWorkdays(), nil, clock)
	notifier := &platform.LogNotifier{Logger: logger}

	repository := auth.NewMySQLRepository(pool)
	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL, clock)
	authService := auth.NewService(repository, tokenManager, clock, logger, auth.Options{
		RefreshTTL:  cfg.RefreshTokenTTL,
		LoginLimit:  10, // AUTH-3: 10 РЅРµСѓРґР°С‡РЅС‹С… РІС…РѕРґРѕРІ РЅР° login+IP Р·Р° 10 РјРёРЅСѓС‚
		LoginWindow: 10 * time.Minute,
	})
	authHandler := auth.NewHandler(authService, logger, cfg.IsProduction())
	authenticator := auth.NewAuthenticator(repository, tokenManager)

	// Р‘2: РїРѕР»СЊР·РѕРІР°С‚РµР»Рё, СЂРѕР»Рё Рё Р·РґР°РЅРёРµ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ. РћС‚Р·С‹РІ СЃРµСЃСЃРёР№ РїСЂРё
	// Р±Р»РѕРєРёСЂРѕРІРєРµ/СЃР±СЂРѕСЃРµ РїР°СЂРѕР»СЏ РёСЃРїРѕР»СЊР·СѓРµС‚ С‚РѕС‚ Р¶Рµ auth.Repository.
	usersRepository := users.NewMySQLRepository(pool)
	usersService := users.NewService(usersRepository, repository, users.NewMySQLTicketResolver(pool), logger)
	usersHandler := users.NewHandler(usersService, logger)

	// Р‘3: С„Р°Р№Р»С‹ Рё РјРµРґРёР° вЂ” Р»РѕРєР°Р»СЊРЅС‹Р№ РґРёСЃРє (UPLOAD_DIR), РјРµС‚Р°РґР°РЅРЅС‹Рµ РІ MySQL,
	// РѕРїСЂРµРґРµР»РµРЅРёРµ MIME РїРѕ СЃРёРіРЅР°С‚СѓСЂРµ Рё Р»РёРјРёС‚С‹ 10 РњР‘/200 РњР‘.
	filesRepo := files.NewMySQLRepository(pool)
	filesStore := files.NewStorage(cfg.UploadDir, cfg.Location, clock)
	filesService := files.NewService(filesRepo, filesStore, files.Options{
		Clock:          clock,
		Logger:         logger,
		MaxUploadBytes: cfg.MaxUploadBytes,
	})
	filesHandler := files.NewHandler(filesService, logger)

	// Р‘4: Р·РґР°РЅРёСЏ, СЃРєР»Р°РґС‹, РѕР±РѕСЂСѓРґРѕРІР°РЅРёРµ Рё Р¶СѓСЂРЅР°Р» РїРµСЂРµРјРµС‰РµРЅРёР№ (EQ-1..EQ-7).
	buildingsHandler := buildings.NewHandler(
		buildings.NewService(buildings.NewMySQLRepository(pool)), logger)
	equipmentHandler := equipment.NewHandler(
		equipment.NewService(equipment.NewMySQLRepository(pool)), logger)

	httpServer := httpserver.New(httpserver.Deps{
		Config:           cfg,
		Logger:           logger,
		DB:               pool,
		Clock:            clock,
		Calendar:         calendar,
		Notifier:         notifier,
		AuthService:      authService,
		AuthHandler:      authHandler,
		Authenticator:    authenticator,
		UsersHandler:     usersHandler,
		FilesHandler:     filesHandler,
		BuildingsHandler: buildingsHandler,
		EquipmentHandler: equipmentHandler,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// ReadTimeout/WriteTimeout РЅРµ Р·Р°РґР°СЋС‚СЃСЏ: РІС‹РіСЂСѓР·РєР° РІРёРґРµРѕ РґРѕ 200 РњР‘ (Р‘3).
		IdleTimeout: 120 * time.Second,
	}

	// Р’РѕСЂРєРµСЂС‹-Р·Р°РіР»СѓС€РєРё (outbox, РЅР°РїРѕРјРёРЅР°РЅРёСЏ, Р°РІС‚РѕРѕС‡РёСЃС‚РєР°) РѕС‚РєР»СЋС‡Р°СЋС‚СЃСЏ РІРјРµСЃС‚Рµ СЃ
	// РєРѕРЅС‚РµРєСЃС‚РѕРј; СЂРµР°Р»СЊРЅС‹Рµ СЂРµР°Р»РёР·Р°С†РёРё РїРѕСЏРІСЏС‚СЃСЏ РІ Р‘8/Р‘10.
	workerCtx, stopWorkers := context.WithCancel(ctx)
	workers := startWorkers(workerCtx, logger, calendar)
	defer func() {
		stopWorkers()
		workers.Wait()
	}()

	// Р‘3: СЂР°Р· РІ С‡Р°СЃ СѓРґР°Р»СЏСЋС‚СЃСЏ С„Р°Р№Р»С‹ Р±РµР· РїСЂРёРІСЏР·РєРё (linked_at IS NULL) СЃС‚Р°СЂС€Рµ
	// 24 С‡Р°СЃРѕРІ; РѕС€РёР±РєРё РѕС‡РёСЃС‚РєРё С‚РѕР»СЊРєРѕ Р»РѕРіРёСЂСѓСЋС‚СЃСЏ Рё РЅРµ РІР»РёСЏСЋС‚ РЅР° API.
	go files.NewCleaner(filesRepo, filesStore, logger, clock, files.CleanerOptions{}).Run(workerCtx)

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("api: HTTP-СЃРµСЂРІРµСЂ СЃР»СѓС€Р°РµС‚", "addr", cfg.HTTPAddr)
		serveErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("api: РїРѕР»СѓС‡РµРЅ СЃРёРіРЅР°Р» РѕСЃС‚Р°РЅРѕРІРєРё")
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("api: HTTP-СЃРµСЂРІРµСЂ: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("api: graceful shutdown: %w", err)
	}
	logger.Info("api: РѕСЃС‚Р°РЅРѕРІР»РµРЅ")
	return nil
}

// newLogger вЂ” JSON-Р»РѕРіРё РІ stdout СЃ СѓСЂРѕРІРЅРµРј РёР· LOG_LEVEL (NFR-4).
func newLogger(cfg *config.Config) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	return slog.New(handler).With("service", "polar-star-api", "env", cfg.AppEnv)
}

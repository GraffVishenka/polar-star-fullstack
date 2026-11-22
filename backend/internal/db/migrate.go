package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/graffvishenka/polar-star-service/backend/migrations"
)

// migrationLogger адаптирует slog к интерфейсу migrate.Logger.
type migrationLogger struct {
	logger *slog.Logger
}

func (l migrationLogger) Printf(format string, v ...any) {
	l.logger.Info("migrate: " + fmt.Sprintf(format, v...))
}

func (l migrationLogger) Verbose() bool { return false }

// Migrate применяет все миграции «вперёд» (NFR-5). Для DSN нужен
// multiStatements=true: golang-migrate выполняет файл миграции целиком.
// Пул соединений для миграций отдельный и закрывается после применения.
func Migrate(ctx context.Context, dsnWithMultiStatements string, logger *slog.Logger) error {
	pool, err := sql.Open("mysql", dsnWithMultiStatements)
	if err != nil {
		return fmt.Errorf("migrate: открытие MySQL: %w", err)
	}
	defer func() { _ = pool.Close() }()

	if err := pool.PingContext(ctx); err != nil {
		return fmt.Errorf("migrate: ping MySQL: %w", err)
	}

	driver, err := migratemysql.WithInstance(pool, &migratemysql.Config{})
	if err != nil {
		return fmt.Errorf("migrate: инициализация драйвера: %w", err)
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migrate: чтение встроенных миграций: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		return fmt.Errorf("migrate: создание мигратора: %w", err)
	}
	m.Log = migrationLogger{logger: logger}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: применение миграций: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("migrate: чтение версии: %w", err)
	}
	if dirty {
		return fmt.Errorf("migrate: схема помечена как dirty на версии %d — требуется ручное вмешательство", version)
	}
	logger.Info("migrate: схема актуальна", "version", version)
	return nil
}

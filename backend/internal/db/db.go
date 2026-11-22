// Package db отвечает за подключение к MySQL, применение миграций и
// идемпотентный сидинг базовых данных.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	// Драйвер MySQL регистрируется побочным эффектом импорта.
	_ "github.com/go-sql-driver/mysql"
)

// Open создаёт пул соединений. Параметры DSN приходят из config (parseTime,
// loc=UTC), поэтому time.Time сканируется корректно.
func Open(ctx context.Context, dsn string, maxOpen, maxIdle int, connMaxLifetime time.Duration) (*sql.DB, error) {
	if maxOpen <= 0 {
		maxOpen = 25
	}
	if maxIdle <= 0 {
		maxIdle = 10
	}
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	if connMaxLifetime <= 0 {
		connMaxLifetime = 3 * time.Minute
	}

	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: открытие MySQL: %w", err)
	}
	pool.SetMaxOpenConns(maxOpen)
	pool.SetMaxIdleConns(maxIdle)
	pool.SetConnMaxLifetime(connMaxLifetime)
	pool.SetConnMaxIdleTime(time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("db: ping MySQL: %w", err)
	}
	return pool, nil
}

// WaitReady ждёт доступности БД (Docker поднимает MySQL раньше API).
// Возвращает ошибку, если БД не поднялась за timeout.
func WaitReady(ctx context.Context, pool *sql.DB, timeout time.Duration, logger *slog.Logger) error {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	attempt := 0
	for {
		attempt++
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := pool.PingContext(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("db: MySQL недоступен за %s: %w", timeout, err)
		}
		logger.Warn("db: MySQL пока недоступен, повтор", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

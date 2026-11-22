package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// SeedOptions — данные первичного администратора из env (AUTH-4, PLAN §4 п. 6).
type SeedOptions struct {
	AdminLogin    string
	AdminFullName string
	AdminPassword string
}

// Значения по умолчанию для настроек (CAL-1..CAL-3, TICKET-1).
const (
	defaultAppName          = "Заявки · База знаний · Оборудование"
	defaultTimezone         = "Asia/Yekaterinburg"
	defaultReminderTime     = "10:00"
	defaultDeadlineDays     = 2
	defaultWorkdaysJSON     = `[1, 2, 3, 4, 5, 6]`
	defaultStatusLabelsJSON = `{"new": "Новая", "in_progress": "В работе", "deferred": "Отложена", "done": "Выполнена"}`
	defaultStatusColorsJSON = `{"new": "#1677ff", "in_progress": "#fa8c16", "deferred": "#8c8c8c", "done": "#52c41a"}`
)

var defaultProblemTypes = []string{
	"Не работает оборудование",
	"Проблемы с доступом",
	"Мебель и помещения",
	"Прочее",
}

var defaultEquipmentTypes = []string{
	"Компьютер",
	"Монитор",
	"Принтер",
	"Сканер",
	"Сетевое оборудование",
	"Прочее",
}

// Seed идемпотентно заполняет базовые данные: настройки-singleton, справочники
// и первичного ADMIN из env. Повторный запуск не меняет уже настроенные данные
// (INSERT IGNORE) и не перезаписывает пароль существующего администратора.
func Seed(ctx context.Context, pool *sql.DB, logger *slog.Logger, opts SeedOptions) error {
	if logger == nil {
		logger = slog.Default()
	}

	if err := seedSettings(ctx, pool); err != nil {
		return err
	}
	if err := seedDictionary(ctx, pool, "problem_types", defaultProblemTypes); err != nil {
		return err
	}
	if err := seedDictionary(ctx, pool, "equipment_types", defaultEquipmentTypes); err != nil {
		return err
	}
	if err := seedAdmin(ctx, pool, logger, opts); err != nil {
		return err
	}
	return nil
}

// seedSettings создаёт singleton-настройки. INSERT IGNORE не перезаписывает
// значения, изменённые администратором в Admin-панели (NFR-5).
func seedSettings(ctx context.Context, pool *sql.DB) error {
	_, err := pool.ExecContext(ctx, `
		INSERT IGNORE INTO settings
			(id, app_name, deadline_days, workdays, timezone, deadline_reminder_time,
			 push_enabled, status_labels, status_colors)
		VALUES (1, ?, ?, ?, ?, ?, 1, ?, ?)`,
		defaultAppName, defaultDeadlineDays, defaultWorkdaysJSON, defaultTimezone,
		defaultReminderTime, defaultStatusLabelsJSON, defaultStatusColorsJSON)
	if err != nil {
		return fmt.Errorf("seed: настройки: %w", err)
	}
	return nil
}

// seedDictionary добавляет отсутствующие элементы справочника по уникальному
// имени. Имя таблицы — константа из кода, не пользовательский ввод.
func seedDictionary(ctx context.Context, pool *sql.DB, table string, names []string) error {
	query := fmt.Sprintf(`INSERT IGNORE INTO %s (name, is_active, sort_order) VALUES (?, 1, ?)`, table)
	for i, name := range names {
		if _, err := pool.ExecContext(ctx, query, name, i); err != nil {
			return fmt.Errorf("seed: справочник %s: %w", table, err)
		}
	}
	return nil
}

// seedAdmin создаёт первичного ADMIN, если пользователя с таким логином нет.
// Пароль хешируется argon2id уже при сидинге (AUTH-1), в лог не попадает.
func seedAdmin(ctx context.Context, pool *sql.DB, logger *slog.Logger, opts SeedOptions) error {
	login := opts.AdminLogin
	if login == "" {
		login = "admin"
	}
	if opts.AdminPassword == "" {
		logger.Warn("seed: SEED_ADMIN_PASSWORD не задан — первичный администратор не создан")
		return nil
	}

	var existingID int64
	err := pool.QueryRowContext(ctx, `SELECT id FROM users WHERE login = ? LIMIT 1`, login).Scan(&existingID)
	switch {
	case err == nil:
		logger.Info("seed: администратор уже существует", "login", login, "user_id", existingID)
		return nil
	case errors.Is(err, sql.ErrNoRows):
		// Продолжаем создание.
	default:
		return fmt.Errorf("seed: проверка администратора: %w", err)
	}

	hash, err := auth.HashPassword(opts.AdminPassword)
	if err != nil {
		return fmt.Errorf("seed: хеширование пароля администратора: %w", err)
	}
	fullName := opts.AdminFullName
	if fullName == "" {
		fullName = "Администратор"
	}
	res, err := pool.ExecContext(ctx, `
		INSERT INTO users (login, full_name, role, password_hash, building_id,
		                   is_active, must_change_password, created_at, updated_at)
		VALUES (?, ?, 'ADMIN', ?, NULL, 1, 0, UTC_TIMESTAMP(), UTC_TIMESTAMP())`,
		login, fullName, hash)
	if err != nil {
		return fmt.Errorf("seed: создание администратора: %w", err)
	}
	id, _ := res.LastInsertId()
	logger.Info("seed: создан первичный администратор", "login", login, "user_id", id)
	return nil
}

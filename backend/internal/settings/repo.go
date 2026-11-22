package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Ошибки репозитория. Сервис переводит их в типизированные ответы API.
var (
	// ErrNotFound — строка не найдена (например, праздник).
	ErrNotFound = errors.New("settings: запись не найдена")
	// ErrDuplicateHoliday — дата уже есть в праздниках (CAL-1: дата уникальна).
	ErrDuplicateHoliday = errors.New("settings: праздник с такой датой уже есть")
)

// mysqlErrDupEntry — ER_DUP_ENTRY (UNIQUE по дате праздника).
const mysqlErrDupEntry = 1062

// Repository — хранилище singleton-настроек и праздников. Интерфейс позволяет
// тестировать правила домена без MySQL.
type Repository interface {
	Settings(ctx context.Context) (*Settings, error)
	UpdateSettings(ctx context.Context, in SettingsInput) (*Settings, error)

	Holidays(ctx context.Context, year *int) ([]Holiday, error)
	HolidayByID(ctx context.Context, id int64) (*Holiday, error)
	CreateHoliday(ctx context.Context, date time.Time) (*Holiday, error)
	UpdateHoliday(ctx context.Context, id int64, date time.Time) (*Holiday, error)
	DeleteHoliday(ctx context.Context, id int64) error
}

// MySQLRepository — реализация Repository на database/sql.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

const settingsColumns = `
	app_name, deadline_days, workdays, timezone, deadline_reminder_time,
	push_enabled, status_labels, status_colors, updated_at`

// Settings читает singleton-настройки. Если строки нет (сидинг не выполнялся),
// возвращаются значения по умолчанию: API и админка должны работать, а PUT
// создаст строку (upsert).
func (r *MySQLRepository) Settings(ctx context.Context) (*Settings, error) {
	var (
		settings    Settings
		workdaysRaw []byte
		labelsRaw   []byte
		colorsRaw   []byte
		pushEnabled int
	)
	err := r.db.QueryRowContext(ctx, `SELECT `+settingsColumns+` FROM settings WHERE id = 1`).Scan(
		&settings.AppName, &settings.DeadlineDays, &workdaysRaw, &settings.Timezone,
		&settings.DeadlineReminderTime, &pushEnabled, &labelsRaw, &colorsRaw, &settings.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		fallback := Defaults()
		return &fallback, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: чтение настроек: %w", err)
	}

	settings.PushEnabled = pushEnabled != 0
	settings.Workdays = decodeWorkdays(workdaysRaw)
	settings.Statuses = decodeStatuses(labelsRaw, colorsRaw)
	return &settings, nil
}

// UpdateSettings сохраняет настройки одним upsert: строка id=1 создаётся, если
// её нет (повторный PUT идемпотентен).
func (r *MySQLRepository) UpdateSettings(ctx context.Context, in SettingsInput) (*Settings, error) {
	workdaysRaw, err := json.Marshal(in.Workdays)
	if err != nil {
		return nil, fmt.Errorf("settings: сериализация рабочих дней: %w", err)
	}
	labels := make(map[string]string, len(in.Statuses))
	colors := make(map[string]string, len(in.Statuses))
	for _, status := range in.Statuses {
		labels[status.Code] = status.Label
		colors[status.Code] = status.Color
	}
	labelsRaw, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("settings: сериализация названий статусов: %w", err)
	}
	colorsRaw, err := json.Marshal(colors)
	if err != nil {
		return nil, fmt.Errorf("settings: сериализация цветов статусов: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO settings
			(id, app_name, deadline_days, workdays, timezone, deadline_reminder_time,
			 push_enabled, status_labels, status_colors)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?) AS vals
		ON DUPLICATE KEY UPDATE
			app_name = vals.app_name,
			deadline_days = vals.deadline_days,
			workdays = vals.workdays,
			timezone = vals.timezone,
			deadline_reminder_time = vals.deadline_reminder_time,
			push_enabled = vals.push_enabled,
			status_labels = vals.status_labels,
			status_colors = vals.status_colors,
			updated_at = UTC_TIMESTAMP()`,
		in.AppName, in.DeadlineDays, workdaysRaw, in.Timezone, in.DeadlineReminderTime,
		boolToInt(in.PushEnabled), labelsRaw, colorsRaw)
	if err != nil {
		return nil, fmt.Errorf("settings: сохранение настроек: %w", err)
	}
	return r.Settings(ctx)
}

// Holidays возвращает праздники по возрастанию даты (все или за год).
func (r *MySQLRepository) Holidays(ctx context.Context, year *int) ([]Holiday, error) {
	query := `SELECT id, holiday_date, created_at FROM holidays`
	args := make([]any, 0, 1)
	if year != nil {
		// holiday_year — generated-колонка (миграция 0007), чтобы фильтр по
		// году использовал индекс.
		query += ` WHERE holiday_year = ?`
		args = append(args, *year)
	}
	query += ` ORDER BY holiday_date ASC, id ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("settings: список праздников: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Holiday, 0, 16)
	for rows.Next() {
		holiday, err := scanHoliday(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *holiday)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("settings: список праздников: %w", err)
	}
	return items, nil
}

// HolidayByID читает праздник по id.
func (r *MySQLRepository) HolidayByID(ctx context.Context, id int64) (*Holiday, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, holiday_date, created_at FROM holidays WHERE id = ? LIMIT 1`, id)
	return scanHoliday(row)
}

// CreateHoliday добавляет праздник; повтор даты → ErrDuplicateHoliday (CAL-1).
func (r *MySQLRepository) CreateHoliday(ctx context.Context, date time.Time) (*Holiday, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO holidays (holiday_date) VALUES (?)`, normalizeDate(date))
	if err != nil {
		if mysqlErrorNumber(err) == mysqlErrDupEntry {
			return nil, ErrDuplicateHoliday
		}
		return nil, fmt.Errorf("settings: создание праздника: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("settings: создание праздника: %w", err)
	}
	return r.HolidayByID(ctx, id)
}

// UpdateHoliday меняет дату праздника.
func (r *MySQLRepository) UpdateHoliday(ctx context.Context, id int64, date time.Time) (*Holiday, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE holidays SET holiday_date = ? WHERE id = ?`, normalizeDate(date), id)
	if err != nil {
		if mysqlErrorNumber(err) == mysqlErrDupEntry {
			return nil, ErrDuplicateHoliday
		}
		return nil, fmt.Errorf("settings: изменение праздника: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("settings: изменение праздника: %w", err)
	}
	if affected == 0 {
		// Либо строки нет, либо дата не изменилась — проверяем существование.
		return r.HolidayByID(ctx, id)
	}
	return r.HolidayByID(ctx, id)
}

// DeleteHoliday удаляет праздник.
func (r *MySQLRepository) DeleteHoliday(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM holidays WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("settings: удаление праздника: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("settings: удаление праздника: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// decodeWorkdays разбирает JSON-массив ISO-дней; при повреждённых данных
// возвращает маску по умолчанию (CAL-1), чтобы календарь не остался пустым.
func decodeWorkdays(raw []byte) []int {
	var values []int
	if err := json.Unmarshal(raw, &values); err != nil || len(values) == 0 {
		return append([]int(nil), Defaults().Workdays...)
	}
	return values
}

// decodeStatuses собирает статусы в фиксированном порядке кодов; отсутствующие
// подписи/цвета берутся из значений по умолчанию (TICKET-1).
func decodeStatuses(labelsRaw, colorsRaw []byte) []StatusSetting {
	labels := map[string]string{}
	colors := map[string]string{}
	_ = json.Unmarshal(labelsRaw, &labels)
	_ = json.Unmarshal(colorsRaw, &colors)

	statuses := make([]StatusSetting, 0, len(StatusCodes))
	for _, code := range StatusCodes {
		status := StatusSetting{Code: code, Label: labels[code], Color: colors[code]}
		if status.Label == "" || status.Color == "" {
			if fallback, ok := defaultStatus(code); ok {
				if status.Label == "" {
					status.Label = fallback.Label
				}
				if status.Color == "" {
					status.Color = fallback.Color
				}
			}
		}
		statuses = append(statuses, status)
	}
	return statuses
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanHoliday(row rowScanner) (*Holiday, error) {
	var holiday Holiday
	err := row.Scan(&holiday.ID, &holiday.Date, &holiday.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("settings: чтение праздника: %w", err)
	}
	return &holiday, nil
}

// normalizeDate приводит дату к календарному дню в UTC: MySQL DATE не хранит
// время и смещение, а Date() у отсканированного значения остаётся верным.
func normalizeDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// mysqlErrorNumber достаёт номер ошибки MySQL (0 — не MySQL-ошибка).
func mysqlErrorNumber(err error) uint16 {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number
	}
	return 0
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

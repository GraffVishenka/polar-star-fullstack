package dictionaries

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// Ошибки репозитория. Сервис переводит их в типизированные ответы API.
var (
	// ErrNotFound — элемент справочника не найден.
	ErrNotFound = errors.New("dictionaries: запись не найдена")
	// ErrNameTaken — имя уже используется в этом справочнике (UNIQUE name).
	ErrNameTaken = errors.New("dictionaries: имя уже используется")
)

// mysqlErrDupEntry — ER_DUP_ENTRY (UNIQUE по имени справочника).
const mysqlErrDupEntry = 1062

// Repository — хранилище справочников. Kind задаёт таблицу (whitelist),
// поэтому интерфейс один на оба справочника.
type Repository interface {
	List(ctx context.Context, kind Kind, includeInactive bool) ([]Item, error)
	ByID(ctx context.Context, kind Kind, id int64) (*Item, error)
	Create(ctx context.Context, kind Kind, in CreateInput) (*Item, error)
	Update(ctx context.Context, kind Kind, id int64, in UpdateInput) (*Item, error)
	SetActive(ctx context.Context, kind Kind, id int64, active bool) (*Item, error)
	NameTaken(ctx context.Context, kind Kind, name string, excludeID int64) (bool, error)
}

// MySQLRepository — реализация Repository на database/sql.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

const itemColumns = `id, name, is_active, sort_order, created_at, updated_at`

// List возвращает элементы справочника: активные (по умолчанию) или все.
// Сортировка — sort_order, затем name (как в выпадающих списках админки).
func (r *MySQLRepository) List(ctx context.Context, kind Kind, includeInactive bool) ([]Item, error) {
	table, ok := kind.table()
	if !ok {
		return nil, fmt.Errorf("dictionaries: неизвестный справочник %q", kind)
	}

	query := `SELECT ` + itemColumns + ` FROM ` + table
	if !includeInactive {
		query += ` WHERE is_active = 1`
	}
	query += ` ORDER BY sort_order ASC, name ASC, id ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("dictionaries: список %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Item, 0, 16)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dictionaries: список %s: %w", table, err)
	}
	return items, nil
}

// ByID читает элемент справочника.
func (r *MySQLRepository) ByID(ctx context.Context, kind Kind, id int64) (*Item, error) {
	table, ok := kind.table()
	if !ok {
		return nil, fmt.Errorf("dictionaries: неизвестный справочник %q", kind)
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT `+itemColumns+` FROM `+table+` WHERE id = ? LIMIT 1`, id)
	return scanItem(row)
}

// Create добавляет элемент; дубль имени → ErrNameTaken.
func (r *MySQLRepository) Create(ctx context.Context, kind Kind, in CreateInput) (*Item, error) {
	table, ok := kind.table()
	if !ok {
		return nil, fmt.Errorf("dictionaries: неизвестный справочник %q", kind)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO `+table+` (name, is_active, sort_order) VALUES (?, ?, ?)`,
		in.Name, boolToInt(in.IsActive), in.SortOrder)
	if err != nil {
		if mysqlErrorNumber(err) == mysqlErrDupEntry {
			return nil, ErrNameTaken
		}
		return nil, fmt.Errorf("dictionaries: создание %s: %w", table, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("dictionaries: создание %s: %w", table, err)
	}
	return r.ByID(ctx, kind, id)
}

// Update частично обновляет элемент; дубль имени → ErrNameTaken.
func (r *MySQLRepository) Update(ctx context.Context, kind Kind, id int64, in UpdateInput) (*Item, error) {
	table, ok := kind.table()
	if !ok {
		return nil, fmt.Errorf("dictionaries: неизвестный справочник %q", kind)
	}
	if in.Empty() {
		return r.ByID(ctx, kind, id)
	}

	sets := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if in.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *in.Name)
	}
	if in.IsActive != nil {
		sets = append(sets, "is_active = ?")
		args = append(args, boolToInt(*in.IsActive))
	}
	if in.SortOrder != nil {
		sets = append(sets, "sort_order = ?")
		args = append(args, *in.SortOrder)
	}
	sets = append(sets, "updated_at = UTC_TIMESTAMP()")
	args = append(args, id)

	query := `UPDATE ` + table + ` SET ` + strings.Join(sets, ", ") + ` WHERE id = ?`
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		if mysqlErrorNumber(err) == mysqlErrDupEntry {
			return nil, ErrNameTaken
		}
		return nil, fmt.Errorf("dictionaries: обновление %s: %w", table, err)
	}
	return r.ByID(ctx, kind, id)
}

// SetActive включает/выключает элемент. Физического удаления нет: справочники
// используются в исторических заявках и оборудовании (деактивация вместо
// удаления).
func (r *MySQLRepository) SetActive(ctx context.Context, kind Kind, id int64, active bool) (*Item, error) {
	table, ok := kind.table()
	if !ok {
		return nil, fmt.Errorf("dictionaries: неизвестный справочник %q", kind)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE `+table+` SET is_active = ?, updated_at = UTC_TIMESTAMP() WHERE id = ?`,
		boolToInt(active), id); err != nil {
		return nil, fmt.Errorf("dictionaries: деактивация %s: %w", table, err)
	}
	return r.ByID(ctx, kind, id)
}

// NameTaken проверяет занятость имени в справочнике (excludeID=0 — все).
func (r *MySQLRepository) NameTaken(ctx context.Context, kind Kind, name string, excludeID int64) (bool, error) {
	table, ok := kind.table()
	if !ok {
		return false, fmt.Errorf("dictionaries: неизвестный справочник %q", kind)
	}
	var taken bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM `+table+` WHERE name = ? AND id <> ?)`,
		name, excludeID).Scan(&taken)
	if err != nil {
		return false, fmt.Errorf("dictionaries: проверка имени в %s: %w", table, err)
	}
	return taken, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(row rowScanner) (*Item, error) {
	var (
		item     Item
		isActive int
	)
	err := row.Scan(&item.ID, &item.Name, &isActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("dictionaries: чтение элемента: %w", err)
	}
	item.IsActive = isActive != 0
	return &item, nil
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

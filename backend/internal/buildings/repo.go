package buildings

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
	// ErrNotFound — здание/склад не найден.
	ErrNotFound = errors.New("buildings: запись не найдена")
	// ErrNameTaken — название склада уже занято в этом здании (EQ-1).
	ErrNameTaken = errors.New("buildings: название склада уже используется")
	// ErrWarehouseNotEmpty — склад с оборудованием или историей перемещений (EQ-6).
	ErrWarehouseNotEmpty = errors.New("buildings: склад не пуст")
	// ErrBuildingInUse — здание используется (FK RESTRICT, EQ-6).
	ErrBuildingInUse = errors.New("buildings: здание используется")
)

// Коды ошибок MySQL, которые транслируются в доменные.
const (
	mysqlErrDupEntry        = 1062 // ER_DUP_ENTRY
	mysqlErrRowIsReferenced = 1451 // ER_ROW_IS_REFERENCED_2 (FK ON DELETE RESTRICT)
)

// Repository — хранилище зданий и складов. Интерфейс позволяет тестировать
// правила домена без MySQL (в окружении разработки БД нет).
type Repository interface {
	Buildings(ctx context.Context, includeInactive bool) ([]Building, error)
	BuildingByID(ctx context.Context, id int64) (*Building, error)
	CreateBuilding(ctx context.Context, name string, address *string) (*Building, error)
	UpdateBuilding(ctx context.Context, id int64, patch BuildingPatch) (*Building, error)
	// BuildingUsage — счётчики складов/оборудования/пользователей для EQ-6.
	BuildingUsage(ctx context.Context, id int64) (BuildingUsage, error)
	DeleteBuilding(ctx context.Context, id int64) error

	Warehouses(ctx context.Context, buildingID *int64) ([]Warehouse, error)
	WarehouseByID(ctx context.Context, id int64) (*Warehouse, error)
	CreateWarehouse(ctx context.Context, buildingID int64, name string) (*Warehouse, error)
	UpdateWarehouse(ctx context.Context, id int64, name string) (*Warehouse, error)
	DeleteWarehouse(ctx context.Context, id int64) error
	// WarehouseNameTaken — проверка уникальности имени в здании (excludeID=0 — все).
	WarehouseNameTaken(ctx context.Context, buildingID int64, name string, excludeID int64) (bool, error)
	// WarehouseEquipmentCount — оборудование на складе (включая удалённое в
	// корзину: оно всё ещё ссылается на склад через FK).
	WarehouseEquipmentCount(ctx context.Context, id int64) (int, error)
}

// MySQLRepository — реализация Repository на database/sql.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

const buildingColumns = `id, name, address, is_active, created_at, updated_at`

// Buildings возвращает здания по названию. includeInactive=false отдаёт только
// активные (публичный выбор, форма заявки — EQ-1/EQ-6); авторизованный список
// по умолчанию включает неактивные.
func (r *MySQLRepository) Buildings(ctx context.Context, includeInactive bool) ([]Building, error) {
	query := `SELECT ` + buildingColumns + ` FROM buildings`
	if !includeInactive {
		query += ` WHERE is_active = 1`
	}
	query += ` ORDER BY name ASC, id ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("buildings: список зданий: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Building, 0, 16)
	for rows.Next() {
		building, err := scanBuilding(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *building)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("buildings: список зданий: %w", err)
	}
	return items, nil
}

// BuildingByID читает здание по id.
func (r *MySQLRepository) BuildingByID(ctx context.Context, id int64) (*Building, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+buildingColumns+` FROM buildings WHERE id = ? LIMIT 1`, id)
	return scanBuilding(row)
}

// CreateBuilding создаёт здание.
func (r *MySQLRepository) CreateBuilding(ctx context.Context, name string, address *string) (*Building, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO buildings (name, address) VALUES (?, ?)`, name, nullableString(address))
	if err != nil {
		return nil, fmt.Errorf("buildings: создание здания: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("buildings: создание здания: %w", err)
	}
	return r.BuildingByID(ctx, id)
}

// UpdateBuilding применяет частичное обновление (имя, адрес, is_active).
// Деактивация — это is_active=false (EQ-6: здание не удаляется, а скрывается).
func (r *MySQLRepository) UpdateBuilding(ctx context.Context, id int64, patch BuildingPatch) (*Building, error) {
	if patch.Empty() {
		return r.BuildingByID(ctx, id)
	}

	sets := make([]string, 0, 4)
	args := make([]any, 0, 5)
	if patch.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *patch.Name)
	}
	if patch.AddressSet {
		sets = append(sets, "address = ?")
		args = append(args, nullableString(patch.Address))
	}
	if patch.IsActive != nil {
		sets = append(sets, "is_active = ?")
		args = append(args, boolToInt(*patch.IsActive))
	}
	sets = append(sets, "updated_at = UTC_TIMESTAMP()")
	args = append(args, id)

	query := `UPDATE buildings SET ` + strings.Join(sets, ", ") + ` WHERE id = ?`
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("buildings: обновление здания: %w", err)
	}
	// UPDATE несуществующей строки не ошибка — существование проверяет re-read.
	return r.BuildingByID(ctx, id)
}

// BuildingUsage собирает счётчики связанных сущностей одним запросом.
func (r *MySQLRepository) BuildingUsage(ctx context.Context, id int64) (BuildingUsage, error) {
	var usage BuildingUsage
	err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM warehouses w WHERE w.building_id = ?),
			(SELECT COUNT(*) FROM equipment e
				JOIN warehouses w ON w.id = e.warehouse_id
				WHERE w.building_id = ?),
			(SELECT COUNT(*) FROM users u WHERE u.building_id = ?)`,
		id, id, id).Scan(&usage.Warehouses, &usage.Equipment, &usage.Users)
	if err != nil {
		return BuildingUsage{}, fmt.Errorf("buildings: связанные данные здания: %w", err)
	}
	return usage, nil
}

// DeleteBuilding физически удаляет здание. FK RESTRICT превращает попытку
// удалить используемое здание в ErrBuildingInUse (EQ-6). Заявки (Б5) попадут
// сюда тем же путём — через FK-ошибку.
func (r *MySQLRepository) DeleteBuilding(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM buildings WHERE id = ?`, id)
	if err != nil {
		if mysqlErrorNumber(err) == mysqlErrRowIsReferenced {
			return ErrBuildingInUse
		}
		return fmt.Errorf("buildings: удаление здания: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("buildings: удаление здания: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

const warehouseColumns = `w.id, w.building_id, b.name, w.name, w.created_at`

// Warehouses возвращает склады (все или одного здания).
func (r *MySQLRepository) Warehouses(ctx context.Context, buildingID *int64) ([]Warehouse, error) {
	query := `SELECT ` + warehouseColumns + `
		FROM warehouses w
		JOIN buildings b ON b.id = w.building_id
		WHERE w.deleted_at IS NULL`
	args := make([]any, 0, 1)
	if buildingID != nil {
		query += ` AND w.building_id = ?`
		args = append(args, *buildingID)
	}
	query += ` ORDER BY b.name ASC, w.name ASC, w.id ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("buildings: список складов: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Warehouse, 0, 16)
	for rows.Next() {
		warehouse, err := scanWarehouse(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *warehouse)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("buildings: список складов: %w", err)
	}
	return items, nil
}

// WarehouseByID читает склад по id.
func (r *MySQLRepository) WarehouseByID(ctx context.Context, id int64) (*Warehouse, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+warehouseColumns+`
		FROM warehouses w
		JOIN buildings b ON b.id = w.building_id
		WHERE w.id = ? AND w.deleted_at IS NULL
		LIMIT 1`, id)
	return scanWarehouse(row)
}

// CreateWarehouse создаёт склад; дубль имени в здании → ErrNameTaken (EQ-1).
func (r *MySQLRepository) CreateWarehouse(ctx context.Context, buildingID int64, name string) (*Warehouse, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO warehouses (building_id, name) VALUES (?, ?)`, buildingID, name)
	if err != nil {
		if mysqlErrorNumber(err) == mysqlErrDupEntry {
			return nil, ErrNameTaken
		}
		return nil, fmt.Errorf("buildings: создание склада: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("buildings: создание склада: %w", err)
	}
	return r.WarehouseByID(ctx, id)
}

// UpdateWarehouse переименовывает склад.
func (r *MySQLRepository) UpdateWarehouse(ctx context.Context, id int64, name string) (*Warehouse, error) {
	_, err := r.db.ExecContext(ctx,
		`UPDATE warehouses SET name = ? WHERE id = ? AND deleted_at IS NULL`, name, id)
	if err != nil {
		if mysqlErrorNumber(err) == mysqlErrDupEntry {
			return nil, ErrNameTaken
		}
		return nil, fmt.Errorf("buildings: переименование склада: %w", err)
	}
	return r.WarehouseByID(ctx, id)
}

// DeleteWarehouse удаляет пустой склад; FK RESTRICT по оборудованию и журналу
// перемещений даёт ErrWarehouseNotEmpty (EQ-6).
func (r *MySQLRepository) DeleteWarehouse(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM warehouses WHERE id = ?`, id)
	if err != nil {
		if mysqlErrorNumber(err) == mysqlErrRowIsReferenced {
			return ErrWarehouseNotEmpty
		}
		return fmt.Errorf("buildings: удаление склада: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("buildings: удаление склада: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// WarehouseNameTaken проверяет занятость имени склада среди неудалённых.
func (r *MySQLRepository) WarehouseNameTaken(ctx context.Context, buildingID int64, name string, excludeID int64) (bool, error) {
	var taken bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM warehouses
			WHERE building_id = ? AND name = ? AND deleted_at IS NULL AND id <> ?
		)`, buildingID, name, excludeID).Scan(&taken)
	if err != nil {
		return false, fmt.Errorf("buildings: проверка имени склада: %w", err)
	}
	return taken, nil
}

// WarehouseEquipmentCount считает оборудование склада (включая корзину: FK
// RESTRICT не даст удалить склад, пока на него ссылаются строки equipment).
func (r *MySQLRepository) WarehouseEquipmentCount(ctx context.Context, id int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM equipment WHERE warehouse_id = ?`, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("buildings: оборудование склада: %w", err)
	}
	return count, nil
}

// rowScanner — общий интерфейс *sql.Row и *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanBuilding(row rowScanner) (*Building, error) {
	var (
		building Building
		address  sql.NullString
		isActive int
	)
	err := row.Scan(&building.ID, &building.Name, &address, &isActive, &building.CreatedAt, &building.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buildings: чтение здания: %w", err)
	}
	building.IsActive = isActive != 0
	if address.Valid {
		building.Address = &address.String
	}
	return &building, nil
}

func scanWarehouse(row rowScanner) (*Warehouse, error) {
	var warehouse Warehouse
	err := row.Scan(
		&warehouse.ID, &warehouse.BuildingID, &warehouse.BuildingName,
		&warehouse.Name, &warehouse.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buildings: чтение склада: %w", err)
	}
	return &warehouse, nil
}

// mysqlErrorNumber достаёт номер ошибки MySQL (0 — не MySQL-ошибка).
func mysqlErrorNumber(err error) uint16 {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number
	}
	return 0
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

package equipment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// mysqlErrDupEntry — ER_DUP_ENTRY: гонка двух созданий с одинаковым
// регистрационным номером (UNIQUE по generated column).
const mysqlErrDupEntry = 1062

// Repository — хранилище оборудования и журнала перемещений. Интерфейс
// позволяет тестировать правила домена без MySQL.
type Repository interface {
	List(ctx context.Context, filter ListFilter) ([]Equipment, int, error)
	ByID(ctx context.Context, id int64) (*Equipment, error)
	Create(ctx context.Context, in CreateInput) (*Equipment, error)
	Update(ctx context.Context, id int64, in UpdateInput) (*Equipment, error)
	SoftDelete(ctx context.Context, id int64) error
	// RegistrationNumberTaken — проверка уникальности среди неудалённых (EQ-2).
	RegistrationNumberTaken(ctx context.Context, registrationNumber string, excludeID int64) (bool, error)
	EquipmentTypeExists(ctx context.Context, id int64) (bool, error)
	// WarehouseInfo — склад с зданием для правил размещения (EQ-3).
	WarehouseInfo(ctx context.Context, id int64) (*WarehouseInfo, error)
	// Move выполняет перемещение в транзакции с SELECT ... FOR UPDATE и
	// возвращает обновлённую единицу и запись журнала (EQ-3).
	Move(ctx context.Context, id int64, in MoveInput, actor Actor) (*Equipment, *Movement, error)
	// BulkMove перемещает набор единиц в один склад одной транзакцией (EQ-4):
	// блокировки по возрастанию id, журнал по каждой перемещённой единице.
	// Единицы, уже находящиеся в целевом складе, пропускаются; если таких все —
	// ErrSameWarehouse. Возвращает записи журнала и число пропущенных.
	BulkMove(ctx context.Context, in BulkMoveInput, actor Actor) ([]*Movement, int, error)
	// Movements — журнал единицы по убыванию moved_at (EQ-5).
	Movements(ctx context.Context, equipmentID int64, page, size int) ([]Movement, int, error)
}

// MySQLRepository — реализация Repository на database/sql.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// equipmentSelect/equipmentFrom добавляют к единице имена типа, склада и здания
// (контракт Equipment требует *_name без отдельных запросов).
const equipmentSelect = `
	e.id, e.name,
	e.equipment_type_id, et.name,
	e.serial_number, e.registration_number,
	e.warehouse_id, w.name, w.building_id, b.name,
	e.created_at, e.updated_at, e.deleted_at`

const equipmentFrom = `
	FROM equipment e
	JOIN equipment_types et ON et.id = e.equipment_type_id
	JOIN warehouses w ON w.id = e.warehouse_id
	JOIN buildings b ON b.id = w.building_id`

const movementSelect = `
	m.id, m.equipment_id,
	m.from_warehouse_id, fw.name, fb.name,
	m.to_warehouse_id, tw.name, tb.name,
	m.moved_by_user_id, u.full_name,
	m.moved_at, m.comment`

const movementFrom = `
	FROM equipment_movements m
	JOIN warehouses fw ON fw.id = m.from_warehouse_id
	JOIN buildings fb ON fb.id = fw.building_id
	JOIN warehouses tw ON tw.id = m.to_warehouse_id
	JOIN buildings tb ON tb.id = tw.building_id
	JOIN users u ON u.id = m.moved_by_user_id`

// List возвращает страницу оборудования по фильтрам EQ-7 (без ограничения по
// зданию для USER) и общее число совпадений.
func (r *MySQLRepository) List(ctx context.Context, filter ListFilter) ([]Equipment, int, error) {
	conditions := []string{"e.deleted_at IS NULL"}
	args := make([]any, 0, 6)

	if filter.BuildingID != nil {
		conditions = append(conditions, "w.building_id = ?")
		args = append(args, *filter.BuildingID)
	}
	if filter.WarehouseID != nil {
		conditions = append(conditions, "e.warehouse_id = ?")
		args = append(args, *filter.WarehouseID)
	}
	if filter.EquipmentTypeID != nil {
		conditions = append(conditions, "e.equipment_type_id = ?")
		args = append(args, *filter.EquipmentTypeID)
	}
	if filter.Search != "" {
		pattern := LikePattern(filter.Search)
		conditions = append(conditions,
			"(e.name LIKE ? OR e.serial_number LIKE ? OR e.registration_number LIKE ?)")
		args = append(args, pattern, pattern, pattern)
	}
	where := " WHERE " + strings.Join(conditions, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*)`+equipmentFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("equipment: количество оборудования: %w", err)
	}

	// Порядок стабилен для пагинации: по названию, затем по id.
	query := `SELECT ` + equipmentSelect + equipmentFrom + where +
		` ORDER BY e.name ASC, e.id ASC LIMIT ? OFFSET ?`
	listArgs := make([]any, 0, len(args)+2)
	listArgs = append(listArgs, args...)
	listArgs = append(listArgs, filter.Size, (filter.Page-1)*filter.Size)

	rows, err := r.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("equipment: список оборудования: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Equipment, 0, filter.Size)
	for rows.Next() {
		item, err := scanEquipment(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("equipment: список оборудования: %w", err)
	}
	return items, total, nil
}

// ByID читает неудалённую единицу оборудования.
func (r *MySQLRepository) ByID(ctx context.Context, id int64) (*Equipment, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+equipmentSelect+equipmentFrom+` WHERE e.id = ? AND e.deleted_at IS NULL LIMIT 1`, id)
	return scanEquipment(row)
}

// Create добавляет единицу оборудования.
func (r *MySQLRepository) Create(ctx context.Context, in CreateInput) (*Equipment, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO equipment (name, equipment_type_id, serial_number, registration_number, warehouse_id)
		VALUES (?, ?, ?, ?, ?)`,
		in.Name, in.EquipmentTypeID, in.SerialNumber, in.RegistrationNumber, in.WarehouseID)
	if err != nil {
		if mysqlErrorNumber(err) == mysqlErrDupEntry {
			return nil, ErrDuplicateRegistration
		}
		return nil, fmt.Errorf("equipment: создание единицы: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("equipment: создание единицы: %w", err)
	}
	return r.ByID(ctx, id)
}

// Update применяет частичное обновление. Смена склада здесь журнал не создаёт
// (контракт PATCH /api/equipment/{id}) — для журналируемого перемещения есть /move.
func (r *MySQLRepository) Update(ctx context.Context, id int64, in UpdateInput) (*Equipment, error) {
	if in.Empty() {
		return r.ByID(ctx, id)
	}

	sets := make([]string, 0, 6)
	args := make([]any, 0, 7)
	if in.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *in.Name)
	}
	if in.EquipmentTypeID != nil {
		sets = append(sets, "equipment_type_id = ?")
		args = append(args, *in.EquipmentTypeID)
	}
	if in.SerialNumber != nil {
		sets = append(sets, "serial_number = ?")
		args = append(args, *in.SerialNumber)
	}
	if in.RegistrationNumber != nil {
		sets = append(sets, "registration_number = ?")
		args = append(args, *in.RegistrationNumber)
	}
	if in.WarehouseID != nil {
		sets = append(sets, "warehouse_id = ?")
		args = append(args, *in.WarehouseID)
	}
	sets = append(sets, "updated_at = UTC_TIMESTAMP()")
	args = append(args, id)

	query := `UPDATE equipment SET ` + strings.Join(sets, ", ") + ` WHERE id = ? AND deleted_at IS NULL`
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		if mysqlErrorNumber(err) == mysqlErrDupEntry {
			return nil, ErrDuplicateRegistration
		}
		return nil, fmt.Errorf("equipment: обновление единицы: %w", err)
	}
	return r.ByID(ctx, id)
}

// SoftDelete отправляет единицу в корзину (EQ-6: удаление — только ADMIN,
// восстановление — через /api/admin/trash в Б10).
func (r *MySQLRepository) SoftDelete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE equipment
		SET deleted_at = UTC_TIMESTAMP(), updated_at = UTC_TIMESTAMP()
		WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("equipment: удаление единицы: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("equipment: удаление единицы: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// RegistrationNumberTaken проверяет занятость регистрационного номера среди
// неудалённых единиц (EQ-2); excludeID=0 — искать по всем.
func (r *MySQLRepository) RegistrationNumberTaken(ctx context.Context, registrationNumber string, excludeID int64) (bool, error) {
	var taken bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM equipment
			WHERE registration_number = ? AND deleted_at IS NULL AND id <> ?
		)`, registrationNumber, excludeID).Scan(&taken)
	if err != nil {
		return false, fmt.Errorf("equipment: проверка регистрационного номера: %w", err)
	}
	return taken, nil
}

// EquipmentTypeExists проверяет существование типа оборудования (EQ-2).
func (r *MySQLRepository) EquipmentTypeExists(ctx context.Context, id int64) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM equipment_types WHERE id = ?)`, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("equipment: проверка типа оборудования: %w", err)
	}
	return exists, nil
}

// WarehouseInfo читает склад вместе с зданием (для EQ-3).
func (r *MySQLRepository) WarehouseInfo(ctx context.Context, id int64) (*WarehouseInfo, error) {
	return warehouseInfo(ctx, r.db, id)
}

// queryRower — общий интерфейс *sql.DB и *sql.Tx.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func warehouseInfo(ctx context.Context, q queryRower, id int64) (*WarehouseInfo, error) {
	var (
		info     WarehouseInfo
		isActive int
	)
	err := q.QueryRowContext(ctx, `
		SELECT w.id, w.name, w.building_id, b.name, b.is_active
		FROM warehouses w
		JOIN buildings b ON b.id = w.building_id
		WHERE w.id = ? AND w.deleted_at IS NULL
		LIMIT 1`, id).Scan(&info.ID, &info.Name, &info.BuildingID, &info.BuildingName, &isActive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWarehouseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("equipment: чтение склада: %w", err)
	}
	info.BuildingActive = isActive != 0
	return &info, nil
}

// Move — перемещение единицы (EQ-3). Вся работа в одной транзакции:
//  1. SELECT warehouse_id ... FOR UPDATE — сериализует параллельные перемещения
//     одной единицы, поэтому каждое изменение попадает в журнал (50
//     параллельных запросов = 50 записей, без потерь);
//  2. проверка целевого склада/здания по правилу CheckMove;
//  3. UPDATE склада единицы + INSERT записи журнала.
func (r *MySQLRepository) Move(ctx context.Context, id int64, in MoveInput, actor Actor) (*Equipment, *Movement, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("equipment: транзакция перемещения: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentWarehouseID int64
	err = tx.QueryRowContext(ctx,
		`SELECT warehouse_id FROM equipment WHERE id = ? AND deleted_at IS NULL FOR UPDATE`, id).
		Scan(&currentWarehouseID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("equipment: блокировка единицы: %w", err)
	}

	target, err := warehouseInfo(ctx, tx, in.ToWarehouseID)
	if err != nil {
		return nil, nil, err
	}
	if err := CheckMove(currentWarehouseID, *target, actor); err != nil {
		return nil, nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE equipment SET warehouse_id = ?, updated_at = UTC_TIMESTAMP() WHERE id = ?`,
		in.ToWarehouseID, id); err != nil {
		return nil, nil, fmt.Errorf("equipment: смена склада: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO equipment_movements
			(equipment_id, from_warehouse_id, to_warehouse_id, moved_by_user_id, moved_at, comment)
		VALUES (?, ?, ?, ?, UTC_TIMESTAMP(), ?)`,
		id, currentWarehouseID, in.ToWarehouseID, actor.UserID, nullableString(in.Comment))
	if err != nil {
		return nil, nil, fmt.Errorf("equipment: запись журнала: %w", err)
	}
	movementID, err := res.LastInsertId()
	if err != nil {
		return nil, nil, fmt.Errorf("equipment: запись журнала: %w", err)
	}

	equipment, err := equipmentByID(ctx, tx, id)
	if err != nil {
		return nil, nil, err
	}
	movement, err := movementByID(ctx, tx, movementID)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("equipment: фиксация перемещения: %w", err)
	}
	return equipment, movement, nil
}

// BulkMove — массовое перемещение единиц в один склад (EQ-4).
//
// Вся операция в одной транзакции:
//  1. целевой склад проверяется по правилу EQ-3 (CheckWarehouseTarget):
//     USER не может перемещать в деактивированное здание, ADMIN может;
//  2. строки оборудования блокируются одним запросом с ORDER BY id — обход
//     блокировок по возрастанию id исключает взаимоблокировки параллельных
//     операций на пересекающихся наборах;
//  3. любая отсутствующая/удалённая единица → ErrNotFound и откат всего набора;
//  4. единицы, уже находящиеся в целевом складе, пропускаются (skipped); если
//     пропускать нужно все — ErrSameWarehouse (409);
//  5. для каждой перемещённой единицы — UPDATE склада и INSERT записи журнала.
func (r *MySQLRepository) BulkMove(ctx context.Context, in BulkMoveInput, actor Actor) ([]*Movement, int, error) {
	// Дубликаты и порядок нормализуются здесь же: сервис валидирует набор, но
	// репозиторий должен быть безопасен и при прямом вызове.
	ids := normalizeBulkIDs(in.EquipmentIDs)
	if len(ids) == 0 || len(ids) > MaxBulkMoveItems {
		return nil, 0, ErrInvalidSelection
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("equipment: транзакция массового перемещения: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	target, err := warehouseInfo(ctx, tx, in.ToWarehouseID)
	if err != nil {
		return nil, 0, err
	}
	if err := CheckWarehouseTarget(*target, actor); err != nil {
		return nil, 0, err
	}

	current, err := lockEquipmentWarehouses(ctx, tx, ids)
	if err != nil {
		return nil, 0, err
	}
	if len(current) != len(ids) {
		return nil, 0, ErrNotFound
	}

	toMove := make([]int64, 0, len(ids))
	skipped := 0
	for _, id := range ids {
		if current[id] == in.ToWarehouseID {
			skipped++
			continue
		}
		toMove = append(toMove, id)
	}
	if len(toMove) == 0 {
		return nil, 0, ErrSameWarehouse
	}

	movementIDs := make([]int64, 0, len(toMove))
	for _, id := range toMove {
		fromWarehouseID := current[id]
		if _, err := tx.ExecContext(ctx, `
			UPDATE equipment SET warehouse_id = ?, updated_at = UTC_TIMESTAMP() WHERE id = ?`,
			in.ToWarehouseID, id); err != nil {
			return nil, 0, fmt.Errorf("equipment: массовая смена склада: %w", err)
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO equipment_movements
				(equipment_id, from_warehouse_id, to_warehouse_id, moved_by_user_id, moved_at, comment)
			VALUES (?, ?, ?, ?, UTC_TIMESTAMP(), ?)`,
			id, fromWarehouseID, in.ToWarehouseID, actor.UserID, nullableString(in.Comment))
		if err != nil {
			return nil, 0, fmt.Errorf("equipment: журнал массового перемещения: %w", err)
		}
		movementID, err := res.LastInsertId()
		if err != nil {
			return nil, 0, fmt.Errorf("equipment: журнал массового перемещения: %w", err)
		}
		movementIDs = append(movementIDs, movementID)
	}

	movements, err := movementsByIDs(ctx, tx, movementIDs)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("equipment: фиксация массового перемещения: %w", err)
	}
	return movements, skipped, nil
}

// normalizeBulkIDs убирает дубликаты и сортирует id по возрастанию: порядок
// блокировок строк должен быть одинаковым у всех параллельных операций.
func normalizeBulkIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// lockEquipmentWarehouses блокирует строки оборудования по возрастанию id и
// возвращает текущий склад каждой единицы.
func lockEquipmentWarehouses(ctx context.Context, tx *sql.Tx, ids []int64) (map[int64]int64, error) {
	query := `SELECT id, warehouse_id FROM equipment
		WHERE id IN (` + placeholders(len(ids)) + `) AND deleted_at IS NULL
		ORDER BY id FOR UPDATE`
	rows, err := tx.QueryContext(ctx, query, int64Args(ids)...)
	if err != nil {
		return nil, fmt.Errorf("equipment: блокировка оборудования: %w", err)
	}
	defer func() { _ = rows.Close() }()

	current := make(map[int64]int64, len(ids))
	for rows.Next() {
		var id, warehouseID int64
		if err := rows.Scan(&id, &warehouseID); err != nil {
			return nil, fmt.Errorf("equipment: чтение оборудования: %w", err)
		}
		current[id] = warehouseID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("equipment: чтение оборудования: %w", err)
	}
	return current, nil
}

// movementsByIDs читает записи журнала внутри транзакции (для ответа EQ-4).
func movementsByIDs(ctx context.Context, tx *sql.Tx, ids []int64) ([]*Movement, error) {
	query := `SELECT ` + movementSelect + movementFrom +
		` WHERE m.id IN (` + placeholders(len(ids)) + `) ORDER BY m.id`
	rows, err := tx.QueryContext(ctx, query, int64Args(ids)...)
	if err != nil {
		return nil, fmt.Errorf("equipment: чтение журнала перемещений: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]*Movement, 0, len(ids))
	for rows.Next() {
		movement, err := scanMovement(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, movement)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("equipment: чтение журнала перемещений: %w", err)
	}
	return items, nil
}

// placeholders возвращает «?, ?, ?» для IN (...).
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// int64Args превращает идентификаторы в аргументы запроса.
func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// Movements — страница журнала перемещений единицы (EQ-5):// ORDER BY moved_at DESC, id DESC.
func (r *MySQLRepository) Movements(ctx context.Context, equipmentID int64, page, size int) ([]Movement, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM equipment_movements WHERE equipment_id = ?`, equipmentID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("equipment: количество перемещений: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+movementSelect+movementFrom+`
		WHERE m.equipment_id = ?
		ORDER BY m.moved_at DESC, m.id DESC
		LIMIT ? OFFSET ?`, equipmentID, size, (page-1)*size)
	if err != nil {
		return nil, 0, fmt.Errorf("equipment: журнал перемещений: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Movement, 0, size)
	for rows.Next() {
		movement, err := scanMovement(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *movement)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("equipment: журнал перемещений: %w", err)
	}
	return items, total, nil
}

func equipmentByID(ctx context.Context, q queryRower, id int64) (*Equipment, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+equipmentSelect+equipmentFrom+` WHERE e.id = ? AND e.deleted_at IS NULL LIMIT 1`, id)
	return scanEquipment(row)
}

func movementByID(ctx context.Context, q queryRower, id int64) (*Movement, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+movementSelect+movementFrom+` WHERE m.id = ? LIMIT 1`, id)
	return scanMovement(row)
}

// rowScanner — общий интерфейс *sql.Row и *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanEquipment(row rowScanner) (*Equipment, error) {
	var (
		item      Equipment
		deletedAt sql.NullTime
	)
	err := row.Scan(
		&item.ID, &item.Name,
		&item.EquipmentTypeID, &item.EquipmentTypeName,
		&item.SerialNumber, &item.RegistrationNumber,
		&item.WarehouseID, &item.WarehouseName, &item.BuildingID, &item.BuildingName,
		&item.CreatedAt, &item.UpdatedAt, &deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("equipment: чтение единицы: %w", err)
	}
	if deletedAt.Valid {
		item.DeletedAt = &deletedAt.Time
	}
	return &item, nil
}

func scanMovement(row rowScanner) (*Movement, error) {
	var (
		movement Movement
		comment  sql.NullString
	)
	err := row.Scan(
		&movement.ID, &movement.EquipmentID,
		&movement.FromWarehouseID, &movement.FromWarehouseName, &movement.FromBuildingName,
		&movement.ToWarehouseID, &movement.ToWarehouseName, &movement.ToBuildingName,
		&movement.MovedByUserID, &movement.MovedByName,
		&movement.MovedAt, &comment,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("equipment: чтение перемещения: %w", err)
	}
	if comment.Valid {
		movement.Comment = &comment.String
	}
	return &movement, nil
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

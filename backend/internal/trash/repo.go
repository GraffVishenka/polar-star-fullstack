package trash

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// mysqlErrDupEntry — ER_DUP_ENTRY: гонка при восстановлении оборудования с
// занятым регистрационным номером.
const mysqlErrDupEntry = 1062

// Repository — хранилище корзины. Тип передаётся параметром (whitelist таблиц),
// поэтому интерфейс один на все три сущности.
type Repository interface {
	List(ctx context.Context, kind Type, page, size int) ([]Item, int, error)
	RestoreTicket(ctx context.Context, id int64) (TicketRestore, error)
	RestoreKB(ctx context.Context, id int64) (KBRestore, error)
	RestoreEquipment(ctx context.Context, id int64) (EquipmentRestore, error)
	// PurgeTicket удаляет заявку с комментариями и файлами; возвращает пути
	// файлов на диске, которые нужно удалить после коммита.
	PurgeTicket(ctx context.Context, id int64) ([]string, error)
	PurgeKB(ctx context.Context, id int64) error
	PurgeEquipment(ctx context.Context, id int64) error
	Expired(ctx context.Context, kind Type, before time.Time, limit int) ([]int64, error)
}

// MySQLRepository — реализация Repository на database/sql.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// List возвращает страницу корзины по типу (TRASH-1).
func (r *MySQLRepository) List(ctx context.Context, kind Type, page, size int) ([]Item, int, error) {
	switch kind {
	case TypeTicket:
		return r.listTickets(ctx, page, size)
	case TypeKB:
		return r.listKB(ctx, page, size)
	case TypeEquipment:
		return r.listEquipment(ctx, page, size)
	default:
		return nil, 0, ErrUnknownType
	}
}

// listTickets — удалённые заявки: дата удаления, автор и краткое описание.
func (r *MySQLRepository) listTickets(ctx context.Context, page, size int) ([]Item, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tickets WHERE deleted_at IS NOT NULL`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("trash: количество заявок: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.applicant_name, t.cabinet, b.id, b.name, t.deleted_at,
		       u.id, u.full_name
		FROM tickets t
		JOIN buildings b ON b.id = t.building_id
		LEFT JOIN users u ON u.id = t.deleted_by
		WHERE t.deleted_at IS NOT NULL
		ORDER BY t.deleted_at DESC, t.id DESC
		LIMIT ? OFFSET ?`, size, (page-1)*size)
	if err != nil {
		return nil, 0, fmt.Errorf("trash: список заявок: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Item, 0, size)
	for rows.Next() {
		var (
			id            int64
			applicant     string
			cabinet       string
			buildingID    int64
			buildingName  string
			deletedAt     time.Time
			deletedByID   sql.NullInt64
			deletedByName sql.NullString
		)
		if err := rows.Scan(&id, &applicant, &cabinet, &buildingID, &buildingName,
			&deletedAt, &deletedByID, &deletedByName); err != nil {
			return nil, 0, fmt.Errorf("trash: чтение заявки: %w", err)
		}
		item := Item{
			Type:         TypeTicket,
			ID:           id,
			Title:        ticketTitle(id, applicant, cabinet, buildingName),
			DeletedAt:    deletedAt,
			BuildingID:   int64Ptr(buildingID),
			BuildingName: stringPtr(buildingName),
		}
		item.DeletedByUserID = nullableInt64(deletedByID)
		item.DeletedByName = nullableString(deletedByName)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("trash: список заявок: %w", err)
	}
	return items, total, nil
}

// listKB — удалённые узлы БЗ (заголовок и родитель).
func (r *MySQLRepository) listKB(ctx context.Context, page, size int) ([]Item, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kb_nodes WHERE deleted_at IS NOT NULL`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("trash: количество узлов БЗ: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT n.id, n.title, p.title, n.deleted_at, u.id, u.full_name
		FROM kb_nodes n
		LEFT JOIN kb_nodes p ON p.id = n.parent_id
		LEFT JOIN users u ON u.id = n.deleted_by
		WHERE n.deleted_at IS NOT NULL
		ORDER BY n.deleted_at DESC, n.id DESC
		LIMIT ? OFFSET ?`, size, (page-1)*size)
	if err != nil {
		return nil, 0, fmt.Errorf("trash: список узлов БЗ: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Item, 0, size)
	for rows.Next() {
		var (
			id            int64
			title         string
			parentTitle   sql.NullString
			deletedAt     time.Time
			deletedByID   sql.NullInt64
			deletedByName sql.NullString
		)
		if err := rows.Scan(&id, &title, &parentTitle, &deletedAt, &deletedByID, &deletedByName); err != nil {
			return nil, 0, fmt.Errorf("trash: чтение узла БЗ: %w", err)
		}
		item := Item{
			Type:      TypeKB,
			ID:        id,
			Title:     kbTitle(title, parentTitle),
			DeletedAt: deletedAt,
		}
		item.DeletedByUserID = nullableInt64(deletedByID)
		item.DeletedByName = nullableString(deletedByName)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("trash: список узлов БЗ: %w", err)
	}
	return items, total, nil
}

// listEquipment — удалённое оборудование (название, рег. номер, склад/здание).
func (r *MySQLRepository) listEquipment(ctx context.Context, page, size int) ([]Item, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM equipment WHERE deleted_at IS NOT NULL`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("trash: количество оборудования: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT e.id, e.name, e.registration_number, w.name, b.id, b.name,
		       e.deleted_at, u.id, u.full_name
		FROM equipment e
		JOIN warehouses w ON w.id = e.warehouse_id
		JOIN buildings b ON b.id = w.building_id
		LEFT JOIN users u ON u.id = e.deleted_by
		WHERE e.deleted_at IS NOT NULL
		ORDER BY e.deleted_at DESC, e.id DESC
		LIMIT ? OFFSET ?`, size, (page-1)*size)
	if err != nil {
		return nil, 0, fmt.Errorf("trash: список оборудования: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Item, 0, size)
	for rows.Next() {
		var (
			id                                     int64
			name, reg, warehouseName, buildingName string
			buildingID                             int64
			deletedAt                              time.Time
			deletedByID                            sql.NullInt64
			deletedByName                          sql.NullString
		)
		if err := rows.Scan(&id, &name, &reg, &warehouseName, &buildingID, &buildingName,
			&deletedAt, &deletedByID, &deletedByName); err != nil {
			return nil, 0, fmt.Errorf("trash: чтение оборудования: %w", err)
		}
		item := Item{
			Type:         TypeEquipment,
			ID:           id,
			Title:        equipmentTitle(name, reg, warehouseName),
			DeletedAt:    deletedAt,
			BuildingID:   int64Ptr(buildingID),
			BuildingName: stringPtr(buildingName),
		}
		item.DeletedByUserID = nullableInt64(deletedByID)
		item.DeletedByName = nullableString(deletedByName)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("trash: список оборудования: %w", err)
	}
	return items, total, nil
}

// RestoreTicket возвращает заявку из корзины (TRASH-2). Статус, здание и
// completed_at не меняются — они и так сохранены в строке.
func (r *MySQLRepository) RestoreTicket(ctx context.Context, id int64) (TicketRestore, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return TicketRestore{}, fmt.Errorf("trash: транзакция восстановления заявки: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		info   TicketRestore
		active int
	)
	err = tx.QueryRowContext(ctx, `
		SELECT t.building_id, b.name, b.is_active
		FROM tickets t
		JOIN buildings b ON b.id = t.building_id
		WHERE t.id = ? AND t.deleted_at IS NOT NULL
		FOR UPDATE`, id).Scan(&info.BuildingID, &info.BuildingName, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return TicketRestore{}, ErrNotFound
	}
	if err != nil {
		return TicketRestore{}, fmt.Errorf("trash: чтение заявки из корзины: %w", err)
	}
	info.BuildingActive = active != 0

	if _, err := tx.ExecContext(ctx, `
		UPDATE tickets
		SET deleted_at = NULL, deleted_by = NULL, updated_at = UTC_TIMESTAMP()
		WHERE id = ? AND deleted_at IS NOT NULL`, id); err != nil {
		return TicketRestore{}, fmt.Errorf("trash: восстановление заявки: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return TicketRestore{}, fmt.Errorf("trash: фиксация восстановления заявки: %w", err)
	}
	return info, nil
}

// RestoreKB возвращает узел БЗ из корзины (TRASH-2, TRASH-5). Потомки остаются
// в корзине: их восстановление — отдельными запросами.
func (r *MySQLRepository) RestoreKB(ctx context.Context, id int64) (KBRestore, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return KBRestore{}, fmt.Errorf("trash: транзакция восстановления узла: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var parentID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT parent_id FROM kb_nodes
		WHERE id = ? AND deleted_at IS NOT NULL
		FOR UPDATE`, id).Scan(&parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return KBRestore{}, ErrNotFound
	}
	if err != nil {
		return KBRestore{}, fmt.Errorf("trash: чтение узла из корзины: %w", err)
	}

	info := KBRestore{ParentAlive: true}
	if parentID.Valid {
		value := parentID.Int64
		info.ParentID = &value

		var parentDeleted sql.NullTime
		err = tx.QueryRowContext(ctx, `SELECT deleted_at FROM kb_nodes WHERE id = ?`, value).
			Scan(&parentDeleted)
		if errors.Is(err, sql.ErrNoRows) {
			// Родитель физически удалён — восстановление невозможно.
			return KBRestore{}, ErrParentDeleted
		}
		if err != nil {
			return KBRestore{}, fmt.Errorf("trash: чтение родителя узла: %w", err)
		}
		if parentDeleted.Valid {
			return KBRestore{}, ErrParentDeleted
		}
	}

	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM kb_nodes
		WHERE parent_id = ? AND deleted_at IS NOT NULL`, id).Scan(&info.DeletedChildren); err != nil {
		return KBRestore{}, fmt.Errorf("trash: подсчёт потомков в корзине: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE kb_nodes
		SET deleted_at = NULL, deleted_by = NULL, updated_at = UTC_TIMESTAMP()
		WHERE id = ? AND deleted_at IS NOT NULL`, id); err != nil {
		return KBRestore{}, fmt.Errorf("trash: восстановление узла: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return KBRestore{}, fmt.Errorf("trash: фиксация восстановления узла: %w", err)
	}
	return info, nil
}

// RestoreEquipment возвращает единицу оборудования из корзины (TRASH-2):
// требуется живой склад и свободный регистрационный номер.
func (r *MySQLRepository) RestoreEquipment(ctx context.Context, id int64) (EquipmentRestore, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return EquipmentRestore{}, fmt.Errorf("trash: транзакция восстановления оборудования: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		warehouseID        int64
		registrationNumber string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT warehouse_id, registration_number FROM equipment
		WHERE id = ? AND deleted_at IS NOT NULL
		FOR UPDATE`, id).Scan(&warehouseID, &registrationNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return EquipmentRestore{}, ErrNotFound
	}
	if err != nil {
		return EquipmentRestore{}, fmt.Errorf("trash: чтение оборудования из корзины: %w", err)
	}

	// Склад должен существовать и быть живым (EQ-6: удалённый склад означает,
	// что оборудование размещать некуда).
	var (
		warehouseName    string
		warehouseDeleted sql.NullTime
		buildingName     string
		buildingActive   int
	)
	err = tx.QueryRowContext(ctx, `
		SELECT w.name, w.deleted_at, b.name, b.is_active
		FROM warehouses w
		JOIN buildings b ON b.id = w.building_id
		WHERE w.id = ?`, warehouseID).
		Scan(&warehouseName, &warehouseDeleted, &buildingName, &buildingActive)
	if errors.Is(err, sql.ErrNoRows) {
		return EquipmentRestore{}, ErrWarehouseDeleted
	}
	if err != nil {
		return EquipmentRestore{}, fmt.Errorf("trash: чтение склада оборудования: %w", err)
	}
	if warehouseDeleted.Valid {
		return EquipmentRestore{}, ErrWarehouseDeleted
	}

	var taken bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM equipment
			WHERE registration_number = ? AND deleted_at IS NULL AND id <> ?
		)`, registrationNumber, id).Scan(&taken); err != nil {
		return EquipmentRestore{}, fmt.Errorf("trash: проверка регистрационного номера: %w", err)
	}
	if taken {
		return EquipmentRestore{}, ErrDuplicateRegistration
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE equipment
		SET deleted_at = NULL, deleted_by = NULL, updated_at = UTC_TIMESTAMP()
		WHERE id = ? AND deleted_at IS NOT NULL`, id); err != nil {
		if mysqlErrorNumber(err) == mysqlErrDupEntry {
			return EquipmentRestore{}, ErrDuplicateRegistration
		}
		return EquipmentRestore{}, fmt.Errorf("trash: восстановление оборудования: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return EquipmentRestore{}, fmt.Errorf("trash: фиксация восстановления оборудования: %w", err)
	}

	return EquipmentRestore{
		WarehouseID:    warehouseID,
		WarehouseName:  warehouseName,
		BuildingName:   buildingName,
		BuildingActive: buildingActive != 0,
	}, nil
}

// PurgeTicket физически удаляет заявку вместе с комментариями и файлами
// (TRASH-3). Возвращает пути файлов для удаления с диска.
func (r *MySQLRepository) PurgeTicket(ctx context.Context, id int64) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("trash: транзакция удаления заявки: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var deletedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT deleted_at FROM tickets WHERE id = ? FOR UPDATE`, id).
		Scan(&deletedAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !deletedAt.Valid) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("trash: чтение заявки: %w", err)
	}

	files, err := ticketFiles(ctx, tx, id)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM ticket_comments WHERE ticket_id = ?`, id); err != nil {
		return nil, fmt.Errorf("trash: удаление комментариев заявки: %w", err)
	}

	// Файл мог быть привязан к нескольким комментариям: удаляем только те,
	// которые больше нигде не используются.
	paths := make([]string, 0, len(files))
	for _, file := range files {
		res, err := tx.ExecContext(ctx, `
			DELETE FROM files
			WHERE id = ? AND NOT EXISTS (
				SELECT 1 FROM comment_files cf WHERE cf.file_id = ?
			)`, file.id, file.id)
		if err != nil {
			return nil, fmt.Errorf("trash: удаление файла заявки: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("trash: удаление файла заявки: %w", err)
		}
		if affected > 0 {
			paths = append(paths, file.path)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tickets WHERE id = ? AND deleted_at IS NOT NULL`, id); err != nil {
		return nil, fmt.Errorf("trash: удаление заявки: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("trash: фиксация удаления заявки: %w", err)
	}
	return paths, nil
}

// PurgeKB физически удаляет узел и всё удалённое поддерево (TRASH-3). Удаление
// идёт от листьев к корню: FK parent_id — ON DELETE RESTRICT.
func (r *MySQLRepository) PurgeKB(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("trash: транзакция удаления узла: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `WITH RECURSIVE sub AS (
		SELECT id, 0 AS depth FROM kb_nodes WHERE id = ? AND deleted_at IS NOT NULL
		UNION ALL
		SELECT child.id, sub.depth + 1
		FROM kb_nodes child
		JOIN sub ON child.parent_id = sub.id
		WHERE child.deleted_at IS NOT NULL
	) SELECT id, depth FROM sub`, id)
	if err != nil {
		return fmt.Errorf("trash: выборка поддерева для удаления: %w", err)
	}

	byDepth := map[int][]int64{}
	maxDepth := -1
	count := 0
	for rows.Next() {
		var (
			memberID int64
			depth    int
		)
		if err := rows.Scan(&memberID, &depth); err != nil {
			_ = rows.Close()
			return fmt.Errorf("trash: чтение поддерева: %w", err)
		}
		byDepth[depth] = append(byDepth[depth], memberID)
		if depth > maxDepth {
			maxDepth = depth
		}
		count++
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("trash: чтение поддерева: %w", err)
	}
	_ = rows.Close()
	if count == 0 {
		return ErrNotFound
	}

	for depth := maxDepth; depth >= 0; depth-- {
		ids := byDepth[depth]
		if len(ids) == 0 {
			continue
		}
		args := make([]any, len(ids))
		for i, memberID := range ids {
			args[i] = memberID
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM kb_nodes WHERE id IN (`+placeholders(len(ids))+`)`, args...); err != nil {
			return fmt.Errorf("trash: удаление поддерева: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("trash: фиксация удаления поддерева: %w", err)
	}
	return nil
}

// PurgeEquipment физически удаляет единицу оборудования вместе с журналом
// перемещений (TRASH-3).
func (r *MySQLRepository) PurgeEquipment(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("trash: транзакция удаления оборудования: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var deletedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT deleted_at FROM equipment WHERE id = ? FOR UPDATE`, id).
		Scan(&deletedAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !deletedAt.Valid) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("trash: чтение оборудования: %w", err)
	}

	// Журнал ссылается на оборудование через FK RESTRICT: сначала строки
	// журнала, иначе удаление будет отклонено.
	if _, err := tx.ExecContext(ctx, `DELETE FROM equipment_movements WHERE equipment_id = ?`, id); err != nil {
		return fmt.Errorf("trash: удаление журнала оборудования: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM equipment WHERE id = ?`, id); err != nil {
		return fmt.Errorf("trash: удаление оборудования: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("trash: фиксация удаления оборудования: %w", err)
	}
	return nil
}

// Expired возвращает id элементов, удалённых раньше before (TRASH-4).
func (r *MySQLRepository) Expired(ctx context.Context, kind Type, before time.Time, limit int) ([]int64, error) {
	table, ok := tableName(kind)
	if !ok {
		return nil, ErrUnknownType
	}
	if limit <= 0 {
		limit = defaultBatch
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM `+table+`
		WHERE deleted_at IS NOT NULL AND deleted_at < ?
		ORDER BY deleted_at ASC, id ASC
		LIMIT ?`, before.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("trash: выборка просроченных (%s): %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("trash: чтение просроченных (%s): %w", table, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trash: чтение просроченных (%s): %w", table, err)
	}
	return ids, nil
}

type fileRef struct {
	id   int64
	path string
}

// ticketFiles собирает файлы, привязанные к комментариям заявки.
func ticketFiles(ctx context.Context, tx *sql.Tx, ticketID int64) ([]fileRef, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT f.id, f.stored_path
		FROM files f
		JOIN comment_files cf ON cf.file_id = f.id
		JOIN ticket_comments c ON c.id = cf.comment_id
		WHERE c.ticket_id = ?`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("trash: файлы заявки: %w", err)
	}
	defer func() { _ = rows.Close() }()

	files := make([]fileRef, 0, 8)
	for rows.Next() {
		var file fileRef
		if err := rows.Scan(&file.id, &file.path); err != nil {
			return nil, fmt.Errorf("trash: чтение файла заявки: %w", err)
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trash: файлы заявки: %w", err)
	}
	return files, nil
}

// tableName — whitelist таблиц по типу (в SQL не попадает пользовательский ввод).
func tableName(kind Type) (string, bool) {
	switch kind {
	case TypeTicket:
		return "tickets", true
	case TypeKB:
		return "kb_nodes", true
	case TypeEquipment:
		return "equipment", true
	default:
		return "", false
	}
}

func ticketTitle(id int64, applicant, cabinet, building string) string {
	return fmt.Sprintf("#%d · %s · каб. %s · %s", id, applicant, cabinet, building)
}

func kbTitle(title string, parent sql.NullString) string {
	if parent.Valid && strings.TrimSpace(parent.String) != "" {
		return fmt.Sprintf("%s · родитель: %s", title, parent.String)
	}
	return title + " · корень"
}

func equipmentTitle(name, registrationNumber, warehouse string) string {
	return fmt.Sprintf("%s · %s · %s", name, registrationNumber, warehouse)
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func int64Ptr(value int64) *int64 { return &value }

func stringPtr(value string) *string { return &value }

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// mysqlErrorNumber достаёт номер ошибки MySQL (0 — не MySQL-ошибка).
func mysqlErrorNumber(err error) uint16 {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number
	}
	return 0
}

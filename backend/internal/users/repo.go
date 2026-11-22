package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// MySQLRepository — реализация Repository на database/sql.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий пользователей.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// userColumns — единый набор полей с названием здания. Пароль наружу не
// отдаётся (DTO его не содержит), но нужен auth для проверок.
const userColumns = `
	u.id, u.login, u.full_name, u.role, u.password_hash,
	u.building_id, b.name, u.is_active, u.must_change_password,
	u.created_at, u.updated_at`

// List возвращает страницу пользователей с фильтрами. Все значения передаются
// параметрами; имя таблицы/колонок — константы кода.
func (r *MySQLRepository) List(ctx context.Context, filter ListFilter) ([]*auth.User, int, error) {
	where := []string{"1 = 1"}
	args := make([]any, 0, 6)

	if filter.Role != "" {
		where = append(where, "u.role = ?")
		args = append(args, string(filter.Role))
	}
	if filter.BuildingID != nil {
		where = append(where, "u.building_id = ?")
		args = append(args, *filter.BuildingID)
	}
	if filter.IsActive != nil {
		where = append(where, "u.is_active = ?")
		args = append(args, boolToInt(*filter.IsActive))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		where = append(where, "(u.full_name LIKE ? OR u.login LIKE ?)")
		like := "%" + search + "%"
		args = append(args, like, like)
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users u`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("users: подсчёт списка: %w", err)
	}

	page, size := filter.Page, filter.Size
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	offset := (page - 1) * size

	query := `SELECT ` + userColumns + `
		FROM users u
		LEFT JOIN buildings b ON b.id = u.building_id` + whereSQL + `
		ORDER BY u.full_name, u.id
		LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, query, append(args, size, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("users: список: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]*auth.User, 0, size)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("users: чтение списка: %w", err)
	}
	return items, total, nil
}

// Get загружает пользователя по id.
func (r *MySQLRepository) Get(ctx context.Context, id int64) (*auth.User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+userColumns+`
		FROM users u
		LEFT JOIN buildings b ON b.id = u.building_id
		WHERE u.id = ?
		LIMIT 1`, id)
	return scanUser(row)
}

// Create вставляет пользователя. Дубликат логина возвращается как
// ErrDuplicateLogin (AUTH-4: логин уникален).
func (r *MySQLRepository) Create(ctx context.Context, params CreateParams) (*auth.User, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO users (login, full_name, role, password_hash, building_id,
		                   is_active, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(), UTC_TIMESTAMP())`,
		params.Login, params.FullName, string(params.Role), params.PasswordHash,
		params.BuildingID, boolToInt(params.IsActive), boolToInt(params.MustChangePassword))
	if err != nil {
		if isDuplicateKey(err) {
			return nil, ErrDuplicateLogin
		}
		return nil, fmt.Errorf("users: создание: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("users: создание: %w", err)
	}
	return r.Get(ctx, id)
}

// Update применяет изменения в транзакции.
//
// AUTH-7: нельзя оставить систему без активного ADMIN. Для защиты от гонки
// блокируем все активные админские строки в порядке id (FOR UPDATE), затем
// целевую строку — это исключает взаимные блокировки и потерю последнего ADMIN.
func (r *MySQLRepository) Update(ctx context.Context, id int64, params UpdateParams) (*auth.User, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("users: транзакция изменения: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	adminIDs, err := lockActiveAdmins(ctx, tx)
	if err != nil {
		return nil, err
	}

	var (
		currentRole     string
		currentActive   int
		currentBuilding sql.NullInt64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT role, is_active, building_id
		FROM users
		WHERE id = ?
		FOR UPDATE`, id).Scan(&currentRole, &currentActive, &currentBuilding)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("users: чтение изменяемого пользователя: %w", err)
	}

	resultRole := auth.Role(currentRole)
	if params.Role != nil {
		resultRole = *params.Role
	}
	resultActive := currentActive != 0
	if params.IsActive != nil {
		resultActive = *params.IsActive
	}

	// AUTH-7: если активный ADMIN теряет роль или активность, должен остаться
	// хотя бы один другой активный ADMIN.
	if auth.Role(currentRole) == auth.RoleAdmin && currentActive != 0 &&
		(resultRole != auth.RoleAdmin || !resultActive) {
		others := 0
		for adminID := range adminIDs {
			if adminID != id {
				others++
			}
		}
		if others == 0 {
			return nil, ErrLastAdmin
		}
	}

	sets := make([]string, 0, 6)
	args := make([]any, 0, 7)
	sets = append(sets, "updated_at = UTC_TIMESTAMP()")
	if params.FullName != nil {
		sets = append(sets, "full_name = ?")
		args = append(args, *params.FullName)
	}
	if params.Role != nil {
		sets = append(sets, "role = ?")
		args = append(args, string(*params.Role))
	}
	if params.BuildingIDSet {
		sets = append(sets, "building_id = ?")
		args = append(args, params.BuildingID)
	}
	if params.IsActive != nil {
		sets = append(sets, "is_active = ?")
		args = append(args, boolToInt(*params.IsActive))
	}
	args = append(args, id)

	if _, err := tx.ExecContext(ctx, `UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, fmt.Errorf("users: изменение: %w", err)
	}

	updated, err := getTx(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("users: фиксация изменения: %w", err)
	}
	return updated, nil
}

// SetPassword меняет хеш пароля и флаг обязательной смены (AUTH-4/AUTH-5).
func (r *MySQLRepository) SetPassword(ctx context.Context, id int64, passwordHash string, mustChange bool) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET password_hash = ?, must_change_password = ?, updated_at = UTC_TIMESTAMP()
		WHERE id = ?`, passwordHash, boolToInt(mustChange), id)
	if err != nil {
		return fmt.Errorf("users: сброс пароля: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("users: сброс пароля: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// BuildingStatus проверяет существование и активность здания (AUTH-8).
func (r *MySQLRepository) BuildingStatus(ctx context.Context, id int64) (exists bool, active bool, err error) {
	var isActive int
	err = r.db.QueryRowContext(ctx, `SELECT is_active FROM buildings WHERE id = ? LIMIT 1`, id).Scan(&isActive)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("users: чтение здания: %w", err)
	}
	return true, isActive != 0, nil
}

// ListAssignable возвращает активных USER указанного здания и всех активных
// ADMIN (TICKET-8: ADMIN назначает USER из здания заявки либо ADMIN).
func (r *MySQLRepository) ListAssignable(ctx context.Context, buildingID int64) ([]*auth.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+userColumns+`
		FROM users u
		LEFT JOIN buildings b ON b.id = u.building_id
		WHERE u.is_active = 1
		  AND (u.role = 'ADMIN' OR (u.role = 'USER' AND u.building_id = ?))
		ORDER BY u.role DESC, u.full_name, u.id`, buildingID)
	if err != nil {
		return nil, fmt.Errorf("users: assignable: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]*auth.User, 0, 8)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("users: чтение assignable: %w", err)
	}
	return items, nil
}

// lockActiveAdmins блокирует активные админские записи в порядке id и
// возвращает их идентификаторы. Используется для проверки AUTH-7 в транзакции.
func lockActiveAdmins(ctx context.Context, tx *sql.Tx) (map[int64]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM users
		WHERE role = 'ADMIN' AND is_active = 1
		ORDER BY id
		FOR UPDATE`)
	if err != nil {
		return nil, fmt.Errorf("users: блокировка администраторов: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := make(map[int64]struct{}, 4)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("users: чтение администраторов: %w", err)
		}
		ids[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("users: чтение администраторов: %w", err)
	}
	return ids, nil
}

func getTx(ctx context.Context, tx *sql.Tx, id int64) (*auth.User, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+userColumns+`
		FROM users u
		LEFT JOIN buildings b ON b.id = u.building_id
		WHERE u.id = ?
		LIMIT 1`, id)
	return scanUser(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*auth.User, error) {
	var (
		user       auth.User
		buildingID sql.NullInt64
		buildingNm sql.NullString
		role       string
		isActive   int
		mustChange int
	)
	err := row.Scan(
		&user.ID, &user.Login, &user.FullName, &role, &user.PasswordHash,
		&buildingID, &buildingNm, &isActive, &mustChange,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("users: чтение пользователя: %w", err)
	}
	user.Role = auth.Role(role)
	user.IsActive = isActive != 0
	user.MustChangePassword = mustChange != 0
	if buildingID.Valid {
		user.BuildingID = &buildingID.Int64
	}
	if buildingNm.Valid {
		user.BuildingName = &buildingNm.String
	}
	return &user, nil
}

// isDuplicateKey распознаёт нарушение уникального индекса MySQL (1062).
func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// MySQLTicketResolver читает здание заявки для assignable. Таблица tickets
// создаётся в Б5; до этого вызов вернёт ошибку БД.
type MySQLTicketResolver struct {
	db *sql.DB
}

// NewMySQLTicketResolver создаёт резолвер зданий заявок.
func NewMySQLTicketResolver(db *sql.DB) *MySQLTicketResolver {
	return &MySQLTicketResolver{db: db}
}

// TicketBuildingID возвращает building_id неудалённой заявки.
func (r *MySQLTicketResolver) TicketBuildingID(ctx context.Context, ticketID int64) (int64, error) {
	var buildingID int64
	err := r.db.QueryRowContext(ctx, `
		SELECT building_id FROM tickets
		WHERE id = ? AND deleted_at IS NULL
		LIMIT 1`, ticketID).Scan(&buildingID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrTicketNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("users: чтение здания заявки: %w", err)
	}
	return buildingID, nil
}

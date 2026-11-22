package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Ошибки репозитория, которые сервис переводит в типизированные ответы API.
var (
	// ErrNotFound — пользователь/токен не найден.
	ErrNotFound = errors.New("auth: запись не найдена")
	// ErrRefreshReuse — токен уже был использован/отозван (AUTH-2: повтор
	// старого refresh-токена → 401 и отзыв всей цепочки).
	ErrRefreshReuse = errors.New("auth: refresh-токен уже использован")
)

// Repository — хранилище пользователей и refresh-токенов. Интерфейс позволяет
// тестировать сервис без MySQL.
type Repository interface {
	UserByLogin(ctx context.Context, login string) (*User, error)
	UserByID(ctx context.Context, id int64) (*User, error)
	UpdatePassword(ctx context.Context, userID int64, passwordHash string, mustChange bool) error

	CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (int64, error)
	RefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	// RotateRefreshToken атомарно отзывает старый токен и выпускает новый.
	// Если старый уже отозван/не найден — ErrRefreshReuse.
	RotateRefreshToken(ctx context.Context, oldID, userID int64, newHash string, newExpiresAt time.Time) (int64, error)
	RevokeRefreshToken(ctx context.Context, id int64) error
	// RevokeAllRefreshTokens отзывает все активные токены пользователя, кроме
	// указанного хеша (используется при смене пароля).
	RevokeAllRefreshTokens(ctx context.Context, userID int64, exceptHash string) error
	// RevokeRefreshTokenChain отзывает токен и всю цепочку его замен.
	RevokeRefreshTokenChain(ctx context.Context, startID int64) error
}

// MySQLRepository — реализация Repository на database/sql.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

const userSelectColumns = `
	u.id, u.login, u.full_name, u.role, u.password_hash,
	u.building_id, b.name, u.is_active, u.must_change_password,
	u.created_at, u.updated_at`

// UserByLogin ищет пользователя по логину (вместе с названием здания).
func (r *MySQLRepository) UserByLogin(ctx context.Context, login string) (*User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+userSelectColumns+`
		FROM users u
		LEFT JOIN buildings b ON b.id = u.building_id
		WHERE u.login = ?
		LIMIT 1`, login)
	return scanUser(row)
}

// UserByID загружает пользователя по id — используется requireAuth на каждом
// запросе (AUTH-6: проверка is_active в БД).
func (r *MySQLRepository) UserByID(ctx context.Context, id int64) (*User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+userSelectColumns+`
		FROM users u
		LEFT JOIN buildings b ON b.id = u.building_id
		WHERE u.id = ?
		LIMIT 1`, id)
	return scanUser(row)
}

// UpdatePassword меняет хеш пароля и флаг обязательной смены.
func (r *MySQLRepository) UpdatePassword(ctx context.Context, userID int64, passwordHash string, mustChange bool) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET password_hash = ?, must_change_password = ?, updated_at = UTC_TIMESTAMP()
		WHERE id = ?`, passwordHash, boolToInt(mustChange), userID)
	if err != nil {
		return fmt.Errorf("auth: обновление пароля: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("auth: обновление пароля: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateRefreshToken сохраняет SHA-256 хеш нового refresh-токена.
func (r *MySQLRepository) CreateRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, UTC_TIMESTAMP())`, userID, tokenHash, expiresAt.UTC())
	if err != nil {
		return 0, fmt.Errorf("auth: сохранение refresh-токена: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("auth: сохранение refresh-токена: %w", err)
	}
	return id, nil
}

// RefreshTokenByHash находит токен по хешу.
func (r *MySQLRepository) RefreshTokenByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var (
		t          RefreshToken
		revokedAt  sql.NullTime
		replacedBy sql.NullInt64
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, expires_at, revoked_at, replaced_by, created_at
		FROM refresh_tokens
		WHERE token_hash = ?
		LIMIT 1`, tokenHash).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &revokedAt, &replacedBy, &t.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: чтение refresh-токена: %w", err)
	}
	if revokedAt.Valid {
		t.RevokedAt = &revokedAt.Time
	}
	if replacedBy.Valid {
		t.ReplacedBy = &replacedBy.Int64
	}
	return &t, nil
}

// RotateRefreshToken в одной транзакции отзывает старый токен и создаёт новый,
// проставляя replaced_by. Условный UPDATE защищает от гонки двух одновременных
// refresh: второй получит ErrRefreshReuse и цепочка будет отозвана (AUTH-2).
func (r *MySQLRepository) RotateRefreshToken(ctx context.Context, oldID, userID int64, newHash string, newExpiresAt time.Time) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("auth: транзакция ротации refresh-токена: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = UTC_TIMESTAMP()
		WHERE id = ? AND revoked_at IS NULL`, oldID)
	if err != nil {
		return 0, fmt.Errorf("auth: отзыв refresh-токена: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("auth: отзыв refresh-токена: %w", err)
	}
	if affected == 0 {
		return 0, ErrRefreshReuse
	}

	insert, err := tx.ExecContext(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, UTC_TIMESTAMP())`, userID, newHash, newExpiresAt.UTC())
	if err != nil {
		return 0, fmt.Errorf("auth: создание refresh-токена: %w", err)
	}
	newID, err := insert.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("auth: создание refresh-токена: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET replaced_by = ? WHERE id = ?`, newID, oldID); err != nil {
		return 0, fmt.Errorf("auth: связывание refresh-токенов: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("auth: фиксация ротации refresh-токена: %w", err)
	}
	return newID, nil
}

// RevokeRefreshToken отзывает конкретный токен (logout).
func (r *MySQLRepository) RevokeRefreshToken(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, UTC_TIMESTAMP())
		WHERE id = ?`, id); err != nil {
		return fmt.Errorf("auth: отзыв refresh-токена: %w", err)
	}
	return nil
}

// RevokeAllRefreshTokens отзывает все активные токены пользователя, кроме
// exceptHash (смена пароля оставляет текущую сессию рабочей).
func (r *MySQLRepository) RevokeAllRefreshTokens(ctx context.Context, userID int64, exceptHash string) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, UTC_TIMESTAMP())
		WHERE user_id = ? AND revoked_at IS NULL AND token_hash <> ?`, userID, exceptHash); err != nil {
		return fmt.Errorf("auth: отзыв refresh-токенов пользователя: %w", err)
	}
	return nil
}

// RevokeRefreshTokenChain отзывает токен и всех его потомков по replaced_by.
// Итеративный обход вместо рекурсивного CTE: в MySQL UPDATE с CTE по той же
// таблице ограничен, а цепочка короткая (одна замена на refresh).
func (r *MySQLRepository) RevokeRefreshTokenChain(ctx context.Context, startID int64) error {
	id := startID
	for i := 0; i < 256 && id != 0; i++ {
		if _, err := r.db.ExecContext(ctx, `
			UPDATE refresh_tokens
			SET revoked_at = COALESCE(revoked_at, UTC_TIMESTAMP())
			WHERE id = ?`, id); err != nil {
			return fmt.Errorf("auth: отзыв цепочки refresh-токенов: %w", err)
		}
		var next sql.NullInt64
		err := r.db.QueryRowContext(ctx, `SELECT replaced_by FROM refresh_tokens WHERE id = ?`, id).Scan(&next)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("auth: обход цепочки refresh-токенов: %w", err)
		}
		id = next.Int64
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*User, error) {
	var (
		u          User
		buildingID sql.NullInt64
		buildingNm sql.NullString
		isActive   int
		mustChange int
		role       string
	)
	err := row.Scan(
		&u.ID, &u.Login, &u.FullName, &role, &u.PasswordHash,
		&buildingID, &buildingNm, &isActive, &mustChange,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: чтение пользователя: %w", err)
	}
	u.Role = Role(role)
	u.IsActive = isActive != 0
	u.MustChangePassword = mustChange != 0
	if buildingID.Valid {
		u.BuildingID = &buildingID.Int64
	}
	if buildingNm.Valid {
		u.BuildingName = &buildingNm.String
	}
	return &u, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

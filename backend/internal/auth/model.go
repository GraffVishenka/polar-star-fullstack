package auth

import "time"

// Role — роль пользователя (PLAN §3.1). Роли фиксированы.
type Role string

const (
	RoleUser  Role = "USER"
	RoleAdmin Role = "ADMIN"
)

// Valid сообщает, что роль известна системе.
func (r Role) Valid() bool { return r == RoleUser || r == RoleAdmin }

// User — аккаунт из таблицы users (без секретов в JSON-представлении).
type User struct {
	ID                 int64
	Login              string
	FullName           string
	Role               Role
	PasswordHash       string
	BuildingID         *int64
	BuildingName       *string
	IsActive           bool
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// IsAdmin — удобная проверка роли.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// RefreshToken — строка refresh-токена. В БД хранится только SHA-256 хеш
// (AUTH-2), открытый токен живёт лишь в httpOnly cookie.
type RefreshToken struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *int64
	CreatedAt  time.Time
}

// Active сообщает, что токен ещё можно использовать.
func (t *RefreshToken) Active(now time.Time) bool {
	return t.RevokedAt == nil && now.Before(t.ExpiresAt)
}

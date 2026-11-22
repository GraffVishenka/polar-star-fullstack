// Package users реализует администрирование пользователей (AUTH-4..AUTH-9, Б2):
// список с фильтрами, создание, изменение роли/здания/активности, сброс пароля
// и подбор исполнителей для заявки (TICKET-8).
//
// Слои: Handler → Service → Repository. Бизнес-правила ролей и зданий живут в
// Service, атомарная защита «последнего ADMIN» — в транзакции Repository.
package users

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// Ошибки репозитория/сервиса, которые handler переводит в типизированные ответы.
var (
	// ErrNotFound — пользователь не найден.
	ErrNotFound = errors.New("users: пользователь не найден")
	// ErrDuplicateLogin — логин уже занят (уникальный индекс users.login).
	ErrDuplicateLogin = errors.New("users: логин уже занят")
	// ErrLastAdmin — попытка оставить систему без активного ADMIN (AUTH-7).
	ErrLastAdmin = errors.New("users: последний активный администратор")
	// ErrTicketNotFound — заявка не найдена (assignable).
	ErrTicketNotFound = errors.New("users: заявка не найдена")
)

// ListFilter — фильтры админ-списка пользователей.
type ListFilter struct {
	Role       auth.Role
	BuildingID *int64
	IsActive   *bool
	Search     string
	Page       int
	Size       int
}

// ListResult — страница пользователей.
type ListResult struct {
	Items []*auth.User
	Total int
	Page  int
	Size  int
}

// CreateInput — данные создания пользователя ADMIN'ом (AUTH-4).
type CreateInput struct {
	Login      string
	FullName   string
	Role       auth.Role
	Password   string
	BuildingID *int64
}

// UpdateInput — частичное изменение пользователя. Указатели означают
// «переданное поле», а BuildingID.Set позволяет отличить отсутствие поля от
// явного `null` (очистка здания при USER→ADMIN, AUTH-8).
type UpdateInput struct {
	FullName   *string
	Role       *auth.Role
	BuildingID OptionalInt64
	IsActive   *bool
}

// CreateParams — нормализованные данные для INSERT.
type CreateParams struct {
	Login              string
	FullName           string
	Role               auth.Role
	PasswordHash       string
	BuildingID         *int64
	IsActive           bool
	MustChangePassword bool
}

// UpdateParams — итоговые значения для UPDATE (после бизнес-валидации).
type UpdateParams struct {
	FullName      *string
	Role          *auth.Role
	BuildingID    *int64
	BuildingIDSet bool
	IsActive      *bool
}

// Repository — хранилище пользователей (MySQL в проде, in-memory в тестах).
type Repository interface {
	List(ctx context.Context, filter ListFilter) ([]*auth.User, int, error)
	Get(ctx context.Context, id int64) (*auth.User, error)
	Create(ctx context.Context, params CreateParams) (*auth.User, error)
	// Update применяет изменения в транзакции и атомарно проверяет, что в
	// системе останется хотя бы один активный ADMIN (AUTH-7).
	Update(ctx context.Context, id int64, params UpdateParams) (*auth.User, error)
	SetPassword(ctx context.Context, id int64, passwordHash string, mustChange bool) error
	// BuildingStatus сообщает, существует ли здание и активно ли оно (AUTH-8).
	BuildingStatus(ctx context.Context, id int64) (exists bool, active bool, err error)
	// ListAssignable — активные USER указанного здания + все активные ADMIN.
	ListAssignable(ctx context.Context, buildingID int64) ([]*auth.User, error)
}

// TokenRevoker отзывает refresh-сессии пользователя (AUTH-2/AUTH-6).
// Реализуется auth.Repository, поэтому пакет users не зависит от SQL auth-слоя.
type TokenRevoker interface {
	RevokeAllRefreshTokens(ctx context.Context, userID int64, exceptHash string) error
}

// TicketBuildingResolver сообщает здание заявки для assignable (TICKET-8).
// До Б5 таблица tickets отсутствует — интерфейс позволяет подменить реализацию.
type TicketBuildingResolver interface {
	TicketBuildingID(ctx context.Context, ticketID int64) (int64, error)
}

// OptionalInt64 — поле JSON, различающее отсутствие (`Set=false`) и `null`
// (`Set=true, Value=nil`). Нужно для PATCH building_id (AUTH-8).
type OptionalInt64 struct {
	Set   bool
	Value *int64
}

// UnmarshalJSON реализует разбор с сохранением факта присутствия поля.
func (o *OptionalInt64) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value int64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

// UserDTO — ответ API; переиспользует единое представление auth (без хеша).
type UserDTO = auth.UserDTO

// AssignableUserDTO — кандидат в исполнители (GET /api/users/assignable).
type AssignableUserDTO struct {
	ID           int64     `json:"id"`
	Login        string    `json:"login"`
	FullName     string    `json:"full_name"`
	Role         auth.Role `json:"role"`
	BuildingID   *int64    `json:"building_id"`
	BuildingName *string   `json:"building_name"`
}

// NewAssignableUserDTO конвертирует доменную модель.
func NewAssignableUserDTO(u *auth.User) AssignableUserDTO {
	return AssignableUserDTO{
		ID:           u.ID,
		Login:        u.Login,
		FullName:     u.FullName,
		Role:         u.Role,
		BuildingID:   u.BuildingID,
		BuildingName: u.BuildingName,
	}
}

// UserListDTO — ответ списка пользователей (контракт UserListResponse).
type UserListDTO struct {
	Items []UserDTO `json:"items"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Size  int       `json:"size"`
}

// Package tickets реализует заявки (TICKET-1..TICKET-16), публичный периметр
// заявок (TICKET-5, TICKET-6, TICKET-13, PUB-2..PUB-6) и комментарии.
//
// Слои: Handler → Service → Repository. Правила видимости по зданию и ролям
// живут в Service; дедлайн считается через platform.Calendar и настройки
// TicketSettings (до Б7 — статический дефолт deadline_days=2).
package tickets

import (
	"context"
	"errors"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// Status — фиксированный код статуса заявки (TICKET-1). Названия/цвета
// настраиваются и отдаются через StatusLabel/StatusColor.
type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusDeferred   Status = "deferred"
	StatusDone       Status = "done"
)

// Valid сообщает, что код статуса известен.
func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusInProgress, StatusDeferred, StatusDone:
		return true
	default:
		return false
	}
}

// PublicOnly — статусы, которые видны анонимам (TICKET-5: без done).
func (s Status) PublicOnly() bool {
	return s == StatusNew || s == StatusInProgress || s == StatusDeferred
}

// AllStatuses — все допустимые статусы.
func AllStatuses() []Status {
	return []Status{StatusNew, StatusInProgress, StatusDeferred, StatusDone}
}

// PublicStatuses — три публичных статуса.
func PublicStatuses() []Status {
	return []Status{StatusNew, StatusInProgress, StatusDeferred}
}

// Ticket — доменная модель заявки с раскрытыми именами здания, вида проблемы
// и исполнителя.
type Ticket struct {
	ID              int64
	BuildingID      int64
	BuildingName    string
	ApplicantName   string
	Cabinet         string
	ProblemTypeID   int64
	ProblemTypeName string
	Description     string
	Deadline        *time.Time
	ExecutorID      *int64
	ExecutorName    *string
	Status          Status
	CompletedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
	AccessTokenHash *string
	CreatedByUserID *int64
}

// Comment — комментарий заявки (TICKET-14).
type Comment struct {
	ID           int64
	TicketID     int64
	AuthorUserID *int64
	AuthorName   string
	Body         string
	CreatedAt    time.Time
	DeletedAt    *time.Time
	// Files — вложения комментария (comment_files + files, Б3).
	Files []*Attachment
}

// Attachment — метаданные вложения комментария (таблица files, Б3).
type Attachment struct {
	ID        int64
	Filename  string
	MimeType  string
	Size      int64
	Scope     string
	CreatedAt time.Time
}

// BuildingCount — активное здание со счётчиками публичного экрана (TICKET-6).
type BuildingCount struct {
	ID         int64
	Name       string
	New        int
	InProgress int
	Deferred   int
}

// Building — минимальные данные здания для проверки активности (EQ-1/EQ-6).
type Building struct {
	ID       int64
	Name     string
	IsActive bool
}

// OptionalInt64 различает отсутствие поля и явный null (PATCH executor_id /
// building_id).
type OptionalInt64 struct {
	Set   bool
	Value *int64
}

// OptionalDate различает отсутствие поля и явный null (PATCH deadline).
type OptionalDate struct {
	Set   bool
	Value *string
}

// ListFilter — фильтры авторизованного списка (TICKET-15).
type ListFilter struct {
	BuildingID *int64
	Statuses   []Status
	ExecutorID *int64
	// ExecutorNone — сентинел `executor_id=none`: только заявки без
	// исполнителя (клиентский фильтр «Не назначен», Ф2).
	ExecutorNone  bool
	ProblemTypeID *int64
	DateFrom      *time.Time // локальная дата начала (включительно)
	DateTo        *time.Time // локальная дата конца (включительно)
	Search        string
	Sort          string
	Order         string
	Page          int
	Size          int
}

// ListResult — страница заявок.
type ListResult struct {
	Items []*Ticket
	Total int
	Page  int
	Size  int
}

// CommentPage — страница комментариев.
type CommentPage struct {
	Items []*Comment
	Total int
	Page  int
	Size  int
}

// CreateInput — создание заявки авторизованным пользователем (TICKET-2).
type CreateInput struct {
	BuildingID    *int64
	ApplicantName string
	Cabinet       string
	ProblemTypeID int64
	Description   string
	ExecutorID    *int64
}

// PublicCreateInput — создание заявки анонимом (TICKET-4).
type PublicCreateInput struct {
	BuildingID    int64
	ApplicantName string
	Cabinet       string
	ProblemTypeID int64
	Description   string
}

// UpdateInput — частичное изменение заявки (TICKET-8..TICKET-11).
type UpdateInput struct {
	ApplicantName *string
	Cabinet       *string
	ProblemTypeID *int64
	Description   *string
	Status        *Status
	ExecutorID    OptionalInt64
	Deadline      OptionalDate
	BuildingID    OptionalInt64
}

// PublicBoard — ответ публичного дашборда (TICKET-5).
type PublicBoard struct {
	BuildingID   *int64
	BuildingName *string
	Items        []*Ticket
}

// Ошибки репозитория, которые сервис переводит в типизированные ответы.
var (
	ErrNotFound            = errors.New("tickets: заявка не найдена")
	ErrCommentNotFound     = errors.New("tickets: комментарий не найден")
	ErrBuildingNotFound    = errors.New("tickets: здание не найдено")
	ErrExecutorNotFound    = errors.New("tickets: исполнитель не найден")
	ErrProblemTypeNotFound = errors.New("tickets: вид проблемы не найден")
	ErrAccessTokenNotFound = errors.New("tickets: токен доступа не найден")
)

// TicketSettings — настройки, от которых зависит расчёт дедлайна (CAL-1..CAL-4).
// До Б7 используется StaticSettings; Б7 подключит настройки из БД.
type TicketSettings interface {
	DeadlineDays() int
	Calendar() platform.Calendar
}

// StatusStyler — необязательное расширение TicketSettings: настраиваемые
// названия и цвета статусов (TICKET-1, Б7). Если настройки его не реализуют,
// используются дефолты из dto.go.
type StatusStyler interface {
	StatusStyle(status Status) (label, color string)
}

// FileAttacher — интеграция с доменом файлов (Б3). ValidateScope проверяет
// существование/область файлов до создания комментария, ValidateAndAttach
// создаёт связи comment_files. Пока Б3 не смержен, в main передаётся
// DisabledFileAttacher.
type FileAttacher interface {
	ValidateScope(ctx context.Context, fileIDs []int64, scope string) error
	ValidateAndAttach(ctx context.Context, fileIDs []int64, commentID int64) error
}

// UserLookup — доступ к пользователям для проверки исполнителя (TICKET-8).
// Реализуется users.Repository.
type UserLookup interface {
	Get(ctx context.Context, id int64) (*auth.User, error)
}

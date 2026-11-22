// Package stats реализует статистику по сотрудникам (Б9, STATS-1..STATS-4):
// метрики завершённых заявок за период, сводную статистику для ADMIN и
// подготовку строк для Excel-выгрузки Б11.
//
// Все расчёты выполняются в часовом поясе приложения (CAL-2, NFR-7): период
// задаётся датами `from`/`to`, границы суток переводятся в UTC для запроса к БД,
// а группировка по месяцам и сравнение с дедлайном — в поясе приложения.
package stats

import (
	"context"
	"errors"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// ErrNotFound — сотрудник не найден (ADMIN запросил несуществующий user_id).
var ErrNotFound = errors.New("stats: сотрудник не найден")

// Filter — условия выборки завершённых заявок (STATS-2): корзина исключается,
// учитываются только заявки с completed_at.
type Filter struct {
	// ExecutorID — исполнитель; nil — все сотрудники (сводный отчёт).
	ExecutorID *int64
	// BuildingID — фильтр по зданию (только ADMIN); nil — все здания.
	BuildingID *int64
	// From — начало периода по completed_at включительно (UTC).
	From *time.Time
	// ToExclusive — конец периода по completed_at исключительно (UTC).
	ToExclusive *time.Time
}

// WithoutPeriod возвращает тот же фильтр без ограничения по периоду: нужно для
// метрики total_done (всё время, STATS-1).
func (f Filter) WithoutPeriod() Filter {
	f.From = nil
	f.ToExclusive = nil
	return f
}

// CompletedTicket — завершённая заявка, по которой считаются метрики и строки
// выгрузки. Дедлайн — DATE (полночь UTC), сравнивается по календарной дате.
type CompletedTicket struct {
	TicketID        int64
	BuildingID      int64
	BuildingName    string
	ProblemTypeID   *int64
	ProblemTypeName string
	Cabinet         string
	ApplicantName   string
	CreatedAt       time.Time
	CompletedAt     time.Time
	Deadline        *time.Time
}

// Employee — сотрудник для сводной статистики (STATS-1).
type Employee struct {
	ID           int64
	Login        string
	FullName     string
	Role         auth.Role
	BuildingID   *int64
	BuildingName *string
}

// ProblemTypeCount — разбивка по видам проблем (контракт StatsProblemType).
type ProblemTypeCount struct {
	ProblemTypeID *int64
	Name          string
	Count         int
}

// MonthCount — разбивка по месяцам (контракт StatsMonth, `YYYY-MM`).
type MonthCount struct {
	Month string
	Count int
}

// Metrics — метрики набора завершённых заявок (STATS-2).
type Metrics struct {
	// DoneInPeriod — число завершённых заявок периода.
	DoneInPeriod int
	// AvgResolutionHours — среднее время выполнения, часы с округлением до 0.1;
	// nil, если завершённых заявок нет.
	AvgResolutionHours *float64
	// MedianResolutionHours — медиана времени выполнения, часы до 0.1; nil при
	// пустом наборе.
	MedianResolutionHours *float64
	// OnTimePercent — доля выполненных в срок среди заявок с дедлайном; nil,
	// если заявок с дедлайном нет.
	OnTimePercent *float64
	// OverdueCount — число заявок периода, выполненных позже дедлайна.
	OverdueCount int
	// ByProblemType — разбивка по видам проблем (по убыванию количества).
	ByProblemType []ProblemTypeCount
	// ByMonth — разбивка по месяцам (по возрастанию `YYYY-MM`).
	ByMonth []MonthCount
}

// Stats — статистика сотрудника (контракт StatsResponse).
type Stats struct {
	Employee
	// Period — запрошенный период (nil-границы означают «без ограничения»).
	Period Period
	// TotalDone — все завершённые заявки сотрудника за всё время (без корзины).
	TotalDone int
	Metrics
}

// Period — запрошенный период. Даты — в часовом поясе приложения.
type Period struct {
	From *time.Time
	To   *time.Time
}

// Empty сообщает, что период не задан.
func (p Period) Empty() bool { return p.From == nil && p.To == nil }

// Query — параметры запроса статистики, разобранные из HTTP.
type Query struct {
	// UserID — сотрудник (для ADMIN); у /api/stats/me игнорируется.
	UserID *int64
	// BuildingID — фильтр по зданию (для ADMIN и сводного отчёта).
	BuildingID *int64
	// From и To — даты периода в часовом поясе приложения (полночь).
	From *time.Time
	To   *time.Time
}

// ExportRow — строка выгрузки Excel (STATS-3, Б11): Б11 формирует `.xlsx` из
// этих данных без повторных SQL-запросов.
type ExportRow struct {
	TicketID      int64
	Building      string
	CreatedAt     time.Time
	CompletedAt   time.Time
	Deadline      *time.Time
	OnTime        *bool
	ProblemType   string
	Cabinet       string
	ApplicantName string
}

// Repository — источник данных статистики (MySQL в проде, in-memory в тестах).
type Repository interface {
	// CountCompleted — число завершённых заявок (без корзины) по фильтру.
	CountCompleted(ctx context.Context, filter Filter) (int, error)
	// CompletedTickets — завершённые заявки по фильтру с именами здания и вида
	// проблемы, упорядоченные по completed_at.
	CompletedTickets(ctx context.Context, filter Filter) ([]CompletedTicket, error)
	// Users — активные сотрудники для сводной статистики; buildingID != nil —
	// только сотрудники этого здания (ADMIN без закрепления не попадают).
	Users(ctx context.Context, buildingID *int64) ([]Employee, error)
	// UserByID — сотрудник по id (включая заблокированных: их история важна).
	UserByID(ctx context.Context, id int64) (*Employee, error)
}

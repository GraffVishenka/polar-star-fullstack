package stats

import (
	"context"
	"errors"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Options — зависимости сервиса статистики.
type Options struct {
	// Calendar — источник часового пояса приложения (CAL-2); Б7 заменит
	// реализацию на настройки из БД, сервис менять не потребуется.
	Calendar platform.Calendar
}

// Service — бизнес-логика статистики (STATS-1, STATS-2, STATS-3).
type Service struct {
	repo Repository
	loc  *time.Location
}

// NewService создаёт сервис статистики.
func NewService(repo Repository, opts Options) *Service {
	loc := time.UTC
	if opts.Calendar != nil && opts.Calendar.Location() != nil {
		loc = opts.Calendar.Location()
	}
	return &Service{repo: repo, loc: loc}
}

// Location — часовой пояс приложения (нужен handler'у для разбора дат).
func (s *Service) Location() *time.Location { return s.loc }

// MyStats — личная статистика текущего пользователя (STATS-1: USER/ADMIN).
// STATS-2: личная статистика не ограничена зданием, поэтому фильтр building_id
// для этого запроса игнорируется.
func (s *Service) MyStats(ctx context.Context, actor *auth.User, query Query) (*Stats, error) {
	if actor == nil {
		return nil, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
	}
	period, err := s.validatePeriod(query)
	if err != nil {
		return nil, err
	}
	query.UserID = nil
	query.BuildingID = nil
	employee := Employee{
		ID:           actor.ID,
		Login:        actor.Login,
		FullName:     actor.FullName,
		Role:         actor.Role,
		BuildingID:   actor.BuildingID,
		BuildingName: actor.BuildingName,
	}
	return s.buildStats(ctx, employee, query, period)
}

// AdminStats — сводная статистика (STATS-1, только ADMIN).
//
// Без user_id возвращает список по активным сотрудникам (при фильтре по зданию —
// только закреплённые за ним USER), с user_id — одного сотрудника, включая
// заблокированного: его история завершённых заявок остаётся доступной.
func (s *Service) AdminStats(ctx context.Context, actor *auth.User, query Query) ([]*Stats, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	period, err := s.validatePeriod(query)
	if err != nil {
		return nil, err
	}

	if query.UserID != nil {
		employee, err := s.repo.UserByID(ctx, *query.UserID)
		if err != nil {
			return nil, mapRepoError(err)
		}
		stats, err := s.buildStats(ctx, *employee, query, period)
		if err != nil {
			return nil, err
		}
		return []*Stats{stats}, nil
	}

	employees, err := s.repo.Users(ctx, query.BuildingID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	items := make([]*Stats, 0, len(employees))
	for i := range employees {
		stats, err := s.buildStats(ctx, employees[i], query, period)
		if err != nil {
			return nil, err
		}
		items = append(items, stats)
	}
	return items, nil
}

// ExportRows готовит строки для Excel-выгрузки Б11 (STATS-3): данные выбираются
// один раз, генератор `.xlsx` не делает повторных SQL.
//
// STATS-3: свой отчёт получает любой, чужой — только ADMIN; USER с чужим
// user_id получает 403 FORBIDDEN.
func (s *Service) ExportRows(ctx context.Context, actor *auth.User, query Query) ([]ExportRow, error) {
	if actor == nil {
		return nil, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
	}
	targetID := actor.ID
	if query.UserID != nil {
		if actor.Role != auth.RoleAdmin && *query.UserID != actor.ID {
			return nil, apierr.Forbidden("Недостаточно прав для выгрузки чужой статистики")
		}
		targetID = *query.UserID
		if targetID != actor.ID {
			// ADMIN выгружает чужой отчёт — сотрудник должен существовать.
			if _, err := s.repo.UserByID(ctx, targetID); err != nil {
				return nil, mapRepoError(err)
			}
		}
	}
	if _, err := s.validatePeriod(query); err != nil {
		return nil, err
	}

	tickets, err := s.repo.CompletedTickets(ctx, s.buildFilter(query, targetID))
	if err != nil {
		return nil, apierr.Internal(err)
	}
	rows := make([]ExportRow, 0, len(tickets))
	for _, ticket := range tickets {
		row := ExportRow{
			TicketID:      ticket.TicketID,
			Building:      ticket.BuildingName,
			CreatedAt:     ticket.CreatedAt,
			CompletedAt:   ticket.CompletedAt,
			Deadline:      ticket.Deadline,
			ProblemType:   ticket.ProblemTypeName,
			Cabinet:       ticket.Cabinet,
			ApplicantName: ticket.ApplicantName,
		}
		if onTime, hasDeadline := OnTimeStatus(ticket, s.loc); hasDeadline {
			value := onTime
			row.OnTime = &value
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// buildStats собирает метрики одного сотрудника: total_done за всё время и
// метрики периода (STATS-1, STATS-2).
func (s *Service) buildStats(ctx context.Context, employee Employee, query Query, period Period) (*Stats, error) {
	filter := s.buildFilter(query, employee.ID)
	totalDone, err := s.repo.CountCompleted(ctx, filter.WithoutPeriod())
	if err != nil {
		return nil, apierr.Internal(err)
	}
	tickets, err := s.repo.CompletedTickets(ctx, filter)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return &Stats{
		Employee:  employee,
		Period:    period,
		TotalDone: totalDone,
		Metrics:   ComputeMetrics(tickets, s.loc),
	}, nil
}

// buildFilter переводит Query в фильтр репозитория. Границы периода задаются в
// поясе приложения и передаются в БД в UTC: `from` — начало суток, `to` —
// начало следующих суток (период включается включительно, STATS-2).
func (s *Service) buildFilter(query Query, executorID int64) Filter {
	filter := Filter{ExecutorID: &executorID, BuildingID: query.BuildingID}
	if query.From != nil {
		from := s.startOfDayUTC(*query.From)
		filter.From = &from
	}
	if query.To != nil {
		to := s.startOfDayUTC(*query.To).AddDate(0, 0, 1)
		filter.ToExclusive = &to
	}
	return filter
}

// startOfDayUTC возвращает полночь даты в поясе приложения, выраженную в UTC.
func (s *Service) startOfDayUTC(date time.Time) time.Time {
	year, month, day := date.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, s.loc).UTC()
}

// validatePeriod проверяет порядок границ периода (422 VALIDATION_ERROR).
func (s *Service) validatePeriod(query Query) (Period, error) {
	if query.From != nil && query.To != nil && query.From.After(*query.To) {
		return Period{}, apierr.Validation("Некорректный период",
			map[string]any{"to": "должен быть не раньше from"})
	}
	return Period{From: query.From, To: query.To}, nil
}

// requireAdmin — проверка роли для сводной статистики (STATS-1).
func requireAdmin(actor *auth.User) error {
	if actor == nil {
		return apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
	}
	if actor.Role != auth.RoleAdmin {
		return apierr.Forbidden("Недостаточно прав: сводная статистика доступна только администратору")
	}
	return nil
}

// mapRepoError переводит ошибки репозитория в коды контракта.
func mapRepoError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return apierr.NotFound("Сотрудник не найден")
	}
	return apierr.Internal(err)
}

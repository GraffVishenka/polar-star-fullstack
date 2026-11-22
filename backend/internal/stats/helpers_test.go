package stats

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// baseTime — фиксированный момент времени для детерминированных тестов.
var baseTime = time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)

// yekaterinburg загружает пояс приложения (CAL-2).
func yekaterinburg(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		t.Fatalf("LoadLocation(Asia/Yekaterinburg): %v", err)
	}
	return loc
}

// fakeTicket — завершённая заявка в памяти с исполнителем и признаком корзины:
// фейковый репозиторий должен повторять фильтры SQL (STATS-2).
type fakeTicket struct {
	ExecutorID int64
	Deleted    bool
	CompletedTicket
}

// fakeRepo — реализация Repository в памяти: сервис и handler'ы тестируются без
// MySQL (docs/ENVIRONMENT.md), при этом записи фильтров позволяют проверить,
// какие границы периода уходят в SQL.
type fakeRepo struct {
	mu        sync.Mutex
	employees []Employee
	tickets   []fakeTicket

	countFilters []Filter
	listFilters  []Filter
	usersFilters []*int64

	countErr error
	listErr  error
	usersErr error
	userErr  error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{} }

// withEmployees задаёт сотрудников для сводной статистики.
func (r *fakeRepo) withEmployees(employees ...Employee) *fakeRepo {
	r.employees = append(r.employees, employees...)
	return r
}

// withTickets добавляет заявки.
func (r *fakeRepo) withTickets(tickets ...fakeTicket) *fakeRepo {
	r.tickets = append(r.tickets, tickets...)
	return r
}

func (r *fakeRepo) CountCompleted(_ context.Context, filter Filter) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.countFilters = append(r.countFilters, filter)
	if r.countErr != nil {
		return 0, r.countErr
	}
	count := 0
	for _, ticket := range r.tickets {
		if !ticket.Deleted && matchesFilter(ticket, filter) {
			count++
		}
	}
	return count, nil
}

func (r *fakeRepo) CompletedTickets(_ context.Context, filter Filter) ([]CompletedTicket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listFilters = append(r.listFilters, filter)
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]CompletedTicket, 0, len(r.tickets))
	for _, ticket := range r.tickets {
		if ticket.Deleted || !matchesFilter(ticket, filter) {
			continue
		}
		out = append(out, ticket.CompletedTicket)
	}
	return out, nil
}

func (r *fakeRepo) Users(_ context.Context, buildingID *int64) ([]Employee, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usersFilters = append(r.usersFilters, buildingID)
	if r.usersErr != nil {
		return nil, r.usersErr
	}
	out := make([]Employee, 0, len(r.employees))
	for _, employee := range r.employees {
		if buildingID != nil {
			if employee.Role != auth.RoleUser || employee.BuildingID == nil || *employee.BuildingID != *buildingID {
				continue
			}
		}
		out = append(out, employee)
	}
	return out, nil
}

func (r *fakeRepo) UserByID(_ context.Context, id int64) (*Employee, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.userErr != nil {
		return nil, r.userErr
	}
	for i := range r.employees {
		if r.employees[i].ID == id {
			employee := r.employees[i]
			return &employee, nil
		}
	}
	return nil, ErrNotFound
}

// matchesFilter повторяет условие SQL: исполнитель, здание и период completed_at.
func matchesFilter(ticket fakeTicket, filter Filter) bool {
	if ticket.CompletedAt.IsZero() {
		return false
	}
	if filter.ExecutorID != nil && ticket.ExecutorID != *filter.ExecutorID {
		return false
	}
	if filter.BuildingID != nil && ticket.BuildingID != *filter.BuildingID {
		return false
	}
	if filter.From != nil && ticket.CompletedAt.Before(*filter.From) {
		return false
	}
	if filter.ToExclusive != nil && !ticket.CompletedAt.Before(*filter.ToExclusive) {
		return false
	}
	return true
}

// newTestService собирает сервис с фейковым репозиторием и календарём в поясе
// Екатеринбурга.
func newTestService(t *testing.T, repo *fakeRepo) *Service {
	t.Helper()
	loc := yekaterinburg(t)
	clock := platform.FixedClock{Time: baseTime}
	calendar := platform.NewDefaultCalendar(loc, platform.DefaultWorkdays(), nil, clock)
	return NewService(repo, Options{Calendar: calendar})
}

// discardLogger — логгер, не засоряющий вывод тестов.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// admin и user — актёры тестов.
func admin(id int64) *auth.User {
	return &auth.User{ID: id, Login: "admin", FullName: "Администратор", Role: auth.RoleAdmin}
}

func user(id int64, login, fullName string, buildingID *int64, buildingName *string) *auth.User {
	return &auth.User{
		ID:           id,
		Login:        login,
		FullName:     fullName,
		Role:         auth.RoleUser,
		BuildingID:   buildingID,
		BuildingName: buildingName,
		IsActive:     true,
	}
}

// ptr — указатель на значение.
func ptr[T any](value T) *T { return &value }

// mustDate разбирает дату YYYY-MM-DD в поясе приложения.
func mustDate(t *testing.T, loc *time.Location, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation(platform.DateLayout, value, loc)
	if err != nil {
		t.Fatalf("ParseInLocation(%q): %v", value, err)
	}
	return parsed
}

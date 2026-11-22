package stats

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// apiError достаёт *apierr.Error из ошибки сервиса.
func apiError(t *testing.T, err error) *apierr.Error {
	t.Helper()
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("ожидалась ошибка API, получено %v", err)
	}
	return apiErr
}

// ticketFor собирает завершённую заявку исполнителя executorID.
func ticketFor(id, executorID int64, completedAt time.Time, duration time.Duration, deadline *time.Time, problemType string) fakeTicket {
	return fakeTicket{
		ExecutorID: executorID,
		CompletedTicket: CompletedTicket{
			TicketID:        id,
			BuildingID:      1,
			BuildingName:    "Здание А",
			ProblemTypeID:   ptr(int64(1)),
			ProblemTypeName: problemType,
			Cabinet:         "101",
			ApplicantName:   "Заявитель",
			CreatedAt:       completedAt.Add(-duration),
			CompletedAt:     completedAt,
			Deadline:        deadline,
		},
	}
}

// STATS-1/STATS-2: личная статистика — только свои завершённые заявки, корзина
// исключена, total_done за всё время, метрики периода — по completed_at.
func TestService_MyStats(t *testing.T) {
	repo := newFakeRepo().withTickets(
		ticketFor(1, 5, time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC), 2*time.Hour, nil, "Электрика"),
		ticketFor(2, 5, time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC), 4*time.Hour, nil, "Сантехника"),
		func() fakeTicket {
			deleted := ticketFor(3, 5, time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC), time.Hour, nil, "Прочее")
			deleted.Deleted = true
			return deleted
		}(),
		ticketFor(4, 9, time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC), time.Hour, nil, "Прочее"),
	)
	svc := newTestService(t, repo)

	from := mustDate(t, svc.Location(), "2026-05-01")
	to := mustDate(t, svc.Location(), "2026-05-31")
	stats, err := svc.MyStats(context.Background(), user(5, "ivanov", "Иванов", nil, nil), Query{
		From: &from,
		To:   &to,
	})
	if err != nil {
		t.Fatalf("MyStats: %v", err)
	}
	if stats.ID != 5 || stats.Login != "ivanov" {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.TotalDone != 2 {
		t.Fatalf("total_done = %d, want 2 (всё время, без корзины)", stats.TotalDone)
	}
	if stats.DoneInPeriod != 1 || *stats.AvgResolutionHours != 2 {
		t.Fatalf("период: done=%d avg=%v", stats.DoneInPeriod, stats.AvgResolutionHours)
	}
	if !stats.Period.From.Equal(from) || !stats.Period.To.Equal(to) {
		t.Fatalf("period = %+v", stats.Period)
	}

	// total_done считается запросом без периода, метрики — с периодом.
	if len(repo.countFilters) != 1 {
		t.Fatalf("countFilters = %+v", repo.countFilters)
	}
	if repo.countFilters[0].From != nil || repo.countFilters[0].ToExclusive != nil {
		t.Fatalf("total_done должен считаться без периода: %+v", repo.countFilters[0])
	}
	if len(repo.listFilters) != 1 || repo.listFilters[0].From == nil || repo.listFilters[0].ToExclusive == nil {
		t.Fatalf("метрики должны учитывать период: %+v", repo.listFilters)
	}
}

// STATS-2: личная статистика не ограничена зданием — фильтр building_id
// игнорируется.
func TestService_MyStatsIgnoresBuildingFilter(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	buildingID := int64(2)

	_, err := svc.MyStats(context.Background(), user(5, "ivanov", "Иванов", nil, nil), Query{BuildingID: &buildingID})
	if err != nil {
		t.Fatalf("MyStats: %v", err)
	}
	if len(repo.listFilters) != 1 || repo.listFilters[0].BuildingID != nil {
		t.Fatalf("фильтр здания не должен применяться: %+v", repo.listFilters)
	}
}

// STATS-1: сводная статистика по всем сотрудникам и по одному user_id.
func TestService_AdminStats(t *testing.T) {
	buildingID := int64(1)
	buildingName := "Здание А"
	repo := newFakeRepo().
		withEmployees(
			Employee{ID: 1, Login: "u1", FullName: "Первый", Role: auth.RoleUser, BuildingID: &buildingID, BuildingName: &buildingName},
			Employee{ID: 2, Login: "u2", FullName: "Второй", Role: auth.RoleUser, BuildingID: &buildingID, BuildingName: &buildingName},
		).
		withTickets(
			ticketFor(1, 1, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC), time.Hour, nil, "Электрика"),
			ticketFor(2, 1, time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC), 3*time.Hour, nil, "Электрика"),
			ticketFor(3, 2, time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC), 2*time.Hour, nil, "Прочее"),
		)
	svc := newTestService(t, repo)
	ctx := context.Background()

	items, err := svc.AdminStats(ctx, admin(100), Query{})
	if err != nil {
		t.Fatalf("AdminStats: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].ID != 1 || items[0].TotalDone != 2 || items[0].DoneInPeriod != 2 {
		t.Fatalf("items[0] = %+v", items[0])
	}
	if items[1].ID != 2 || items[1].TotalDone != 1 {
		t.Fatalf("items[1] = %+v", items[1])
	}

	// user_id — один сотрудник, включая заблокированного.
	items, err = svc.AdminStats(ctx, admin(100), Query{UserID: ptr(int64(2))})
	if err != nil {
		t.Fatalf("AdminStats(user_id): %v", err)
	}
	if len(items) != 1 || items[0].ID != 2 {
		t.Fatalf("items = %+v", items)
	}

	// Несуществующий сотрудник — 404.
	if _, err := svc.AdminStats(ctx, admin(100), Query{UserID: ptr(int64(999))}); apiError(t, err).Status != http.StatusNotFound {
		t.Fatalf("нет сотрудника: %v", err)
	}
	// USER не имеет доступа к сводной статистике — 403.
	if _, err := svc.AdminStats(ctx, user(5, "u5", "Пятый", nil, nil), Query{}); apiError(t, err).Status != http.StatusForbidden {
		t.Fatalf("USER: %v", err)
	}
	// Без актёра — 401.
	if _, err := svc.AdminStats(ctx, nil, Query{}); apiError(t, err).Status != http.StatusUnauthorized {
		t.Fatalf("nil actor: %v", err)
	}
}

// Фильтр building_id уходит в оба запроса (сотрудники и метрики).
func TestService_AdminStatsBuildingFilter(t *testing.T) {
	buildingID := int64(2)
	repo := newFakeRepo().withEmployees(
		Employee{ID: 1, Login: "u1", FullName: "Первый", Role: auth.RoleUser, BuildingID: &buildingID},
	)
	svc := newTestService(t, repo)

	if _, err := svc.AdminStats(context.Background(), admin(100), Query{BuildingID: &buildingID}); err != nil {
		t.Fatalf("AdminStats: %v", err)
	}
	if len(repo.usersFilters) != 1 || repo.usersFilters[0] == nil || *repo.usersFilters[0] != buildingID {
		t.Fatalf("usersFilters = %+v", repo.usersFilters)
	}
	if len(repo.listFilters) != 1 || repo.listFilters[0].BuildingID == nil || *repo.listFilters[0].BuildingID != buildingID {
		t.Fatalf("listFilters = %+v", repo.listFilters)
	}
}

// STATS-2: период задаётся датами в поясе приложения, а в SQL уходит в UTC:
// 1 мая 00:00 +05 = 30 апреля 19:00 UTC, конец — начало 1 июня по местному.
func TestService_PeriodBoundsInTimezone(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	from := mustDate(t, svc.Location(), "2026-05-01")
	to := mustDate(t, svc.Location(), "2026-05-31")

	if _, err := svc.MyStats(context.Background(), user(5, "u", "U", nil, nil), Query{From: &from, To: &to}); err != nil {
		t.Fatalf("MyStats: %v", err)
	}
	if len(repo.listFilters) != 1 {
		t.Fatalf("listFilters = %+v", repo.listFilters)
	}
	filter := repo.listFilters[0]
	wantFrom := time.Date(2026, 4, 30, 19, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 5, 31, 19, 0, 0, 0, time.UTC)
	if filter.From == nil || !filter.From.Equal(wantFrom) {
		t.Fatalf("from = %v, want %v", filter.From, wantFrom)
	}
	if filter.ToExclusive == nil || !filter.ToExclusive.Equal(wantTo) {
		t.Fatalf("toExclusive = %v, want %v", filter.ToExclusive, wantTo)
	}
}

// Неверный порядок границ периода — 422.
func TestService_PeriodValidation(t *testing.T) {
	svc := newTestService(t, newFakeRepo())
	from := mustDate(t, svc.Location(), "2026-05-31")
	to := mustDate(t, svc.Location(), "2026-05-01")
	if _, err := svc.MyStats(context.Background(), user(5, "u", "U", nil, nil), Query{From: &from, To: &to}); apiError(t, err).Status != http.StatusUnprocessableEntity {
		t.Fatalf("from > to: %v", err)
	}
}

// STATS-3: строки для Excel-выгрузки Б11 — один SQL, поля контракта, «в срок»
// только при наличии дедлайна; чужой отчёт USER не отдаётся (403).
func TestService_ExportRows(t *testing.T) {
	onTimeDeadline := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	overdueDeadline := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	repo := newFakeRepo().
		withEmployees(Employee{ID: 5, Login: "u5", FullName: "Пятый", Role: auth.RoleUser}).
		withTickets(
			ticketFor(1, 5, time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC), 2*time.Hour, &onTimeDeadline, "Электрика"),
			ticketFor(2, 5, time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC), 2*time.Hour, &overdueDeadline, "Сантехника"),
			ticketFor(3, 5, time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC), time.Hour, nil, "Прочее"),
			ticketFor(4, 9, time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC), time.Hour, nil, "Прочее"),
		)
	svc := newTestService(t, repo)
	ctx := context.Background()
	owner := user(5, "u5", "Пятый", nil, nil)

	rows, err := svc.ExportRows(ctx, owner, Query{})
	if err != nil {
		t.Fatalf("ExportRows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (только свои)", len(rows))
	}
	if rows[0].TicketID != 1 || rows[0].Building != "Здание А" || rows[0].ProblemType != "Электрика" {
		t.Fatalf("rows[0] = %+v", rows[0])
	}
	if rows[0].OnTime == nil || !*rows[0].OnTime {
		t.Fatalf("rows[0].OnTime = %v, want true", rows[0].OnTime)
	}
	if rows[1].OnTime == nil || *rows[1].OnTime {
		t.Fatalf("rows[1].OnTime = %v, want false", rows[1].OnTime)
	}
	if rows[2].OnTime != nil {
		t.Fatalf("rows[2].OnTime = %v, want nil (нет дедлайна)", *rows[2].OnTime)
	}

	// USER не может выгрузить чужой отчёт — 403.
	if _, err := svc.ExportRows(ctx, owner, Query{UserID: ptr(int64(9))}); apiError(t, err).Status != http.StatusForbidden {
		t.Fatalf("чужой отчёт: %v", err)
	}
	// ADMIN выгружает чужой отчёт.
	rows, err = svc.ExportRows(ctx, admin(100), Query{UserID: ptr(int64(5))})
	if err != nil || len(rows) != 3 {
		t.Fatalf("ADMIN чужой отчёт: rows=%d err=%v", len(rows), err)
	}
	// ADMIN запросил несуществующего сотрудника — 404.
	if _, err := svc.ExportRows(ctx, admin(100), Query{UserID: ptr(int64(999))}); apiError(t, err).Status != http.StatusNotFound {
		t.Fatalf("нет сотрудника: %v", err)
	}
}

// STATS-2: пустой набор — null в avg/median/on_time, счётчики 0.
func TestService_EmptyStats(t *testing.T) {
	svc := newTestService(t, newFakeRepo())
	stats, err := svc.MyStats(context.Background(), user(5, "u", "U", nil, nil), Query{})
	if err != nil {
		t.Fatalf("MyStats: %v", err)
	}
	if stats.TotalDone != 0 || stats.DoneInPeriod != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.AvgResolutionHours != nil || stats.MedianResolutionHours != nil || stats.OnTimePercent != nil {
		t.Fatalf("метрики должны быть nil: %+v", stats)
	}
}

// Ошибки БД превращаются в 500, а не в панику.
func TestService_RepoErrors(t *testing.T) {
	ctx := context.Background()

	countRepo := newFakeRepo()
	countRepo.countErr = errors.New("db is down")
	if _, err := newTestService(t, countRepo).MyStats(ctx, user(5, "u", "U", nil, nil), Query{}); apiError(t, err).Status != http.StatusInternalServerError {
		t.Fatalf("count error: %v", err)
	}

	listRepo := newFakeRepo()
	listRepo.listErr = errors.New("db is down")
	if _, err := newTestService(t, listRepo).MyStats(ctx, user(5, "u", "U", nil, nil), Query{}); apiError(t, err).Status != http.StatusInternalServerError {
		t.Fatalf("list error: %v", err)
	}

	usersRepo := newFakeRepo()
	usersRepo.usersErr = errors.New("db is down")
	if _, err := newTestService(t, usersRepo).AdminStats(ctx, admin(100), Query{}); apiError(t, err).Status != http.StatusInternalServerError {
		t.Fatalf("users error: %v", err)
	}

	userRepo := newFakeRepo()
	userRepo.userErr = errors.New("db is down")
	if _, err := newTestService(t, userRepo).AdminStats(ctx, admin(100), Query{UserID: ptr(int64(5))}); apiError(t, err).Status != http.StatusInternalServerError {
		t.Fatalf("user error: %v", err)
	}
}

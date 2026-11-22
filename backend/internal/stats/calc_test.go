package stats

import (
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// twentyTickets — фикстура из 20 завершённых заявок с известными датами
// (STATS-2): длительности 1..20 часов, дедлайн у 10 заявок (7 в срок, 3 с
// опозданием), четыре вида проблем по 5 заявок, все завершены в мае 2026.
func twentyTickets(t *testing.T, loc *time.Location) []CompletedTicket {
	t.Helper()
	problemIDs := []int64{1, 2, 3, 4}
	problemNames := []string{"Электрика", "Сантехника", "Мебель", "Прочее"}

	tickets := make([]CompletedTicket, 0, 20)
	for day := 1; day <= 20; day++ {
		completedLocal := time.Date(2026, 5, day, 10, 0, 0, 0, loc)
		completedAt := completedLocal.UTC()
		createdAt := completedAt.Add(-time.Duration(day) * time.Hour) // длительность = day часов

		var deadline *time.Time
		switch {
		case day <= 7:
			// Дедлайн в тот же день — выполнено в срок.
			value := time.Date(2026, 5, day, 0, 0, 0, 0, time.UTC)
			deadline = &value
		case day <= 10:
			// Дедлайн на день раньше — просрочено (day 8..10).
			value := time.Date(2026, 5, day-1, 0, 0, 0, 0, time.UTC)
			deadline = &value
		}

		index := (day - 1) % 4
		problemID := problemIDs[index]
		tickets = append(tickets, CompletedTicket{
			TicketID:        int64(day),
			BuildingID:      1,
			BuildingName:    "Здание А",
			ProblemTypeID:   &problemID,
			ProblemTypeName: problemNames[index],
			Cabinet:         "101",
			ApplicantName:   "Заявитель",
			CreatedAt:       createdAt,
			CompletedAt:     completedAt,
			Deadline:        deadline,
		})
	}
	return tickets
}

// Статистика пустого набора: счётчики 0, avg/median/on_time — null (STATS-2).
func TestComputeMetrics_Empty(t *testing.T) {
	metrics := ComputeMetrics(nil, time.UTC)
	if metrics.DoneInPeriod != 0 || metrics.OverdueCount != 0 {
		t.Fatalf("счётчики = %d/%d", metrics.DoneInPeriod, metrics.OverdueCount)
	}
	if metrics.AvgResolutionHours != nil || metrics.MedianResolutionHours != nil || metrics.OnTimePercent != nil {
		t.Fatalf("avg/median/on_time должны быть nil: %+v", metrics)
	}
	if metrics.ByProblemType == nil || len(metrics.ByProblemType) != 0 {
		t.Fatalf("by_problem_type = %v", metrics.ByProblemType)
	}
	if metrics.ByMonth == nil || len(metrics.ByMonth) != 0 {
		t.Fatalf("by_month = %v", metrics.ByMonth)
	}
}

// Фикстура 20 заявок: среднее, медиана, «в срок», просрочки и разбивки.
func TestComputeMetrics_TwentyTickets(t *testing.T) {
	loc := yekaterinburg(t)
	tickets := twentyTickets(t, loc)
	metrics := ComputeMetrics(tickets, loc)

	if metrics.DoneInPeriod != 20 {
		t.Fatalf("done_in_period = %d, want 20", metrics.DoneInPeriod)
	}
	// Длительности 1..20 часов: сумма 210, среднее 10.5, медиана (10+11)/2.
	assertFloat(t, "avg_resolution_hours", metrics.AvgResolutionHours, 10.5)
	assertFloat(t, "median_resolution_hours", metrics.MedianResolutionHours, 10.5)
	// Дедлайн у 10 заявок: 7 в срок → 70 %; просрочено 3.
	assertFloat(t, "on_time_percent", metrics.OnTimePercent, 70.0)
	if metrics.OverdueCount != 3 {
		t.Fatalf("overdue_count = %d, want 3", metrics.OverdueCount)
	}

	// Четыре вида проблем по 5 заявок; при равенстве — по названию.
	if len(metrics.ByProblemType) != 4 {
		t.Fatalf("by_problem_type = %+v", metrics.ByProblemType)
	}
	wantNames := []string{"Мебель", "Прочее", "Сантехника", "Электрика"}
	for i, item := range metrics.ByProblemType {
		if item.Name != wantNames[i] || item.Count != 5 {
			t.Fatalf("by_problem_type[%d] = %+v, want %s×5", i, item, wantNames[i])
		}
	}

	if len(metrics.ByMonth) != 1 || metrics.ByMonth[0].Month != "2026-05" || metrics.ByMonth[0].Count != 20 {
		t.Fatalf("by_month = %+v, want [{2026-05 20}]", metrics.ByMonth)
	}
}

// Медиана: на нечётном количестве — центральное значение, среднее — по всем.
func TestComputeMetrics_MedianOdd(t *testing.T) {
	loc := time.UTC
	tickets := []CompletedTicket{
		completedTicket(1, time.Date(2026, 5, 1, 8, 0, 0, 0, loc), 2*time.Hour),
		completedTicket(2, time.Date(2026, 5, 1, 8, 0, 0, 0, loc), 9*time.Hour),
		completedTicket(3, time.Date(2026, 5, 1, 8, 0, 0, 0, loc), 4*time.Hour),
	}
	metrics := ComputeMetrics(tickets, loc)
	// Сортировка длительностей: 2, 4, 9 → медиана 4, среднее 5.
	assertFloat(t, "median", metrics.MedianResolutionHours, 4)
	assertFloat(t, "avg", metrics.AvgResolutionHours, 5)
}

// Медиана на чётном количестве — среднее двух центральных значений.
func TestComputeMetrics_MedianEven(t *testing.T) {
	loc := time.UTC
	tickets := []CompletedTicket{
		completedTicket(1, time.Date(2026, 5, 1, 8, 0, 0, 0, loc), 1*time.Hour),
		completedTicket(2, time.Date(2026, 5, 1, 8, 0, 0, 0, loc), 2*time.Hour),
		completedTicket(3, time.Date(2026, 5, 1, 8, 0, 0, 0, loc), 3*time.Hour),
		completedTicket(4, time.Date(2026, 5, 1, 8, 0, 0, 0, loc), 10*time.Hour),
	}
	metrics := ComputeMetrics(tickets, loc)
	// (2 + 3) / 2 = 2.5; среднее (1+2+3+10)/4 = 4.
	assertFloat(t, "median", metrics.MedianResolutionHours, 2.5)
	assertFloat(t, "avg", metrics.AvgResolutionHours, 4)
}

// STATS-2: «в срок» — completed_at ≤ deadline по календарной дате в поясе
// приложения. Заявка завершена 2 мая 20:00 UTC = 3 мая 01:00 по Екатеринбургу
// при дедлайне 2 мая — просрочена (в UTC была бы «в срок»).
func TestComputeMetrics_OnTimeUsesAppTimezone(t *testing.T) {
	loc := yekaterinburg(t)
	deadline := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	ticket := CompletedTicket{
		TicketID:    1,
		CreatedAt:   time.Date(2026, 5, 2, 15, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 5, 2, 20, 0, 0, 0, time.UTC),
		Deadline:    &deadline,
	}
	metrics := ComputeMetrics([]CompletedTicket{ticket}, loc)
	if metrics.OverdueCount != 1 || metrics.OnTimePercent == nil || *metrics.OnTimePercent != 0 {
		t.Fatalf("metrics = %+v, want overdue=1 on_time=0", metrics)
	}

	// Та же заявка, завершённая до полуночи по местному времени, — в срок.
	ticket.CompletedAt = time.Date(2026, 5, 2, 14, 0, 0, 0, time.UTC)
	metrics = ComputeMetrics([]CompletedTicket{ticket}, loc)
	if metrics.OverdueCount != 0 || metrics.OnTimePercent == nil || *metrics.OnTimePercent != 100 {
		t.Fatalf("metrics = %+v, want overdue=0 on_time=100", metrics)
	}
}

// Заявки без дедлайна не входят в знаменатель on_time_percent.
func TestComputeMetrics_NoDeadlines(t *testing.T) {
	loc := time.UTC
	tickets := []CompletedTicket{
		completedTicket(1, time.Date(2026, 5, 1, 8, 0, 0, 0, loc), time.Hour),
		completedTicket(2, time.Date(2026, 5, 1, 8, 0, 0, 0, loc), 2*time.Hour),
	}
	metrics := ComputeMetrics(tickets, loc)
	if metrics.OnTimePercent != nil {
		t.Fatalf("on_time_percent = %v, want nil", *metrics.OnTimePercent)
	}
	if metrics.OverdueCount != 0 {
		t.Fatalf("overdue_count = %d, want 0", metrics.OverdueCount)
	}
}

// Месяц считается по completed_at в поясе приложения.
func TestComputeMetrics_MonthByTimezone(t *testing.T) {
	loc := yekaterinburg(t)
	tickets := []CompletedTicket{
		// 31 января 20:00 UTC = 1 февраля 01:00 по Екатеринбургу.
		{TicketID: 1, CreatedAt: time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC), CompletedAt: time.Date(2026, 1, 31, 20, 0, 0, 0, time.UTC)},
		{TicketID: 2, CreatedAt: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC), CompletedAt: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)},
		{TicketID: 3, CreatedAt: time.Date(2026, 2, 10, 10, 0, 0, 0, time.UTC), CompletedAt: time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)},
	}
	metrics := ComputeMetrics(tickets, loc)
	if len(metrics.ByMonth) != 2 {
		t.Fatalf("by_month = %+v", metrics.ByMonth)
	}
	if metrics.ByMonth[0].Month != "2026-01" || metrics.ByMonth[0].Count != 1 {
		t.Fatalf("by_month[0] = %+v, want 2026-01×1", metrics.ByMonth[0])
	}
	if metrics.ByMonth[1].Month != "2026-02" || metrics.ByMonth[1].Count != 2 {
		t.Fatalf("by_month[1] = %+v, want 2026-02×2", metrics.ByMonth[1])
	}
}

// Аномалия данных (завершение раньше создания) не даёт отрицательного времени.
func TestComputeMetrics_ClampsNegativeDuration(t *testing.T) {
	loc := time.UTC
	tickets := []CompletedTicket{
		{
			TicketID:    1,
			CreatedAt:   time.Date(2026, 5, 1, 12, 0, 0, 0, loc),
			CompletedAt: time.Date(2026, 5, 1, 10, 0, 0, 0, loc),
		},
	}
	metrics := ComputeMetrics(tickets, loc)
	assertFloat(t, "avg", metrics.AvgResolutionHours, 0)
	assertFloat(t, "median", metrics.MedianResolutionHours, 0)
}

// STATS-2: время выполнения округляется до 0.1.
func TestComputeMetrics_RoundsToTenths(t *testing.T) {
	loc := time.UTC
	base := time.Date(2026, 5, 1, 8, 0, 0, 0, loc)
	tickets := []CompletedTicket{
		completedTicket(1, base, 74*time.Minute),  // 1.2333… ч
		completedTicket(2, base, 121*time.Minute), // 2.0166… ч
	}
	metrics := ComputeMetrics(tickets, loc)
	// (1.2333+2.0166)/2 = 1.625 → 1.6; медиана того же набора — 1.6.
	assertFloat(t, "avg", metrics.AvgResolutionHours, 1.6)
	assertFloat(t, "median", metrics.MedianResolutionHours, 1.6)
}

// completedTicket собирает заявку с длительностью duration от created.
func completedTicket(id int64, created time.Time, duration time.Duration) CompletedTicket {
	return CompletedTicket{
		TicketID:    id,
		BuildingID:  1,
		CreatedAt:   created,
		CompletedAt: created.Add(duration),
	}
}

// assertFloat проверяет непустое значение с плавающей точкой.
func assertFloat(t *testing.T, name string, value *float64, want float64) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s = nil, want %v", name, want)
	}
	if *value != want {
		t.Fatalf("%s = %v, want %v", name, *value, want)
	}
}

// Проверка, что DateLayout используется для дат периода (контракт).
func TestDateLayout(t *testing.T) {
	if platform.DateLayout != "2006-01-02" {
		t.Fatalf("DateLayout = %q", platform.DateLayout)
	}
}

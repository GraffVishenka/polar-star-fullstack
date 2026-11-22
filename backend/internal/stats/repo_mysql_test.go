package stats

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	// Драйвер MySQL регистрируется побочным эффектом импорта.
	_ "github.com/go-sql-driver/mysql"
)

// testDB подключается к MySQL из TEST_DATABASE_URL. На машине разработки БД нет
// (docs/ENVIRONMENT.md), поэтому без переменной тест пропускается. Если база
// недоступна или схема не применена — тоже пропуск: `go test ./...` не должен
// падать там, где MySQL не подготовлен.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан: интеграционный тест MySQL пропущен")
	}
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("TEST_DATABASE_URL некорректен: %v", err)
	}
	pool.SetMaxOpenConns(5)
	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		t.Skipf("тестовая БД недоступна (нужен MySQL и применённые миграции): %v", err)
	}
	for _, table := range []string{"tickets", "users", "buildings", "problem_types"} {
		if !hasTable(t, pool, table) {
			_ = pool.Close()
			t.Skipf("схема не применена: нет таблицы %s", table)
		}
	}
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

// hasTable проверяет наличие таблицы текущей схемы.
func hasTable(t *testing.T, pool *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := pool.QueryRow(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = ? LIMIT 1`, table).Scan(&name)
	return err == nil && name == table
}

// uniqueSuffix даёт непересекающиеся имена между прогонами тестов.
func uniqueSuffix() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }

// statsFixture — минимальный набор связанных записей для заявок.
type statsFixture struct {
	BuildingID    int64
	ProblemTypeID int64
	UserID        int64
}

// insertFixture создаёт здание, вид проблемы и сотрудника.
func insertFixture(t *testing.T, pool *sql.DB, suffix string) statsFixture {
	t.Helper()
	ctx := context.Background()
	fixture := statsFixture{}

	res, err := pool.ExecContext(ctx, `INSERT INTO buildings (name, is_active) VALUES (?, 1)`,
		"Здание stats "+suffix)
	if err != nil {
		t.Fatalf("INSERT buildings: %v", err)
	}
	if fixture.BuildingID, err = res.LastInsertId(); err != nil {
		t.Fatalf("LastInsertId(buildings): %v", err)
	}

	res, err = pool.ExecContext(ctx, `INSERT INTO problem_types (name) VALUES (?)`,
		"Вид проблемы "+suffix)
	if err != nil {
		t.Fatalf("INSERT problem_types: %v", err)
	}
	if fixture.ProblemTypeID, err = res.LastInsertId(); err != nil {
		t.Fatalf("LastInsertId(problem_types): %v", err)
	}

	res, err = pool.ExecContext(ctx, `INSERT INTO users
		(login, full_name, role, password_hash, building_id, is_active, must_change_password)
		VALUES (?, ?, 'USER', 'x', ?, 1, 0)`,
		"stats_user_"+suffix, "Статистиков "+suffix, fixture.BuildingID)
	if err != nil {
		t.Fatalf("INSERT users: %v", err)
	}
	if fixture.UserID, err = res.LastInsertId(); err != nil {
		t.Fatalf("LastInsertId(users): %v", err)
	}
	return fixture
}

// cleanup удаляет заявки и справочные записи в порядке, безопасном для FK.
func (f statsFixture) cleanup(t *testing.T, pool *sql.DB) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.ExecContext(ctx, `DELETE FROM tickets WHERE building_id = ?`, f.BuildingID)
		_, _ = pool.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, f.UserID)
		_, _ = pool.ExecContext(ctx, `DELETE FROM buildings WHERE id = ?`, f.BuildingID)
		_, _ = pool.ExecContext(ctx, `DELETE FROM problem_types WHERE id = ?`, f.ProblemTypeID)
	})
}

// insertCompletedTicket вставляет завершённую заявку и возвращает id.
func insertCompletedTicket(t *testing.T, pool *sql.DB, fixture statsFixture,
	createdAt, completedAt time.Time, deadline *time.Time, deleted bool) int64 {
	t.Helper()
	var deletedAt any
	if deleted {
		deletedAt = completedAt
	}
	res, err := pool.ExecContext(context.Background(), `INSERT INTO tickets
		(building_id, applicant_name, cabinet, problem_type_id, description, deadline,
		 executor_id, status, completed_at, created_at, deleted_at)
		VALUES (?, 'Заявитель', '101', ?, 'описание', ?, ?, 'done', ?, ?, ?)`,
		fixture.BuildingID, fixture.ProblemTypeID, deadline, fixture.UserID,
		completedAt, createdAt, deletedAt)
	if err != nil {
		t.Fatalf("INSERT tickets: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId(tickets): %v", err)
	}
	return id
}

// STATS-1/STATS-2 поверх реальной схемы: корзина исключается, период считается
// по completed_at, имена здания и вида проблемы подставляются, total_done не
// зависит от периода.
func TestMySQLStatsRepository(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	suffix := uniqueSuffix()
	fixture := insertFixture(t, pool, suffix)
	fixture.cleanup(t, pool)

	deadline := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	inPeriodA := insertCompletedTicket(t, pool, fixture,
		time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC), time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC), &deadline, false)
	inPeriodB := insertCompletedTicket(t, pool, fixture,
		time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC), time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC), nil, false)
	outsidePeriod := insertCompletedTicket(t, pool, fixture,
		time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC), time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), nil, false)
	deleted := insertCompletedTicket(t, pool, fixture,
		time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC), time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC), nil, true)
	if inPeriodA == 0 || inPeriodB == 0 || outsidePeriod == 0 || deleted == 0 {
		t.Fatal("не удалось подготовить заявки")
	}

	executor := fixture.UserID
	building := fixture.BuildingID
	baseFilter := Filter{ExecutorID: &executor, BuildingID: &building}

	// total_done — все завершённые без корзины, период не учитывается.
	total, err := repo.CountCompleted(ctx, baseFilter)
	if err != nil {
		t.Fatalf("CountCompleted: %v", err)
	}
	if total != 3 {
		t.Fatalf("total_done = %d, want 3 (корзина исключена)", total)
	}

	// Метрики периода — только заявки мая.
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	toExclusive := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	periodFilter := baseFilter
	periodFilter.From = &from
	periodFilter.ToExclusive = &toExclusive

	tickets, err := repo.CompletedTickets(ctx, periodFilter)
	if err != nil {
		t.Fatalf("CompletedTickets: %v", err)
	}
	if len(tickets) != 2 {
		t.Fatalf("tickets = %d, want 2", len(tickets))
	}
	first := tickets[0]
	if first.TicketID != inPeriodA {
		t.Fatalf("порядок по completed_at нарушен: первый id = %d, want %d", first.TicketID, inPeriodA)
	}
	if first.BuildingName != "Здание stats "+suffix {
		t.Fatalf("building_name = %q", first.BuildingName)
	}
	if first.ProblemTypeName != "Вид проблемы "+suffix || first.ProblemTypeID == nil {
		t.Fatalf("problem_type = %v/%q", first.ProblemTypeID, first.ProblemTypeName)
	}
	if first.Deadline == nil || !first.Deadline.Equal(deadline) {
		t.Fatalf("deadline = %v, want %v", first.Deadline, deadline)
	}
	if !first.CompletedAt.Equal(time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("completed_at = %v", first.CompletedAt)
	}

	// Метрики по этим данным: 2 заявки, среднее 3 часа, медиана 3 часа,
	// дедлайн у одной и она в срок.
	metrics := ComputeMetrics(tickets, time.UTC)
	if metrics.DoneInPeriod != 2 {
		t.Fatalf("done_in_period = %d", metrics.DoneInPeriod)
	}
	assertFloat(t, "avg", metrics.AvgResolutionHours, 3)
	assertFloat(t, "median", metrics.MedianResolutionHours, 3)
	assertFloat(t, "on_time", metrics.OnTimePercent, 100)

	// Список сотрудников здания содержит нашего сотрудника.
	employees, err := repo.Users(ctx, &building)
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	found := false
	for _, employee := range employees {
		if employee.ID == fixture.UserID && employee.Login == "stats_user_"+suffix {
			found = true
		}
	}
	if !found {
		t.Fatalf("сотрудник %d отсутствует в списке Users", fixture.UserID)
	}

	// UserByID и отсутствующий сотрудник.
	employee, err := repo.UserByID(ctx, fixture.UserID)
	if err != nil || employee.ID != fixture.UserID {
		t.Fatalf("UserByID = %+v, %v", employee, err)
	}
	if _, err := repo.UserByID(ctx, 9_999_999); err != ErrNotFound {
		t.Fatalf("UserByID(нет) = %v, want ErrNotFound", err)
	}
}

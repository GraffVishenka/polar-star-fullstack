package db

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	// Драйвер MySQL регистрируется побочным эффектом импорта.
	_ "github.com/go-sql-driver/mysql"
)

// discardLogger — тихий логгер для сида в тестах.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// seedDemoDB подключается к TEST_DATABASE_URL. Без MySQL тест пропускается:
// локальная машина разработки не имеет БД (docs/ENVIRONMENT.md).
func seedDemoDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан: интеграционный тест демо-сида пропущен")
	}
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("TEST_DATABASE_URL некорректен: %v", err)
	}
	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		t.Skipf("тестовая БД недоступна: %v", err)
	}
	var name string
	if err := pool.QueryRow(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'tickets' LIMIT 1`).Scan(&name); err != nil {
		_ = pool.Close()
		t.Skip("схема не применена: выполните migrate")
	}
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

// countRows — COUNT(*) по условию (условие — константа из теста).
func countRows(t *testing.T, pool *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("COUNT: %v (%s)", err, query)
	}
	return count
}

// Б13: демо-сид идемпотентен — второй запуск не создаёт дубликаты, а данные
// соответствуют инвариантам плана (4 здания, 12 пользователей, 50 заявок,
// 25 единиц оборудования, 25 узлов БЗ, 5 праздников).
func TestSeedDemoIdempotent(t *testing.T) {
	pool := seedDemoDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	data := buildDemoData(now)

	// Базовый сид нужен для справочников и настроек; ADMIN создаётся с паролем.
	if err := Seed(ctx, pool, discardLogger(), SeedOptions{
		AdminLogin: "seed-demo-admin", AdminFullName: "Сид Демо", AdminPassword: "Seed_12345",
	}); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	// Первый и второй запуск: второй должен быть no-op по количеству строк.
	for run := 1; run <= 2; run++ {
		if err := SeedDemo(ctx, pool, discardLogger(), DemoOptions{Password: "Demo_12345", Now: now}); err != nil {
			t.Fatalf("SeedDemo (запуск %d): %v", run, err)
		}
	}

	buildings := countRows(t, pool, `SELECT COUNT(*) FROM buildings WHERE name LIKE 'Демо-%'`)
	if buildings != len(data.Buildings) {
		t.Fatalf("демо-зданий = %d, want %d", buildings, len(data.Buildings))
	}
	users := countRows(t, pool, `SELECT COUNT(*) FROM users WHERE login LIKE 'demo.%'`)
	if users != len(data.Users) {
		t.Fatalf("демо-пользователей = %d, want %d", users, len(data.Users))
	}
	tickets := countRows(t, pool, `SELECT COUNT(*) FROM tickets WHERE description LIKE '%[демо]%'`)
	if tickets != len(data.Tickets) {
		t.Fatalf("демо-заявок = %d, want %d", tickets, len(data.Tickets))
	}
	equipment := countRows(t, pool, `SELECT COUNT(*) FROM equipment WHERE registration_number LIKE 'DEMO-EQ-%'`)
	if equipment != len(data.Equipment) {
		t.Fatalf("демо-оборудования = %d, want %d", equipment, len(data.Equipment))
	}
	movements := countRows(t, pool, `SELECT COUNT(*) FROM equipment_movements m
		JOIN equipment e ON e.id = m.equipment_id WHERE e.registration_number LIKE 'DEMO-EQ-%'`)
	if movements != len(data.Movements) {
		t.Fatalf("демо-перемещений = %d, want %d", movements, len(data.Movements))
	}
	nodes := countRows(t, pool, `SELECT COUNT(*) FROM kb_nodes WHERE title LIKE 'Демо:%'`)
	if nodes != len(data.Nodes) {
		t.Fatalf("демо-узлов БЗ = %d, want %d", nodes, len(data.Nodes))
	}
	hiddenRoots := countRows(t, pool, `SELECT COUNT(*) FROM kb_nodes
		WHERE title LIKE 'Демо:%' AND parent_id IS NULL AND is_hidden_for_anon = 1`)
	if hiddenRoots != 2 {
		t.Fatalf("скрытых корней = %d, want 2", hiddenRoots)
	}
	// Заявки во всех статусах.
	for _, status := range []string{"new", "in_progress", "deferred", "done"} {
		count := countRows(t, pool, `SELECT COUNT(*) FROM tickets
			WHERE description LIKE '%[демо]%' AND status = ?`, status)
		if count == 0 {
			t.Fatalf("нет демо-заявок в статусе %s", status)
		}
	}
}

package trash

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	// Драйвер MySQL регистрируется побочным эффектом импорта.
	_ "github.com/go-sql-driver/mysql"
)

// testDB подключается к MySQL из TEST_DATABASE_URL; без переменной, без БД или
// без применённой схемы тест пропускается (docs/ENVIRONMENT.md).
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
	pool.SetMaxOpenConns(10)
	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		t.Skipf("тестовая БД недоступна (нужен MySQL и применённые миграции): %v", err)
	}
	var name string
	if err := pool.QueryRow(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'tickets' LIMIT 1`).Scan(&name); err != nil {
		_ = pool.Close()
		t.Skip("схема не применена: выполните make migrate для тестовой БД")
	}
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())
}

// fixture создаёт здание и вид проблемы; пользователь для deleted_by берётся
// первый существующий (после сидинга в CI он есть).
type fixture struct {
	buildingID    int64
	problemTypeID int64
	userID        sql.NullInt64
}

func newFixture(t *testing.T, pool *sql.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	fx := &fixture{}

	res, err := pool.ExecContext(ctx, `INSERT INTO buildings (name, is_active) VALUES (?, 0)`, uniqueName("Здание"))
	if err != nil {
		t.Fatalf("insert building: %v", err)
	}
	fx.buildingID, _ = res.LastInsertId()
	res, err = pool.ExecContext(ctx, `INSERT INTO problem_types (name) VALUES (?)`, uniqueName("Вид"))
	if err != nil {
		t.Fatalf("insert problem_type: %v", err)
	}
	fx.problemTypeID, _ = res.LastInsertId()

	if err := pool.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&fx.userID.Int64); err == nil {
		fx.userID.Valid = true
	}

	t.Cleanup(func() {
		cleanup := context.Background()
		_, _ = pool.ExecContext(cleanup, `DELETE FROM tickets WHERE building_id = ?`, fx.buildingID)
		_, _ = pool.ExecContext(cleanup, `DELETE FROM buildings WHERE id = ?`, fx.buildingID)
		_, _ = pool.ExecContext(cleanup, `DELETE FROM problem_types WHERE id = ?`, fx.problemTypeID)
	})
	return fx
}

// TRASH-1..TRASH-3: список, восстановление и физическое удаление заявки вместе
// с комментариями и файлами.
func TestMySQLTicketTrashLifecycle(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	fx := newFixture(t, pool)

	res, err := pool.ExecContext(ctx, `
		INSERT INTO tickets (building_id, applicant_name, cabinet, problem_type_id, description,
		                     status, deleted_at, deleted_by)
		VALUES (?, 'Иванов Иван', '101', ?, 'Тестовая заявка', 'done',
		        UTC_TIMESTAMP(), ?)`,
		fx.buildingID, fx.problemTypeID, fx.userID)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	ticketID, _ := res.LastInsertId()

	// Комментарий с вложением.
	commentRes, err := pool.ExecContext(ctx,
		`INSERT INTO ticket_comments (ticket_id, author_name, body) VALUES (?, 'Иванов', 'Комментарий')`, ticketID)
	if err != nil {
		t.Fatalf("insert comment: %v", err)
	}
	commentID, _ := commentRes.LastInsertId()

	storedPath := uniqueName("2026/11/trash")
	fileRes, err := pool.ExecContext(ctx, `
		INSERT INTO files (original_name, stored_path, mime, size_bytes, sha256, scope, linked_at)
		VALUES ('a.png', ?, 'image/png', 10, REPEAT('a', 64), 'ticket', UTC_TIMESTAMP())`, storedPath)
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}
	fileID, _ := fileRes.LastInsertId()
	if _, err := pool.ExecContext(ctx,
		`INSERT INTO comment_files (comment_id, file_id) VALUES (?, ?)`, commentID, fileID); err != nil {
		t.Fatalf("insert comment_files: %v", err)
	}

	// Список корзины: автор удаления и краткое описание (TRASH-1).
	items, total, err := repo.List(ctx, TypeTicket, 1, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total < 1 {
		t.Fatalf("total = %d", total)
	}
	var listed *Item
	for i := range items {
		if items[i].ID == ticketID {
			listed = &items[i]
			break
		}
	}
	if listed == nil {
		t.Fatal("заявка не найдена в корзине")
	}
	if listed.Type != TypeTicket || listed.DeletedByName == nil {
		t.Fatalf("item = %+v", listed)
	}
	if listed.BuildingID == nil || *listed.BuildingID != fx.buildingID {
		t.Fatalf("building = %v", listed.BuildingID)
	}
	if !strings.Contains(listed.Title, "Иванов") || !strings.Contains(listed.Title, "101") {
		t.Fatalf("title = %q", listed.Title)
	}

	// Восстановление: deleted_at/deleted_by очищаются, статус сохраняется.
	info, err := repo.RestoreTicket(ctx, ticketID)
	if err != nil {
		t.Fatalf("RestoreTicket: %v", err)
	}
	if info.BuildingID != fx.buildingID || info.BuildingActive {
		t.Fatalf("info = %+v", info)
	}
	var (
		deletedAt sql.NullTime
		deletedBy sql.NullInt64
		status    string
	)
	if err := pool.QueryRowContext(ctx,
		`SELECT deleted_at, deleted_by, status FROM tickets WHERE id = ?`, ticketID).
		Scan(&deletedAt, &deletedBy, &status); err != nil {
		t.Fatalf("reread ticket: %v", err)
	}
	if deletedAt.Valid || deletedBy.Valid {
		t.Fatalf("восстановленная заявка всё ещё удалена: %v/%v", deletedAt, deletedBy)
	}
	if status != "done" {
		t.Fatalf("статус потерян: %q", status)
	}

	// Повторное удаление и физическая очистка (TRASH-3).
	if _, err := pool.ExecContext(ctx,
		`UPDATE tickets SET deleted_at = UTC_TIMESTAMP(), deleted_by = ? WHERE id = ?`,
		fx.userID, ticketID); err != nil {
		t.Fatalf("re-delete ticket: %v", err)
	}
	paths, err := repo.PurgeTicket(ctx, ticketID)
	if err != nil {
		t.Fatalf("PurgeTicket: %v", err)
	}
	if len(paths) != 1 || paths[0] != storedPath {
		t.Fatalf("paths = %v", paths)
	}
	for _, check := range []struct {
		query string
		arg   int64
		what  string
	}{
		{`SELECT COUNT(*) FROM tickets WHERE id = ?`, ticketID, "заявка"},
		{`SELECT COUNT(*) FROM ticket_comments WHERE ticket_id = ?`, ticketID, "комментарии"},
		{`SELECT COUNT(*) FROM files WHERE id = ?`, fileID, "файл"},
		{`SELECT COUNT(*) FROM comment_files WHERE file_id = ?`, fileID, "связь файла"},
	} {
		var count int
		if err := pool.QueryRowContext(ctx, check.query, check.arg).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", check.what, err)
		}
		if count != 0 {
			t.Fatalf("%s не удалены: %d", check.what, count)
		}
	}

	if _, err := repo.RestoreTicket(ctx, ticketID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RestoreTicket после purge = %v, want ErrNotFound", err)
	}
}

// TRASH-3: физическое удаление узла БЗ удаляет поддерево от листьев к корню.
func TestMySQLPurgeKBSubtree(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()

	rootTitle := uniqueName("Корзина root")
	res, err := pool.ExecContext(ctx,
		`INSERT INTO kb_nodes (title, content_html, deleted_at) VALUES (?, '', UTC_TIMESTAMP())`, rootTitle)
	if err != nil {
		t.Fatalf("insert root: %v", err)
	}
	rootID, _ := res.LastInsertId()
	childTitle := uniqueName("Корзина child")
	res, err = pool.ExecContext(ctx,
		`INSERT INTO kb_nodes (title, content_html, parent_id, deleted_at) VALUES (?, '', ?, UTC_TIMESTAMP())`,
		childTitle, rootID)
	if err != nil {
		t.Fatalf("insert child: %v", err)
	}
	childID, _ := res.LastInsertId()
	t.Cleanup(func() {
		cleanup := context.Background()
		_, _ = pool.ExecContext(cleanup, `DELETE FROM kb_nodes WHERE id IN (?, ?)`, childID, rootID)
	})

	items, _, err := repo.List(ctx, TypeKB, 1, 100)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var childItem *Item
	for i := range items {
		if items[i].ID == childID {
			childItem = &items[i]
		}
	}
	if childItem == nil {
		t.Fatal("дочерний узел не найден в корзине")
	}
	if !strings.Contains(childItem.Title, "родитель") {
		t.Fatalf("title дочернего узла = %q", childItem.Title)
	}

	if err := repo.PurgeKB(ctx, rootID); err != nil {
		t.Fatalf("PurgeKB: %v", err)
	}
	var count int
	if err := pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kb_nodes WHERE id IN (?, ?)`, rootID, childID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("поддерево не удалено: %d", count)
	}
}

// TRASH-4: в выборку просроченных попадают только элементы старше порога.
func TestMySQLExpiredThreshold(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	fx := newFixture(t, pool)

	insert := func(interval string) int64 {
		res, err := pool.ExecContext(ctx, `
			INSERT INTO tickets (building_id, applicant_name, cabinet, problem_type_id, description,
			                     status, deleted_at)
			VALUES (?, 'Просрочка', '1', ?, 'Тест', 'new', UTC_TIMESTAMP() - INTERVAL `+interval+`)`,
			fx.buildingID, fx.problemTypeID)
		if err != nil {
			t.Fatalf("insert ticket (%s): %v", interval, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	oldID := insert("100 DAY")
	recentID := insert("10 DAY")

	cutoff := time.Now().UTC().Add(-DefaultRetention)
	ids, err := repo.Expired(ctx, TypeTicket, cutoff, 100)
	if err != nil {
		t.Fatalf("Expired: %v", err)
	}
	hasOld, hasRecent := false, false
	for _, id := range ids {
		if id == oldID {
			hasOld = true
		}
		if id == recentID {
			hasRecent = true
		}
	}
	if !hasOld {
		t.Fatal("элемент старше 90 дней не попал в автоочистку")
	}
	if hasRecent {
		t.Fatal("свежий элемент не должен попадать в автоочистку")
	}
}

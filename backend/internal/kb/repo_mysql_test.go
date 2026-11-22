package kb

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
// недоступна или схема ещё не применена — тоже пропуск: интеграционные тесты не
// должны ломать прогон `go test ./...` там, где MySQL не подготовлен.
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
	if !hasTable(t, pool, "kb_nodes") {
		_ = pool.Close()
		t.Skip("схема не применена: выполните make migrate для тестовой БД")
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

// uniqueTitle даёт непересекающиеся заголовки между прогонами тестов.
func uniqueTitle(prefix string) string {
	return fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())
}

// Жизненный цикл дерева поверх реальной схемы (KB-1..KB-8): создание, список,
// предки, перемещение с переиндексацией, видимость, публичность узла, мягкое
// удаление поддерева и поиск медиа-ссылок.
func TestMySQLKbTreeLifecycle(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()

	// t.Cleanup выполняется LIFO, поэтому регистрируем по мере создания:
	// потомки удалятся раньше родителей (self-FK RESTRICT).
	cleanup := func(id int64) {
		t.Cleanup(func() {
			_, _ = pool.ExecContext(context.Background(), `DELETE FROM kb_nodes WHERE id = ?`, id)
		})
	}

	rootTitle := uniqueTitle("Корень")
	root, err := repo.Create(ctx, CreateParams{
		Title:       rootTitle,
		ContentHTML: `<p>корень</p>`,
		SortOrder:   SortOrderStep,
		ActorID:     0,
	})
	if err != nil {
		t.Fatalf("Create(root): %v", err)
	}
	cleanup(root.ID)
	if root.ID == 0 || root.Title != rootTitle || root.ParentID != nil {
		t.Fatalf("root = %+v", root)
	}

	childA, err := repo.Create(ctx, CreateParams{Title: uniqueTitle("A"), SortOrder: 10, ParentID: ptr(root.ID)})
	if err != nil {
		t.Fatalf("Create(childA): %v", err)
	}
	cleanup(childA.ID)
	childB, err := repo.Create(ctx, CreateParams{Title: uniqueTitle("B"), SortOrder: 20, ParentID: ptr(root.ID)})
	if err != nil {
		t.Fatalf("Create(childB): %v", err)
	}
	cleanup(childB.ID)
	grand, err := repo.Create(ctx, CreateParams{Title: uniqueTitle("Внук"), SortOrder: 10, ParentID: ptr(childA.ID)})
	if err != nil {
		t.Fatalf("Create(grand): %v", err)
	}
	cleanup(grand.ID)

	// List отдаёт только живые узлы (KB-8).
	nodes, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, want := range []int64{root.ID, childA.ID, childB.ID, grand.ID} {
		found := false
		for _, node := range nodes {
			if node.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("узел %d отсутствует в List", want)
		}
	}

	// Get и ParentStatus (KB-2).
	if _, err := repo.Get(ctx, 9_999_999); err != ErrNotFound {
		t.Fatalf("Get(нет узла) = %v, want ErrNotFound", err)
	}
	exists, deleted, err := repo.ParentStatus(ctx, root.ID)
	if err != nil || !exists || deleted {
		t.Fatalf("ParentStatus(root) = %v,%v,%v", exists, deleted, err)
	}
	if exists, _, _ := repo.ParentStatus(ctx, 9_999_999); exists {
		t.Fatal("ParentStatus(нет узла) = exists")
	}

	// MaxSortOrder и предки.
	maxOrder, err := repo.MaxSortOrder(ctx, ptr(root.ID))
	if err != nil || maxOrder != 20 {
		t.Fatalf("MaxSortOrder(root) = %d, %v", maxOrder, err)
	}
	crumbs, err := repo.Ancestors(ctx, grand.ID)
	if err != nil {
		t.Fatalf("Ancestors: %v", err)
	}
	if len(crumbs) != 3 || crumbs[0].ID != root.ID || crumbs[1].ID != childA.ID || crumbs[2].ID != grand.ID {
		t.Fatalf("Ancestors = %+v", crumbs)
	}

	// Цикл: root — предок grand, обратное неверно (KB-2).
	if ok, err := repo.IsAncestorOrSelf(ctx, root.ID, grand.ID); err != nil || !ok {
		t.Fatalf("IsAncestorOrSelf(root, grand) = %v, %v", ok, err)
	}
	if ok, _ := repo.IsAncestorOrSelf(ctx, grand.ID, root.ID); ok {
		t.Fatal("IsAncestorOrSelf(grand, root) не должен быть предком")
	}

	// Перемещение: B перед A → B=10, A=20 (KB-6, шаг 10).
	if err := repo.Move(ctx, childB.ID, ptr(root.ID), ptr(childA.ID), 0); err != nil {
		t.Fatalf("Move(B перед A): %v", err)
	}
	movedA, _ := repo.Get(ctx, childA.ID)
	movedB, _ := repo.Get(ctx, childB.ID)
	if movedB.SortOrder != 10 || movedA.SortOrder != 20 {
		t.Fatalf("после move: B=%d A=%d, want 10/20", movedB.SortOrder, movedA.SortOrder)
	}

	// Перемещение в корень в конец → parent_id NULL и sort_order = max+10.
	// На верхнем уровне сейчас только сам root (sort_order=10), поэтому
	// добавленный в конец узел получает 20 (а не 30: A и B — его дети).
	if err := repo.Move(ctx, grand.ID, nil, nil, 0); err != nil {
		t.Fatalf("Move(grand в корень): %v", err)
	}
	movedGrand, _ := repo.Get(ctx, grand.ID)
	if movedGrand.ParentID != nil {
		t.Fatalf("parent_id = %v, want NULL", movedGrand.ParentID)
	}
	if movedGrand.SortOrder != 2*SortOrderStep {
		t.Fatalf("sort_order = %d, want %d", movedGrand.SortOrder, 2*SortOrderStep)
	}

	// Правка: повторный PATCH с теми же значениями не должен давать 404
	// (MySQL RowsAffected считает только изменённые строки).
	title := uniqueTitle("A+")
	if _, err := repo.Update(ctx, UpdateParams{ID: childA.ID, Title: &title, ActorID: 0}); err != nil {
		t.Fatalf("Update(title): %v", err)
	}
	if _, err := repo.Update(ctx, UpdateParams{ID: childA.ID, Title: &title, ActorID: 0}); err != nil {
		t.Fatalf("повторный Update = %v, want nil", err)
	}
	if _, err := repo.Update(ctx, UpdateParams{ID: 9_999_999, Title: &title}); err != ErrNotFound {
		t.Fatalf("Update(нет узла) = %v, want ErrNotFound", err)
	}

	// Видимость и публичность узла (KB-7): скрытый корень закрывает потомков.
	if _, err := repo.SetVisibility(ctx, root.ID, true, 0); err != nil {
		t.Fatalf("SetVisibility(true): %v", err)
	}
	if visible, err := repo.NodePublic(ctx, grand.ID); err != nil || visible {
		t.Fatalf("NodePublic при скрытом корне = %v, %v", visible, err)
	}
	if _, err := repo.SetVisibility(ctx, root.ID, false, 0); err != nil {
		t.Fatalf("SetVisibility(false): %v", err)
	}
	if visible, err := repo.NodePublic(ctx, grand.ID); err != nil || !visible {
		t.Fatalf("NodePublic при видимом корне = %v, %v", visible, err)
	}

	// Медиа: поиск ссылки и точное совпадение id.
	content := `<p><img src="/api/files/424242" alt="x"></p>`
	if _, err := repo.Update(ctx, UpdateParams{ID: root.ID, ContentHTML: &content}); err != nil {
		t.Fatalf("Update(content): %v", err)
	}
	candidates, err := repo.MediaCandidates(ctx, "%/api/files/424242%")
	if err != nil {
		t.Fatalf("MediaCandidates: %v", err)
	}
	found := false
	for _, candidate := range candidates {
		if candidate.ID == root.ID && ContentReferencesFile(candidate.ContentHTML, 424242) {
			found = true
		}
	}
	if !found {
		t.Fatalf("MediaCandidates не нашёл ссылку: %+v", candidates)
	}

	// Мягкое удаление поддерева: root, A, B, внук в корне → 3 потомка (KB-3).
	descendants, err := repo.SoftDelete(ctx, root.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if descendants != 3 {
		t.Fatalf("descendants = %d, want 3", descendants)
	}
	if _, err := repo.Get(ctx, root.ID); err != ErrNotFound {
		t.Fatalf("Get(удалённый) = %v, want ErrNotFound", err)
	}
	if _, _, err := repo.ParentStatus(ctx, root.ID); err != nil {
		t.Fatalf("ParentStatus(удалённый): %v", err)
	}
	if exists, deleted, _ := repo.ParentStatus(ctx, childA.ID); !exists || !deleted {
		t.Fatalf("ParentStatus(childA после каскада) = exists=%v deleted=%v", exists, deleted)
	}
	if _, err := repo.SoftDelete(ctx, root.ID, time.Now().UTC()); err != ErrNotFound {
		t.Fatalf("повторный SoftDelete = %v, want ErrNotFound", err)
	}
}

package dictionaries

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

// testDB подключается к MySQL из TEST_DATABASE_URL; без переменной или без
// подготовленной схемы тест пропускается (docs/ENVIRONMENT.md).
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
		WHERE table_schema = DATABASE() AND table_name = 'equipment_types' LIMIT 1`).Scan(&name); err != nil {
		_ = pool.Close()
		t.Skip("схема не применена: выполните make migrate для тестовой БД")
	}
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())
}

// Б7: CRUD справочника, дубль имени → ErrNameTaken, удаление = деактивация.
func TestMySQLDictionariesCRUD(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, KindEquipmentTypes, CreateInput{
		Name: uniqueName("Тест-тип"), IsActive: true, SortOrder: 5,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.Background(), `DELETE FROM equipment_types WHERE id = ?`, created.ID)
	})
	if created.SortOrder != 5 || !created.IsActive {
		t.Fatalf("created = %+v", created)
	}

	if _, err := repo.Create(ctx, KindEquipmentTypes, CreateInput{Name: created.Name, IsActive: true}); err != ErrNameTaken {
		t.Fatalf("дубль имени = %v, want ErrNameTaken", err)
	}

	items, err := repo.List(ctx, KindEquipmentTypes, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !containsItem(items, created.ID) {
		t.Fatal("активный элемент не найден в списке")
	}

	newName := uniqueName("Тест-тип переименован")
	updated, err := repo.Update(ctx, KindEquipmentTypes, created.ID, UpdateInput{Name: &newName, SortOrder: intPtr(1)})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName || updated.SortOrder != 1 {
		t.Fatalf("updated = %+v", updated)
	}

	// DELETE-семантика: элемент деактивируется, но не удаляется.
	deactivated, err := repo.SetActive(ctx, KindEquipmentTypes, created.ID, false)
	if err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if deactivated.IsActive {
		t.Fatal("элемент должен быть деактивирован")
	}
	active, err := repo.List(ctx, KindEquipmentTypes, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if containsItem(active, created.ID) {
		t.Fatal("деактивированный элемент не должен попадать в активные")
	}
	all, err := repo.List(ctx, KindEquipmentTypes, true)
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if !containsItem(all, created.ID) {
		t.Fatal("деактивированный элемент должен оставаться в полном списке")
	}

	// Старые записи ссылаются на элемент по id и не зависят от is_active:
	// проверяем на уровне SQL, что строка доступна (название не исчезло).
	var storedName string
	if err := pool.QueryRowContext(ctx,
		`SELECT name FROM equipment_types WHERE id = ?`, created.ID).Scan(&storedName); err != nil {
		t.Fatalf("чтение деактивированного элемента: %v", err)
	}
	if storedName != newName {
		t.Fatalf("name = %q, want %q", storedName, newName)
	}
}

func containsItem(items []Item, id int64) bool {
	for i := range items {
		if items[i].ID == id {
			return true
		}
	}
	return false
}

func intPtr(value int) *int { return &value }

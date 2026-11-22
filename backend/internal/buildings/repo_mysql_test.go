package buildings

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
	pool.SetMaxOpenConns(10)
	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		t.Skipf("тестовая БД недоступна (нужен MySQL и применённые миграции): %v", err)
	}
	if !hasTable(t, pool, "warehouses") {
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

// uniqueName даёт непересекающиеся имена между прогонами тестов.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())
}

// EQ-1/EQ-6: CRUD зданий и складов поверх реальной схемы: дубль имени склада,
// запрет удаления используемого здания и непустого склада.
func TestMySQLBuildingsAndWarehouses(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()

	name := uniqueName("Здание")
	building, err := repo.CreateBuilding(ctx, name, nil)
	if err != nil {
		t.Fatalf("CreateBuilding: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.Background(), `DELETE FROM buildings WHERE id = ?`, building.ID)
	})
	if !building.IsActive || building.Name != name {
		t.Fatalf("created = %+v", building)
	}

	// Деактивация: здание остаётся в списке авторизованных, но исчезает из
	// активного (EQ-6).
	inactive := false
	if _, err := repo.UpdateBuilding(ctx, building.ID, BuildingPatch{IsActive: &inactive}); err != nil {
		t.Fatalf("UpdateBuilding: %v", err)
	}
	all, err := repo.Buildings(ctx, true)
	if err != nil {
		t.Fatalf("Buildings(true): %v", err)
	}
	if !containsBuilding(all, building.ID) {
		t.Fatal("неактивное здание должно оставаться в списке авторизованных")
	}
	active, err := repo.Buildings(ctx, false)
	if err != nil {
		t.Fatalf("Buildings(false): %v", err)
	}
	if containsBuilding(active, building.ID) {
		t.Fatal("неактивное здание не должно попадать в список активных")
	}

	warehouse, err := repo.CreateWarehouse(ctx, building.ID, uniqueName("Склад"))
	if err != nil {
		t.Fatalf("CreateWarehouse: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.Background(), `DELETE FROM warehouses WHERE id = ?`, warehouse.ID)
	})

	if _, err := repo.CreateWarehouse(ctx, building.ID, warehouse.Name); err != ErrNameTaken {
		t.Fatalf("дубль имени склада = %v, want ErrNameTaken", err)
	}
	taken, err := repo.WarehouseNameTaken(ctx, building.ID, warehouse.Name, 0)
	if err != nil || !taken {
		t.Fatalf("WarehouseNameTaken = %v, %v", taken, err)
	}
	taken, err = repo.WarehouseNameTaken(ctx, building.ID, warehouse.Name, warehouse.ID)
	if err != nil || taken {
		t.Fatalf("WarehouseNameTaken(exclude self) = %v, %v", taken, err)
	}

	// EQ-6: здание со складом физически не удаляется.
	if err := repo.DeleteBuilding(ctx, building.ID); err != ErrBuildingInUse {
		t.Fatalf("DeleteBuilding со складом = %v, want ErrBuildingInUse", err)
	}

	// EQ-6: склад с оборудованием не удаляется.
	typeRes, err := pool.ExecContext(ctx,
		`INSERT INTO equipment_types (name) VALUES (?)`, uniqueName("Тип"))
	if err != nil {
		t.Fatalf("insert equipment_type: %v", err)
	}
	typeID, _ := typeRes.LastInsertId()
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.Background(), `DELETE FROM equipment_types WHERE id = ?`, typeID)
	})
	res, err := pool.ExecContext(ctx, `
		INSERT INTO equipment (name, equipment_type_id, serial_number, registration_number, warehouse_id)
		VALUES (?, ?, ?, ?, ?)`,
		"Тестовое оборудование", typeID, "SN-1", uniqueName("REG"), warehouse.ID)
	if err != nil {
		t.Fatalf("insert equipment: %v", err)
	}
	equipmentID, _ := res.LastInsertId()
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.Background(), `DELETE FROM equipment WHERE id = ?`, equipmentID)
	})

	if err := repo.DeleteWarehouse(ctx, warehouse.ID); err != ErrWarehouseNotEmpty {
		t.Fatalf("DeleteWarehouse с оборудованием = %v, want ErrWarehouseNotEmpty", err)
	}
	count, err := repo.WarehouseEquipmentCount(ctx, warehouse.ID)
	if err != nil || count != 1 {
		t.Fatalf("WarehouseEquipmentCount = %d, %v", count, err)
	}

	usage, err := repo.BuildingUsage(ctx, building.ID)
	if err != nil {
		t.Fatalf("BuildingUsage: %v", err)
	}
	if usage.Warehouses != 1 || usage.Equipment != 1 {
		t.Fatalf("usage = %+v", usage)
	}

	// Пустой склад и пустое здание удаляются.
	if _, err := pool.ExecContext(ctx, `DELETE FROM equipment WHERE id = ?`, equipmentID); err != nil {
		t.Fatalf("cleanup equipment: %v", err)
	}
	if err := repo.DeleteWarehouse(ctx, warehouse.ID); err != nil {
		t.Fatalf("DeleteWarehouse пустого склада: %v", err)
	}
	if err := repo.DeleteBuilding(ctx, building.ID); err != nil {
		t.Fatalf("DeleteBuilding пустого здания: %v", err)
	}
}

func containsBuilding(items []Building, id int64) bool {
	for i := range items {
		if items[i].ID == id {
			return true
		}
	}
	return false
}

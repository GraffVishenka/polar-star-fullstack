package equipment

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	// Драйвер MySQL регистрируется побочным эффектом импорта.
	_ "github.com/go-sql-driver/mysql"
)

// testDB подключается к MySQL из TEST_DATABASE_URL. На машине разработки БД нет
// (docs/ENVIRONMENT.md), поэтому без переменной тест пропускается; в CI схема
// уже создана `make migrate`.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан: интеграционный тест MySQL пропущен")
	}
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	pool.SetMaxOpenConns(50)
	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		t.Fatalf("ping MySQL: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())
}

// moveFixture создаёт здание с двумя складами, деактивированное здание со
// складом, тип оборудования и пользователя для журнала.
type moveFixture struct {
	buildingID  int64
	inactiveID  int64
	warehouseA  int64
	warehouseB  int64
	inactiveWh  int64
	typeID      int64
	userID      int64
	equipmentID int64
}

func newMoveFixture(t *testing.T, pool *sql.DB, repo *MySQLRepository) *moveFixture {
	t.Helper()
	ctx := context.Background()
	fx := &moveFixture{}

	if err := pool.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&fx.userID); err != nil {
		t.Skipf("нет пользователя для журнала перемещений: %v", err)
	}

	res, err := pool.ExecContext(ctx, `INSERT INTO equipment_types (name) VALUES (?)`, uniqueName("Тип"))
	if err != nil {
		t.Fatalf("insert equipment_type: %v", err)
	}
	fx.typeID, _ = res.LastInsertId()

	createBuilding := func(name string, active bool) int64 {
		res, err := pool.ExecContext(ctx, `INSERT INTO buildings (name, is_active) VALUES (?, ?)`, name, active)
		if err != nil {
			t.Fatalf("insert building: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	fx.buildingID = createBuilding(uniqueName("Здание"), true)
	fx.inactiveID = createBuilding(uniqueName("Здание (неактивное)"), false)

	createWarehouse := func(buildingID int64, name string) int64 {
		res, err := pool.ExecContext(ctx,
			`INSERT INTO warehouses (building_id, name) VALUES (?, ?)`, buildingID, name)
		if err != nil {
			t.Fatalf("insert warehouse: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	fx.warehouseA = createWarehouse(fx.buildingID, uniqueName("Склад A"))
	fx.warehouseB = createWarehouse(fx.buildingID, uniqueName("Склад B"))
	fx.inactiveWh = createWarehouse(fx.inactiveID, uniqueName("Склад C"))

	item, err := repo.Create(ctx, CreateInput{
		Name:               "Тестовый ноутбук",
		EquipmentTypeID:    fx.typeID,
		SerialNumber:       "SN-TEST",
		RegistrationNumber: uniqueName("REG"),
		WarehouseID:        fx.warehouseA,
	})
	if err != nil {
		t.Fatalf("Create equipment: %v", err)
	}
	fx.equipmentID = item.ID

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.ExecContext(cleanupCtx, `DELETE FROM equipment_movements WHERE equipment_id = ?`, fx.equipmentID)
		_, _ = pool.ExecContext(cleanupCtx, `DELETE FROM equipment WHERE id = ?`, fx.equipmentID)
		for _, id := range []int64{fx.warehouseA, fx.warehouseB, fx.inactiveWh} {
			_, _ = pool.ExecContext(cleanupCtx, `DELETE FROM warehouses WHERE id = ?`, id)
		}
		for _, id := range []int64{fx.buildingID, fx.inactiveID} {
			_, _ = pool.ExecContext(cleanupCtx, `DELETE FROM buildings WHERE id = ?`, id)
		}
		_, _ = pool.ExecContext(cleanupCtx, `DELETE FROM equipment_types WHERE id = ?`, fx.typeID)
	})

	return fx
}

// EQ-2/EQ-3/EQ-5 на реальной схеме: дубль регистрационного номера, перемещение
// с записью журнала, запрет USER перемещать в неактивное здание, неизменяемость
// журнала (нет методов UPDATE/DELETE).
func TestMySQLEquipmentMoveAndJournal(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	fx := newMoveFixture(t, pool, repo)

	// EQ-2: дубль регистрационного номера среди неудалённых.
	existing, err := repo.ByID(ctx, fx.equipmentID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if _, err := repo.Create(ctx, CreateInput{
		Name: "Дубль", EquipmentTypeID: fx.typeID, SerialNumber: "SN-2",
		RegistrationNumber: existing.RegistrationNumber, WarehouseID: fx.warehouseA,
	}); err != ErrDuplicateRegistration {
		t.Fatalf("Create с дублем = %v, want ErrDuplicateRegistration", err)
	}
	taken, err := repo.RegistrationNumberTaken(ctx, existing.RegistrationNumber, fx.equipmentID)
	if err != nil || taken {
		t.Fatalf("RegistrationNumberTaken(exclude self) = %v, %v", taken, err)
	}

	// EQ-3: перемещение меняет склад и создаёт ровно одну запись журнала.
	equipment, movement, err := repo.Move(ctx, fx.equipmentID,
		MoveInput{ToWarehouseID: fx.warehouseB, Comment: strPtr("перенос")},
		Actor{UserID: fx.userID, IsAdmin: false})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if equipment.WarehouseID != fx.warehouseB || equipment.BuildingID != fx.buildingID {
		t.Fatalf("equipment = %+v", equipment)
	}
	if movement.FromWarehouseID != fx.warehouseA || movement.ToWarehouseID != fx.warehouseB {
		t.Fatalf("movement = %+v", movement)
	}
	if movement.MovedByUserID != fx.userID || movement.FromWarehouseName == "" || movement.ToBuildingName == "" {
		t.Fatalf("movement без имён = %+v", movement)
	}

	if _, _, err := repo.Move(ctx, fx.equipmentID, MoveInput{ToWarehouseID: fx.warehouseB},
		Actor{UserID: fx.userID}); err != ErrSameWarehouse {
		t.Fatalf("Move в тот же склад = %v, want ErrSameWarehouse", err)
	}

	// EQ-3: USER не может переместить в неактивное здание, ADMIN может.
	if _, _, err := repo.Move(ctx, fx.equipmentID, MoveInput{ToWarehouseID: fx.inactiveWh},
		Actor{UserID: fx.userID, IsAdmin: false}); err != ErrBuildingInactive {
		t.Fatalf("USER в неактивное здание = %v, want ErrBuildingInactive", err)
	}
	if _, _, err := repo.Move(ctx, fx.equipmentID, MoveInput{ToWarehouseID: fx.inactiveWh},
		Actor{UserID: fx.userID, IsAdmin: true}); err != nil {
		t.Fatalf("ADMIN в неактивное здание: %v", err)
	}

	// EQ-5: журнал отдаётся по убыванию moved_at с пагинацией.
	items, total, err := repo.Movements(ctx, fx.equipmentID, 1, 1)
	if err != nil {
		t.Fatalf("Movements: %v", err)
	}
	if total != 3 || len(items) != 1 {
		t.Fatalf("total=%d len=%d", total, len(items))
	}
	if items[0].ToWarehouseID != fx.inactiveWh {
		t.Fatalf("первым должен идти последний перенос: %+v", items[0])
	}
}

// Б4: 50 параллельных перемещений одной единицы дают 50 записей журнала без
// потерь (транзакция + SELECT ... FOR UPDATE).
func TestMySQLConcurrentMoves(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	fx := newMoveFixture(t, pool, repo)

	const moves = 50
	targets := []int64{fx.warehouseA, fx.warehouseB}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		errs  []error
		moved int
	)
	for i := 0; i < moves; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for attempt := 0; attempt < 10; attempt++ {
				target := targets[(index+attempt)%len(targets)]
				_, _, err := repo.Move(ctx, fx.equipmentID,
					MoveInput{ToWarehouseID: target},
					Actor{UserID: fx.userID, IsAdmin: true})
				if err == nil {
					mu.Lock()
					moved++
					mu.Unlock()
					return
				}
				if err != ErrSameWarehouse { // гонка «в тот же склад» — повторяем
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					return
				}
			}
			mu.Lock()
			errs = append(errs, fmt.Errorf("перемещение %d не удалось за 10 попыток", index))
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("ошибки параллельных перемещений: %v", errs)
	}
	if moved != moves {
		t.Fatalf("успешных перемещений = %d, want %d", moved, moves)
	}
	_, total, err := repo.Movements(ctx, fx.equipmentID, 1, 100)
	if err != nil {
		t.Fatalf("Movements: %v", err)
	}
	if total != moves {
		t.Fatalf("записей журнала = %d, want %d", total, moves)
	}
}

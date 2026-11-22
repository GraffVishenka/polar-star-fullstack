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
// (docs/ENVIRONMENT.md), поэтому без переменной тест пропускается. Недоступная
// база или неприменённая схема — тоже пропуск, чтобы `go test ./...` проходил
// там, где MySQL не подготовлен.
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
	pool.SetMaxOpenConns(50)
	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		t.Skipf("тестовая БД недоступна (нужен MySQL и применённые миграции): %v", err)
	}
	var name string
	if err := pool.QueryRow(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'equipment_movements' LIMIT 1`).Scan(&name); err != nil {
		_ = pool.Close()
		t.Skip("схема не применена: выполните make migrate для тестовой БД")
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
	// Перемещений ровно два: A→B и B→неактивный склад (создание в журнал не пишется).
	items, total, err := repo.Movements(ctx, fx.equipmentID, 1, 1)
	if err != nil {
		t.Fatalf("Movements: %v", err)
	}
	if total != 2 || len(items) != 1 {
		t.Fatalf("total=%d len=%d", total, len(items))
	}
	if items[0].ToWarehouseID != fx.inactiveWh {
		t.Fatalf("первым должен идти последний перенос: %+v", items[0])
	}
}

// Б7: деактивация типа оборудования (справочник) не ломает старые записи —
// карточка оборудования продолжает отдавать название типа.
func TestMySQLDeactivatedEquipmentTypeVisibleInExistingRecords(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	fx := newMoveFixture(t, pool, repo)

	if _, err := pool.ExecContext(ctx,
		`UPDATE equipment_types SET is_active = 0 WHERE id = ?`, fx.typeID); err != nil {
		t.Fatalf("деактивация типа: %v", err)
	}

	item, err := repo.ByID(ctx, fx.equipmentID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if item.EquipmentTypeName == "" {
		t.Fatalf("название деактивированного типа потеряно: %+v", item)
	}

	items, _, err := repo.List(ctx, ListFilter{EquipmentTypeID: &fx.typeID, Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].EquipmentTypeName == "" {
		t.Fatalf("фильтр по деактивированному типу = %+v", items)
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

// createBulkEquipment создаёт count единиц на складе и регистрирует их очистку
// (до очистки складов из newMoveFixture: t.Cleanup выполняется LIFO).
func createBulkEquipment(t *testing.T, pool *sql.DB, repo *MySQLRepository,
	fx *moveFixture, warehouseID int64, count int) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		item, err := repo.Create(ctx, CreateInput{
			Name:               fmt.Sprintf("Массовая единица %d", i),
			EquipmentTypeID:    fx.typeID,
			SerialNumber:       fmt.Sprintf("SN-BULK-%d", i),
			RegistrationNumber: uniqueName("REG-BULK"),
			WarehouseID:        warehouseID,
		})
		if err != nil {
			t.Fatalf("Create equipment %d: %v", i, err)
		}
		ids = append(ids, item.ID)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, id := range ids {
			_, _ = pool.ExecContext(cleanupCtx, `DELETE FROM equipment_movements WHERE equipment_id = ?`, id)
			_, _ = pool.ExecContext(cleanupCtx, `DELETE FROM equipment WHERE id = ?`, id)
		}
	})
	return ids
}

// warehouseOf читает текущий склад единицы.
func warehouseOf(t *testing.T, repo *MySQLRepository, id int64) int64 {
	t.Helper()
	item, err := repo.ByID(context.Background(), id)
	if err != nil {
		t.Fatalf("ByID(%d): %v", id, err)
	}
	return item.WarehouseID
}

// EQ-4: 50 единиц перемещаются одной операцией, по каждой — запись журнала;
// повторный запуск по тому же складу даёт 409 SAME_WAREHOUSE.
func TestMySQLBulkMoveFiftyUnits(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	fx := newMoveFixture(t, pool, repo)

	extra := createBulkEquipment(t, pool, repo, fx, fx.warehouseA, 49)
	ids := append([]int64{fx.equipmentID}, extra...)
	if len(ids) != 50 {
		t.Fatalf("подготовлено %d единиц, want 50", len(ids))
	}

	movements, skipped, err := repo.BulkMove(ctx,
		BulkMoveInput{EquipmentIDs: ids, ToWarehouseID: fx.warehouseB, Comment: strPtr("массовый перенос")},
		Actor{UserID: fx.userID, IsAdmin: false})
	if err != nil {
		t.Fatalf("BulkMove: %v", err)
	}
	if len(movements) != 50 || skipped != 0 {
		t.Fatalf("movements=%d skipped=%d, want 50/0", len(movements), skipped)
	}
	for _, movement := range movements {
		if movement.FromWarehouseID != fx.warehouseA || movement.ToWarehouseID != fx.warehouseB {
			t.Fatalf("movement = %+v", movement)
		}
		if movement.Comment == nil || *movement.Comment != "массовый перенос" {
			t.Fatalf("comment = %v", movement.Comment)
		}
	}
	for _, id := range ids {
		if warehouseOf(t, repo, id) != fx.warehouseB {
			t.Fatalf("единица %d не перемещена", id)
		}
	}
	// Ровно одна запись журнала на единицу.
	_, total, err := repo.Movements(ctx, ids[0], 1, 10)
	if err != nil || total != 1 {
		t.Fatalf("Movements = %d, %v; want 1", total, err)
	}

	// Все уже в целевом складе → 409 и без новых записей журнала.
	if _, _, err := repo.BulkMove(ctx, BulkMoveInput{EquipmentIDs: ids, ToWarehouseID: fx.warehouseB},
		Actor{UserID: fx.userID}); err != ErrSameWarehouse {
		t.Fatalf("повторный BulkMove = %v, want ErrSameWarehouse", err)
	}
	if _, total, _ := repo.Movements(ctx, ids[0], 1, 10); total != 1 {
		t.Fatalf("журнал вырос при 409: %d", total)
	}
}

// EQ-4: часть единиц уже в целевом складе — они пропускаются (skipped), журнал
// создаётся только для перемещённых.
func TestMySQLBulkMoveSkipsSameWarehouse(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	fx := newMoveFixture(t, pool, repo)

	inTarget := createBulkEquipment(t, pool, repo, fx, fx.warehouseB, 1)[0]
	movements, skipped, err := repo.BulkMove(ctx,
		BulkMoveInput{EquipmentIDs: []int64{fx.equipmentID, inTarget}, ToWarehouseID: fx.warehouseB},
		Actor{UserID: fx.userID})
	if err != nil {
		t.Fatalf("BulkMove: %v", err)
	}
	if len(movements) != 1 || skipped != 1 {
		t.Fatalf("movements=%d skipped=%d, want 1/1", len(movements), skipped)
	}
	if movements[0].EquipmentID != fx.equipmentID {
		t.Fatalf("перемещена не та единица: %+v", movements[0])
	}
	if _, total, _ := repo.Movements(ctx, inTarget, 1, 10); total != 0 {
		t.Fatalf("для пропущенной единицы создан журнал: %d", total)
	}
}

// EQ-4: несуществующий id откатывает всю операцию — ни одна единица не меняет
// склад и журнал не пополняется.
func TestMySQLBulkMoveRollsBackOnMissingID(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	fx := newMoveFixture(t, pool, repo)

	ids := append([]int64{fx.equipmentID}, createBulkEquipment(t, pool, repo, fx, fx.warehouseA, 4)...)
	bad := append(append([]int64{}, ids...), 9_999_999)

	if _, _, err := repo.BulkMove(ctx, BulkMoveInput{EquipmentIDs: bad, ToWarehouseID: fx.warehouseB},
		Actor{UserID: fx.userID}); err != ErrNotFound {
		t.Fatalf("BulkMove с битым id = %v, want ErrNotFound", err)
	}
	for _, id := range ids {
		if warehouseOf(t, repo, id) != fx.warehouseA {
			t.Fatalf("единица %d перемещена при откате", id)
		}
		if _, total, _ := repo.Movements(ctx, id, 1, 10); total != 0 {
			t.Fatalf("журнал единицы %d пополнен при откате: %d", id, total)
		}
	}
}

// EQ-4/EQ-3: USER не может массово переместить в деактивированное здание,
// ADMIN может; набор больше 200 единиц отклоняется репозиторием защитно.
func TestMySQLBulkMoveRules(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()
	fx := newMoveFixture(t, pool, repo)

	if _, _, err := repo.BulkMove(ctx,
		BulkMoveInput{EquipmentIDs: []int64{fx.equipmentID}, ToWarehouseID: fx.inactiveWh},
		Actor{UserID: fx.userID, IsAdmin: false}); err != ErrBuildingInactive {
		t.Fatalf("USER в неактивное здание = %v, want ErrBuildingInactive", err)
	}
	if warehouseOf(t, repo, fx.equipmentID) != fx.warehouseA {
		t.Fatal("единица перемещена несмотря на запрет")
	}
	if _, _, err := repo.BulkMove(ctx,
		BulkMoveInput{EquipmentIDs: []int64{fx.equipmentID}, ToWarehouseID: fx.inactiveWh},
		Actor{UserID: fx.userID, IsAdmin: true}); err != nil {
		t.Fatalf("ADMIN в неактивное здание: %v", err)
	}

	tooMany := make([]int64, MaxBulkMoveItems+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	if _, _, err := repo.BulkMove(ctx, BulkMoveInput{EquipmentIDs: tooMany, ToWarehouseID: fx.warehouseB},
		Actor{UserID: fx.userID}); err != ErrInvalidSelection {
		t.Fatalf("201 единица = %v, want ErrInvalidSelection", err)
	}
}

// EQ-4: 20 параллельных операций на пересекающихся наборах не приводят к
// deadlock (блокировки по возрастанию id); каждая единица перемещается ровно
// один раз, общий журнал — по числу единиц.
func TestMySQLBulkMoveConcurrentNoDeadlock(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fx := newMoveFixture(t, pool, repo)
	ids := createBulkEquipment(t, pool, repo, fx, fx.warehouseA, 29) // + fixture = 30
	ids = append([]int64{fx.equipmentID}, ids...)

	// 20 операций, каждая берёт скользящее окно из 20 единиц: наборы сильно
	// пересекаются, порядок внутри набора разный — проверяем, что сортировка
	// id в репозитории исключает взаимоблокировки.
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		errs   []error
		moved  int
		actors = []Actor{{UserID: fx.userID, IsAdmin: false}, {UserID: fx.userID, IsAdmin: true}}
	)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			start := index
			window := make([]int64, 0, 20)
			for j := 0; j < 20; j++ {
				window = append(window, ids[(start+j)%len(ids)])
			}
			movements, _, err := repo.BulkMove(ctx,
				BulkMoveInput{EquipmentIDs: window, ToWarehouseID: fx.warehouseB},
				actors[index%len(actors)])
			if err != nil && err != ErrSameWarehouse {
				mu.Lock()
				errs = append(errs, fmt.Errorf("операция %d: %w", index, err))
				mu.Unlock()
				return
			}
			mu.Lock()
			moved += len(movements)
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("ошибки параллельных массовых перемещений: %v", errs)
	}
	// Каждая из 30 единиц перемещается не более одного раза.
	if moved != len(ids) {
		t.Fatalf("перемещений = %d, want %d", moved, len(ids))
	}
	totalMovements := 0
	for _, id := range ids {
		if warehouseOf(t, repo, id) != fx.warehouseB {
			t.Fatalf("единица %d не в целевом складе", id)
		}
		_, total, err := repo.Movements(ctx, id, 1, 100)
		if err != nil {
			t.Fatalf("Movements(%d): %v", id, err)
		}
		totalMovements += total
	}
	if totalMovements != len(ids) {
		t.Fatalf("записей журнала = %d, want %d", totalMovements, len(ids))
	}
}

package equipment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// fakeRepo — in-memory репозиторий: правила EQ-2/EQ-3/EQ-5 проверяются без MySQL
// (в окружении разработки БД нет, docs/ENVIRONMENT.md).
type fakeRepo struct {
	items        map[int64]*Equipment
	warehouses   map[int64]WarehouseInfo
	types        map[int64]bool
	registration map[string]int64 // регистрационный номер → id неудалённой единицы

	listItems  []Equipment
	listTotal  int
	listFilter ListFilter

	movements map[int64][]Movement

	createErr error
	updateErr error
	deleteErr error

	deletedID int64
	created   *Equipment
	nextID    int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items:        map[int64]*Equipment{},
		warehouses:   map[int64]WarehouseInfo{},
		types:        map[int64]bool{},
		registration: map[string]int64{},
		movements:    map[int64][]Movement{},
		nextID:       100,
	}
}

func (r *fakeRepo) List(_ context.Context, filter ListFilter) ([]Equipment, int, error) {
	r.listFilter = filter
	return r.listItems, r.listTotal, nil
}

func (r *fakeRepo) ByID(_ context.Context, id int64) (*Equipment, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *fakeRepo) Create(_ context.Context, in CreateInput) (*Equipment, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	r.nextID++
	warehouse := r.warehouses[in.WarehouseID]
	item := &Equipment{
		ID:                 r.nextID,
		Name:               in.Name,
		EquipmentTypeID:    in.EquipmentTypeID,
		SerialNumber:       in.SerialNumber,
		RegistrationNumber: in.RegistrationNumber,
		WarehouseID:        in.WarehouseID,
		WarehouseName:      warehouse.Name,
		BuildingID:         warehouse.BuildingID,
		BuildingName:       warehouse.BuildingName,
		CreatedAt:          time.Unix(0, 0).UTC(),
		UpdatedAt:          time.Unix(0, 0).UTC(),
	}
	r.items[item.ID] = item
	return item, nil
}

func (r *fakeRepo) Update(_ context.Context, id int64, in UpdateInput) (*Equipment, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	if in.Name != nil {
		item.Name = *in.Name
	}
	if in.SerialNumber != nil {
		item.SerialNumber = *in.SerialNumber
	}
	if in.RegistrationNumber != nil {
		item.RegistrationNumber = *in.RegistrationNumber
	}
	if in.EquipmentTypeID != nil {
		item.EquipmentTypeID = *in.EquipmentTypeID
	}
	if in.WarehouseID != nil {
		item.WarehouseID = *in.WarehouseID
	}
	return item, nil
}

func (r *fakeRepo) SoftDelete(_ context.Context, id int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	r.deletedID = id
	return nil
}

func (r *fakeRepo) RegistrationNumberTaken(_ context.Context, registrationNumber string, excludeID int64) (bool, error) {
	id := r.registration[registrationNumber]
	return id != 0 && id != excludeID, nil
}

func (r *fakeRepo) EquipmentTypeExists(_ context.Context, id int64) (bool, error) {
	return r.types[id], nil
}

func (r *fakeRepo) WarehouseInfo(_ context.Context, id int64) (*WarehouseInfo, error) {
	info, ok := r.warehouses[id]
	if !ok {
		return nil, ErrWarehouseNotFound
	}
	return &info, nil
}

func (r *fakeRepo) Move(_ context.Context, id int64, in MoveInput, actor Actor) (*Equipment, *Movement, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, nil, ErrNotFound
	}
	target, ok := r.warehouses[in.ToWarehouseID]
	if !ok {
		return nil, nil, ErrWarehouseNotFound
	}
	if err := CheckMove(item.WarehouseID, target, actor); err != nil {
		return nil, nil, err
	}
	from := r.warehouses[item.WarehouseID]
	item.WarehouseID = target.ID
	item.WarehouseName = target.Name
	item.BuildingID = target.BuildingID
	item.BuildingName = target.BuildingName

	movement := Movement{
		ID:                int64(len(r.movements[id]) + 1),
		EquipmentID:       id,
		FromWarehouseID:   from.ID,
		FromWarehouseName: from.Name,
		FromBuildingName:  from.BuildingName,
		ToWarehouseID:     target.ID,
		ToWarehouseName:   target.Name,
		ToBuildingName:    target.BuildingName,
		MovedByUserID:     actor.UserID,
		MovedAt:           time.Unix(0, 0).UTC(),
		Comment:           in.Comment,
	}
	r.movements[id] = append(r.movements[id], movement)
	return item, &movement, nil
}

func (r *fakeRepo) Movements(_ context.Context, equipmentID int64, _, _ int) ([]Movement, int, error) {
	items := r.movements[equipmentID]
	return items, len(items), nil
}

// requireAPIError проверяет статус и код типизированной ошибки API.
func requireAPIError(t *testing.T, err error, status int, code apierr.Code) *apierr.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка %s, получен nil", code)
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("ошибка не является *apierr.Error: %v", err)
	}
	if apiErr.Status != status {
		t.Fatalf("status = %d, want %d (%s)", apiErr.Status, status, apiErr.Message)
	}
	if apiErr.Code != code {
		t.Fatalf("code = %s, want %s", apiErr.Code, code)
	}
	return apiErr
}

func fixture() *fakeRepo {
	repo := newFakeRepo()
	repo.types[1] = true
	repo.types[2] = true
	repo.warehouses[10] = WarehouseInfo{ID: 10, Name: "Склад A", BuildingID: 1, BuildingName: "Здание A", BuildingActive: true}
	repo.warehouses[20] = WarehouseInfo{ID: 20, Name: "Склад B", BuildingID: 2, BuildingName: "Здание B", BuildingActive: true}
	repo.warehouses[30] = WarehouseInfo{ID: 30, Name: "Склад C", BuildingID: 3, BuildingName: "Здание C", BuildingActive: false}
	repo.items[1] = &Equipment{
		ID: 1, Name: "Ноутбук", EquipmentTypeID: 1, SerialNumber: "SN-1",
		RegistrationNumber: "REG-1", WarehouseID: 10, WarehouseName: "Склад A",
		BuildingID: 1, BuildingName: "Здание A",
	}
	repo.registration["REG-1"] = 1
	return repo
}

var userActor = Actor{UserID: 7, IsAdmin: false}
var adminActor = Actor{UserID: 1, IsAdmin: true}

// EQ-2: дубль регистрационного номера среди неудалённых → 409
// DUPLICATE_REGISTRATION_NUMBER (и предпроверкой, и гонкой на UNIQUE).
func TestServiceCreateDuplicateRegistration(t *testing.T) {
	repo := fixture()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), CreateInput{
		Name: "Второй", EquipmentTypeID: 1, SerialNumber: "SN-2",
		RegistrationNumber: "REG-1", WarehouseID: 10,
	})
	requireAPIError(t, err, 409, apierr.CodeDuplicateRegistrationNumber)

	repo.createErr = ErrDuplicateRegistration
	_, err = svc.Create(context.Background(), CreateInput{
		Name: "Третий", EquipmentTypeID: 1, SerialNumber: "SN-3",
		RegistrationNumber: "REG-NEW", WarehouseID: 10,
	})
	requireAPIError(t, err, 409, apierr.CodeDuplicateRegistrationNumber)
}

// Создание: несуществующий тип или склад → 404; корректный вход → единица.
func TestServiceCreate(t *testing.T) {
	repo := fixture()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), CreateInput{
		Name: "Новый", EquipmentTypeID: 99, SerialNumber: "SN", RegistrationNumber: "R-1", WarehouseID: 10,
	})
	requireAPIError(t, err, 404, apierr.CodeNotFound)

	_, err = svc.Create(context.Background(), CreateInput{
		Name: "Новый", EquipmentTypeID: 1, SerialNumber: "SN", RegistrationNumber: "R-2", WarehouseID: 99,
	})
	requireAPIError(t, err, 404, apierr.CodeNotFound)

	item, err := svc.Create(context.Background(), CreateInput{
		Name: "Новый", EquipmentTypeID: 2, SerialNumber: "SN-9", RegistrationNumber: "R-3", WarehouseID: 20,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if item.WarehouseID != 20 || item.BuildingID != 2 || item.RegistrationNumber != "R-3" {
		t.Fatalf("item = %+v", item)
	}
}

// PATCH-смена склада: USER в неактивное здание → 422 BUILDING_INACTIVE,
// ADMIN — можно, тот же склад — можно (склада не меняется).
func TestServiceUpdateWarehouseRules(t *testing.T) {
	warehouseID := func(id int64) *int64 { return &id }

	t.Run("USER в неактивное здание", func(t *testing.T) {
		repo := fixture()
		_, err := NewService(repo).Update(context.Background(), 1,
			UpdateInput{WarehouseID: warehouseID(30)}, userActor)
		requireAPIError(t, err, 422, apierr.CodeBuildingInactive)
		if repo.items[1].WarehouseID != 10 {
			t.Fatal("склад не должен меняться при ошибке")
		}
	})

	t.Run("ADMIN в неактивное здание", func(t *testing.T) {
		repo := fixture()
		item, err := NewService(repo).Update(context.Background(), 1,
			UpdateInput{WarehouseID: warehouseID(30)}, adminActor)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if item.WarehouseID != 30 {
			t.Fatalf("warehouse_id = %d", item.WarehouseID)
		}
	})

	t.Run("USER в активное здание", func(t *testing.T) {
		repo := fixture()
		item, err := NewService(repo).Update(context.Background(), 1,
			UpdateInput{WarehouseID: warehouseID(20)}, userActor)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if item.WarehouseID != 20 {
			t.Fatalf("warehouse_id = %d", item.WarehouseID)
		}
	})

	t.Run("тот же склад неактивного здания", func(t *testing.T) {
		repo := fixture()
		repo.items[1].WarehouseID = 30
		if _, err := NewService(repo).Update(context.Background(), 1,
			UpdateInput{WarehouseID: warehouseID(30)}, userActor); err != nil {
			t.Fatalf("смена на тот же склад не должна проверяться: %v", err)
		}
	})

	t.Run("несуществующий склад", func(t *testing.T) {
		repo := fixture()
		_, err := NewService(repo).Update(context.Background(), 1,
			UpdateInput{WarehouseID: warehouseID(99)}, userActor)
		requireAPIError(t, err, 404, apierr.CodeNotFound)
	})

	t.Run("свой регистрационный номер не конфликтует", func(t *testing.T) {
		repo := fixture()
		same := "REG-1"
		if _, err := NewService(repo).Update(context.Background(), 1,
			UpdateInput{RegistrationNumber: &same}, userActor); err != nil {
			t.Fatalf("Update: %v", err)
		}
	})

	t.Run("чужой регистрационный номер", func(t *testing.T) {
		repo := fixture()
		repo.items[2] = &Equipment{ID: 2, RegistrationNumber: "REG-2", WarehouseID: 10}
		repo.registration["REG-2"] = 2
		other := "REG-2"
		_, err := NewService(repo).Update(context.Background(), 1,
			UpdateInput{RegistrationNumber: &other}, userActor)
		requireAPIError(t, err, 409, apierr.CodeDuplicateRegistrationNumber)
	})

	t.Run("несуществующая единица", func(t *testing.T) {
		_, err := NewService(fixture()).Update(context.Background(), 99, UpdateInput{}, userActor)
		requireAPIError(t, err, 404, apierr.CodeNotFound)
	})
}

// EQ-3: перемещение — смена склада и одна запись журнала; тот же склад → 409,
// неактивное здание для USER → 422, для ADMIN — можно.
func TestServiceMove(t *testing.T) {
	t.Run("успешное перемещение", func(t *testing.T) {
		repo := fixture()
		result, err := NewService(repo).Move(context.Background(), 1,
			MoveInput{ToWarehouseID: 20, Comment: strPtr("перенос")}, userActor)
		if err != nil {
			t.Fatalf("Move: %v", err)
		}
		if result.Equipment.WarehouseID != 20 || result.Equipment.BuildingID != 2 {
			t.Fatalf("equipment = %+v", result.Equipment)
		}
		if result.Movement.FromWarehouseID != 10 || result.Movement.ToWarehouseID != 20 {
			t.Fatalf("movement = %+v", result.Movement)
		}
		if result.Movement.MovedByUserID != userActor.UserID {
			t.Fatalf("moved_by_user_id = %d", result.Movement.MovedByUserID)
		}
		if result.Movement.FromWarehouseName != "Склад A" || result.Movement.ToBuildingName != "Здание B" {
			t.Fatalf("имена в журнале = %+v", result.Movement)
		}
		if len(repo.movements[1]) != 1 {
			t.Fatalf("журнал = %v", repo.movements[1])
		}
	})

	t.Run("тот же склад", func(t *testing.T) {
		repo := fixture()
		_, err := NewService(repo).Move(context.Background(), 1, MoveInput{ToWarehouseID: 10}, userActor)
		requireAPIError(t, err, 409, apierr.CodeSameWarehouse)
		if len(repo.movements[1]) != 0 {
			t.Fatal("журнал не должен пополняться при ошибке")
		}
	})

	t.Run("USER в неактивное здание", func(t *testing.T) {
		repo := fixture()
		_, err := NewService(repo).Move(context.Background(), 1, MoveInput{ToWarehouseID: 30}, userActor)
		requireAPIError(t, err, 422, apierr.CodeBuildingInactive)
	})

	t.Run("ADMIN в неактивное здание", func(t *testing.T) {
		repo := fixture()
		result, err := NewService(repo).Move(context.Background(), 1, MoveInput{ToWarehouseID: 30}, adminActor)
		if err != nil {
			t.Fatalf("Move: %v", err)
		}
		if result.Equipment.WarehouseID != 30 {
			t.Fatalf("warehouse_id = %d", result.Equipment.WarehouseID)
		}
	})

	t.Run("несуществующая единица", func(t *testing.T) {
		_, err := NewService(fixture()).Move(context.Background(), 99, MoveInput{ToWarehouseID: 20}, userActor)
		requireAPIError(t, err, 404, apierr.CodeNotFound)
	})

	t.Run("несуществующий склад", func(t *testing.T) {
		_, err := NewService(fixture()).Move(context.Background(), 1, MoveInput{ToWarehouseID: 99}, userActor)
		requireAPIError(t, err, 404, apierr.CodeNotFound)
	})
}

// EQ-5: журнал отдаётся только для существующей единицы.
func TestServiceMovements(t *testing.T) {
	repo := fixture()
	svc := NewService(repo)

	if _, err := svc.Move(context.Background(), 1, MoveInput{ToWarehouseID: 20}, userActor); err != nil {
		t.Fatalf("Move: %v", err)
	}
	page, err := svc.Movements(context.Background(), 1, 1, 20)
	if err != nil {
		t.Fatalf("Movements: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Page != 1 || page.Size != 20 {
		t.Fatalf("page = %+v", page)
	}

	_, err = svc.Movements(context.Background(), 99, 1, 20)
	requireAPIError(t, err, 404, apierr.CodeNotFound)
}

// Удаление (soft delete, ADMIN проверяется маршрутом).
func TestServiceDelete(t *testing.T) {
	repo := fixture()
	if err := NewService(repo).Delete(context.Background(), 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if repo.deletedID != 1 {
		t.Fatalf("deletedID = %d", repo.deletedID)
	}

	requireAPIError(t, NewService(fixture()).Delete(context.Background(), 99),
		404, apierr.CodeNotFound)
}

// Чистые правила EQ-3.
func TestCheckMove(t *testing.T) {
	active := WarehouseInfo{ID: 20, BuildingActive: true}
	inactive := WarehouseInfo{ID: 30, BuildingActive: false}

	if err := CheckMove(10, active, userActor); err != nil {
		t.Fatalf("активное здание: %v", err)
	}
	if err := CheckMove(20, active, userActor); !errors.Is(err, ErrSameWarehouse) {
		t.Fatalf("тот же склад = %v", err)
	}
	if err := CheckMove(10, inactive, userActor); !errors.Is(err, ErrBuildingInactive) {
		t.Fatalf("USER в неактивное здание = %v", err)
	}
	if err := CheckMove(10, inactive, adminActor); err != nil {
		t.Fatalf("ADMIN в неактивное здание = %v", err)
	}
	// Тот же склад важнее активности: изменений нет, ответ SAME_WAREHOUSE.
	if err := CheckMove(30, inactive, userActor); !errors.Is(err, ErrSameWarehouse) {
		t.Fatalf("тот же склад в неактивном здании = %v", err)
	}
}

// Таблица маппинга доменных ошибок в коды контракта.
func TestMapError(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   apierr.Code
	}{
		{ErrNotFound, 404, apierr.CodeNotFound},
		{ErrWarehouseNotFound, 404, apierr.CodeNotFound},
		{ErrDuplicateRegistration, 409, apierr.CodeDuplicateRegistrationNumber},
		{ErrSameWarehouse, 409, apierr.CodeSameWarehouse},
		{ErrBuildingInactive, 422, apierr.CodeBuildingInactive},
		{errors.New("boom"), 500, apierr.CodeInternal},
	}
	for _, tc := range tests {
		requireAPIError(t, mapError(tc.err), tc.status, tc.code)
	}
	if err := mapError(nil); err != nil {
		t.Fatalf("mapError(nil) = %v", err)
	}
}

func strPtr(value string) *string { return &value }

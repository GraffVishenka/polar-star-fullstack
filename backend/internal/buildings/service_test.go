package buildings

import (
	"context"
	"errors"
	"testing"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// fakeRepo — in-memory репозиторий: правила домена проверяются без MySQL
// (в окружении разработки БД нет, см. docs/ENVIRONMENT.md).
type fakeRepo struct {
	buildings  []Building
	warehouses []Warehouse
	building   *Building

	usage    BuildingUsage
	usageErr error

	deleteBuildingErr  error
	deleteWarehouseErr error
	warehouseCount     int
	warehouseCountErr  error
	nameTaken          bool
	nameTakenErr       error

	deletedBuildingID  int64
	deletedWarehouseID int64
	createdWarehouse   *Warehouse
}

func (r *fakeRepo) Buildings(context.Context, bool) ([]Building, error) { return r.buildings, nil }

func (r *fakeRepo) BuildingByID(_ context.Context, id int64) (*Building, error) {
	if r.building == nil || r.building.ID != id {
		return nil, ErrNotFound
	}
	return r.building, nil
}

func (r *fakeRepo) CreateBuilding(_ context.Context, name string, address *string) (*Building, error) {
	return &Building{ID: 1, Name: name, Address: address, IsActive: true}, nil
}

func (r *fakeRepo) UpdateBuilding(_ context.Context, id int64, patch BuildingPatch) (*Building, error) {
	if r.building == nil || r.building.ID != id {
		return nil, ErrNotFound
	}
	if patch.Name != nil {
		r.building.Name = *patch.Name
	}
	if patch.AddressSet {
		r.building.Address = patch.Address
	}
	if patch.IsActive != nil {
		r.building.IsActive = *patch.IsActive
	}
	return r.building, nil
}

func (r *fakeRepo) BuildingUsage(context.Context, int64) (BuildingUsage, error) {
	return r.usage, r.usageErr
}

func (r *fakeRepo) DeleteBuilding(_ context.Context, id int64) error {
	if r.deleteBuildingErr != nil {
		return r.deleteBuildingErr
	}
	r.deletedBuildingID = id
	return nil
}

func (r *fakeRepo) Warehouses(context.Context, *int64) ([]Warehouse, error) {
	return r.warehouses, nil
}

func (r *fakeRepo) WarehouseByID(_ context.Context, id int64) (*Warehouse, error) {
	for i := range r.warehouses {
		if r.warehouses[i].ID == id {
			return &r.warehouses[i], nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepo) CreateWarehouse(_ context.Context, buildingID int64, name string) (*Warehouse, error) {
	if r.nameTakenErr != nil {
		return nil, r.nameTakenErr
	}
	if r.nameTaken {
		return nil, ErrNameTaken
	}
	r.createdWarehouse = &Warehouse{ID: 10, BuildingID: buildingID, BuildingName: "Здание", Name: name}
	return r.createdWarehouse, nil
}

func (r *fakeRepo) UpdateWarehouse(_ context.Context, id int64, name string) (*Warehouse, error) {
	for i := range r.warehouses {
		if r.warehouses[i].ID == id {
			r.warehouses[i].Name = name
			return &r.warehouses[i], nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepo) DeleteWarehouse(_ context.Context, id int64) error {
	if r.deleteWarehouseErr != nil {
		return r.deleteWarehouseErr
	}
	r.deletedWarehouseID = id
	return nil
}

func (r *fakeRepo) WarehouseNameTaken(context.Context, int64, string, int64) (bool, error) {
	return r.nameTaken, r.nameTakenErr
}

func (r *fakeRepo) WarehouseEquipmentCount(context.Context, int64) (int, error) {
	return r.warehouseCount, r.warehouseCountErr
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

// EQ-6: физическое удаление здания запрещено при складах, оборудовании и
// закреплённых пользователях → 409 BUILDING_IN_USE с причиной в details.
func TestServiceDeleteBuildingInUse(t *testing.T) {
	tests := []struct {
		name    string
		usage   BuildingUsage
		reasons []any
	}{
		{name: "склады", usage: BuildingUsage{Warehouses: 2}, reasons: []any{"warehouses"}},
		{name: "оборудование", usage: BuildingUsage{Equipment: 5}, reasons: []any{"equipment"}},
		{name: "пользователи", usage: BuildingUsage{Users: 1}, reasons: []any{"users"}},
		{name: "всё сразу", usage: BuildingUsage{Warehouses: 1, Equipment: 1, Users: 2},
			reasons: []any{"warehouses", "equipment", "users"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{building: &Building{ID: 7, Name: "Здание"}, usage: tc.usage}
			svc := NewService(repo)

			err := svc.DeleteBuilding(context.Background(), 7)
			apiErr := requireAPIError(t, err, 409, apierr.CodeBuildingInUse)
			reasons, ok := apiErr.Details["reasons"].([]string)
			if !ok {
				t.Fatalf("details.reasons отсутствует: %v", apiErr.Details)
			}
			if len(reasons) != len(tc.reasons) {
				t.Fatalf("reasons = %v, want %v", reasons, tc.reasons)
			}
			if repo.deletedBuildingID != 0 {
				t.Fatal("удаление не должно вызываться")
			}
		})
	}
}

// EQ-6: пустое здание удаляется.
func TestServiceDeleteBuildingEmpty(t *testing.T) {
	repo := &fakeRepo{building: &Building{ID: 3, Name: "Пустое"}}
	if err := NewService(repo).DeleteBuilding(context.Background(), 3); err != nil {
		t.Fatalf("DeleteBuilding: %v", err)
	}
	if repo.deletedBuildingID != 3 {
		t.Fatalf("deletedBuildingID = %d", repo.deletedBuildingID)
	}
}

// EQ-6: FK RESTRICT по заявкам (появится в Б5) маппится в 409 BUILDING_IN_USE.
func TestServiceDeleteBuildingForeignKey(t *testing.T) {
	repo := &fakeRepo{building: &Building{ID: 3}, deleteBuildingErr: ErrBuildingInUse}
	requireAPIError(t, NewService(repo).DeleteBuilding(context.Background(), 3),
		409, apierr.CodeBuildingInUse)
}

// Несуществующее здание → 404.
func TestServiceDeleteBuildingNotFound(t *testing.T) {
	repo := &fakeRepo{}
	requireAPIError(t, NewService(repo).DeleteBuilding(context.Background(), 42),
		404, apierr.CodeNotFound)
}

// EQ-6: удаление непустого склада → 409 WAREHOUSE_NOT_EMPTY.
func TestServiceDeleteWarehouseNotEmpty(t *testing.T) {
	repo := &fakeRepo{
		warehouses:     []Warehouse{{ID: 4, BuildingID: 1, Name: "Склад"}},
		warehouseCount: 3,
	}
	err := NewService(repo).DeleteWarehouse(context.Background(), 4)
	apiErr := requireAPIError(t, err, 409, apierr.CodeWarehouseNotEmpty)
	if apiErr.Details["equipment"] != 3 {
		t.Fatalf("details = %v", apiErr.Details)
	}
	if repo.deletedWarehouseID != 0 {
		t.Fatal("удаление не должно вызываться")
	}
}

// Журнал перемещений ссылается на склад через FK: удаление защищено той же
// ошибкой WAREHOUSE_NOT_EMPTY.
func TestServiceDeleteWarehouseWithHistory(t *testing.T) {
	repo := &fakeRepo{
		warehouses:         []Warehouse{{ID: 4, BuildingID: 1, Name: "Склад"}},
		deleteWarehouseErr: ErrWarehouseNotEmpty,
	}
	requireAPIError(t, NewService(repo).DeleteWarehouse(context.Background(), 4),
		409, apierr.CodeWarehouseNotEmpty)
}

// Пустой склад без истории удаляется.
func TestServiceDeleteWarehouseEmpty(t *testing.T) {
	repo := &fakeRepo{warehouses: []Warehouse{{ID: 5, BuildingID: 1, Name: "Склад"}}}
	if err := NewService(repo).DeleteWarehouse(context.Background(), 5); err != nil {
		t.Fatalf("DeleteWarehouse: %v", err)
	}
	if repo.deletedWarehouseID != 5 {
		t.Fatalf("deletedWarehouseID = %d", repo.deletedWarehouseID)
	}
}

// EQ-1: имя склада уникально в рамках здания → 409 CONFLICT.
func TestServiceCreateWarehouseDuplicateName(t *testing.T) {
	repo := &fakeRepo{
		building:  &Building{ID: 1, Name: "Здание"},
		nameTaken: true,
	}
	svc := NewService(repo)

	_, err := svc.CreateWarehouse(context.Background(), WarehouseInput{BuildingID: 1, Name: "Склад"})
	requireAPIError(t, err, 409, apierr.CodeConflict)
	if repo.createdWarehouse != nil {
		t.Fatal("склад-дубль не должен создаваться")
	}
}

// Несуществующее здание для нового склада → 404.
func TestServiceCreateWarehouseBuildingNotFound(t *testing.T) {
	svc := NewService(&fakeRepo{})
	_, err := svc.CreateWarehouse(context.Background(), WarehouseInput{BuildingID: 99, Name: "Склад"})
	requireAPIError(t, err, 404, apierr.CodeNotFound)
}

// Деактивация здания — это PATCH is_active=false (EQ-6), оно остаётся доступным.
func TestServiceUpdateBuildingDeactivate(t *testing.T) {
	repo := &fakeRepo{building: &Building{ID: 2, Name: "Здание", IsActive: true}}
	inactive := false

	updated, err := NewService(repo).UpdateBuilding(context.Background(), 2, BuildingPatch{IsActive: &inactive})
	if err != nil {
		t.Fatalf("UpdateBuilding: %v", err)
	}
	if updated.IsActive {
		t.Fatal("здание должно быть деактивировано")
	}
	if updated.Name != "Здание" {
		t.Fatalf("name = %q", updated.Name)
	}
}

// Таблица маппинга доменных ошибок: коды и статусы должны совпадать с
// контрактом (openapi ErrorCode).
func TestMapError(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   apierr.Code
	}{
		{ErrNotFound, 404, apierr.CodeNotFound},
		{ErrNameTaken, 409, apierr.CodeConflict},
		{ErrWarehouseNotEmpty, 409, apierr.CodeWarehouseNotEmpty},
		{ErrBuildingInUse, 409, apierr.CodeBuildingInUse},
		{errors.New("boom"), 500, apierr.CodeInternal},
	}

	for _, tc := range tests {
		requireAPIError(t, mapError(tc.err, buildingNotFoundMessage), tc.status, tc.code)
	}
	if err := mapError(nil, buildingNotFoundMessage); err != nil {
		t.Fatalf("mapError(nil) = %v", err)
	}
}

// Чистые правила EQ-6 (без репозитория).
func TestCheckDeletableRules(t *testing.T) {
	if err := checkBuildingDeletable(BuildingUsage{}); err != nil {
		t.Fatalf("пустое здание должно быть удаляемым: %v", err)
	}
	requireAPIError(t, checkBuildingDeletable(BuildingUsage{Users: 1}), 409, apierr.CodeBuildingInUse)

	if err := checkWarehouseDeletable(0); err != nil {
		t.Fatalf("пустой склад должен быть удаляемым: %v", err)
	}
	requireAPIError(t, checkWarehouseDeletable(1), 409, apierr.CodeWarehouseNotEmpty)
}

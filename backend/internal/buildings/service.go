package buildings

import (
	"context"
	"errors"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Сообщения об отсутствии сущности (используются в 404 по всему домену).
const (
	buildingNotFoundMessage  = "Здание не найдено"
	warehouseNotFoundMessage = "Склад не найден"
)

// Service — бизнес-правила зданий и складов (EQ-1, EQ-6). HTTP-слой только
// валидирует вход и маппит ошибки, репозиторий — только SQL.
type Service struct {
	repo Repository
}

// NewService создаёт сервис поверх репозитория.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Buildings — список зданий. includeInactive=true (по умолчанию в API) отдаёт и
// деактивированные: они нужны в истории и фильтрах (EQ-1, EQ-6).
func (s *Service) Buildings(ctx context.Context, includeInactive bool) ([]Building, error) {
	items, err := s.repo.Buildings(ctx, includeInactive)
	if err != nil {
		return nil, mapError(err, buildingNotFoundMessage)
	}
	return items, nil
}

// Building — карточка здания.
func (s *Service) Building(ctx context.Context, id int64) (*Building, error) {
	building, err := s.repo.BuildingByID(ctx, id)
	if err != nil {
		return nil, mapError(err, buildingNotFoundMessage)
	}
	return building, nil
}

// CreateBuilding создаёт здание (только ADMIN — на уровне маршрута).
func (s *Service) CreateBuilding(ctx context.Context, in BuildingInput) (*Building, error) {
	building, err := s.repo.CreateBuilding(ctx, in.Name, in.Address)
	if err != nil {
		return nil, mapError(err, buildingNotFoundMessage)
	}
	return building, nil
}

// UpdateBuilding правит здание. Деактивация — это is_active=false (EQ-6):
// здание исчезает из публичного выбора и форм, история сохраняется.
func (s *Service) UpdateBuilding(ctx context.Context, id int64, patch BuildingPatch) (*Building, error) {
	building, err := s.repo.UpdateBuilding(ctx, id, patch)
	if err != nil {
		return nil, mapError(err, buildingNotFoundMessage)
	}
	return building, nil
}

// DeleteBuilding физически удаляет здание. EQ-6: разрешено, только если нет
// складов, оборудования, закреплённых пользователей и заявок; иначе 409
// BUILDING_IN_USE. Заявки проверяются FK-ом (таблица появится в Б5), остальные
// зависимости — явными счётчиками, чтобы в details была понятная причина.
func (s *Service) DeleteBuilding(ctx context.Context, id int64) error {
	if _, err := s.repo.BuildingByID(ctx, id); err != nil {
		return mapError(err, buildingNotFoundMessage)
	}
	usage, err := s.repo.BuildingUsage(ctx, id)
	if err != nil {
		return mapError(err, buildingNotFoundMessage)
	}
	if err := checkBuildingDeletable(usage); err != nil {
		return err
	}
	return mapError(s.repo.DeleteBuilding(ctx, id), buildingNotFoundMessage)
}

// Warehouses — склады (все или одного здания). Чтение — всем авторизованным.
func (s *Service) Warehouses(ctx context.Context, buildingID *int64) ([]Warehouse, error) {
	items, err := s.repo.Warehouses(ctx, buildingID)
	if err != nil {
		return nil, mapError(err, warehouseNotFoundMessage)
	}
	return items, nil
}

// Warehouse — карточка склада.
func (s *Service) Warehouse(ctx context.Context, id int64) (*Warehouse, error) {
	warehouse, err := s.repo.WarehouseByID(ctx, id)
	if err != nil {
		return nil, mapError(err, warehouseNotFoundMessage)
	}
	return warehouse, nil
}

// CreateWarehouse создаёт склад в здании (только ADMIN). EQ-1: имя уникально в
// рамках здания, дубль → 409 CONFLICT.
func (s *Service) CreateWarehouse(ctx context.Context, in WarehouseInput) (*Warehouse, error) {
	if _, err := s.repo.BuildingByID(ctx, in.BuildingID); err != nil {
		return nil, mapError(err, buildingNotFoundMessage)
	}
	taken, err := s.repo.WarehouseNameTaken(ctx, in.BuildingID, in.Name, 0)
	if err != nil {
		return nil, mapError(err, warehouseNotFoundMessage)
	}
	if taken {
		return nil, errWarehouseNameTaken()
	}
	warehouse, err := s.repo.CreateWarehouse(ctx, in.BuildingID, in.Name)
	if err != nil {
		return nil, mapError(err, warehouseNotFoundMessage)
	}
	return warehouse, nil
}

// UpdateWarehouse переименовывает склад (только ADMIN).
func (s *Service) UpdateWarehouse(ctx context.Context, id int64, in WarehouseInput) (*Warehouse, error) {
	current, err := s.repo.WarehouseByID(ctx, id)
	if err != nil {
		return nil, mapError(err, warehouseNotFoundMessage)
	}
	taken, err := s.repo.WarehouseNameTaken(ctx, current.BuildingID, in.Name, id)
	if err != nil {
		return nil, mapError(err, warehouseNotFoundMessage)
	}
	if taken {
		return nil, errWarehouseNameTaken()
	}
	warehouse, err := s.repo.UpdateWarehouse(ctx, id, in.Name)
	if err != nil {
		return nil, mapError(err, warehouseNotFoundMessage)
	}
	return warehouse, nil
}

// DeleteWarehouse удаляет пустой склад (только ADMIN). EQ-6: склад с
// оборудованием → 409 WAREHOUSE_NOT_EMPTY; склад, на который ссылается журнал
// перемещений, защищён FK — история не должна «портиться».
func (s *Service) DeleteWarehouse(ctx context.Context, id int64) error {
	if _, err := s.repo.WarehouseByID(ctx, id); err != nil {
		return mapError(err, warehouseNotFoundMessage)
	}
	count, err := s.repo.WarehouseEquipmentCount(ctx, id)
	if err != nil {
		return mapError(err, warehouseNotFoundMessage)
	}
	if err := checkWarehouseDeletable(count); err != nil {
		return err
	}
	return mapError(s.repo.DeleteWarehouse(ctx, id), warehouseNotFoundMessage)
}

// checkBuildingDeletable — чистое правило EQ-6 (тестируется без БД).
func checkBuildingDeletable(usage BuildingUsage) error {
	reasons := make([]string, 0, 3)
	if usage.Warehouses > 0 {
		reasons = append(reasons, "warehouses")
	}
	if usage.Equipment > 0 {
		reasons = append(reasons, "equipment")
	}
	if usage.Users > 0 {
		reasons = append(reasons, "users")
	}
	if len(reasons) == 0 {
		return nil
	}
	return apierr.Conflict(apierr.CodeBuildingInUse,
		"Здание нельзя удалить: есть связанные данные, используйте деактивацию").
		WithDetails(map[string]any{
			"reasons":    reasons,
			"warehouses": usage.Warehouses,
			"equipment":  usage.Equipment,
			"users":      usage.Users,
		})
}

// checkWarehouseDeletable — чистое правило EQ-6 для склада.
func checkWarehouseDeletable(equipmentCount int) error {
	if equipmentCount <= 0 {
		return nil
	}
	return apierr.Conflict(apierr.CodeWarehouseNotEmpty,
		"Склад нельзя удалить: в нём есть оборудование").
		WithDetails(map[string]any{"reason": "equipment", "equipment": equipmentCount})
}

func errWarehouseNameTaken() error {
	return apierr.Conflict(apierr.CodeConflict, "Склад с таким названием уже есть в этом здании")
}

// mapError переводит доменные ошибки репозитория в типизированные ошибки API.
func mapError(err error, notFoundMessage string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apierr.NotFound(notFoundMessage)
	case errors.Is(err, ErrNameTaken):
		return apierr.Conflict(apierr.CodeConflict, "Склад с таким названием уже есть в этом здании")
	case errors.Is(err, ErrWarehouseNotEmpty):
		return apierr.Conflict(apierr.CodeWarehouseNotEmpty,
			"Склад нельзя удалить: он используется оборудованием или журналом перемещений")
	case errors.Is(err, ErrBuildingInUse):
		return apierr.Conflict(apierr.CodeBuildingInUse,
			"Здание нельзя удалить: есть связанные данные, используйте деактивацию")
	default:
		return apierr.Internal(err)
	}
}

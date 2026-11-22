package equipment

import (
	"context"
	"errors"
	"net/http"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Service — бизнес-правила оборудования (EQ-2, EQ-3, EQ-5, EQ-7).
type Service struct {
	repo Repository
}

// NewService создаёт сервис поверх репозитория.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// List — страница оборудования с фильтрами и поиском (EQ-7).
func (s *Service) List(ctx context.Context, filter ListFilter) (*ListResult, error) {
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, mapError(err)
	}
	return &ListResult{Items: items, Total: total, Page: filter.Page, Size: filter.Size}, nil
}

// Get — карточка единицы.
func (s *Service) Get(ctx context.Context, id int64) (*Equipment, error) {
	item, err := s.repo.ByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return item, nil
}

// Create добавляет единицу оборудования. Проверяет существование типа и склада
// (404) и уникальность регистрационного номера среди неудалённых (409, EQ-2).
func (s *Service) Create(ctx context.Context, in CreateInput) (*Equipment, error) {
	if err := s.requireEquipmentType(ctx, in.EquipmentTypeID); err != nil {
		return nil, err
	}
	if _, err := s.repo.WarehouseInfo(ctx, in.WarehouseID); err != nil {
		return nil, mapError(err)
	}
	taken, err := s.repo.RegistrationNumberTaken(ctx, in.RegistrationNumber, 0)
	if err != nil {
		return nil, mapError(err)
	}
	if taken {
		return nil, errDuplicateRegistration()
	}

	item, err := s.repo.Create(ctx, in)
	if err != nil {
		return nil, mapError(err)
	}
	return item, nil
}

// Update правит единицу. Смена склада через PATCH разрешена контрактом, но
// журнал не создаёт и подчиняется тому же правилу неактивного здания, что и
// перемещение (иначе USER обошёл бы EQ-3 другим эндпоинтом).
func (s *Service) Update(ctx context.Context, id int64, in UpdateInput, actor Actor) (*Equipment, error) {
	current, err := s.repo.ByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	if in.EquipmentTypeID != nil {
		if err := s.requireEquipmentType(ctx, *in.EquipmentTypeID); err != nil {
			return nil, err
		}
	}
	if in.WarehouseID != nil && *in.WarehouseID != current.WarehouseID {
		target, err := s.repo.WarehouseInfo(ctx, *in.WarehouseID)
		if err != nil {
			return nil, mapError(err)
		}
		if err := CheckWarehouseTarget(*target, actor); err != nil {
			return nil, mapError(err)
		}
	}
	if in.RegistrationNumber != nil && *in.RegistrationNumber != current.RegistrationNumber {
		taken, err := s.repo.RegistrationNumberTaken(ctx, *in.RegistrationNumber, id)
		if err != nil {
			return nil, mapError(err)
		}
		if taken {
			return nil, errDuplicateRegistration()
		}
	}

	item, err := s.repo.Update(ctx, id, in)
	if err != nil {
		return nil, mapError(err)
	}
	return item, nil
}

// Delete отправляет единицу в корзину (soft delete, EQ-6). Права ADMIN
// проверяет middleware маршрута.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.ByID(ctx, id); err != nil {
		return mapError(err)
	}
	return mapError(s.repo.SoftDelete(ctx, id))
}

// Move перемещает единицу на другой склад в транзакции (EQ-3): обновление
// склада и запись журнала выполняет репозиторий, правило — CheckMove.
func (s *Service) Move(ctx context.Context, id int64, in MoveInput, actor Actor) (*MoveResult, error) {
	item, movement, err := s.repo.Move(ctx, id, in, actor)
	if err != nil {
		return nil, mapError(err)
	}
	return &MoveResult{Equipment: item, Movement: movement}, nil
}

// Movements — журнал перемещений единицы (EQ-5): сначала проверяем, что сама
// единица существует, иначе отдаём 404 и пустой список.
func (s *Service) Movements(ctx context.Context, id int64, page, size int) (*MovementPage, error) {
	if _, err := s.repo.ByID(ctx, id); err != nil {
		return nil, mapError(err)
	}
	items, total, err := s.repo.Movements(ctx, id, page, size)
	if err != nil {
		return nil, mapError(err)
	}
	return &MovementPage{Items: items, Total: total, Page: page, Size: size}, nil
}

// requireEquipmentType отдаёт 404, если типа оборудования нет (EQ-2).
func (s *Service) requireEquipmentType(ctx context.Context, id int64) error {
	exists, err := s.repo.EquipmentTypeExists(ctx, id)
	if err != nil {
		return mapError(err)
	}
	if !exists {
		return apierr.NotFound("Тип оборудования не найден")
	}
	return nil
}

func errDuplicateRegistration() error {
	return apierr.Conflict(apierr.CodeDuplicateRegistrationNumber,
		"Регистрационный номер уже используется другим оборудованием")
}

// mapError переводит доменные ошибки в типизированные ответы API.
func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apierr.NotFound("Оборудование не найдено")
	case errors.Is(err, ErrWarehouseNotFound):
		return apierr.NotFound("Склад не найден")
	case errors.Is(err, ErrDuplicateRegistration):
		return errDuplicateRegistration()
	case errors.Is(err, ErrSameWarehouse):
		return apierr.Conflict(apierr.CodeSameWarehouse, "Оборудование уже находится в этом складе")
	case errors.Is(err, ErrBuildingInactive):
		return apierr.New(http.StatusUnprocessableEntity, apierr.CodeBuildingInactive,
			"Здание деактивировано: размещение оборудования доступно только администратору")
	default:
		return apierr.Internal(err)
	}
}

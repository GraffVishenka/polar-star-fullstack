package dictionaries

import (
	"context"
	"errors"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Service — правила справочников (Б7): имена уникальны, удаление заменено
// деактивацией, чтение активных — всем сотрудникам.
type Service struct {
	repo Repository
}

// NewService создаёт сервис поверх репозитория.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// List возвращает элементы справочника: активные или все (include_inactive).
func (s *Service) List(ctx context.Context, kind Kind, includeInactive bool) ([]Item, error) {
	items, err := s.repo.List(ctx, kind, includeInactive)
	if err != nil {
		return nil, mapError(err)
	}
	return items, nil
}

// PublicList — только активные элементы: используется формой анонимной заявки.
func (s *Service) PublicList(ctx context.Context, kind Kind) ([]Item, error) {
	return s.List(ctx, kind, false)
}

// Create добавляет элемент; дубль имени → 409 CONFLICT.
func (s *Service) Create(ctx context.Context, kind Kind, in CreateInput) (*Item, error) {
	taken, err := s.repo.NameTaken(ctx, kind, in.Name, 0)
	if err != nil {
		return nil, mapError(err)
	}
	if taken {
		return nil, errNameTaken()
	}

	item, err := s.repo.Create(ctx, kind, in)
	if err != nil {
		return nil, mapError(err)
	}
	return item, nil
}

// Update правит элемент; переименование в занятое имя → 409 CONFLICT.
func (s *Service) Update(ctx context.Context, kind Kind, id int64, in UpdateInput) (*Item, error) {
	current, err := s.repo.ByID(ctx, kind, id)
	if err != nil {
		return nil, mapError(err)
	}
	if in.Name != nil && *in.Name != current.Name {
		taken, err := s.repo.NameTaken(ctx, kind, *in.Name, id)
		if err != nil {
			return nil, mapError(err)
		}
		if taken {
			return nil, errNameTaken()
		}
	}

	item, err := s.repo.Update(ctx, kind, id, in)
	if err != nil {
		return nil, mapError(err)
	}
	return item, nil
}

// Delete удаляет элемент из активных. Физического удаления нет: справочники
// используются в исторических заявках и оборудовании, поэтому DELETE =
// деактивация (is_active=false). Повторный DELETE идемпотентен.
func (s *Service) Delete(ctx context.Context, kind Kind, id int64) error {
	if _, err := s.repo.ByID(ctx, kind, id); err != nil {
		return mapError(err)
	}
	if _, err := s.repo.SetActive(ctx, kind, id, false); err != nil {
		return mapError(err)
	}
	return nil
}

func errNameTaken() error {
	return apierr.Conflict(apierr.CodeConflict, "Элемент с таким названием уже существует")
}

// mapError переводит доменные ошибки репозитория в типизированные ошибки API.
func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apierr.NotFound("Элемент справочника не найден")
	case errors.Is(err, ErrNameTaken):
		return errNameTaken()
	default:
		return apierr.Internal(err)
	}
}

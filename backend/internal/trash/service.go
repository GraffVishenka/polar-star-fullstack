package trash

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Service — правила корзины (TRASH-1..TRASH-5). Чтение и запись выполняет
// репозиторий, домен проверяет только правила восстановления.
type Service struct {
	repo      Repository
	files     FileRemover
	clock     platform.Clock
	logger    *slog.Logger
	retention time.Duration
	batch     int
}

// Option настраивает сервис.
type Option func(*serviceOptions)

type serviceOptions struct {
	retention time.Duration
	batch     int
}

// WithRetention меняет срок хранения корзины (по умолчанию 90 дней, TRASH-4).
func WithRetention(retention time.Duration) Option {
	return func(options *serviceOptions) {
		if retention > 0 {
			options.retention = retention
		}
	}
}

// WithBatch задаёт размер пачки автоочистки.
func WithBatch(batch int) Option {
	return func(options *serviceOptions) {
		if batch > 0 {
			options.batch = batch
		}
	}
}

// NewService создаёт сервис корзины. files может быть nil — тогда файлы
// удаляются только из БД (без удаления с диска).
func NewService(repo Repository, files FileRemover, clock platform.Clock, logger *slog.Logger, opts ...Option) *Service {
	if clock == nil {
		clock = platform.RealClock{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	options := serviceOptions{retention: DefaultRetention, batch: defaultBatch}
	for _, opt := range opts {
		opt(&options)
	}
	return &Service{
		repo:      repo,
		files:     files,
		clock:     clock,
		logger:    logger,
		retention: options.retention,
		batch:     options.batch,
	}
}

// Retention — срок хранения корзины.
func (s *Service) Retention() time.Duration { return s.retention }

// List — страница корзины по типу (TRASH-1).
func (s *Service) List(ctx context.Context, kind Type, page, size int) (*Page, error) {
	if !kind.Valid() {
		return nil, apierr.Validation("Неизвестный тип элемента корзины",
			map[string]any{"type": "ожидается ticket, kb или equipment"})
	}
	items, total, err := s.repo.List(ctx, kind, page, size)
	if err != nil {
		return nil, mapError(err)
	}
	return &Page{Items: items, Total: total, Page: page, Size: size}, nil
}

// Restore возвращает элемент из корзины (TRASH-2). Предупреждения не мешают
// восстановлению: деактивированное здание допустимо, потомки БЗ не
// восстанавливаются (TRASH-5).
func (s *Service) Restore(ctx context.Context, kind Type, id int64) (*RestoreResult, error) {
	if !kind.Valid() {
		return nil, apierr.Validation("Неизвестный тип элемента корзины",
			map[string]any{"type": "ожидается ticket, kb или equipment"})
	}

	var warnings []string
	switch kind {
	case TypeTicket:
		info, err := s.repo.RestoreTicket(ctx, id)
		if err != nil {
			return nil, mapError(err)
		}
		warnings = ticketRestoreWarnings(info)
	case TypeKB:
		info, err := s.repo.RestoreKB(ctx, id)
		if err != nil {
			return nil, mapError(err)
		}
		warnings = kbRestoreWarnings(info)
	case TypeEquipment:
		info, err := s.repo.RestoreEquipment(ctx, id)
		if err != nil {
			return nil, mapError(err)
		}
		warnings = equipmentRestoreWarnings(info)
	}

	s.logger.Info("trash: элемент восстановлен", "type", kind, "id", id, "warnings", len(warnings))
	return &RestoreResult{Type: kind, ID: id, Restored: true, Warnings: warnings}, nil
}

// Purge физически удаляет элемент из корзины (TRASH-3). Действие необратимо:
// для заявки удаляются комментарии и файлы, для узла БЗ — поддерево, для
// оборудования — журнал перемещений.
func (s *Service) Purge(ctx context.Context, kind Type, id int64) error {
	if !kind.Valid() {
		return apierr.Validation("Неизвестный тип элемента корзины",
			map[string]any{"type": "ожидается ticket, kb или equipment"})
	}
	paths, err := s.purge(ctx, kind, id)
	if err != nil {
		return mapError(err)
	}
	s.removeFiles(paths)
	s.logger.Info("trash: элемент удалён окончательно", "type", kind, "id", id, "files", len(paths))
	return nil
}

// Cleanup удаляет элементы, пролежавшие в корзине дольше retention (TRASH-4).
// Возвращает число удалённых элементов; ошибки отдельных элементов собираются
// и логируются, но не прерывают очистку остальных.
func (s *Service) Cleanup(ctx context.Context) (int, error) {
	cutoff := s.clock.Now().UTC().Add(-s.retention)
	total := 0
	var errs []error

	for _, kind := range Types {
		for {
			ids, err := s.repo.Expired(ctx, kind, cutoff, s.batch)
			if err != nil {
				errs = append(errs, err)
				break
			}
			if len(ids) == 0 {
				break
			}
			for _, id := range ids {
				if ctx.Err() != nil {
					errs = append(errs, ctx.Err())
					break
				}
				paths, err := s.purge(ctx, kind, id)
				if err != nil {
					if errors.Is(err, ErrNotFound) {
						// Элемент удалили параллельно — не ошибка.
						continue
					}
					errs = append(errs, fmt.Errorf("%s #%d: %w", kind, id, err))
					continue
				}
				s.removeFiles(paths)
				total++
			}
			if len(ids) < s.batch {
				break
			}
		}
	}

	s.logger.Info("trash: автоочистка завершена",
		"deleted", total,
		"retention", s.retention.String(),
		"cutoff", cutoff.Format(time.RFC3339))
	return total, errors.Join(errs...)
}

// purge — внутреннее физическое удаление; возвращает пути файлов на диске.
func (s *Service) purge(ctx context.Context, kind Type, id int64) ([]string, error) {
	switch kind {
	case TypeTicket:
		return s.repo.PurgeTicket(ctx, id)
	case TypeKB:
		return nil, s.repo.PurgeKB(ctx, id)
	case TypeEquipment:
		return nil, s.repo.PurgeEquipment(ctx, id)
	default:
		return nil, ErrUnknownType
	}
}

// removeFiles удаляет файлы с диска; ошибки не отменяют уже выполненное
// удаление в БД, поэтому только логируются.
func (s *Service) removeFiles(paths []string) {
	if len(paths) == 0 {
		return
	}
	if s.files == nil {
		s.logger.Warn("trash: удаление файлов с диска не настроено", "files", len(paths))
		return
	}
	for _, path := range paths {
		if err := s.files.Remove(path); err != nil {
			s.logger.Error("trash: не удалось удалить файл с диска", "path", path, "error", err)
		}
	}
}

// ticketRestoreWarnings — TRASH-2: восстановление в деактивированное здание
// разрешено, но админ должен об этом знать.
func ticketRestoreWarnings(info TicketRestore) []string {
	if info.BuildingActive {
		return nil
	}
	return []string{fmt.Sprintf(
		"Здание %q деактивировано: заявка восстановлена, но здание скрыто из публичного выбора",
		info.BuildingName)}
}

// kbRestoreWarnings — TRASH-5: потомки узла не восстанавливаются.
func kbRestoreWarnings(info KBRestore) []string {
	if info.DeletedChildren <= 0 {
		return nil
	}
	return []string{fmt.Sprintf(
		"Потомки не восстановлены (%d): восстановите их отдельно", info.DeletedChildren)}
}

// equipmentRestoreWarnings — деактивированное здание склада не мешает
// восстановлению, но склад скрыт из форм.
func equipmentRestoreWarnings(info EquipmentRestore) []string {
	if info.BuildingActive {
		return nil
	}
	return []string{fmt.Sprintf(
		"Здание %q деактивировано: оборудование восстановлено, но склад скрыт из форм",
		info.BuildingName)}
}

// mapError переводит доменные ошибки в типизированные ответы API.
func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apierr.NotFound("Элемент корзины не найден")
	case errors.Is(err, ErrParentDeleted):
		return apierr.Conflict(apierr.CodeConflict,
			"Родительский раздел удалён: сначала восстановите его").
			WithDetail("reason", "parent_deleted")
	case errors.Is(err, ErrWarehouseDeleted):
		return apierr.Conflict(apierr.CodeConflict,
			"Склад оборудования удалён: восстановите склад").
			WithDetail("reason", "warehouse_deleted")
	case errors.Is(err, ErrDuplicateRegistration):
		return apierr.Conflict(apierr.CodeDuplicateRegistrationNumber,
			"Регистрационный номер уже используется другим оборудованием").
			WithDetail("reason", "duplicate_registration_number")
	case errors.Is(err, ErrUnknownType):
		return apierr.Validation("Неизвестный тип элемента корзины",
			map[string]any{"type": "ожидается ticket, kb или equipment"})
	default:
		return apierr.Internal(err)
	}
}

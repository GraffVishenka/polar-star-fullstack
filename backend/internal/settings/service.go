package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Service — настройки, календарь и праздники (CAL-1..CAL-6, TICKET-1).
// Держит атомарный снапшот календаря: чтение настроек никогда не блокирует
// запросы, а любое изменение пересобирает снапшот без рестарта (CAL-5).
type Service struct {
	repo     Repository
	provider *CalendarProvider
	clock    platform.Clock
	logger   *slog.Logger
}

// Option настраивает сервис.
type Option func(*serviceOptions)

type serviceOptions struct {
	location *time.Location
}

// WithLocation задаёт часовой пояс по умолчанию до первой загрузки из БД
// (если БД недоступна): используется TZ из окружения, но только если это
// IANA-идентификатор.
func WithLocation(location *time.Location) Option {
	return func(options *serviceOptions) { options.location = location }
}

// NewService создаёт сервис с дефолтным снапшотом. Загрузка из БД — отдельным
// вызовом Reload: ошибка чтения не должна мешать старту API, при ней остаются
// значения по умолчанию (Пн–Сб, Asia/Yekaterinburg, 10:00).
func NewService(repo Repository, clock platform.Clock, logger *slog.Logger, opts ...Option) *Service {
	if clock == nil {
		clock = platform.RealClock{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	options := serviceOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	base := Defaults()
	if options.location != nil {
		if name := options.location.String(); name != "" && name != "Local" {
			if _, err := time.LoadLocation(name); err == nil {
				base.Timezone = name
			}
		}
	}

	return &Service{
		repo:     repo,
		provider: NewCalendarProvider(clock, base, nil),
		clock:    clock,
		logger:   logger,
	}
}

// Provider возвращает провайдер календаря/настроек: он же реализует
// platform.Calendar для воркеров (Б8) и TicketSettings для заявок (Б5).
func (s *Service) Provider() *CalendarProvider { return s.provider }

// Reload перечитывает настройки и праздники и атомарно подменяет снапшот.
// Если календарь собрать не удалось, текущий снапшот остаётся прежним.
func (s *Service) Reload(ctx context.Context) error {
	current, err := s.repo.Settings(ctx)
	if err != nil {
		return fmt.Errorf("settings: загрузка настроек: %w", err)
	}
	holidays, err := s.repo.Holidays(ctx, nil)
	if err != nil {
		return fmt.Errorf("settings: загрузка праздников: %w", err)
	}
	snapshot, err := buildSnapshot(*current, holidays, s.clock)
	if err != nil {
		return err
	}
	s.provider.store(snapshot)
	s.logger.Info("settings: календарь пересобран",
		"timezone", current.Timezone,
		"workdays", current.Workdays,
		"holidays", len(holidays),
		"deadline_days", current.DeadlineDays,
		"reminder_time", current.DeadlineReminderTime)
	return nil
}

// Settings возвращает сохранённые настройки (источник истины — БД).
func (s *Service) Settings(ctx context.Context) (*Settings, error) {
	current, err := s.repo.Settings(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	return current, nil
}

// UpdateSettings сохраняет настройки и пересобирает календарь (CAL-5: новые
// заявки считаются по новым правилам, существующие дедлайны не меняются).
func (s *Service) UpdateSettings(ctx context.Context, in SettingsInput) (*Settings, error) {
	updated, err := s.repo.UpdateSettings(ctx, in)
	if err != nil {
		return nil, mapError(err)
	}
	if err := s.Reload(ctx); err != nil {
		// Настройки уже сохранены: данные в БД — источник истины, снапшот
		// обновится при следующем изменении или старте.
		s.logger.Error("settings: календарь не пересобран после изменения настроек", "error", err)
	}
	return updated, nil
}

// Holidays возвращает праздники (все или за конкретный год).
func (s *Service) Holidays(ctx context.Context, year *int) ([]Holiday, error) {
	items, err := s.repo.Holidays(ctx, year)
	if err != nil {
		return nil, mapError(err)
	}
	return items, nil
}

// Holiday — карточка праздника.
func (s *Service) Holiday(ctx context.Context, id int64) (*Holiday, error) {
	holiday, err := s.repo.HolidayByID(ctx, id)
	if err != nil {
		return nil, mapHolidayError(err)
	}
	return holiday, nil
}

// CreateHoliday добавляет праздник и пересобирает календарь (CAL-1).
func (s *Service) CreateHoliday(ctx context.Context, date time.Time) (*Holiday, error) {
	holiday, err := s.repo.CreateHoliday(ctx, date)
	if err != nil {
		return nil, mapHolidayError(err)
	}
	s.reloadAfterChange(ctx)
	return holiday, nil
}

// UpdateHoliday меняет дату праздника и пересобирает календарь.
func (s *Service) UpdateHoliday(ctx context.Context, id int64, date time.Time) (*Holiday, error) {
	holiday, err := s.repo.UpdateHoliday(ctx, id, date)
	if err != nil {
		return nil, mapHolidayError(err)
	}
	s.reloadAfterChange(ctx)
	return holiday, nil
}

// DeleteHoliday удаляет праздник и пересобирает календарь.
func (s *Service) DeleteHoliday(ctx context.Context, id int64) error {
	if err := s.repo.DeleteHoliday(ctx, id); err != nil {
		return mapHolidayError(err)
	}
	s.reloadAfterChange(ctx)
	return nil
}

// reloadAfterChange пересобирает календарь после изменения праздников.
// Ошибка не отменяет уже сохранённое изменение: снапшот восстановится при
// следующем изменении или старте API.
func (s *Service) reloadAfterChange(ctx context.Context) {
	if err := s.Reload(ctx); err != nil {
		s.logger.Error("settings: календарь не пересобран после изменения праздников", "error", err)
	}
}

func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apierr.NotFound("Запись не найдена")
	default:
		return apierr.Internal(err)
	}
}

func mapHolidayError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apierr.NotFound("Праздник не найден")
	case errors.Is(err, ErrDuplicateHoliday):
		return apierr.Conflict(apierr.CodeConflict, "Праздник с такой датой уже добавлен")
	default:
		return apierr.Internal(err)
	}
}

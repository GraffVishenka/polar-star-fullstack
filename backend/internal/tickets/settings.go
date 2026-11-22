package tickets

import (
	"context"
	"strings"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// StaticSettings — дефолтные настройки до Б7: deadline_days и календарь.
// Б7 заменит реализацию TicketSettings на настройки из БД через тот же интерфейс.
type StaticSettings struct {
	Days             int
	CalendarInstance platform.Calendar
	Labels           map[Status]string
	Colors           map[Status]string
}

// NewStaticSettings создаёт настройки по умолчанию (deadline_days=2 при Days<=0).
func NewStaticSettings(days int, calendar platform.Calendar) *StaticSettings {
	if days <= 0 {
		days = 2
	}
	return &StaticSettings{Days: days, CalendarInstance: calendar}
}

// DeadlineDays возвращает N рабочих дней (CAL-1, TICKET-3).
func (s *StaticSettings) DeadlineDays() int {
	if s == nil || s.Days <= 0 {
		return 2
	}
	return s.Days
}

// Calendar возвращает рабочий календарь (CAL-4).
func (s *StaticSettings) Calendar() platform.Calendar { return s.CalendarInstance }

// StatusStyle позволяет Б7 задать названия/цвета статусов; StaticSettings
// поддерживает это же расширение.
func (s *StaticSettings) StatusStyle(status Status) (string, string) {
	label, ok := s.Labels[status]
	if !ok {
		label = defaultStatusLabels[status]
	}
	color, ok := s.Colors[status]
	if !ok {
		color = defaultStatusColors[status]
	}
	return label, color
}

// DisabledFileAttacher — временная реализация FileAttacher до мержа Б3:
// вложения не принимаются, но заявки/комментарии без файлов работают.
type DisabledFileAttacher struct{}

// ValidateScope отклоняет любые вложения.
func (DisabledFileAttacher) ValidateScope(context.Context, []int64, string) error {
	return apierr.Validation("Вложения временно недоступны: домен файлов ещё не подключён",
		map[string]any{"file_ids": "не поддерживаются"})
}

// ValidateAndAttach отклоняет любые вложения.
func (DisabledFileAttacher) ValidateAndAttach(context.Context, []int64, int64) error {
	return apierr.Validation("Вложения временно недоступны: домен файлов ещё не подключён",
		map[string]any{"file_ids": "не поддерживаются"})
}

// dateOnlyUTC нормализует дату (год/месяц/день) в UTC-полночь — так DATE
// хранится и сравнивается независимо от пояса.
func dateOnlyUTC(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// parseDateUTC разбирает YYYY-MM-DD в UTC-полночь (дедлайн, TICKET-9).
func parseDateUTC(value string) (time.Time, error) {
	parsed, err := time.Parse(platform.DateLayout, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, err
	}
	return dateOnlyUTC(parsed), nil
}

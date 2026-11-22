package notifications

import (
	"strings"
	"time"
)

// PushSettings — источник настроек push (NOTIF-1/NOTIF-7).
//
// До Б7 используется StaticPushSettings; Б7 заменит его реализацией поверх
// таблицы settings (push_enabled, deadline_reminder_time, timezone) без
// изменений dispatcher'а и worker'а.
type PushSettings interface {
	// Enabled — глобальный флаг push_enabled: при false события не создаются.
	Enabled() bool
	// ReminderTime — локальное время ежедневной джобы в формате HH:MM.
	ReminderTime() string
	// Timezone — часовой пояс настроек (дедлайн «сегодня» считается в нём).
	Timezone() *time.Location
}

// StaticPushSettings — настройки по умолчанию до Б7.
type StaticPushSettings struct {
	enabled  bool
	reminder string
	location *time.Location
}

// NewStaticPushSettings создаёт статические настройки. Некорректное время
// заменяется на 10:00 (значение по умолчанию из ТЗ), пустой пояс — UTC.
func NewStaticPushSettings(enabled bool, reminder string, location *time.Location) *StaticPushSettings {
	reminder = strings.TrimSpace(reminder)
	if _, _, ok := ParseReminderTime(reminder); !ok {
		reminder = "10:00"
	}
	if location == nil {
		location = time.UTC
	}
	return &StaticPushSettings{enabled: enabled, reminder: reminder, location: location}
}

// Enabled реализует PushSettings.
func (s *StaticPushSettings) Enabled() bool { return s.enabled }

// ReminderTime реализует PushSettings.
func (s *StaticPushSettings) ReminderTime() string { return s.reminder }

// Timezone реализует PushSettings.
func (s *StaticPushSettings) Timezone() *time.Location { return s.location }

// ParseReminderTime разбирает HH:MM (00:00..23:59).
func ParseReminderTime(value string) (hour, minute int, ok bool) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, 0, false
	}
	return parsed.Hour(), parsed.Minute(), true
}

var _ PushSettings = (*StaticPushSettings)(nil)

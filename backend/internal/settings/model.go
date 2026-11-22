// Package settings реализует настройки приложения, рабочий календарь и
// справочник праздников (Б7, CAL-1..CAL-6, TICKET-1):
//
//   - singleton-настройки `settings` (имя приложения, срок дедлайна, маска
//     рабочих дней, часовой пояс, время напоминаний, push, названия/цвета
//     статусов) с валидацией полей;
//   - CalendarProvider — атомарный снапшот календаря/пояса: после PUT настройки
//     пересобираются без рестарта, новые заявки считают дедлайн по новым
//     правилам, существующие дедлайны не пересчитываются (CAL-5);
//   - порт TicketSettings для домена заявок (Б5);
//   - CRUD праздников (CAL-1) с пересборкой календаря после изменения.
package settings

import "time"

// StatusCodes — коды четырёх фиксированных статусов заявки (TICKET-1).
// Порядок задаёт порядок в ответе API и в документах.
var StatusCodes = []string{"new", "in_progress", "deferred", "done"}

// Ограничения полей (openapi Settings/SettingsUpdateRequest, CAL-1..CAL-3).
const (
	maxAppNameLength     = 200
	maxStatusLabelLength = 60
	minHolidayYear       = 2000
	maxHolidayYear       = 2100
)

// Статусы по умолчанию (совпадают с сидингом Б1, db/seed.go).
var defaultStatuses = []StatusSetting{
	{Code: "new", Label: "Новая", Color: "#1677ff"},
	{Code: "in_progress", Label: "В работе", Color: "#fa8c16"},
	{Code: "deferred", Label: "Отложена", Color: "#8c8c8c"},
	{Code: "done", Label: "Выполнена", Color: "#52c41a"},
}

// StatusSetting — название и цвет одного статуса заявки (TICKET-1).
type StatusSetting struct {
	Code  string
	Label string
	Color string
}

// Settings — singleton-настройки приложения (строка id=1).
type Settings struct {
	AppName              string
	DeadlineDays         int
	Workdays             []int // ISO-8601: 1 = Пн … 7 = Вс (CAL-1)
	Timezone             string
	DeadlineReminderTime string // HH:MM (CAL-3)
	PushEnabled          bool
	Statuses             []StatusSetting
	UpdatedAt            time.Time
}

// SettingsInput — провалидированное тело PUT /api/admin/settings.
type SettingsInput struct {
	AppName              string
	DeadlineDays         int
	Workdays             []int
	Timezone             string
	DeadlineReminderTime string
	PushEnabled          bool
	Statuses             []StatusSetting
}

// Holiday — праздник рабочего календаря (CAL-1). Date — календарная дата без
// времени (в БД DATE).
type Holiday struct {
	ID        int64
	Date      time.Time
	CreatedAt time.Time
}

// Defaults возвращает значения по умолчанию (CAL-1..CAL-3, TICKET-1):
// маска Пн–Сб, Asia/Yekaterinburg, напоминание 10:00, дедлайн 2 рабочих дня,
// стандартные названия и цвета статусов. Совпадают с сидингом Б1.
func Defaults() Settings {
	return Settings{
		AppName:              "Заявки · База знаний · Оборудование",
		DeadlineDays:         2,
		Workdays:             []int{1, 2, 3, 4, 5, 6},
		Timezone:             "Asia/Yekaterinburg",
		DeadlineReminderTime: "10:00",
		PushEnabled:          true,
		Statuses:             cloneStatuses(defaultStatuses),
	}
}

// cloneSettings копирует настройки вместе со срезами, чтобы снапшот и
// вызывающий код не делили изменяемую память.
func cloneSettings(in Settings) Settings {
	out := in
	out.Workdays = append([]int(nil), in.Workdays...)
	out.Statuses = cloneStatuses(in.Statuses)
	return out
}

func cloneStatuses(in []StatusSetting) []StatusSetting {
	return append([]StatusSetting(nil), in...)
}

// defaultStatus возвращает статус по умолчанию для кода (или false).
func defaultStatus(code string) (StatusSetting, bool) {
	for _, status := range defaultStatuses {
		if status.Code == code {
			return status, true
		}
	}
	return StatusSetting{}, false
}

// findStatus ищет статус по коду.
func findStatus(statuses []StatusSetting, code string) (StatusSetting, bool) {
	for _, status := range statuses {
		if status.Code == code {
			return status, true
		}
	}
	return StatusSetting{}, false
}

// isoWeekday переводит ISO-8601 (1 = Пн … 7 = Вс) в time.Weekday.
func isoWeekday(iso int) (time.Weekday, bool) {
	if iso < 1 || iso > 7 {
		return 0, false
	}
	return time.Weekday(iso % 7), true
}

// dateOnly отрезает время от даты.
func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

package settings

import (
	"testing"
	"time"
)

// validStatuses собирает корректный набор из 4 статусов.
func validStatuses() []StatusDTO {
	return []StatusDTO{
		{Code: "new", Label: "Новая", Color: "#1677ff"},
		{Code: "in_progress", Label: "В работе", Color: "#fa8c16"},
		{Code: "deferred", Label: "Отложена", Color: "#8c8c8c"},
		{Code: "done", Label: "Выполнена", Color: "#52c41a"},
	}
}

func validSettingsRequest() SettingsUpdateRequest {
	return SettingsUpdateRequest{
		AppName:              "Заявки",
		DeadlineDays:         3,
		Workdays:             []int{6, 1, 2, 3, 4, 5},
		Timezone:             "Asia/Yekaterinburg",
		DeadlineReminderTime: "09:30",
		PushEnabled:          true,
		Statuses:             validStatuses(),
	}
}

// CAL-1..CAL-3, TICKET-1: успешная валидация нормализует значения.
func TestValidateSettingsValid(t *testing.T) {
	in, details := ValidateSettings(validSettingsRequest())
	if len(details) != 0 {
		t.Fatalf("details = %v", details)
	}
	if in.AppName != "Заявки" || in.DeadlineDays != 3 || in.Timezone != "Asia/Yekaterinburg" {
		t.Fatalf("in = %+v", in)
	}
	if in.DeadlineReminderTime != "09:30" || !in.PushEnabled {
		t.Fatalf("in = %+v", in)
	}
	// Маска сортируется по возрастанию ISO-дня.
	want := []int{1, 2, 3, 4, 5, 6}
	if len(in.Workdays) != len(want) {
		t.Fatalf("workdays = %v", in.Workdays)
	}
	for i := range want {
		if in.Workdays[i] != want[i] {
			t.Fatalf("workdays = %v, want %v", in.Workdays, want)
		}
	}
	// Статусы возвращаются в фиксированном порядке кодов.
	if len(in.Statuses) != 4 || in.Statuses[0].Code != "new" || in.Statuses[3].Code != "done" {
		t.Fatalf("statuses = %+v", in.Statuses)
	}
}

// Таблица ошибок валидации: 422 и details по полям.
func TestValidateSettingsErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SettingsUpdateRequest)
		field  string
	}{
		{"пустое имя приложения", func(r *SettingsUpdateRequest) { r.AppName = "   " }, "app_name"},
		{"имя приложения 201 символ", func(r *SettingsUpdateRequest) { r.AppName = repeat("я", 201) }, "app_name"},
		{"deadline_days=0", func(r *SettingsUpdateRequest) { r.DeadlineDays = 0 }, "deadline_days"},
		{"deadline_days=31", func(r *SettingsUpdateRequest) { r.DeadlineDays = 31 }, "deadline_days"},
		{"пустая маска рабочих дней", func(r *SettingsUpdateRequest) { r.Workdays = nil }, "workdays"},
		{"день вне 1..7", func(r *SettingsUpdateRequest) { r.Workdays = []int{1, 8} }, "workdays"},
		{"день 0", func(r *SettingsUpdateRequest) { r.Workdays = []int{0, 1} }, "workdays"},
		{"повтор дня", func(r *SettingsUpdateRequest) { r.Workdays = []int{1, 1} }, "workdays"},
		{"пустой timezone", func(r *SettingsUpdateRequest) { r.Timezone = " " }, "timezone"},
		{"неизвестный timezone", func(r *SettingsUpdateRequest) { r.Timezone = "Europe/Mordor" }, "timezone"},
		{"timezone Local", func(r *SettingsUpdateRequest) { r.Timezone = "Local" }, "timezone"},
		{"время 25:00", func(r *SettingsUpdateRequest) { r.DeadlineReminderTime = "25:00" }, "deadline_reminder_time"},
		{"время без нуля", func(r *SettingsUpdateRequest) { r.DeadlineReminderTime = "9:30" }, "deadline_reminder_time"},
		{"минуты 60", func(r *SettingsUpdateRequest) { r.DeadlineReminderTime = "10:60" }, "deadline_reminder_time"},
		{"пустое время", func(r *SettingsUpdateRequest) { r.DeadlineReminderTime = "" }, "deadline_reminder_time"},
		{"три статуса", func(r *SettingsUpdateRequest) { r.Statuses = r.Statuses[:3] }, "statuses"},
		{"неизвестный код статуса", func(r *SettingsUpdateRequest) { r.Statuses[0].Code = "closed" }, "statuses"},
		{"повтор кода статуса", func(r *SettingsUpdateRequest) { r.Statuses[1].Code = "new" }, "statuses"},
		{"пустая подпись статуса", func(r *SettingsUpdateRequest) { r.Statuses[2].Label = "  " }, "statuses.deferred.label"},
		{"длинная подпись статуса", func(r *SettingsUpdateRequest) { r.Statuses[2].Label = repeat("я", 61) }, "statuses.deferred.label"},
		{"цвет без решётки", func(r *SettingsUpdateRequest) { r.Statuses[3].Color = "52c41a" }, "statuses.done.color"},
		{"короткий цвет", func(r *SettingsUpdateRequest) { r.Statuses[3].Color = "#52c41" }, "statuses.done.color"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validSettingsRequest()
			tc.mutate(&req)
			_, details := ValidateSettings(req)
			if _, ok := details[tc.field]; !ok {
				t.Fatalf("нет ошибки %q: %v", tc.field, details)
			}
		})
	}
}

// CAL-2: валидными считаются известные IANA-пояса, включая UTC.
func TestValidateSettingsTimezone(t *testing.T) {
	for _, timezone := range []string{"UTC", "Europe/Moscow", "Asia/Yekaterinburg", "America/New_York"} {
		req := validSettingsRequest()
		req.Timezone = timezone
		if _, details := ValidateSettings(req); len(details) != 0 {
			t.Fatalf("timezone %q отклонён: %v", timezone, details)
		}
	}
}

// CAL-1: дата праздника — строгий YYYY-MM-DD.
func TestValidateHolidayDate(t *testing.T) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	date, message := ValidateHolidayDate(" 2026-11-27 ", location)
	if message != "" {
		t.Fatalf("message = %q", message)
	}
	if date.Year() != 2026 || date.Month() != time.November || date.Day() != 27 {
		t.Fatalf("date = %v", date)
	}
	if date.Location() != location {
		t.Fatalf("date должен быть в поясе приложения: %v", date.Location())
	}

	for _, raw := range []string{"", "   ", "27.11.2026", "2026-13-01", "2026-11-31", "2026-1-1"} {
		if _, message := ValidateHolidayDate(raw, location); message == "" {
			t.Fatalf("дата %q должна отклоняться", raw)
		}
	}
}

// GET /api/admin/holidays?year= — 2000..2100.
func TestValidateHolidayYear(t *testing.T) {
	if year, ok := validateHolidayYear(""); !ok || year != nil {
		t.Fatalf("пустой год: %v %v", year, ok)
	}
	if year, ok := validateHolidayYear("2026"); !ok || year == nil || *year != 2026 {
		t.Fatalf("2026: %v %v", year, ok)
	}
	for _, raw := range []string{"1999", "2101", "abc", "0"} {
		if _, ok := validateHolidayYear(raw); ok {
			t.Fatalf("год %q должен отклоняться", raw)
		}
	}
}

func repeat(value string, count int) string {
	out := ""
	for i := 0; i < count; i++ {
		out += value
	}
	return out
}

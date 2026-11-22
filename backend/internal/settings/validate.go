package settings

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	// Встроенная база IANA-поясов: проверка timezone (CAL-2) не должна зависеть
	// от наличия tzdata в окружении (в т.ч. в wasm-тестах).
	_ "time/tzdata"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

var (
	// reminderPattern — CAL-3: HH:MM строго в диапазоне 00:00–23:59.
	reminderPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	// colorPattern — HEX-цвет статуса (openapi StatusSetting).
	colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// ValidateSettings проверяет тело PUT /api/admin/settings (CAL-1..CAL-3,
// TICKET-1). Возвращает провалидированный вход и ошибки по полям (пусто — ок).
func ValidateSettings(req SettingsUpdateRequest) (SettingsInput, map[string]string) {
	details := map[string]string{}
	var in SettingsInput

	appName := strings.TrimSpace(req.AppName)
	switch {
	case appName == "":
		details["app_name"] = "обязательное поле"
	case utf8.RuneCountInString(appName) > maxAppNameLength:
		details["app_name"] = "не более 200 символов"
	default:
		in.AppName = appName
	}

	if req.DeadlineDays < 1 || req.DeadlineDays > 30 {
		details["deadline_days"] = "ожидается целое число от 1 до 30"
	} else {
		in.DeadlineDays = req.DeadlineDays
	}

	workdays, workdaysError := normalizeWorkdays(req.Workdays)
	if workdaysError != "" {
		details["workdays"] = workdaysError
	} else {
		in.Workdays = workdays
	}

	timezone := strings.TrimSpace(req.Timezone)
	switch {
	case timezone == "":
		details["timezone"] = "обязательное поле"
	case timezone == "Local":
		// time.LoadLocation("Local") успешен, но это не IANA-идентификатор.
		details["timezone"] = "неизвестный часовой пояс (ожидается IANA-идентификатор)"
	default:
		if _, err := time.LoadLocation(timezone); err != nil {
			details["timezone"] = "неизвестный часовой пояс (ожидается IANA-идентификатор)"
		} else {
			in.Timezone = timezone
		}
	}

	reminder := strings.TrimSpace(req.DeadlineReminderTime)
	if !reminderPattern.MatchString(reminder) {
		details["deadline_reminder_time"] = "ожидается время в формате HH:MM (00:00–23:59)"
	} else {
		in.DeadlineReminderTime = reminder
	}

	statuses, statusDetails := validateStatuses(req.Statuses)
	for field, message := range statusDetails {
		details[field] = message
	}
	in.Statuses = statuses
	in.PushEnabled = req.PushEnabled

	return in, details
}

// normalizeWorkdays проверяет маску рабочих дней: непустое подмножество 1..7
// без повторов; результат сортируется по возрастанию (CAL-1).
func normalizeWorkdays(values []int) ([]int, string) {
	if len(values) == 0 {
		return nil, "нужен хотя бы один рабочий день"
	}
	seen := make([]bool, 8)
	normalized := make([]int, 0, len(values))
	for _, value := range values {
		if value < 1 || value > 7 {
			return nil, "дни недели задаются числами 1..7 (1 = Пн … 7 = Вс)"
		}
		if seen[value] {
			return nil, "дни недели не должны повторяться"
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	sort.Ints(normalized)
	return normalized, ""
}

// validateStatuses проверяет 4 статуса: коды фиксированы, подписи и HEX-цвета
// обязательны (TICKET-1).
func validateStatuses(values []StatusDTO) ([]StatusSetting, map[string]string) {
	details := map[string]string{}
	if len(values) != len(StatusCodes) {
		details["statuses"] = "нужны ровно 4 статуса: new, in_progress, deferred, done"
		return nil, details
	}

	byCode := make(map[string]StatusDTO, len(values))
	for _, value := range values {
		if _, ok := defaultStatus(value.Code); !ok {
			details["statuses"] = "неизвестный код статуса: " + value.Code
			return nil, details
		}
		if _, duplicate := byCode[value.Code]; duplicate {
			details["statuses"] = "код статуса повторяется: " + value.Code
			return nil, details
		}
		byCode[value.Code] = value
	}

	result := make([]StatusSetting, 0, len(StatusCodes))
	for _, code := range StatusCodes {
		value := byCode[code]
		label := strings.TrimSpace(value.Label)
		switch {
		case label == "":
			details["statuses."+code+".label"] = "обязательное поле"
		case utf8.RuneCountInString(label) > maxStatusLabelLength:
			details["statuses."+code+".label"] = "не более 60 символов"
		}
		color := strings.TrimSpace(value.Color)
		if !colorPattern.MatchString(color) {
			details["statuses."+code+".color"] = "ожидается HEX-цвет вида #1677ff"
		}
		result = append(result, StatusSetting{Code: code, Label: label, Color: color})
	}
	return result, details
}

// ValidateHolidayDate разбирает дату праздника (YYYY-MM-DD) в поясе приложения
// (CAL-1). Возвращает дату и текст ошибки (пусто — ок).
func ValidateHolidayDate(raw string, location *time.Location) (time.Time, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, "обязательное поле"
	}
	if location == nil {
		location = time.UTC
	}
	parsed, err := time.ParseInLocation(platform.DateLayout, value, location)
	if err != nil {
		return time.Time{}, "ожидается дата в формате YYYY-MM-DD"
	}
	return parsed, ""
}

// validateHolidayYear проверяет фильтр `?year=` (openapi: 2000..2100).
func validateHolidayYear(raw string) (*int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, true
	}
	year, err := strconv.Atoi(value)
	if err != nil || year < minHolidayYear || year > maxHolidayYear {
		return nil, false
	}
	return &year, true
}

// parseReminder разбирает HH:MM в часы и минуты.
func parseReminder(value string) (int, int, error) {
	if !reminderPattern.MatchString(value) {
		return 0, 0, fmt.Errorf("settings: некорректное время напоминания %q", value)
	}
	hour, err := strconv.Atoi(value[:2])
	if err != nil {
		return 0, 0, fmt.Errorf("settings: некорректное время напоминания %q: %w", value, err)
	}
	minute, err := strconv.Atoi(value[3:])
	if err != nil {
		return 0, 0, fmt.Errorf("settings: некорректное время напоминания %q: %w", value, err)
	}
	return hour, minute, nil
}

// fieldDetails приводит ошибки валидации к виду details в ответе API.
func fieldDetails(details map[string]string) map[string]any {
	out := make(map[string]any, len(details))
	for field, message := range details {
		out[field] = message
	}
	return out
}

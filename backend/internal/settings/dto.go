package settings

import (
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// StatusDTO — название и цвет статуса в API (openapi StatusSetting).
type StatusDTO struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Color string `json:"color"`
}

// SettingsDTO — ответ GET/PUT /api/admin/settings (openapi Settings).
type SettingsDTO struct {
	AppName              string      `json:"app_name"`
	DeadlineDays         int         `json:"deadline_days"`
	Workdays             []int       `json:"workdays"`
	Timezone             string      `json:"timezone"`
	DeadlineReminderTime string      `json:"deadline_reminder_time"`
	PushEnabled          bool        `json:"push_enabled"`
	Statuses             []StatusDTO `json:"statuses"`
}

// PublicSettingsDTO — безопасное подмножество настроек для публичного
// дашборда: имя приложения и названия/цвета статусов (TICKET-1). Календарь,
// часовой пояс, push и прочие настройки наружу не отдаются.
type PublicSettingsDTO struct {
	AppName  string      `json:"app_name"`
	Statuses []StatusDTO `json:"statuses"`
}

// SettingsUpdateRequest — тело PUT /api/admin/settings (все поля обязательны).
type SettingsUpdateRequest struct {
	AppName              string      `json:"app_name"`
	DeadlineDays         int         `json:"deadline_days"`
	Workdays             []int       `json:"workdays"`
	Timezone             string      `json:"timezone"`
	DeadlineReminderTime string      `json:"deadline_reminder_time"`
	PushEnabled          bool        `json:"push_enabled"`
	Statuses             []StatusDTO `json:"statuses"`
}

// HolidayDTO — праздник (openapi Holiday). Дата — календарная, без времени.
type HolidayDTO struct {
	ID        int64     `json:"id"`
	Date      string    `json:"date"`
	CreatedAt time.Time `json:"created_at"`
}

// HolidayRequest — тело POST/PATCH /api/admin/holidays.
type HolidayRequest struct {
	Date string `json:"date"`
}

// NewSettingsDTO конвертирует настройки в DTO.
func NewSettingsDTO(in *Settings) SettingsDTO {
	if in == nil {
		return SettingsDTO{}
	}
	return SettingsDTO{
		AppName:              in.AppName,
		DeadlineDays:         in.DeadlineDays,
		Workdays:             append([]int(nil), in.Workdays...),
		Timezone:             in.Timezone,
		DeadlineReminderTime: in.DeadlineReminderTime,
		PushEnabled:          in.PushEnabled,
		Statuses:             NewStatusDTOs(in.Statuses),
	}
}

// NewStatusDTOs конвертирует статусы (всегда не nil — в JSON будет []).
func NewStatusDTOs(in []StatusSetting) []StatusDTO {
	out := make([]StatusDTO, 0, len(in))
	for _, status := range in {
		out = append(out, StatusDTO{Code: status.Code, Label: status.Label, Color: status.Color})
	}
	return out
}

// NewPublicSettingsDTO собирает публичное подмножество настроек.
func NewPublicSettingsDTO(in Settings) PublicSettingsDTO {
	return PublicSettingsDTO{AppName: in.AppName, Statuses: NewStatusDTOs(in.Statuses)}
}

// NewHolidayDTO конвертирует праздник в DTO.
func NewHolidayDTO(in *Holiday) HolidayDTO {
	if in == nil {
		return HolidayDTO{}
	}
	return HolidayDTO{
		ID:        in.ID,
		Date:      in.Date.Format(platform.DateLayout),
		CreatedAt: in.CreatedAt,
	}
}

// NewHolidayDTOs конвертирует список праздников (всегда не nil).
func NewHolidayDTOs(in []Holiday) []HolidayDTO {
	out := make([]HolidayDTO, 0, len(in))
	for i := range in {
		out = append(out, NewHolidayDTO(&in[i]))
	}
	return out
}

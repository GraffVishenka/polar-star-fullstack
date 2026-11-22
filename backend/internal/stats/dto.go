package stats

import (
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// StatsPeriodDTO — период запроса (контракт StatsPeriod): даты `YYYY-MM-DD` в
// поясе приложения, null — граница не задана.
type StatsPeriodDTO struct {
	From *string `json:"from"`
	To   *string `json:"to"`
}

// ProblemTypeStatsDTO — разбивка по виду проблемы (контракт StatsProblemType).
type ProblemTypeStatsDTO struct {
	ProblemTypeID *int64 `json:"problem_type_id"`
	Name          string `json:"name"`
	Count         int    `json:"count"`
}

// MonthStatsDTO — разбивка по месяцу (контракт StatsMonth).
type MonthStatsDTO struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// StatsResponse — метрики сотрудника (контракт StatsResponse).
type StatsResponse struct {
	UserID                int64                 `json:"user_id"`
	Login                 string                `json:"login"`
	FullName              string                `json:"full_name"`
	BuildingID            *int64                `json:"building_id"`
	BuildingName          *string               `json:"building_name"`
	Period                StatsPeriodDTO        `json:"period"`
	TotalDone             int                   `json:"total_done"`
	DoneInPeriod          int                   `json:"done_in_period"`
	AvgResolutionHours    *float64              `json:"avg_resolution_hours"`
	MedianResolutionHours *float64              `json:"median_resolution_hours"`
	OnTimePercent         *float64              `json:"on_time_percent"`
	OverdueCount          int                   `json:"overdue_count"`
	ByProblemType         []ProblemTypeStatsDTO `json:"by_problem_type"`
	ByMonth               []MonthStatsDTO       `json:"by_month"`
}

// StatsListResponse — сводная статистика по сотрудникам (контракт
// StatsListResponse).
type StatsListResponse struct {
	Items []StatsResponse `json:"items"`
}

// NewStatsDTO конвертирует доменную модель в DTO. Массивы всегда непустые по
// типу (пустой массив, а не null), чтобы клиент не проверял nil.
func NewStatsDTO(stats *Stats, loc *time.Location) StatsResponse {
	if stats == nil {
		return StatsResponse{ByProblemType: []ProblemTypeStatsDTO{}, ByMonth: []MonthStatsDTO{}}
	}
	response := StatsResponse{
		UserID:                stats.ID,
		Login:                 stats.Login,
		FullName:              stats.FullName,
		BuildingID:            stats.BuildingID,
		BuildingName:          stats.BuildingName,
		Period:                newStatsPeriodDTO(stats.Period, loc),
		TotalDone:             stats.TotalDone,
		DoneInPeriod:          stats.DoneInPeriod,
		AvgResolutionHours:    stats.AvgResolutionHours,
		MedianResolutionHours: stats.MedianResolutionHours,
		OnTimePercent:         stats.OnTimePercent,
		OverdueCount:          stats.OverdueCount,
		ByProblemType:         make([]ProblemTypeStatsDTO, 0, len(stats.ByProblemType)),
		ByMonth:               make([]MonthStatsDTO, 0, len(stats.ByMonth)),
	}
	for _, item := range stats.ByProblemType {
		response.ByProblemType = append(response.ByProblemType, ProblemTypeStatsDTO{
			ProblemTypeID: item.ProblemTypeID,
			Name:          item.Name,
			Count:         item.Count,
		})
	}
	for _, item := range stats.ByMonth {
		response.ByMonth = append(response.ByMonth, MonthStatsDTO{Month: item.Month, Count: item.Count})
	}
	return response
}

// NewStatsListDTO конвертирует список сотрудников (items всегда массив).
func NewStatsListDTO(items []*Stats, loc *time.Location) StatsListResponse {
	response := StatsListResponse{Items: make([]StatsResponse, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, NewStatsDTO(item, loc))
	}
	return response
}

// newStatsPeriodDTO форматирует границы периода в даты пояса приложения.
func newStatsPeriodDTO(period Period, loc *time.Location) StatsPeriodDTO {
	if loc == nil {
		loc = time.UTC
	}
	dto := StatsPeriodDTO{}
	if period.From != nil {
		value := period.From.In(loc).Format(platform.DateLayout)
		dto.From = &value
	}
	if period.To != nil {
		value := period.To.In(loc).Format(platform.DateLayout)
		dto.To = &value
	}
	return dto
}

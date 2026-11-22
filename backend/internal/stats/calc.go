package stats

import (
	"math"
	"sort"
	"time"
)

// monthLayout — ключ месяца в разбивке by_month (контракт StatsMonth).
const monthLayout = "2006-01"

// ComputeMetrics считает метрики по завершённым заявкам периода (STATS-2).
//
//   - время выполнения — completed_at − created_at в часах, округление до 0.1;
//   - среднее и медиана считаются по всем заявкам периода; на чётном числе
//     медиана — среднее двух центральных значений;
//   - «в срок» — календарная дата completed_at в поясе приложения не позже даты
//     дедлайна; заявки без дедлайна в знаменатель on_time_percent не входят;
//   - пустой набор даёт 0 в счётчиках и null в avg/median/on_time.
func ComputeMetrics(tickets []CompletedTicket, loc *time.Location) Metrics {
	metrics := Metrics{
		DoneInPeriod:  len(tickets),
		ByProblemType: []ProblemTypeCount{},
		ByMonth:       []MonthCount{},
	}
	if len(tickets) == 0 {
		return metrics
	}
	if loc == nil {
		loc = time.UTC
	}

	durations := make([]float64, 0, len(tickets))
	var totalHours float64
	onTimeCount, withDeadline := 0, 0
	problemCounts := make(map[problemKey]int, 8)
	monthCounts := make(map[string]int, 8)

	for _, ticket := range tickets {
		hours := ticket.CompletedAt.Sub(ticket.CreatedAt).Hours()
		if hours < 0 {
			// Аномалия данных: завершение раньше создания не должно давать
			// отрицательное время выполнения.
			hours = 0
		}
		durations = append(durations, hours)
		totalHours += hours

		if onTime, hasDeadline := OnTimeStatus(ticket, loc); hasDeadline {
			withDeadline++
			if onTime {
				onTimeCount++
			} else {
				metrics.OverdueCount++
			}
		}

		problemCounts[problemKeyOf(ticket)]++
		monthCounts[ticket.CompletedAt.In(loc).Format(monthLayout)]++
	}

	average := roundTenths(totalHours / float64(len(durations)))
	metrics.AvgResolutionHours = &average
	median := roundTenths(medianValue(durations))
	metrics.MedianResolutionHours = &median

	if withDeadline > 0 {
		percent := roundTenths(100 * float64(onTimeCount) / float64(withDeadline))
		metrics.OnTimePercent = &percent
	}

	metrics.ByProblemType = sortedProblemTypes(problemCounts)
	metrics.ByMonth = sortedMonths(monthCounts)
	return metrics
}

// problemKey — ключ группировки по виду проблемы. ID=0 означает отсутствие id
// (на практике не встречается: tickets.problem_type_id NOT NULL).
type problemKey struct {
	ID   int64
	Name string
}

// problemKeyOf нормализует вид проблемы заявки.
func problemKeyOf(ticket CompletedTicket) problemKey {
	key := problemKey{Name: ticket.ProblemTypeName}
	if ticket.ProblemTypeID != nil {
		key.ID = *ticket.ProblemTypeID
	}
	return key
}

// sortedProblemTypes отдаёт разбивку по убыванию количества, затем по названию.
func sortedProblemTypes(counts map[problemKey]int) []ProblemTypeCount {
	out := make([]ProblemTypeCount, 0, len(counts))
	for key, count := range counts {
		item := ProblemTypeCount{Name: key.Name, Count: count}
		if key.ID != 0 {
			id := key.ID
			item.ProblemTypeID = &id
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return problemID(out[i]) < problemID(out[j])
	})
	return out
}

// problemID возвращает id вида проблемы (0 для nil) для стабильной сортировки.
func problemID(item ProblemTypeCount) int64 {
	if item.ProblemTypeID == nil {
		return 0
	}
	return *item.ProblemTypeID
}

// sortedMonths отдаёт разбивку по месяцам в хронологическом порядке.
func sortedMonths(counts map[string]int) []MonthCount {
	out := make([]MonthCount, 0, len(counts))
	for month, count := range counts {
		out = append(out, MonthCount{Month: month, Count: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out
}

// medianValue считает медиану набора: на нечётном количестве — центральное
// значение, на чётном — среднее двух центральных (STATS-2).
func medianValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// OnTimeStatus сообщает, выполнена ли заявка в срок, и был ли у неё дедлайн.
// Сравниваются календарные даты: completed_at в поясе приложения и deadline
// (DATE, хранится как полночь UTC — берём его календарную дату как есть).
func OnTimeStatus(ticket CompletedTicket, loc *time.Location) (onTime bool, hasDeadline bool) {
	if ticket.Deadline == nil {
		return false, false
	}
	if loc == nil {
		loc = time.UTC
	}
	completedYear, completedMonth, completedDay := ticket.CompletedAt.In(loc).Date()
	deadlineYear, deadlineMonth, deadlineDay := ticket.Deadline.UTC().Date()
	completedDate := time.Date(completedYear, completedMonth, completedDay, 0, 0, 0, 0, time.UTC)
	deadlineDate := time.Date(deadlineYear, deadlineMonth, deadlineDay, 0, 0, 0, 0, time.UTC)
	return !completedDate.After(deadlineDate), true
}

// roundTenths округляет до 0.1 (STATS-2: время выполнения в часах до 0.1).
func roundTenths(value float64) float64 {
	return math.Round(value*10) / 10
}

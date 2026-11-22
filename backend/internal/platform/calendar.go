package platform

import (
	"fmt"
	"time"
)

// DateLayout — формат дат (DATE) в API и документах. Даты всегда передаются
// в часовом поясе приложения (CAL-2, NFR-7).
const DateLayout = "2006-01-02"

// DateTimeLayout — формат дат-времени в API: ISO 8601 с offset.
const DateTimeLayout = time.RFC3339

// Calendar — расчёт рабочих дней и границ суток в часовом поясе приложения.
// Домены заявок и напоминаний работают только через этот интерфейс
// (PLAN §6.1, правила для агентов).
type Calendar interface {
	// WorkingDays возвращает дату через days рабочих дней после from (CAL-3, CAL-4).
	WorkingDays(from time.Time, days int) time.Time
	// Today — текущая дата (начало суток) в часовом поясе приложения.
	Today() time.Time
	// FormatDate форматирует момент как дату в часовом поясе приложения.
	FormatDate(t time.Time) string
	// Location — часовой пояс приложения.
	Location() *time.Location
}

// DefaultWorkdays — маска рабочих дней по умолчанию: Пн–Сб (CAL-1).
func DefaultWorkdays() []time.Weekday {
	return []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday}
}

// DefaultCalendar — эталонная реализация Calendar по маске рабочих дней,
// списку праздников и часовому поясу.
type DefaultCalendar struct {
	loc      *time.Location
	workdays [7]bool
	holidays map[string]struct{}
	clock    Clock
}

// NewDefaultCalendar создаёт календарь. Нулевые аргументы заменяются
// значениями по умолчанию: UTC-пояс, Пн–Сб, реальные часы.
func NewDefaultCalendar(loc *time.Location, workdays []time.Weekday, holidays []time.Time, clock Clock) *DefaultCalendar {
	if loc == nil {
		loc = time.UTC
	}
	if clock == nil {
		clock = RealClock{}
	}
	if len(workdays) == 0 {
		workdays = DefaultWorkdays()
	}

	c := &DefaultCalendar{
		loc:      loc,
		holidays: make(map[string]struct{}, len(holidays)),
		clock:    clock,
	}
	for _, d := range workdays {
		if d >= time.Sunday && d <= time.Saturday {
			c.workdays[d] = true
		}
	}
	for _, h := range holidays {
		c.holidays[h.In(loc).Format(DateLayout)] = struct{}{}
	}
	return c
}

// Location возвращает часовой пояс приложения.
func (c *DefaultCalendar) Location() *time.Location { return c.loc }

// Today возвращает текущую дату (00:00 в поясе приложения).
func (c *DefaultCalendar) Today() time.Time {
	return dateOnly(c.clock.Now().In(c.loc))
}

// FormatDate форматирует момент как YYYY-MM-DD в поясе приложения.
func (c *DefaultCalendar) FormatDate(t time.Time) string {
	return t.In(c.loc).Format(DateLayout)
}

// IsWorkingDay сообщает, является ли дата рабочим днём: день входит в маску и
// не указан в праздниках (CAL-4).
func (c *DefaultCalendar) IsWorkingDay(d time.Time) bool {
	d = d.In(c.loc)
	if !c.workdays[d.Weekday()] {
		return false
	}
	_, isHoliday := c.holidays[d.Format(DateLayout)]
	return !isHoliday
}

// WorkingDays считает N рабочих дней.

// CAL-4: отсчёт начинается со следующего календарного дня после даты создания;
// рабочий день — день из маски `workdays`, отсутствующий в `holidays`.
// Суббота/воскресенье и праздники пропускаются, но не увеличивают счётчик.
//
// Если days <= 0, возвращается дата from (защита от некорректных настроек;
// валидация 1..30 выполняется в сервисе настроек — Б7).
func (c *DefaultCalendar) WorkingDays(from time.Time, days int) time.Time {
	cur := dateOnly(from.In(c.loc))
	if days <= 0 {
		return cur
	}
	count := 0
	// Ограничение на количество итераций защищает от «пустой» маски рабочих
	// дней (иначе цикл никогда не завершится). 7 дней * 366 + праздники.
	for i := 0; i < 8000; i++ {
		cur = cur.AddDate(0, 0, 1)
		if c.IsWorkingDay(cur) {
			count++
			if count == days {
				return cur
			}
		}
	}
	// Теоретически недостижимо при валидной маске; возвращаем консервативный
	// результат вместо зависания.
	return cur
}

// String реализует fmt.Stringer для логов.
func (c *DefaultCalendar) String() string {
	mask := make([]int, 0, 7)
	for i := time.Sunday; i <= time.Saturday; i++ {
		if c.workdays[i] {
			mask = append(mask, int(i))
		}
	}
	return fmt.Sprintf("calendar(tz=%s, workdays=%v, holidays=%d)", c.loc, mask, len(c.holidays))
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

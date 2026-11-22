package settings

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// TicketSettings — порт настроек для домена заявок (Б5): сколько рабочих дней
// прибавлять к дедлайну, календарь (границы суток, праздники) и подписи/цвета
// статусов (TICKET-1, TICKET-3). Реализуется *CalendarProvider: значения
// всегда берутся из актуального снапшота, при недоступной БД — дефолты.
type TicketSettings interface {
	DeadlineDays() int
	Calendar() platform.Calendar
	StatusLabel(code string) string
	StatusColor(code string) string
}

// Snapshot — неизменяемый согласованный набор настроек, календаря и пояса.
// Меняется только целиком (atomic.Pointer), поэтому запрос не увидит
// «полуприменённые» настройки.
type Snapshot struct {
	settings       Settings
	calendar       *platform.DefaultCalendar
	location       *time.Location
	reminderHour   int
	reminderMinute int
}

// CalendarProvider хранит снапшот настроек/календаря и подменяет его атомарно
// (CAL-5: после PUT календарь пересобирается без рестарта; новые заявки
// считаются по новым правилам, существующие дедлайны не пересчитываются).
//
// Реализует platform.Calendar и TicketSettings, поэтому подключается и как
// календарь воркеров (Б8), и как настройки заявок (Б5).
type CalendarProvider struct {
	clock platform.Clock
	snap  atomic.Pointer[Snapshot]
}

var (
	_ platform.Calendar = (*CalendarProvider)(nil)
	_ TicketSettings    = (*CalendarProvider)(nil)
)

// NewCalendarProvider создаёт провайдер с начальным снапшотом. Испорченные
// настройки (например, неизвестный часовой пояс в БД) не должны мешать
// старту — в этом случае берутся значения по умолчанию.
func NewCalendarProvider(clock platform.Clock, base Settings, holidays []Holiday) *CalendarProvider {
	if clock == nil {
		clock = platform.RealClock{}
	}
	provider := &CalendarProvider{clock: clock}
	snapshot, err := buildSnapshot(base, holidays, clock)
	if err != nil {
		snapshot, _ = buildSnapshot(Defaults(), nil, clock)
	}
	provider.snap.Store(snapshot)
	return provider
}

// buildSnapshot собирает календарь по настройкам и списку праздников.
func buildSnapshot(base Settings, holidays []Holiday, clock platform.Clock) (*Snapshot, error) {
	location, err := time.LoadLocation(base.Timezone)
	if err != nil {
		return nil, fmt.Errorf("settings: неизвестный часовой пояс %q: %w", base.Timezone, err)
	}

	workdays := make([]time.Weekday, 0, len(base.Workdays))
	for _, iso := range base.Workdays {
		if weekday, ok := isoWeekday(iso); ok {
			workdays = append(workdays, weekday)
		}
	}

	// Праздники — календарные даты: переносим Y/M/D в пояс приложения, чтобы
	// смена timezone не сдвигала даты (CAL-2, CAL-5).
	holidayDates := make([]time.Time, 0, len(holidays))
	for _, holiday := range holidays {
		year, month, day := holiday.Date.Date()
		holidayDates = append(holidayDates, time.Date(year, month, day, 0, 0, 0, 0, location))
	}

	hour, minute, err := parseReminder(base.DeadlineReminderTime)
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		settings:       cloneSettings(base),
		calendar:       platform.NewDefaultCalendar(location, workdays, holidayDates, clock),
		location:       location,
		reminderHour:   hour,
		reminderMinute: minute,
	}, nil
}

// store подменяет снапшот целиком.
func (p *CalendarProvider) store(snapshot *Snapshot) {
	if snapshot != nil {
		p.snap.Store(snapshot)
	}
}

// current возвращает текущий снапшот; нулевой невозможен, но защищаемся от
// паники в нестандартной инициализации.
func (p *CalendarProvider) current() *Snapshot {
	if snapshot := p.snap.Load(); snapshot != nil {
		return snapshot
	}
	snapshot, _ := buildSnapshot(Defaults(), nil, p.clock)
	p.snap.Store(snapshot)
	return snapshot
}

// Settings возвращает копию текущих настроек.
func (p *CalendarProvider) Settings() Settings {
	return cloneSettings(p.current().settings)
}

// DeadlineDays — срок дедлайна в рабочих днях (TICKET-3).
func (p *CalendarProvider) DeadlineDays() int {
	return p.current().settings.DeadlineDays
}

// Calendar реализует TicketSettings: календарь с актуальными маской и
// праздниками. Возвращается сам провайдер — каждый вызов читает снапшот
// атомарно, поэтому объект всегда актуален.
func (p *CalendarProvider) Calendar() platform.Calendar { return p }

// StatusLabel — название статуса для code (TICKET-1); пустая строка, если код
// неизвестен.
func (p *CalendarProvider) StatusLabel(code string) string {
	status, ok := findStatus(p.current().settings.Statuses, code)
	if !ok {
		return ""
	}
	return status.Label
}

// StatusColor — цвет статуса для code (TICKET-1).
func (p *CalendarProvider) StatusColor(code string) string {
	status, ok := findStatus(p.current().settings.Statuses, code)
	if !ok {
		return ""
	}
	return status.Color
}

// Today — текущая дата (начало суток) в поясе приложения (CAL-2).
func (p *CalendarProvider) Today() time.Time {
	return p.current().calendar.Today()
}

// WorkingDays — дата через days рабочих дней после from (CAL-4).
func (p *CalendarProvider) WorkingDays(from time.Time, days int) time.Time {
	return p.current().calendar.WorkingDays(from, days)
}

// FormatDate форматирует момент как дату в поясе приложения.
func (p *CalendarProvider) FormatDate(value time.Time) string {
	return p.current().calendar.FormatDate(value)
}

// Location — часовой пояс приложения.
func (p *CalendarProvider) Location() *time.Location {
	return p.current().location
}

// IsWorkingDay — рабочий ли день (маска CAL-1 минус праздники).
func (p *CalendarProvider) IsWorkingDay(day time.Time) bool {
	return p.current().calendar.IsWorkingDay(day)
}

// ReminderAt возвращает ближайший момент deadline_reminder_time в поясе
// приложения (CAL-3): сегодня, если время ещё не наступило, иначе завтра.
// Б8 использует его для планировщика ежедневных напоминаний.
func (p *CalendarProvider) ReminderAt() time.Time {
	snapshot := p.current()
	now := p.clock.Now().In(snapshot.location)
	next := time.Date(now.Year(), now.Month(), now.Day(),
		snapshot.reminderHour, snapshot.reminderMinute, 0, 0, snapshot.location)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// ReminderTime — настроенное время напоминаний в формате HH:MM (CAL-3).
func (p *CalendarProvider) ReminderTime() string {
	return p.current().settings.DeadlineReminderTime
}

// Timezone — IANA-идентификатор активного пояса (CAL-2).
func (p *CalendarProvider) Timezone() string {
	return p.current().settings.Timezone
}

// PushEnabled — глобальный тумблер push (NOTIF-6): при false события в outbox
// не создаются.
func (p *CalendarProvider) PushEnabled() bool {
	return p.current().settings.PushEnabled
}

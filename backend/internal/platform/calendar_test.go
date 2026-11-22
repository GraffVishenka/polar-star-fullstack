package platform_test

import (
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

const layout = "2006-01-02 15:04"

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		t.Fatalf("LoadLocation(Asia/Yekaterinburg): %v", err)
	}
	parsed, err := time.ParseInLocation(layout, value, loc)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

func yekaterinburg(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	return loc
}

// CAL-6: пятница + 2 рабочих дня при маске Пн–Сб.
//
// ВНИМАНИЕ: в PLAN (§3.4 CAL-6 и §6.1 Б1) для этого случая указан вторник, но
// это противоречит нормативному правилу CAL-4 («отсчёт N рабочих дней начинается
// со следующего календарного дня», рабочий день = маска минус праздники):
// следующий день после пятницы — суббота (рабочая) = 1-й день, воскресенье
// пропускается, понедельник = 2-й день. Получается понедельник.
// Оба примера из CAL-6 согласованы между собой только при взаимной перестановке
// ответов (пятница→«вторник» и четверг с праздником→«понедельник»), поэтому
// реализация следует CAL-4, а расхождение зафиксировано в отчёте для lead.
func TestDefaultCalendar_WorkingDays_MondayToSaturday(t *testing.T) {
	loc := yekaterinburg(t)
	cal := platform.NewDefaultCalendar(loc, platform.DefaultWorkdays(), nil, platform.FixedClock{})

	creation := mustTime(t, "2026-11-20 10:00") // пятница
	deadline := cal.WorkingDays(creation, 2)
	if got, want := cal.FormatDate(deadline), "2026-11-23"; got != want {
		t.Fatalf("пятница + 2 (Пн–Сб): got %s, want %s (CAL-4: сб=1, пн=2)", got, want)
	}
}

// CAL-6: четверг + 2 при Пн–Пт, праздник в пятницу. CAL-4: пятница — праздник,
// сб/вс — не рабочие, понедельник = 1-й, вторник = 2-й рабочий день.
func TestDefaultCalendar_WorkingDays_Holiday(t *testing.T) {
	loc := yekaterinburg(t)
	holidays := []time.Time{mustTime(t, "2026-11-20 00:00")} // пятница
	workdays := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	cal := platform.NewDefaultCalendar(loc, workdays, holidays, platform.FixedClock{})

	creation := mustTime(t, "2026-11-19 10:00") // четверг
	deadline := cal.WorkingDays(creation, 2)
	if got, want := cal.FormatDate(deadline), "2026-11-24"; got != want {
		t.Fatalf("четверг + 2 (Пн–Пт, праздник в пятницу): got %s, want %s", got, want)
	}
}

// Проверка, что праздник реально пропускается и не увеличивает счётчик:
// четверг + 1 при празднике в пятницу = понедельник.
func TestDefaultCalendar_HolidaySkipped(t *testing.T) {
	loc := yekaterinburg(t)
	holidays := []time.Time{mustTime(t, "2026-11-20 00:00")}
	workdays := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	cal := platform.NewDefaultCalendar(loc, workdays, holidays, platform.FixedClock{})

	creation := mustTime(t, "2026-11-19 10:00") // четверг
	if got, want := cal.FormatDate(cal.WorkingDays(creation, 1)), "2026-11-23"; got != want {
		t.Fatalf("четверг + 1 с праздником в пятницу: got %s, want %s", got, want)
	}
}

// CAL-6 / NFR-7: заявка, созданная в 23:50 по Екатеринбургу, относится к текущим
// суткам этого пояса, а не к UTC-суткам следующего дня.
func TestDefaultCalendar_TimezoneDayBoundary(t *testing.T) {
	loc := yekaterinburg(t)
	// 2026-11-20 23:50 +05:00 == 2026-11-20 18:50 UTC.
	created := time.Date(2026, 11, 20, 23, 50, 0, 0, loc)
	clock := platform.FixedClock{Time: created}
	cal := platform.NewDefaultCalendar(loc, platform.DefaultWorkdays(), nil, clock)

	if got, want := cal.FormatDate(clock.Now()), "2026-11-20"; got != want {
		t.Fatalf("23:50 Екатеринбург: FormatDate got %s, want %s", got, want)
	}
	today := cal.Today()
	if got, want := cal.FormatDate(today), "2026-11-20"; got != want {
		t.Fatalf("23:50 Екатеринбург: Today got %s, want %s", got, want)
	}
	// Дедлайн считается от локальной даты: пятница + 2 (Пн–Сб) = понедельник.
	if got, want := cal.FormatDate(cal.WorkingDays(clock.Now(), 2)), "2026-11-23"; got != want {
		t.Fatalf("23:50 Екатеринбург: deadline got %s, want %s", got, want)
	}
}

// Граница суток в UTC: 00:10 UTC — это уже 05:10 в Екатеринбурге (те же сутки).
func TestDefaultCalendar_UTCtoLocalDate(t *testing.T) {
	loc := yekaterinburg(t)
	utcMoment := time.Date(2026, 11, 21, 0, 10, 0, 0, time.UTC)
	clock := platform.FixedClock{Time: utcMoment}
	cal := platform.NewDefaultCalendar(loc, platform.DefaultWorkdays(), nil, clock)

	if got, want := cal.FormatDate(clock.Now()), "2026-11-21"; got != want {
		t.Fatalf("00:10 UTC: got %s, want %s", got, want)
	}
}

func TestDefaultCalendar_DaysNonPositive(t *testing.T) {
	loc := yekaterinburg(t)
	cal := platform.NewDefaultCalendar(loc, platform.DefaultWorkdays(), nil, platform.FixedClock{})
	creation := mustTime(t, "2026-11-20 10:00")
	if got, want := cal.FormatDate(cal.WorkingDays(creation, 0)), "2026-11-20"; got != want {
		t.Fatalf("WorkingDays(0): got %s, want %s", got, want)
	}
}

package settings

import (
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// yekaterinburg возвращает пояс приложения для тестов.
func yekaterinburg(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		t.Fatalf("LoadLocation(Asia/Yekaterinburg): %v", err)
	}
	return location
}

// CAL-6: пятница + 2 рабочих дня при маске Пн–Сб → понедельник.
func TestProviderWorkingDaysDefaultMask(t *testing.T) {
	friday := time.Date(2026, time.November, 20, 12, 0, 0, 0, yekaterinburg(t))
	provider := NewCalendarProvider(platform.FixedClock{Time: friday}, Defaults(), nil)

	got := provider.WorkingDays(friday, 2)
	want := time.Date(2026, time.November, 23, 0, 0, 0, 0, provider.Location())
	if !got.Equal(want) {
		t.Fatalf("пятница+2 = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

// CAL-6: четверг + 2 при маске Пн–Пт и празднике в пятницу → вторник.
func TestProviderWorkingDaysWithHoliday(t *testing.T) {
	location := yekaterinburg(t)
	thursday := time.Date(2026, time.November, 19, 9, 0, 0, 0, location)
	holidayFriday := time.Date(2026, time.November, 20, 0, 0, 0, 0, location)

	base := Defaults()
	base.Timezone = "Asia/Yekaterinburg"
	base.Workdays = []int{1, 2, 3, 4, 5}

	provider := NewCalendarProvider(platform.FixedClock{Time: thursday}, base,
		[]Holiday{{ID: 1, Date: holidayFriday}})

	got := provider.WorkingDays(thursday, 2)
	want := time.Date(2026, time.November, 24, 0, 0, 0, 0, location)
	if !got.Equal(want) {
		t.Fatalf("четверг+2 с праздником = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if provider.IsWorkingDay(holidayFriday) {
		t.Fatal("праздник не должен быть рабочим днём")
	}
	if provider.IsWorkingDay(time.Date(2026, time.November, 21, 0, 0, 0, 0, location)) {
		t.Fatal("суббота не должна быть рабочей при маске Пн–Пт")
	}
}

// CAL-2, CAL-6: 23:50 по Екатеринбургу — те же сутки этого пояса.
func TestProviderTodayInApplicationTimezone(t *testing.T) {
	location := yekaterinburg(t)
	// 18:50 UTC = 23:50 +05.
	instant := time.Date(2026, time.November, 20, 18, 50, 0, 0, time.UTC)
	provider := NewCalendarProvider(platform.FixedClock{Time: instant}, Defaults(), nil)

	today := provider.Today()
	if today.Format(platformDateLayout) != "2026-11-20" {
		t.Fatalf("Today = %s", today.Format(time.RFC3339))
	}
	if got := provider.FormatDate(instant); got != "2026-11-20" {
		t.Fatalf("FormatDate = %q", got)
	}
	if provider.Location().String() != location.String() {
		t.Fatalf("Location = %s", provider.Location())
	}

	// 19:10 UTC = 00:10 следующего дня по Екатеринбургу.
	afterMidnight := time.Date(2026, time.November, 20, 19, 10, 0, 0, time.UTC)
	provider = NewCalendarProvider(platform.FixedClock{Time: afterMidnight}, Defaults(), nil)
	if got := provider.Today().Format(platformDateLayout); got != "2026-11-21" {
		t.Fatalf("Today после полуночи = %q", got)
	}
}

// CAL-3: ReminderAt — ближайшее время напоминания в поясе приложения.
func TestProviderReminderAt(t *testing.T) {
	// 09:00 по Екатеринбургу (04:00 UTC) — напоминание сегодня в 10:00 (+05).
	before := time.Date(2026, time.November, 20, 4, 0, 0, 0, time.UTC)
	provider := NewCalendarProvider(platform.FixedClock{Time: before}, Defaults(), nil)
	want := time.Date(2026, time.November, 20, 10, 0, 0, 0, yekaterinburg(t))
	if got := provider.ReminderAt(); !got.Equal(want) {
		t.Fatalf("ReminderAt(09:00) = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}

	// 11:00 по Екатеринбургу (06:00 UTC) — напоминание уже прошло, ждём завтра.
	after := time.Date(2026, time.November, 20, 6, 0, 0, 0, time.UTC)
	provider = NewCalendarProvider(platform.FixedClock{Time: after}, Defaults(), nil)
	want = time.Date(2026, time.November, 21, 10, 0, 0, 0, yekaterinburg(t))
	if got := provider.ReminderAt(); !got.Equal(want) {
		t.Fatalf("ReminderAt(11:00) = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if provider.ReminderTime() != "10:00" || !provider.PushEnabled() {
		t.Fatalf("ReminderTime/PushEnabled = %q/%v", provider.ReminderTime(), provider.PushEnabled())
	}
}

// TICKET-1: подписи и цвета статусов из настроек; неизвестный код — пусто.
func TestProviderStatusSettings(t *testing.T) {
	provider := NewCalendarProvider(platform.FixedClock{Time: time.Now()}, Defaults(), nil)

	if got := provider.StatusLabel("new"); got != "Новая" {
		t.Fatalf("StatusLabel(new) = %q", got)
	}
	if got := provider.StatusColor("done"); got != "#52c41a" {
		t.Fatalf("StatusColor(done) = %q", got)
	}
	if got := provider.StatusLabel("unknown"); got != "" {
		t.Fatalf("StatusLabel(unknown) = %q", got)
	}

	base := Defaults()
	base.Statuses = []StatusSetting{{Code: "new", Label: "Свежая", Color: "#000000"}}
	provider = NewCalendarProvider(platform.FixedClock{Time: time.Now()}, base, nil)
	if got := provider.StatusLabel("new"); got != "Свежая" {
		t.Fatalf("StatusLabel(new) = %q", got)
	}
}

// DeadlineDays по умолчанию — 2 (TICKET-3).
func TestProviderDeadlineDays(t *testing.T) {
	provider := NewCalendarProvider(platform.FixedClock{Time: time.Now()}, Defaults(), nil)
	if got := provider.DeadlineDays(); got != 2 {
		t.Fatalf("DeadlineDays = %d", got)
	}
	if provider.Calendar() != provider {
		t.Fatal("Calendar() должен возвращать сам провайдер")
	}
}

// Некорректный часовой пояс в настройках не мешает создать провайдер:
// используются значения по умолчанию.
func TestProviderFallsBackOnBrokenSettings(t *testing.T) {
	broken := Defaults()
	broken.Timezone = "Europe/Mordor"
	provider := NewCalendarProvider(platform.FixedClock{Time: time.Now()}, broken, nil)
	if provider.Timezone() != "Asia/Yekaterinburg" {
		t.Fatalf("Timezone = %q", provider.Timezone())
	}
}

// platformDateLayout дублирует формат даты, чтобы не тянуть константу в тест.
const platformDateLayout = "2006-01-02"

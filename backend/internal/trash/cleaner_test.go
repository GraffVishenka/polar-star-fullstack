package trash

import (
	"testing"
	"time"
)

// TRASH-4: автоочистка запускается раз в сутки в 03:00 по поясу приложения.
func TestNextDailyRun(t *testing.T) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "до полуночи — сегодня",
			now:  time.Date(2026, time.November, 20, 1, 30, 0, 0, location),
			want: time.Date(2026, time.November, 20, 3, 0, 0, 0, location),
		},
		{
			name: "после запуска — завтра",
			now:  time.Date(2026, time.November, 20, 4, 0, 0, 0, location),
			want: time.Date(2026, time.November, 21, 3, 0, 0, 0, location),
		},
		{
			name: "ровно в момент запуска — завтра",
			now:  time.Date(2026, time.November, 20, 3, 0, 0, 0, location),
			want: time.Date(2026, time.November, 21, 3, 0, 0, 0, location),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NextDailyRun(tc.now, 3, 0)
			if !got.Equal(tc.want) {
				t.Fatalf("NextDailyRun = %s, want %s", got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
			if got.Location() != location {
				t.Fatalf("пояс результата = %s", got.Location())
			}
		})
	}
}

// Часовой пояс приложения учитывается при вычислении момента запуска.
func TestNextDailyRunTimezone(t *testing.T) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	// 02:00 UTC = 07:00 +05 — после запуска, значит следующие сутки.
	now := time.Date(2026, time.November, 20, 2, 0, 0, 0, time.UTC).In(location)
	got := NextDailyRun(now, 3, 0)
	want := time.Date(2026, time.November, 21, 3, 0, 0, 0, location)
	if !got.Equal(want) {
		t.Fatalf("NextDailyRun = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

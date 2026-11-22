package settings

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	// Драйвер MySQL регистрируется побочным эффектом импорта.
	_ "github.com/go-sql-driver/mysql"
)

// testDB подключается к MySQL из TEST_DATABASE_URL. На машине разработки БД нет
// (docs/ENVIRONMENT.md), поэтому без переменной тест пропускается; так же
// пропускаем, если БД недоступна или схема не применена.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан: интеграционный тест MySQL пропущен")
	}
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("TEST_DATABASE_URL некорректен: %v", err)
	}
	pool.SetMaxOpenConns(10)
	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		t.Skipf("тестовая БД недоступна (нужен MySQL и применённые миграции): %v", err)
	}
	var name string
	if err := pool.QueryRow(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'settings' LIMIT 1`).Scan(&name); err != nil {
		_ = pool.Close()
		t.Skip("схема не применена: выполните make migrate для тестовой БД")
	}
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

// settingsToInput переводит настройки обратно во вход для восстановления
// singleton-строки после теста.
func settingsToInput(in Settings) SettingsInput {
	return SettingsInput{
		AppName:              in.AppName,
		DeadlineDays:         in.DeadlineDays,
		Workdays:             append([]int(nil), in.Workdays...),
		Timezone:             in.Timezone,
		DeadlineReminderTime: in.DeadlineReminderTime,
		PushEnabled:          in.PushEnabled,
		Statuses:             cloneStatuses(in.Statuses),
	}
}

// CAL-1..CAL-3, TICKET-1: настройки читаются и сохраняются (upsert), повторный
// PUT идемпотентен.
func TestMySQLSettingsRoundTrip(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()

	original, err := repo.Settings(ctx)
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	t.Cleanup(func() {
		if _, err := repo.UpdateSettings(context.Background(), settingsToInput(*original)); err != nil {
			t.Logf("восстановление настроек: %v", err)
		}
	})

	in := SettingsInput{
		AppName:              "Тест Б7",
		DeadlineDays:         7,
		Workdays:             []int{1, 2, 3, 4, 5},
		Timezone:             "UTC",
		DeadlineReminderTime: "06:45",
		PushEnabled:          false,
		Statuses: []StatusSetting{
			{Code: "new", Label: "Свежая", Color: "#111111"},
			{Code: "in_progress", Label: "Делается", Color: "#222222"},
			{Code: "deferred", Label: "Ждёт", Color: "#333333"},
			{Code: "done", Label: "Готово", Color: "#444444"},
		},
	}
	saved, err := repo.UpdateSettings(ctx, in)
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if saved.AppName != "Тест Б7" || saved.DeadlineDays != 7 || saved.Timezone != "UTC" ||
		saved.DeadlineReminderTime != "06:45" || saved.PushEnabled {
		t.Fatalf("saved = %+v", saved)
	}
	if len(saved.Workdays) != 5 || saved.Workdays[0] != 1 || saved.Workdays[4] != 5 {
		t.Fatalf("workdays = %v", saved.Workdays)
	}
	if len(saved.Statuses) != 4 || saved.Statuses[0].Label != "Свежая" || saved.Statuses[3].Color != "#444444" {
		t.Fatalf("statuses = %+v", saved.Statuses)
	}

	// Повторный PUT тех же значений не меняет результат (идемпотентность).
	again, err := repo.UpdateSettings(ctx, in)
	if err != nil {
		t.Fatalf("UpdateSettings #2: %v", err)
	}
	if again.AppName != saved.AppName || again.DeadlineDays != saved.DeadlineDays {
		t.Fatalf("повторный PUT изменил данные: %+v / %+v", saved, again)
	}

	// Чтение через тот же репозиторий даёт сохранённые значения.
	reread, err := repo.Settings(ctx)
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if reread.Timezone != "UTC" || reread.DeadlineReminderTime != "06:45" {
		t.Fatalf("reread = %+v", reread)
	}
}

// CAL-1: праздник уникален по дате, обновляется и удаляется; фильтр по году.
func TestMySQLHolidaysCRUD(t *testing.T) {
	pool := testDB(t)
	repo := NewMySQLRepository(pool)
	ctx := context.Background()

	// Чистим возможный мусор от предыдущего упавшего прогона.
	for _, date := range []time.Time{
		time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC),
	} {
		if _, err := pool.ExecContext(ctx, `DELETE FROM holidays WHERE holiday_date = ?`, date); err != nil {
			t.Fatalf("cleanup holidays: %v", err)
		}
	}

	created, err := repo.CreateHoliday(ctx, time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateHoliday: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.Background(), `DELETE FROM holidays WHERE id = ?`, created.ID)
	})
	if created.Date.Format("2006-01-02") != "2000-01-03" {
		t.Fatalf("created = %+v", created)
	}

	if _, err := repo.CreateHoliday(ctx, time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC)); !errors.Is(err, ErrDuplicateHoliday) {
		t.Fatalf("дубль даты = %v, want ErrDuplicateHoliday", err)
	}

	year := 2000
	items, err := repo.Holidays(ctx, &year)
	if err != nil {
		t.Fatalf("Holidays: %v", err)
	}
	found := false
	for _, item := range items {
		if item.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("праздник не найден в списке за 2000: %+v", items)
	}

	updated, err := repo.UpdateHoliday(ctx, created.ID, time.Date(2000, time.January, 4, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("UpdateHoliday: %v", err)
	}
	if updated.Date.Format("2006-01-02") != "2000-01-04" {
		t.Fatalf("updated = %+v", updated)
	}

	if err := repo.DeleteHoliday(ctx, created.ID); err != nil {
		t.Fatalf("DeleteHoliday: %v", err)
	}
	if err := repo.DeleteHoliday(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("повторное удаление = %v, want ErrNotFound", err)
	}
}

package settings

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// fakeRepo — in-memory репозиторий настроек и праздников: правила Б7
// проверяются без MySQL (в окружении разработки БД нет).
type fakeRepo struct {
	settings Settings
	holidays []Holiday
	nextID   int64

	settingsErr error
	holidaysErr error
	createErr   error
	updateErr   error
	deleteErr   error

	updateCalls int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{settings: Defaults(), nextID: 10}
}

func (r *fakeRepo) Settings(context.Context) (*Settings, error) {
	if r.settingsErr != nil {
		return nil, r.settingsErr
	}
	current := cloneSettings(r.settings)
	return &current, nil
}

func (r *fakeRepo) UpdateSettings(_ context.Context, in SettingsInput) (*Settings, error) {
	r.updateCalls++
	if r.settingsErr != nil {
		return nil, r.settingsErr
	}
	r.settings = Settings{
		AppName:              in.AppName,
		DeadlineDays:         in.DeadlineDays,
		Workdays:             append([]int(nil), in.Workdays...),
		Timezone:             in.Timezone,
		DeadlineReminderTime: in.DeadlineReminderTime,
		PushEnabled:          in.PushEnabled,
		Statuses:             cloneStatuses(in.Statuses),
		UpdatedAt:            time.Unix(0, 0).UTC(),
	}
	current := cloneSettings(r.settings)
	return &current, nil
}

func (r *fakeRepo) Holidays(_ context.Context, year *int) ([]Holiday, error) {
	if r.holidaysErr != nil {
		return nil, r.holidaysErr
	}
	items := make([]Holiday, 0, len(r.holidays))
	for _, holiday := range r.holidays {
		if year != nil && holiday.Date.Year() != *year {
			continue
		}
		items = append(items, holiday)
	}
	return items, nil
}

func (r *fakeRepo) HolidayByID(_ context.Context, id int64) (*Holiday, error) {
	for i := range r.holidays {
		if r.holidays[i].ID == id {
			holiday := r.holidays[i]
			return &holiday, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepo) CreateHoliday(_ context.Context, date time.Time) (*Holiday, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	for _, holiday := range r.holidays {
		if sameDate(holiday.Date, date) {
			return nil, ErrDuplicateHoliday
		}
	}
	r.nextID++
	created := Holiday{ID: r.nextID, Date: normalizeDate(date), CreatedAt: time.Unix(0, 0).UTC()}
	r.holidays = append(r.holidays, created)
	return &created, nil
}

func (r *fakeRepo) UpdateHoliday(_ context.Context, id int64, date time.Time) (*Holiday, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	for i := range r.holidays {
		if r.holidays[i].ID != id {
			continue
		}
		for _, other := range r.holidays {
			if other.ID != id && sameDate(other.Date, date) {
				return nil, ErrDuplicateHoliday
			}
		}
		r.holidays[i].Date = normalizeDate(date)
		updated := r.holidays[i]
		return &updated, nil
	}
	return nil, ErrNotFound
}

func (r *fakeRepo) DeleteHoliday(_ context.Context, id int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	for i := range r.holidays {
		if r.holidays[i].ID == id {
			r.holidays = append(r.holidays[:i], r.holidays[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func sameDate(a, b time.Time) bool {
	aYear, aMonth, aDay := a.Date()
	bYear, bMonth, bDay := b.Date()
	return aYear == bYear && aMonth == bMonth && aDay == bDay
}

func newTestService(t *testing.T, repo *fakeRepo, now time.Time) *Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(repo, platform.FixedClock{Time: now}, logger)
}

func requireAPIError(t *testing.T, err error, status int, code apierr.Code) *apierr.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка %s, получен nil", code)
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("ошибка не является *apierr.Error: %v", err)
	}
	if apiErr.Status != status {
		t.Fatalf("status = %d, want %d (%s)", apiErr.Status, status, apiErr.Message)
	}
	if apiErr.Code != code {
		t.Fatalf("code = %s, want %s", apiErr.Code, code)
	}
	return apiErr
}

// CAL-5: Reload собирает календарь из БД (маска, праздники, дедлайн).
func TestServiceReloadBuildsCalendar(t *testing.T) {
	location := yekaterinburg(t)
	repo := newFakeRepo()
	repo.settings.Workdays = []int{1, 2, 3, 4, 5}
	repo.settings.DeadlineDays = 5
	repo.holidays = []Holiday{{ID: 1, Date: time.Date(2026, time.November, 20, 0, 0, 0, 0, location)}}

	thursday := time.Date(2026, time.November, 19, 9, 0, 0, 0, location)
	service := newTestService(t, repo, thursday)
	if err := service.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	provider := service.Provider()
	if provider.DeadlineDays() != 5 {
		t.Fatalf("DeadlineDays = %d", provider.DeadlineDays())
	}
	if provider.StatusLabel("new") != "Новая" || provider.StatusColor("new") != "#1677ff" {
		t.Fatalf("статусы не загружены: %q/%q", provider.StatusLabel("new"), provider.StatusColor("new"))
	}
	// Праздник в пятницу + маска Пн–Пт: четверг+2 → вторник.
	if got := provider.WorkingDays(thursday, 2).Format(platformDateLayout); got != "2026-11-24" {
		t.Fatalf("рабочие дни = %s", got)
	}
}

// Недоступная БД при старте не роняет сервис: остаются дефолты (CAL-1..CAL-3).
func TestServiceReloadErrorKeepsDefaults(t *testing.T) {
	repo := newFakeRepo()
	repo.settingsErr = errors.New("db is down")
	friday := time.Date(2026, time.November, 20, 12, 0, 0, 0, yekaterinburg(t))
	service := newTestService(t, repo, friday)

	if err := service.Reload(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка Reload")
	}
	provider := service.Provider()
	if provider.Timezone() != "Asia/Yekaterinburg" || provider.DeadlineDays() != 2 {
		t.Fatalf("дефолты не применены: %s/%d", provider.Timezone(), provider.DeadlineDays())
	}
	// Пятница+2 при маске Пн–Сб по умолчанию → понедельник.
	if got := provider.WorkingDays(friday, 2).Format(platformDateLayout); got != "2026-11-23" {
		t.Fatalf("рабочие дни по дефолту = %s", got)
	}
}

// CAL-5: PUT пересобирает календарь без рестарта; старые расчёты не меняются.
func TestServiceUpdateSettingsRebuildsCalendar(t *testing.T) {
	friday := time.Date(2026, time.November, 20, 12, 0, 0, 0, yekaterinburg(t))
	repo := newFakeRepo()
	service := newTestService(t, repo, friday)
	if err := service.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	before := service.Provider().WorkingDays(friday, 2)
	if before.Format(platformDateLayout) != "2026-11-23" {
		t.Fatalf("до изменения = %s", before.Format(platformDateLayout))
	}

	in, details := ValidateSettings(SettingsUpdateRequest{
		AppName:              "Новое имя",
		DeadlineDays:         3,
		Workdays:             []int{1, 2, 3, 4, 5},
		Timezone:             "Asia/Yekaterinburg",
		DeadlineReminderTime: "08:15",
		PushEnabled:          false,
		Statuses:             validStatuses(),
	})
	if len(details) != 0 {
		t.Fatalf("details = %v", details)
	}
	updated, err := service.UpdateSettings(context.Background(), in)
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if updated.AppName != "Новое имя" || updated.DeadlineDays != 3 || updated.PushEnabled {
		t.Fatalf("updated = %+v", updated)
	}

	provider := service.Provider()
	if provider.DeadlineDays() != 3 {
		t.Fatalf("DeadlineDays = %d", provider.DeadlineDays())
	}
	// Пятница+2 при маске Пн–Пт → вторник (CAL-4).
	if got := provider.WorkingDays(friday, 2).Format(platformDateLayout); got != "2026-11-24" {
		t.Fatalf("после изменения = %s", got)
	}
	if provider.ReminderTime() != "08:15" || provider.PushEnabled() {
		t.Fatalf("напоминание/push = %q/%v", provider.ReminderTime(), provider.PushEnabled())
	}
	// CAL-5: уже выданный дедлайн (результат до изменения) не пересчитывается.
	if before.Format(platformDateLayout) != "2026-11-23" {
		t.Fatalf("старый дедлайн изменился: %s", before.Format(platformDateLayout))
	}
}

// Повторный PUT тех же значений идемпотентен.
func TestServiceUpdateSettingsIdempotent(t *testing.T) {
	friday := time.Date(2026, time.November, 20, 12, 0, 0, 0, yekaterinburg(t))
	repo := newFakeRepo()
	service := newTestService(t, repo, friday)

	in, details := ValidateSettings(validSettingsRequest())
	if len(details) != 0 {
		t.Fatalf("details = %v", details)
	}
	first, err := service.UpdateSettings(context.Background(), in)
	if err != nil {
		t.Fatalf("UpdateSettings #1: %v", err)
	}
	second, err := service.UpdateSettings(context.Background(), in)
	if err != nil {
		t.Fatalf("UpdateSettings #2: %v", err)
	}
	if repo.updateCalls != 2 {
		t.Fatalf("updateCalls = %d", repo.updateCalls)
	}
	if first.AppName != second.AppName || first.DeadlineDays != second.DeadlineDays ||
		first.Timezone != second.Timezone || first.DeadlineReminderTime != second.DeadlineReminderTime {
		t.Fatalf("первый и второй PUT различаются: %+v / %+v", first, second)
	}
	if len(first.Statuses) != len(second.Statuses) || len(first.Workdays) != len(second.Workdays) {
		t.Fatalf("срезы различаются: %+v / %+v", first, second)
	}
	if service.Provider().DeadlineDays() != in.DeadlineDays {
		t.Fatalf("снапшот не обновлён: %d", service.Provider().DeadlineDays())
	}
}

// Праздники: создание/правка/удаление пересобирают календарь (CAL-1).
func TestServiceHolidayLifecycleRebuildsCalendar(t *testing.T) {
	location := yekaterinburg(t)
	thursday := time.Date(2026, time.November, 19, 9, 0, 0, 0, location)
	repo := newFakeRepo()
	repo.settings.Workdays = []int{1, 2, 3, 4, 5}
	service := newTestService(t, repo, thursday)
	if err := service.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	provider := service.Provider()
	if got := provider.WorkingDays(thursday, 2).Format(platformDateLayout); got != "2026-11-23" {
		t.Fatalf("без праздников = %s", got)
	}

	// Праздник в пятницу → четверг+2 = вторник.
	created, err := service.CreateHoliday(context.Background(),
		time.Date(2026, time.November, 20, 0, 0, 0, 0, location))
	if err != nil {
		t.Fatalf("CreateHoliday: %v", err)
	}
	if created.Date.Format(platformDateLayout) != "2026-11-20" {
		t.Fatalf("created = %+v", created)
	}
	if provider.IsWorkingDay(time.Date(2026, time.November, 20, 0, 0, 0, 0, location)) {
		t.Fatal("пятница должна стать нерабочей")
	}
	if got := provider.WorkingDays(thursday, 2).Format(platformDateLayout); got != "2026-11-24" {
		t.Fatalf("с праздником = %s", got)
	}

	// Перенос праздника на субботу → пятница снова рабочая.
	updated, err := service.UpdateHoliday(context.Background(), created.ID,
		time.Date(2026, time.November, 21, 0, 0, 0, 0, location))
	if err != nil {
		t.Fatalf("UpdateHoliday: %v", err)
	}
	if updated.Date.Format(platformDateLayout) != "2026-11-21" {
		t.Fatalf("updated = %+v", updated)
	}
	if !provider.IsWorkingDay(time.Date(2026, time.November, 20, 0, 0, 0, 0, location)) {
		t.Fatal("пятница должна снова стать рабочей")
	}
	if got := provider.WorkingDays(thursday, 2).Format(platformDateLayout); got != "2026-11-23" {
		t.Fatalf("после переноса = %s", got)
	}

	// Удаление праздника возвращает субботу в рабочие дни (маска Пн–Пт не
	// включает субботу, поэтому проверяем именно отсутствие праздника).
	if err := service.DeleteHoliday(context.Background(), created.ID); err != nil {
		t.Fatalf("DeleteHoliday: %v", err)
	}
	if _, err := repo.HolidayByID(context.Background(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("праздник не удалён: %v", err)
	}
	items, err := service.Holidays(context.Background(), nil)
	if err != nil || len(items) != 0 {
		t.Fatalf("Holidays = %v, %v", items, err)
	}
}

// Ошибки праздников: дубль даты → 409, отсутствие → 404.
func TestServiceHolidayErrors(t *testing.T) {
	location := yekaterinburg(t)
	repo := newFakeRepo()
	service := newTestService(t, repo, time.Date(2026, time.November, 19, 9, 0, 0, 0, location))

	if _, err := service.CreateHoliday(context.Background(),
		time.Date(2026, time.November, 20, 0, 0, 0, 0, location)); err != nil {
		t.Fatalf("CreateHoliday: %v", err)
	}
	_, err := service.CreateHoliday(context.Background(),
		time.Date(2026, time.November, 20, 0, 0, 0, 0, location))
	requireAPIError(t, err, 409, apierr.CodeConflict)

	second, err := service.CreateHoliday(context.Background(),
		time.Date(2026, time.November, 21, 0, 0, 0, 0, location))
	if err != nil {
		t.Fatalf("CreateHoliday #2: %v", err)
	}
	_, err = service.UpdateHoliday(context.Background(), second.ID,
		time.Date(2026, time.November, 20, 0, 0, 0, 0, location))
	requireAPIError(t, err, 409, apierr.CodeConflict)

	_, err = service.UpdateHoliday(context.Background(), 999,
		time.Date(2026, time.November, 22, 0, 0, 0, 0, location))
	requireAPIError(t, err, 404, apierr.CodeNotFound)

	requireAPIError(t, service.DeleteHoliday(context.Background(), 999), 404, apierr.CodeNotFound)
}

// Фильтр праздников по году.
func TestServiceHolidaysYearFilter(t *testing.T) {
	location := yekaterinburg(t)
	repo := newFakeRepo()
	repo.holidays = []Holiday{
		{ID: 1, Date: time.Date(2026, time.January, 1, 0, 0, 0, 0, location)},
		{ID: 2, Date: time.Date(2027, time.January, 1, 0, 0, 0, 0, location)},
	}
	service := newTestService(t, repo, time.Date(2026, time.November, 19, 9, 0, 0, 0, location))

	year := 2027
	items, err := service.Holidays(context.Background(), &year)
	if err != nil {
		t.Fatalf("Holidays: %v", err)
	}
	if len(items) != 1 || items[0].ID != 2 {
		t.Fatalf("items = %+v", items)
	}
	all, err := service.Holidays(context.Background(), nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %v, %v", all, err)
	}
}

// Ошибка БД → 500 в едином формате.
func TestServiceStorageErrors(t *testing.T) {
	repo := newFakeRepo()
	repo.settingsErr = errors.New("boom")
	service := newTestService(t, repo, time.Now())
	_, err := service.Settings(context.Background())
	requireAPIError(t, err, 500, apierr.CodeInternal)

	repo = newFakeRepo()
	repo.holidaysErr = errors.New("boom")
	service = newTestService(t, repo, time.Now())
	_, err = service.Holidays(context.Background(), nil)
	requireAPIError(t, err, 500, apierr.CodeInternal)

	repo = newFakeRepo()
	repo.createErr = errors.New("boom")
	service = newTestService(t, repo, time.Now())
	_, err = service.CreateHoliday(context.Background(), time.Now())
	requireAPIError(t, err, 500, apierr.CodeInternal)
}

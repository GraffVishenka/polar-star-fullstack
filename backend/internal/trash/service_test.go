package trash

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// fakeRepo — in-memory репозиторий корзины: правила домена проверяются без
// MySQL (в окружении разработки БД нет).
type fakeRepo struct {
	items  map[Type][]Item
	totals map[Type]int

	ticketInfo    TicketRestore
	ticketErr     error
	kbInfo        KBRestore
	kbErr         error
	equipmentInfo EquipmentRestore
	equipmentErr  error

	purgeFiles []string
	purgeErr   error
	purged     map[Type][]int64

	expired    map[Type][]int64
	expiredErr error
	cutoffs    map[Type][]time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items:   map[Type][]Item{},
		totals:  map[Type]int{},
		purged:  map[Type][]int64{},
		expired: map[Type][]int64{},
		cutoffs: map[Type][]time.Time{},
	}
}

func (r *fakeRepo) List(_ context.Context, kind Type, _, _ int) ([]Item, int, error) {
	return r.items[kind], r.totals[kind], nil
}

func (r *fakeRepo) RestoreTicket(context.Context, int64) (TicketRestore, error) {
	return r.ticketInfo, r.ticketErr
}

func (r *fakeRepo) RestoreKB(context.Context, int64) (KBRestore, error) {
	return r.kbInfo, r.kbErr
}

func (r *fakeRepo) RestoreEquipment(context.Context, int64) (EquipmentRestore, error) {
	return r.equipmentInfo, r.equipmentErr
}

func (r *fakeRepo) PurgeTicket(_ context.Context, id int64) ([]string, error) {
	if r.purgeErr != nil {
		return nil, r.purgeErr
	}
	r.purged[TypeTicket] = append(r.purged[TypeTicket], id)
	return r.purgeFiles, nil
}

func (r *fakeRepo) PurgeKB(_ context.Context, id int64) error {
	if r.purgeErr != nil {
		return r.purgeErr
	}
	r.purged[TypeKB] = append(r.purged[TypeKB], id)
	return nil
}

func (r *fakeRepo) PurgeEquipment(_ context.Context, id int64) error {
	if r.purgeErr != nil {
		return r.purgeErr
	}
	r.purged[TypeEquipment] = append(r.purged[TypeEquipment], id)
	return nil
}

func (r *fakeRepo) Expired(_ context.Context, kind Type, before time.Time, limit int) ([]int64, error) {
	r.cutoffs[kind] = append(r.cutoffs[kind], before)
	if r.expiredErr != nil {
		return nil, r.expiredErr
	}
	ids := r.expired[kind]
	if len(ids) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > len(ids) {
		limit = len(ids)
	}
	batch := append([]int64(nil), ids[:limit]...)
	r.expired[kind] = ids[limit:]
	return batch, nil
}

// fakeRemover — файловое хранилище для проверки удаления с диска.
type fakeRemover struct {
	removed []string
	err     error
}

func (r *fakeRemover) Remove(rel string) error {
	r.removed = append(r.removed, rel)
	return r.err
}

func newTestService(t *testing.T, repo *fakeRepo, files FileRemover, now time.Time, opts ...Option) *Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(repo, files, platform.FixedClock{Time: now}, logger, opts...)
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

// TRASH-1: страница корзины сохраняет total/page/size.
func TestServiceList(t *testing.T) {
	repo := newFakeRepo()
	repo.items[TypeTicket] = []Item{{Type: TypeTicket, ID: 1, Title: "#1"}, {Type: TypeTicket, ID: 2}}
	repo.totals[TypeTicket] = 7

	service := newTestService(t, repo, nil, time.Now())
	page, err := service.List(context.Background(), TypeTicket, 2, 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 7 || page.Page != 2 || page.Size != 2 || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}

	_, err = service.List(context.Background(), Type("unknown"), 1, 20)
	apiErr := requireAPIError(t, err, 422, apierr.CodeValidation)
	if apiErr.Details["type"] == nil {
		t.Fatalf("details.type отсутствует: %v", apiErr.Details)
	}
}

// TRASH-2: заявка восстанавливается; деактивированное здание — предупреждение,
// а не отказ.
func TestServiceRestoreTicket(t *testing.T) {
	repo := newFakeRepo()
	repo.ticketInfo = TicketRestore{BuildingID: 1, BuildingName: "Здание A", BuildingActive: true}
	service := newTestService(t, repo, nil, time.Now())

	result, err := service.Restore(context.Background(), TypeTicket, 10)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !result.Restored || result.Type != TypeTicket || result.ID != 10 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v", result.Warnings)
	}

	repo.ticketInfo.BuildingActive = false
	result, err = service.Restore(context.Background(), TypeTicket, 10)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] == "" {
		t.Fatalf("ожидалось предупреждение о деактивированном здании: %v", result.Warnings)
	}

	repo.ticketErr = ErrNotFound
	requireAPIError(t, func() error {
		_, err := service.Restore(context.Background(), TypeTicket, 404)
		return err
	}(), 404, apierr.CodeNotFound)
}

// TRASH-2/TRASH-5: узел БЗ — родитель должен быть живым; потомки не
// восстанавливаются (warning).
func TestServiceRestoreKB(t *testing.T) {
	repo := newFakeRepo()
	service := newTestService(t, repo, nil, time.Now())

	// Корень без потомков — без предупреждений.
	result, err := service.Restore(context.Background(), TypeKB, 1)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v", result.Warnings)
	}

	// Потомки остались в корзине — предупреждение TRASH-5.
	repo.kbInfo = KBRestore{DeletedChildren: 3}
	result, err = service.Restore(context.Background(), TypeKB, 2)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] == "" {
		t.Fatalf("ожидалось предупреждение о потомках: %v", result.Warnings)
	}

	// Родитель удалён — конфликт.
	repo.kbErr = ErrParentDeleted
	_, err = service.Restore(context.Background(), TypeKB, 3)
	requireAPIError(t, err, 409, apierr.CodeConflict)
}

// TRASH-2: оборудование — живой склад, свободный рег. номер; деактивированное
// здание допустимо с предупреждением.
func TestServiceRestoreEquipment(t *testing.T) {
	repo := newFakeRepo()
	repo.equipmentInfo = EquipmentRestore{WarehouseID: 5, WarehouseName: "Склад", BuildingName: "Здание", BuildingActive: true}
	service := newTestService(t, repo, nil, time.Now())

	result, err := service.Restore(context.Background(), TypeEquipment, 7)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v", result.Warnings)
	}

	repo.equipmentInfo.BuildingActive = false
	if result, err = service.Restore(context.Background(), TypeEquipment, 7); err != nil || len(result.Warnings) != 1 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}

	repo.equipmentErr = ErrDuplicateRegistration
	_, err = service.Restore(context.Background(), TypeEquipment, 7)
	requireAPIError(t, err, 409, apierr.CodeDuplicateRegistrationNumber)

	repo.equipmentErr = ErrWarehouseDeleted
	_, err = service.Restore(context.Background(), TypeEquipment, 7)
	requireAPIError(t, err, 409, apierr.CodeConflict)
}

// TRASH-3: физическое удаление заявки возвращает файлы для удаления с диска.
func TestServicePurgeTicketRemovesFiles(t *testing.T) {
	repo := newFakeRepo()
	repo.purgeFiles = []string{"2026/11/a.png", "2026/11/b.mp4"}
	remover := &fakeRemover{}
	service := newTestService(t, repo, remover, time.Now())

	if err := service.Purge(context.Background(), TypeTicket, 5); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if len(repo.purged[TypeTicket]) != 1 || repo.purged[TypeTicket][0] != 5 {
		t.Fatalf("purged = %v", repo.purged[TypeTicket])
	}
	if len(remover.removed) != 2 || remover.removed[0] != "2026/11/a.png" {
		t.Fatalf("removed = %v", remover.removed)
	}

	// Ошибка удаления файла не отменяет уже выполненное удаление в БД.
	remover.err = errors.New("disk is read-only")
	if err := service.Purge(context.Background(), TypeTicket, 6); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	// nil-remover не паникует.
	service = newTestService(t, repo, nil, time.Now())
	if err := service.Purge(context.Background(), TypeTicket, 7); err != nil {
		t.Fatalf("Purge без remover: %v", err)
	}
}

// TRASH-3: удаление узла БЗ/оборудования и ошибки.
func TestServicePurgeOtherTypes(t *testing.T) {
	repo := newFakeRepo()
	service := newTestService(t, repo, nil, time.Now())

	if err := service.Purge(context.Background(), TypeKB, 3); err != nil {
		t.Fatalf("Purge KB: %v", err)
	}
	if err := service.Purge(context.Background(), TypeEquipment, 4); err != nil {
		t.Fatalf("Purge equipment: %v", err)
	}
	if len(repo.purged[TypeKB]) != 1 || len(repo.purged[TypeEquipment]) != 1 {
		t.Fatalf("purged = %v", repo.purged)
	}

	repo.purgeErr = ErrNotFound
	requireAPIError(t, service.Purge(context.Background(), TypeEquipment, 99), 404, apierr.CodeNotFound)
}

// TRASH-4: порог автоочистки — now - 90 дней; очистка идёт пачками.
func TestServiceCleanupThresholdAndBatches(t *testing.T) {
	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.expired[TypeTicket] = []int64{1, 2, 3}
	remover := &fakeRemover{}
	service := newTestService(t, repo, remover, now, WithBatch(2))

	deleted, err := service.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}
	wantCutoff := now.Add(-DefaultRetention)
	cutoffs := repo.cutoffs[TypeTicket]
	if len(cutoffs) == 0 || !cutoffs[0].Equal(wantCutoff) {
		t.Fatalf("cutoff = %v, want %v", cutoffs, wantCutoff)
	}
	if len(repo.purged[TypeTicket]) != 3 {
		t.Fatalf("purged = %v", repo.purged[TypeTicket])
	}
}

// TRASH-4: срок хранения можно переопределить (тесты/конфигурация).
func TestServiceCleanupCustomRetention(t *testing.T) {
	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	service := newTestService(t, repo, nil, now, WithRetention(24*time.Hour))

	if _, err := service.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	want := now.Add(-24 * time.Hour)
	if len(repo.cutoffs[TypeTicket]) == 0 || !repo.cutoffs[TypeTicket][0].Equal(want) {
		t.Fatalf("cutoff = %v, want %v", repo.cutoffs[TypeTicket], want)
	}
}

// Ошибка выборки не прерывает очистку остальных типов и возвращается наружу.
func TestServiceCleanupErrors(t *testing.T) {
	repo := newFakeRepo()
	repo.expiredErr = errors.New("db is down")
	service := newTestService(t, repo, nil, time.Now())

	deleted, err := service.Cleanup(context.Background())
	if err == nil {
		t.Fatal("ожидалась ошибка очистки")
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d", deleted)
	}
}

// TRASH-2: ошибки восстановления маппятся в коды контракта.
func TestMapError(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   apierr.Code
	}{
		{ErrNotFound, 404, apierr.CodeNotFound},
		{ErrParentDeleted, 409, apierr.CodeConflict},
		{ErrWarehouseDeleted, 409, apierr.CodeConflict},
		{ErrDuplicateRegistration, 409, apierr.CodeDuplicateRegistrationNumber},
		{ErrUnknownType, 422, apierr.CodeValidation},
		{errors.New("boom"), 500, apierr.CodeInternal},
	}
	for _, tc := range tests {
		requireAPIError(t, mapError(tc.err), tc.status, tc.code)
	}
	if err := mapError(nil); err != nil {
		t.Fatalf("mapError(nil) = %v", err)
	}
}

// Краткие описания элементов корзины (TRASH-1).
func TestTitles(t *testing.T) {
	if got := ticketTitle(12, "Иванов", "101", "Здание A"); got != "#12 · Иванов · каб. 101 · Здание A" {
		t.Fatalf("ticketTitle = %q", got)
	}
	if got := kbTitle("Инструкция", nullString("Раздел")); got != "Инструкция · родитель: Раздел" {
		t.Fatalf("kbTitle = %q", got)
	}
	if got := kbTitle("Инструкция", nullString("")); got != "Инструкция · корень" {
		t.Fatalf("kbTitle root = %q", got)
	}
	if got := equipmentTitle("Ноутбук", "REG-1", "Склад A"); got != "Ноутбук · REG-1 · Склад A" {
		t.Fatalf("equipmentTitle = %q", got)
	}
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

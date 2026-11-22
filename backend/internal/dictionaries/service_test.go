package dictionaries

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// requireCode проверяет HTTP-статус и код типизированной ошибки API.
func requireCode(t *testing.T, err error, status int, code string) {
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
	if string(apiErr.Code) != code {
		t.Fatalf("code = %s, want %s", apiErr.Code, code)
	}
}

// fakeRepo — in-memory репозиторий справочников: правила Б7 проверяются без
// MySQL (в окружении разработки БД нет).
type fakeRepo struct {
	items     []Item
	nextID    int64
	nameTaken bool
	listAll   []bool // зафиксированные значения includeInactive

	byIDErr      error
	createErr    error
	updateErr    error
	setActiveErr error
	nameTakenErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items: []Item{
			{ID: 1, Name: "Не работает оборудование", IsActive: true, SortOrder: 0, CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC()},
			{ID: 2, Name: "Прочее", IsActive: true, SortOrder: 1, CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC()},
		},
		nextID: 10,
	}
}

func (r *fakeRepo) List(_ context.Context, _ Kind, includeInactive bool) ([]Item, error) {
	r.listAll = append(r.listAll, includeInactive)
	items := make([]Item, 0, len(r.items))
	for _, item := range r.items {
		if !includeInactive && !item.IsActive {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *fakeRepo) ByID(_ context.Context, _ Kind, id int64) (*Item, error) {
	if r.byIDErr != nil {
		return nil, r.byIDErr
	}
	for i := range r.items {
		if r.items[i].ID == id {
			item := r.items[i]
			return &item, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepo) Create(_ context.Context, _ Kind, in CreateInput) (*Item, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	if r.nameTaken {
		return nil, ErrNameTaken
	}
	r.nextID++
	item := Item{ID: r.nextID, Name: in.Name, IsActive: in.IsActive, SortOrder: in.SortOrder,
		CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC()}
	r.items = append(r.items, item)
	return &item, nil
}

func (r *fakeRepo) Update(_ context.Context, _ Kind, id int64, in UpdateInput) (*Item, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	for i := range r.items {
		if r.items[i].ID != id {
			continue
		}
		if in.Name != nil {
			r.items[i].Name = *in.Name
		}
		if in.IsActive != nil {
			r.items[i].IsActive = *in.IsActive
		}
		if in.SortOrder != nil {
			r.items[i].SortOrder = *in.SortOrder
		}
		item := r.items[i]
		return &item, nil
	}
	return nil, ErrNotFound
}

func (r *fakeRepo) SetActive(_ context.Context, _ Kind, id int64, active bool) (*Item, error) {
	if r.setActiveErr != nil {
		return nil, r.setActiveErr
	}
	for i := range r.items {
		if r.items[i].ID == id {
			r.items[i].IsActive = active
			item := r.items[i]
			return &item, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepo) NameTaken(_ context.Context, _ Kind, name string, excludeID int64) (bool, error) {
	if r.nameTakenErr != nil {
		return false, r.nameTakenErr
	}
	for _, item := range r.items {
		if item.Name == name && item.ID != excludeID {
			return true, nil
		}
	}
	return false, nil
}

// Дубль имени → 409 CONFLICT, без создания записи.
func TestServiceCreateDuplicate(t *testing.T) {
	repo := newFakeRepo()
	repo.nameTaken = true
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), KindProblemTypes, CreateInput{Name: "Прочее", IsActive: true})
	requireCode(t, err, 409, "CONFLICT")
	if len(repo.items) != 2 {
		t.Fatalf("элемент-дубль создан: %v", repo.items)
	}
}

// Создание и правка элемента; переименование в своё имя разрешено.
func TestServiceCreateAndUpdate(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	created, err := svc.Create(context.Background(), KindEquipmentTypes, CreateInput{Name: "Принтер"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 || created.Name != "Принтер" {
		t.Fatalf("created = %+v", created)
	}

	name := "Принтер"
	updated, err := svc.Update(context.Background(), KindEquipmentTypes, created.ID, UpdateInput{Name: &name})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "Принтер" {
		t.Fatalf("updated = %+v", updated)
	}

	other := "Прочее"
	_, err = svc.Update(context.Background(), KindEquipmentTypes, created.ID, UpdateInput{Name: &other})
	requireCode(t, err, 409, "CONFLICT")
}

// «Удаление» — деактивация: элемент остаётся в БД, но исчезает из активных.
func TestServiceDeleteDeactivates(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.Delete(context.Background(), KindProblemTypes, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if repo.items[0].IsActive {
		t.Fatal("элемент должен быть деактивирован, а не удалён")
	}

	active, err := svc.List(context.Background(), KindProblemTypes, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(active) != 1 || active[0].ID != 2 {
		t.Fatalf("активные = %+v", active)
	}
	all, err := svc.List(context.Background(), KindProblemTypes, true)
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("все = %+v", all)
	}

	// Повторная деактивация идемпотентна.
	if err := svc.Delete(context.Background(), KindProblemTypes, 1); err != nil {
		t.Fatalf("повторный Delete: %v", err)
	}
}

// PublicList — только активные (форма анонимной заявки).
func TestServicePublicList(t *testing.T) {
	repo := newFakeRepo()
	repo.items[1].IsActive = false
	svc := NewService(repo)

	items, err := svc.PublicList(context.Background(), KindProblemTypes)
	if err != nil {
		t.Fatalf("PublicList: %v", err)
	}
	if len(items) != 1 || items[0].ID != 1 {
		t.Fatalf("items = %+v", items)
	}
	if len(repo.listAll) != 1 || repo.listAll[0] {
		t.Fatalf("PublicList должен запрашивать только активные: %v", repo.listAll)
	}
}

// Маппинг ошибок репозитория.
func TestServiceErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{ErrNotFound, 404, "NOT_FOUND"},
		{ErrNameTaken, 409, "CONFLICT"},
		{errors.New("boom"), 500, "INTERNAL_ERROR"},
	}

	for _, tc := range tests {
		repo := newFakeRepo()
		repo.byIDErr = tc.err
		_, err := NewService(repo).Update(context.Background(), KindProblemTypes, 1, UpdateInput{})
		requireCode(t, err, tc.status, tc.code)
	}

	repo := newFakeRepo()
	repo.updateErr = errors.New("boom")
	_, err := NewService(repo).Update(context.Background(), KindProblemTypes, 1, UpdateInput{})
	requireCode(t, err, 500, "INTERNAL_ERROR")

	repo = newFakeRepo()
	repo.setActiveErr = errors.New("boom")
	requireCode(t, NewService(repo).Delete(context.Background(), KindProblemTypes, 1), 500, "INTERNAL_ERROR")

	repo = newFakeRepo()
	requireCode(t, NewService(repo).Delete(context.Background(), KindProblemTypes, 999), 404, "NOT_FOUND")
}

package users

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// fakeRepo — in-memory Repository: правила сервиса проверяются без MySQL.
type fakeRepo struct {
	users          map[int64]*auth.User
	buildings      map[int64]bool
	buildingNames  map[int64]string
	nextID         int64
	setPasswordIDs []int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		users:         map[int64]*auth.User{},
		buildings:     map[int64]bool{1: true, 3: true, 2: false},
		buildingNames: map[int64]string{1: "Здание A", 2: "Здание B (неактивно)", 3: "Здание C"},
	}
}

func (r *fakeRepo) add(login string, role auth.Role, buildingID *int64, active bool) *auth.User {
	r.nextID++
	now := time.Now().UTC()
	user := &auth.User{
		ID:           r.nextID,
		Login:        login,
		FullName:     "Сотрудник " + login,
		Role:         role,
		BuildingID:   buildingID,
		IsActive:     active,
		PasswordHash: "",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if buildingID != nil {
		if name, ok := r.buildingNames[*buildingID]; ok {
			user.BuildingName = &name
		}
	}
	r.users[user.ID] = user
	return user
}

func (r *fakeRepo) List(_ context.Context, filter ListFilter) ([]*auth.User, int, error) {
	items := make([]*auth.User, 0, len(r.users))
	for _, user := range r.users {
		if filter.Role != "" && user.Role != filter.Role {
			continue
		}
		if filter.BuildingID != nil {
			if user.BuildingID == nil || *user.BuildingID != *filter.BuildingID {
				continue
			}
		}
		if filter.IsActive != nil && user.IsActive != *filter.IsActive {
			continue
		}
		if filter.Search != "" {
			needle := strings.ToLower(filter.Search)
			if !strings.Contains(strings.ToLower(user.FullName), needle) &&
				!strings.Contains(strings.ToLower(user.Login), needle) {
				continue
			}
		}
		items = append(items, user)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].FullName == items[j].FullName {
			return items[i].ID < items[j].ID
		}
		return items[i].FullName < items[j].FullName
	})
	total := len(items)
	start := (filter.Page - 1) * filter.Size
	if start > total {
		start = total
	}
	end := start + filter.Size
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

func (r *fakeRepo) Get(_ context.Context, id int64) (*auth.User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return user, nil
}

func (r *fakeRepo) Create(_ context.Context, params CreateParams) (*auth.User, error) {
	for _, existing := range r.users {
		if existing.Login == params.Login {
			return nil, ErrDuplicateLogin
		}
	}
	r.nextID++
	now := time.Now().UTC()
	user := &auth.User{
		ID:                 r.nextID,
		Login:              params.Login,
		FullName:           params.FullName,
		Role:               params.Role,
		PasswordHash:       params.PasswordHash,
		BuildingID:         params.BuildingID,
		IsActive:           params.IsActive,
		MustChangePassword: params.MustChangePassword,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if params.BuildingID != nil {
		if name, ok := r.buildingNames[*params.BuildingID]; ok {
			user.BuildingName = &name
		}
	}
	r.users[user.ID] = user
	return user, nil
}

func (r *fakeRepo) Update(_ context.Context, id int64, params UpdateParams) (*auth.User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	resultRole := user.Role
	if params.Role != nil {
		resultRole = *params.Role
	}
	resultActive := user.IsActive
	if params.IsActive != nil {
		resultActive = *params.IsActive
	}
	// AUTH-7: последний активный ADMIN не может потерять роль или активность.
	if user.Role == auth.RoleAdmin && user.IsActive && (resultRole != auth.RoleAdmin || !resultActive) {
		others := 0
		for otherID, other := range r.users {
			if otherID != id && other.Role == auth.RoleAdmin && other.IsActive {
				others++
			}
		}
		if others == 0 {
			return nil, ErrLastAdmin
		}
	}
	if params.FullName != nil {
		user.FullName = *params.FullName
	}
	if params.Role != nil {
		user.Role = *params.Role
	}
	if params.BuildingIDSet {
		user.BuildingID = params.BuildingID
		user.BuildingName = nil
		if params.BuildingID != nil {
			if name, exists := r.buildingNames[*params.BuildingID]; exists {
				user.BuildingName = &name
			}
		}
	}
	if params.IsActive != nil {
		user.IsActive = *params.IsActive
	}
	user.UpdatedAt = time.Now().UTC()
	return user, nil
}

func (r *fakeRepo) SetPassword(_ context.Context, id int64, passwordHash string, mustChange bool) error {
	user, ok := r.users[id]
	if !ok {
		return ErrNotFound
	}
	user.PasswordHash = passwordHash
	user.MustChangePassword = mustChange
	r.setPasswordIDs = append(r.setPasswordIDs, id)
	return nil
}

func (r *fakeRepo) BuildingStatus(_ context.Context, id int64) (bool, bool, error) {
	active, ok := r.buildings[id]
	return ok, active, nil
}

func (r *fakeRepo) ListAssignable(_ context.Context, buildingID int64) ([]*auth.User, error) {
	items := make([]*auth.User, 0, 4)
	for _, user := range r.users {
		if !user.IsActive {
			continue
		}
		if user.Role == auth.RoleAdmin ||
			(user.Role == auth.RoleUser && user.BuildingID != nil && *user.BuildingID == buildingID) {
			items = append(items, user)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

type fakeRevoker struct {
	calls []int64
	err   error
}

func (r *fakeRevoker) RevokeAllRefreshTokens(_ context.Context, userID int64, _ string) error {
	r.calls = append(r.calls, userID)
	return r.err
}

type fakeTicketResolver struct {
	buildingByTicket map[int64]int64
	err              error
}

func (r *fakeTicketResolver) TicketBuildingID(_ context.Context, ticketID int64) (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	buildingID, ok := r.buildingByTicket[ticketID]
	if !ok {
		return 0, ErrTicketNotFound
	}
	return buildingID, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fixture struct {
	repo     *fakeRepo
	revoker  *fakeRevoker
	resolver *fakeTicketResolver
	service  *Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	repo := newFakeRepo()
	revoker := &fakeRevoker{}
	resolver := &fakeTicketResolver{buildingByTicket: map[int64]int64{100: 1, 101: 2}}
	return &fixture{
		repo:     repo,
		revoker:  revoker,
		resolver: resolver,
		service:  NewService(repo, revoker, resolver, discardLogger()),
	}
}

func requireAPIError(t *testing.T, err error, status int, code apierr.Code) *apierr.Error {
	t.Helper()
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("ожидалась *apierr.Error, получено %T: %v", err, err)
	}
	if apiErr.Status != status || apiErr.Code != code {
		t.Fatalf("получено %d %s (%s), ожидалось %d %s",
			apiErr.Status, apiErr.Code, apiErr.Message, status, code)
	}
	return apiErr
}

func int64ptr(value int64) *int64 { return &value }

func TestCreate_UserRequiresActiveBuilding(t *testing.T) {
	f := newFixture(t)

	_, err := f.service.Create(context.Background(), CreateInput{
		Login: "ivanov", FullName: "Иванов Иван", Role: auth.RoleUser, Password: "Password123",
	})
	requireAPIError(t, err, 422, apierr.CodeBuildingRequiredForUser)

	_, err = f.service.Create(context.Background(), CreateInput{
		Login: "petrov", FullName: "Петров Пётр", Role: auth.RoleUser, Password: "Password123",
		BuildingID: int64ptr(2), // здание деактивировано
	})
	requireAPIError(t, err, 422, apierr.CodeBuildingInactive)

	_, err = f.service.Create(context.Background(), CreateInput{
		Login: "sidorov", FullName: "Сидоров Сидор", Role: auth.RoleUser, Password: "Password123",
		BuildingID: int64ptr(999), // здания не существует
	})
	requireAPIError(t, err, 422, apierr.CodeValidation)
}

func TestCreate_UserAndAdmin(t *testing.T) {
	f := newFixture(t)

	user, err := f.service.Create(context.Background(), CreateInput{
		Login: "ivanov", FullName: "  Иванов Иван  ", Role: auth.RoleUser,
		Password: "Password123", BuildingID: int64ptr(1),
	})
	if err != nil {
		t.Fatalf("Create USER: %v", err)
	}
	if user.Login != "ivanov" || user.FullName != "Иванов Иван" {
		t.Fatalf("нормализация полей: %+v", user)
	}
	if user.BuildingID == nil || *user.BuildingID != 1 || user.BuildingName == nil {
		t.Fatalf("здание USER: %+v", user)
	}
	if !user.MustChangePassword || !user.IsActive || user.Role != auth.RoleUser {
		t.Fatalf("флаги USER: %+v", user)
	}
	ok, err := auth.VerifyPassword("Password123", user.PasswordHash)
	if err != nil || !ok {
		t.Fatalf("пароль должен быть argon2id-хешем: ok=%v err=%v", ok, err)
	}

	// AUTH-8: ADMIN создаётся без закрепления, даже если здание передано.
	admin, err := f.service.Create(context.Background(), CreateInput{
		Login: "admin2", FullName: "Админ Второй", Role: auth.RoleAdmin,
		Password: "Password123", BuildingID: int64ptr(1),
	})
	if err != nil {
		t.Fatalf("Create ADMIN: %v", err)
	}
	if admin.BuildingID != nil {
		t.Fatalf("у ADMIN не должно быть здания: %+v", admin.BuildingID)
	}
	if admin.Role != auth.RoleAdmin || !admin.MustChangePassword {
		t.Fatalf("флаги ADMIN: %+v", admin)
	}
}

func TestCreate_ValidationAndDuplicate(t *testing.T) {
	f := newFixture(t)
	f.repo.add("existing", auth.RoleAdmin, nil, true)

	_, err := f.service.Create(context.Background(), CreateInput{
		Login: "ab", FullName: "Короткий логин", Role: auth.RoleUser,
		Password: "Password123", BuildingID: int64ptr(1),
	})
	requireAPIError(t, err, 422, apierr.CodeValidation)

	_, err = f.service.Create(context.Background(), CreateInput{
		Login: "иван", FullName: "Кириллица", Role: auth.RoleUser,
		Password: "Password123", BuildingID: int64ptr(1),
	})
	requireAPIError(t, err, 422, apierr.CodeValidation)

	_, err = f.service.Create(context.Background(), CreateInput{
		Login: "shortpass", FullName: "Короткий пароль", Role: auth.RoleUser,
		Password: "1234567", BuildingID: int64ptr(1),
	})
	requireAPIError(t, err, 422, apierr.CodeValidation)

	_, err = f.service.Create(context.Background(), CreateInput{
		Login: "existing", FullName: "Дубль", Role: auth.RoleUser,
		Password: "Password123", BuildingID: int64ptr(1),
	})
	requireAPIError(t, err, 409, apierr.CodeDuplicateLogin)
}

func TestUpdate_RoleAndBuildingRules(t *testing.T) {
	f := newFixture(t)
	actor := f.repo.add("admin", auth.RoleAdmin, nil, true)
	f.repo.add("admin3", auth.RoleAdmin, nil, true)
	target := f.repo.add("target", auth.RoleAdmin, nil, true)

	// ADMIN→USER без здания → 422.
	roleUser := auth.RoleUser
	_, err := f.service.Update(context.Background(), actor.ID, target.ID, UpdateInput{Role: &roleUser})
	requireAPIError(t, err, 422, apierr.CodeBuildingRequiredForUser)

	// ADMIN→USER с неактивным зданием → 422 BUILDING_INACTIVE.
	buildingTwo := int64ptr(2)
	_, err = f.service.Update(context.Background(), actor.ID, target.ID, UpdateInput{
		Role: &roleUser, BuildingID: OptionalInt64{Set: true, Value: buildingTwo},
	})
	requireAPIError(t, err, 422, apierr.CodeBuildingInactive)

	// ADMIN→USER с активным зданием → успех.
	buildingOne := int64ptr(1)
	updated, err := f.service.Update(context.Background(), actor.ID, target.ID, UpdateInput{
		Role: &roleUser, BuildingID: OptionalInt64{Set: true, Value: buildingOne},
	})
	if err != nil {
		t.Fatalf("ADMIN→USER: %v", err)
	}
	if updated.Role != auth.RoleUser || updated.BuildingID == nil || *updated.BuildingID != 1 {
		t.Fatalf("после ADMIN→USER: %+v", updated)
	}

	// USER→ADMIN очищает здание (AUTH-8).
	roleAdmin := auth.RoleAdmin
	updated, err = f.service.Update(context.Background(), actor.ID, target.ID, UpdateInput{Role: &roleAdmin})
	if err != nil {
		t.Fatalf("USER→ADMIN: %v", err)
	}
	if updated.Role != auth.RoleAdmin || updated.BuildingID != nil {
		t.Fatalf("после USER→ADMIN: %+v", updated)
	}

	// Перевод USER в неактивное здание → 422 BUILDING_INACTIVE.
	user := f.repo.add("userA", auth.RoleUser, int64ptr(1), true)
	_, err = f.service.Update(context.Background(), actor.ID, user.ID, UpdateInput{
		BuildingID: OptionalInt64{Set: true, Value: int64ptr(2)},
	})
	requireAPIError(t, err, 422, apierr.CodeBuildingInactive)
}

func TestUpdate_SelfProtectionAndLastAdmin(t *testing.T) {
	f := newFixture(t)
	admin := f.repo.add("admin", auth.RoleAdmin, nil, true)
	f.repo.add("admin2", auth.RoleAdmin, nil, true)

	// AUTH-7: самоблокировка и самопонижение запрещены.
	block := false
	_, err := f.service.Update(context.Background(), admin.ID, admin.ID, UpdateInput{IsActive: &block})
	requireAPIError(t, err, 409, apierr.CodeCannotModifySelf)

	roleUser := auth.RoleUser
	_, err = f.service.Update(context.Background(), admin.ID, admin.ID, UpdateInput{Role: &roleUser})
	requireAPIError(t, err, 409, apierr.CodeCannotModifySelf)

	// Последний активный ADMIN: второй админ деактивирован, цель — единственный.
	f.repo.users[2].IsActive = false
	_, err = f.service.Update(context.Background(), 999, admin.ID, UpdateInput{IsActive: &block})
	requireAPIError(t, err, 409, apierr.CodeLastAdmin)

	_, err = f.service.Update(context.Background(), 999, admin.ID, UpdateInput{
		Role: &roleUser, BuildingID: OptionalInt64{Set: true, Value: int64ptr(1)},
	})
	requireAPIError(t, err, 409, apierr.CodeLastAdmin)
}

func TestUpdate_BlockRevokesSessions(t *testing.T) {
	f := newFixture(t)
	actor := f.repo.add("admin", auth.RoleAdmin, nil, true)
	target := f.repo.add("userA", auth.RoleUser, int64ptr(1), true)

	block := false
	updated, err := f.service.Update(context.Background(), actor.ID, target.ID, UpdateInput{IsActive: &block})
	if err != nil {
		t.Fatalf("блокировка USER: %v", err)
	}
	if updated.IsActive {
		t.Fatal("пользователь должен быть заблокирован")
	}
	if len(f.revoker.calls) != 1 || f.revoker.calls[0] != target.ID {
		t.Fatalf("ожидался отзыв сессий user_id=%d, получено %v", target.ID, f.revoker.calls)
	}

	// Реактивация не отзывает сессии.
	active := true
	if _, err := f.service.Update(context.Background(), actor.ID, target.ID, UpdateInput{IsActive: &active}); err != nil {
		t.Fatalf("разблокировка: %v", err)
	}
	if len(f.revoker.calls) != 1 {
		t.Fatalf("реактивация не должна отзывать сессии: %v", f.revoker.calls)
	}
}

func TestResetPassword(t *testing.T) {
	f := newFixture(t)
	target := f.repo.add("userA", auth.RoleUser, int64ptr(1), true)

	err := f.service.ResetPassword(context.Background(), 1, target.ID, "short")
	requireAPIError(t, err, 422, apierr.CodeValidation)

	err = f.service.ResetPassword(context.Background(), 1, 999, "NewPassword123")
	requireAPIError(t, err, 404, apierr.CodeNotFound)

	if err := f.service.ResetPassword(context.Background(), 1, target.ID, "NewPassword123"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	updated := f.repo.users[target.ID]
	if !updated.MustChangePassword {
		t.Fatal("после сброса must_change_password должен быть true")
	}
	ok, err := auth.VerifyPassword("NewPassword123", updated.PasswordHash)
	if err != nil || !ok {
		t.Fatalf("хеш нового пароля: ok=%v err=%v", ok, err)
	}
	if len(f.revoker.calls) != 1 || f.revoker.calls[0] != target.ID {
		t.Fatalf("сброс пароля должен отзывать сессии: %v", f.revoker.calls)
	}
}

func TestAssignable(t *testing.T) {
	f := newFixture(t)
	admin := f.repo.add("admin", auth.RoleAdmin, nil, true)
	userA := f.repo.add("userA", auth.RoleUser, int64ptr(1), true)
	userB := f.repo.add("userB", auth.RoleUser, int64ptr(3), true)
	inactive := f.repo.add("userOff", auth.RoleUser, int64ptr(1), false)

	// ADMIN видит USER здания заявки + всех активных ADMIN.
	items, err := f.service.Assignable(context.Background(), admin, 100)
	if err != nil {
		t.Fatalf("Assignable ADMIN: %v", err)
	}
	ids := make(map[int64]bool, len(items))
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids[admin.ID] || !ids[userA.ID] {
		t.Fatalf("в assignable нет admin/userA: %v", ids)
	}
	if ids[userB.ID] || ids[inactive.ID] {
		t.Fatalf("в assignable попали чужие/неактивные: %v", ids)
	}

	// USER получает только себя (TICKET-8: назначает только себя).
	items, err = f.service.Assignable(context.Background(), userA, 100)
	if err != nil {
		t.Fatalf("Assignable USER: %v", err)
	}
	if len(items) != 1 || items[0].ID != userA.ID {
		t.Fatalf("USER должен получить только себя: %+v", items)
	}

	// USER без здания → 409 BUILDING_NOT_ASSIGNED (AUTH-9).
	noBuilding := f.repo.add("homeless", auth.RoleUser, nil, true)
	_, err = f.service.Assignable(context.Background(), noBuilding, 100)
	requireAPIError(t, err, 409, apierr.CodeBuildingNotAssigned)

	// Несуществующая заявка → 404.
	_, err = f.service.Assignable(context.Background(), admin, 555)
	requireAPIError(t, err, 404, apierr.CodeNotFound)

	// Некорректный ticket_id → 422.
	_, err = f.service.Assignable(context.Background(), admin, 0)
	requireAPIError(t, err, 422, apierr.CodeValidation)
}

func TestList_FiltersAndPagination(t *testing.T) {
	f := newFixture(t)
	f.repo.add("admin", auth.RoleAdmin, nil, true)
	f.repo.add("userA", auth.RoleUser, int64ptr(1), true)
	f.repo.add("userB", auth.RoleUser, int64ptr(1), true)
	f.repo.add("userC", auth.RoleUser, int64ptr(3), false)

	result, err := f.service.List(context.Background(), ListFilter{BuildingID: int64ptr(1)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 2 || len(result.Items) != 2 || result.Page != 1 || result.Size != 20 {
		t.Fatalf("фильтр по зданию: %+v", result)
	}

	active := true
	result, err = f.service.List(context.Background(), ListFilter{Role: auth.RoleUser, IsActive: &active})
	if err != nil {
		t.Fatalf("List active users: %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("активные USER: total=%d", result.Total)
	}

	result, err = f.service.List(context.Background(), ListFilter{Search: "userC"})
	if err != nil {
		t.Fatalf("List search: %v", err)
	}
	if result.Total != 1 || result.Items[0].Login != "userC" {
		t.Fatalf("поиск: %+v", result)
	}

	_, err = f.service.List(context.Background(), ListFilter{Size: 500})
	requireAPIError(t, err, 422, apierr.CodeValidation)

	_, err = f.service.List(context.Background(), ListFilter{Role: auth.Role("SUPER")})
	requireAPIError(t, err, 422, apierr.CodeValidation)
}

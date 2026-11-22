package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/users"
)

// fakeRepo — in-memory Repository для проверки правил B5 без MySQL.
type fakeRepo struct {
	tickets       map[int64]*Ticket
	comments      map[int64]*Comment
	buildings     map[int64]Building
	problemTypes  map[int64]bool
	problemNames  map[int64]string
	userNames     map[int64]string
	nextTicketID  int64
	nextCommentID int64
	now           time.Time
}

func newFakeRepo(now time.Time) *fakeRepo {
	return &fakeRepo{
		tickets:      map[int64]*Ticket{},
		comments:     map[int64]*Comment{},
		buildings:    map[int64]Building{1: {ID: 1, Name: "Здание A", IsActive: true}, 2: {ID: 2, Name: "Здание B", IsActive: true}, 3: {ID: 3, Name: "Здание C (неактивно)", IsActive: false}},
		problemTypes: map[int64]bool{10: true, 11: false},
		problemNames: map[int64]string{10: "Не работает оборудование", 11: "Старый вид"},
		userNames:    map[int64]string{1: "Админ", 5: "Иванов Иван", 6: "Петров Пётр"},
		now:          now,
	}
}

func (r *fakeRepo) addTicket(t *Ticket) *Ticket {
	r.nextTicketID++
	t.ID = r.nextTicketID
	if t.BuildingID != 0 {
		t.BuildingName = r.buildings[t.BuildingID].Name
	}
	t.ProblemTypeName = r.problemNames[t.ProblemTypeID]
	if t.Status == "" {
		t.Status = StatusNew
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = r.now
		t.UpdatedAt = r.now
	}
	if t.ExecutorID != nil {
		if name, ok := r.userNames[*t.ExecutorID]; ok {
			t.ExecutorName = &name
		}
	}
	r.tickets[t.ID] = t
	return t
}

func (r *fakeRepo) Create(_ context.Context, params CreateParams) (*Ticket, error) {
	ticket := &Ticket{
		BuildingID:      params.BuildingID,
		ApplicantName:   params.ApplicantName,
		Cabinet:         params.Cabinet,
		ProblemTypeID:   params.ProblemTypeID,
		Description:     params.Description,
		Deadline:        params.Deadline,
		ExecutorID:      params.ExecutorID,
		Status:          StatusNew,
		AccessTokenHash: params.AccessTokenHash,
		CreatedByUserID: params.CreatedByUserID,
	}
	return r.addTicket(ticket), nil
}

func (r *fakeRepo) Get(_ context.Context, id int64) (*Ticket, error) {
	ticket, ok := r.tickets[id]
	if !ok || ticket.DeletedAt != nil {
		return nil, ErrNotFound
	}
	return ticket, nil
}

func (r *fakeRepo) Update(_ context.Context, id int64, params UpdateParams) (*Ticket, error) {
	ticket, ok := r.tickets[id]
	if !ok || ticket.DeletedAt != nil {
		return nil, ErrNotFound
	}
	if params.ApplicantName != nil {
		ticket.ApplicantName = *params.ApplicantName
	}
	if params.Cabinet != nil {
		ticket.Cabinet = *params.Cabinet
	}
	if params.ProblemTypeID != nil {
		ticket.ProblemTypeID = *params.ProblemTypeID
		ticket.ProblemTypeName = r.problemNames[*params.ProblemTypeID]
	}
	if params.Description != nil {
		ticket.Description = *params.Description
	}
	if params.Status != nil {
		ticket.Status = *params.Status
	}
	if params.ExecutorIDSet {
		ticket.ExecutorID = params.ExecutorID
		ticket.ExecutorName = nil
		if params.ExecutorID != nil {
			if name, ok := r.userNames[*params.ExecutorID]; ok {
				ticket.ExecutorName = &name
			}
		}
	}
	if params.DeadlineSet {
		ticket.Deadline = params.Deadline
	}
	if params.BuildingIDSet && params.BuildingID != nil {
		ticket.BuildingID = *params.BuildingID
		ticket.BuildingName = r.buildings[*params.BuildingID].Name
	}
	if params.CompletedAtSet {
		ticket.CompletedAt = params.CompletedAt
	}
	ticket.UpdatedAt = r.now
	return ticket, nil
}

func (r *fakeRepo) SoftDelete(_ context.Context, id int64) error {
	ticket, ok := r.tickets[id]
	if !ok || ticket.DeletedAt != nil {
		return ErrNotFound
	}
	now := r.now
	ticket.DeletedAt = &now
	return nil
}

func (r *fakeRepo) List(_ context.Context, params ListParams) ([]*Ticket, int, error) {
	items := make([]*Ticket, 0, len(r.tickets))
	for _, ticket := range r.tickets {
		if ticket.DeletedAt != nil {
			continue
		}
		if params.BuildingID != nil && ticket.BuildingID != *params.BuildingID {
			continue
		}
		if len(params.Statuses) > 0 && !containsStatus(params.Statuses, ticket.Status) {
			continue
		}
		if params.ExecutorID != nil && (ticket.ExecutorID == nil || *ticket.ExecutorID != *params.ExecutorID) {
			continue
		}
		if params.ExecutorNone && ticket.ExecutorID != nil {
			continue
		}
		if params.ProblemTypeID != nil && ticket.ProblemTypeID != *params.ProblemTypeID {
			continue
		}
		if params.DateFrom != nil && ticket.CreatedAt.Before(*params.DateFrom) {
			continue
		}
		if params.DateTo != nil && !ticket.CreatedAt.Before(*params.DateTo) {
			continue
		}
		if params.Search != "" {
			needle := strings.ToLower(params.Search)
			if !strings.Contains(strings.ToLower(ticket.ApplicantName), needle) &&
				!strings.Contains(strings.ToLower(ticket.Cabinet), needle) {
				continue
			}
		}
		items = append(items, ticket)
	}
	sort.Slice(items, func(i, j int) bool {
		if params.Sort == "deadline" {
			if items[i].Deadline == nil || items[j].Deadline == nil {
				return items[i].ID > items[j].ID
			}
			if params.Order == "asc" {
				return items[i].Deadline.Before(*items[j].Deadline)
			}
			return items[i].Deadline.After(*items[j].Deadline)
		}
		if params.Order == "asc" {
			return items[i].ID < items[j].ID
		}
		return items[i].ID > items[j].ID
	})
	total := len(items)
	start := (params.Page - 1) * params.Size
	if start > total {
		start = total
	}
	end := start + params.Size
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

func (r *fakeRepo) PublicList(_ context.Context, buildingID *int64) ([]*Ticket, error) {
	items := make([]*Ticket, 0, len(r.tickets))
	for _, ticket := range r.tickets {
		if ticket.DeletedAt != nil || !ticket.Status.PublicOnly() {
			continue
		}
		if !r.buildings[ticket.BuildingID].IsActive {
			continue
		}
		if buildingID != nil && ticket.BuildingID != *buildingID {
			continue
		}
		items = append(items, ticket)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID > items[j].ID })
	return items, nil
}

func (r *fakeRepo) PublicBuildings(_ context.Context) ([]*BuildingCount, error) {
	items := make([]*BuildingCount, 0, len(r.buildings))
	for id, building := range r.buildings {
		if !building.IsActive {
			continue
		}
		count := &BuildingCount{ID: id, Name: building.Name}
		for _, ticket := range r.tickets {
			if ticket.BuildingID != id || ticket.DeletedAt != nil {
				continue
			}
			switch ticket.Status {
			case StatusNew:
				count.New++
			case StatusInProgress:
				count.InProgress++
			case StatusDeferred:
				count.Deferred++
			}
		}
		items = append(items, count)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (r *fakeRepo) Building(_ context.Context, id int64) (*Building, error) {
	building, ok := r.buildings[id]
	if !ok {
		return nil, ErrBuildingNotFound
	}
	return &building, nil
}

func (r *fakeRepo) ProblemTypeActive(_ context.Context, id int64) (bool, error) {
	active, ok := r.problemTypes[id]
	if !ok {
		return false, ErrProblemTypeNotFound
	}
	return active, nil
}

func (r *fakeRepo) ByAccessTokenHash(_ context.Context, tokenHash string) (*Ticket, error) {
	for _, ticket := range r.tickets {
		if ticket.AccessTokenHash != nil && *ticket.AccessTokenHash == tokenHash && ticket.DeletedAt == nil {
			return ticket, nil
		}
	}
	return nil, ErrAccessTokenNotFound
}

func (r *fakeRepo) CreateComment(_ context.Context, params CommentParams) (*Comment, error) {
	r.nextCommentID++
	comment := &Comment{
		ID:           r.nextCommentID,
		TicketID:     params.TicketID,
		AuthorUserID: params.AuthorUserID,
		AuthorName:   params.AuthorName,
		Body:         params.Body,
		CreatedAt:    r.now,
	}
	r.comments[comment.ID] = comment
	return comment, nil
}

func (r *fakeRepo) GetComment(_ context.Context, id int64) (*Comment, error) {
	comment, ok := r.comments[id]
	if !ok || comment.DeletedAt != nil {
		return nil, ErrCommentNotFound
	}
	return comment, nil
}

func (r *fakeRepo) ListComments(_ context.Context, ticketID int64, page, size int) ([]*Comment, int, error) {
	items := make([]*Comment, 0, len(r.comments))
	for _, comment := range r.comments {
		if comment.TicketID == ticketID && comment.DeletedAt == nil {
			items = append(items, comment)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	total := len(items)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

func (r *fakeRepo) DeleteComment(_ context.Context, id int64) error {
	delete(r.comments, id)
	return nil
}

func containsStatus(statuses []Status, status Status) bool {
	for _, candidate := range statuses {
		if candidate == status {
			return true
		}
	}
	return false
}

// fakeUsers — UserLookup.
type fakeUsers struct {
	users map[int64]*auth.User
}

func (f *fakeUsers) Get(_ context.Context, id int64) (*auth.User, error) {
	if user, ok := f.users[id]; ok {
		return user, nil
	}
	return nil, users.ErrNotFound
}

// fakeNotifier записывает события (NOTIF-2).
type fakeNotifier struct {
	events []platform.NotificationEvent
}

func (f *fakeNotifier) Publish(_ context.Context, event platform.NotificationEvent) error {
	f.events = append(f.events, event)
	return nil
}

// fakeFiles — FileAttacher (Б3).
type fakeFiles struct {
	validateCalls int
	attachCalls   int
	attached      [][]int64
	validateErr   error
	attachErr     error
}

func (f *fakeFiles) ValidateScope(_ context.Context, fileIDs []int64, _ string) error {
	f.validateCalls++
	return f.validateErr
}

func (f *fakeFiles) ValidateAndAttach(_ context.Context, fileIDs []int64, _ int64) error {
	f.attachCalls++
	f.attached = append(f.attached, fileIDs)
	return f.attachErr
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

const testLayout = "2006-01-02 15:04"

type fixture struct {
	repo     *fakeRepo
	users    *fakeUsers
	notifier *fakeNotifier
	files    *fakeFiles
	service  *Service
	clock    platform.FixedClock
	loc      *time.Location
}

func newFixture(t *testing.T, nowLocal string) *fixture {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	parsed, err := time.ParseInLocation(testLayout, nowLocal, loc)
	if err != nil {
		t.Fatalf("parse now: %v", err)
	}
	clock := platform.FixedClock{Time: parsed}
	calendar := platform.NewDefaultCalendar(loc, platform.DefaultWorkdays(), nil, clock)
	repo := newFakeRepo(parsed.UTC())
	userLookup := &fakeUsers{users: map[int64]*auth.User{
		1: {ID: 1, Login: "admin", FullName: "Админ", Role: auth.RoleAdmin, IsActive: true},
		5: {ID: 5, Login: "ivanov", FullName: "Иванов Иван", Role: auth.RoleUser, IsActive: true, BuildingID: int64Ptr(1)},
		6: {ID: 6, Login: "petrov", FullName: "Петров Пётр", Role: auth.RoleUser, IsActive: true, BuildingID: int64Ptr(1)},
		7: {ID: 7, Login: "sidorov", FullName: "Сидоров", Role: auth.RoleUser, IsActive: true, BuildingID: int64Ptr(2)},
	}}
	notifier := &fakeNotifier{}
	files := &fakeFiles{}
	service := NewService(repo, userLookup, NewStaticSettings(2, calendar), clock, notifier, files, discardLogger())
	return &fixture{repo: repo, users: userLookup, notifier: notifier, files: files, service: service, clock: clock, loc: loc}
}

func (f *fixture) userActor(id int64) *auth.User { return f.users.users[id] }

func int64Ptr(value int64) *int64 { return &value }

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

// TICKET-3/CAL-4/CAL-6: пятница + 2 при Пн–Сб → понедельник.
func TestCreate_DeadlineFridayPlusTwo(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00") // пятница
	ticket, err := f.service.Create(context.Background(), f.userActor(5), CreateInput{
		ApplicantName: "Иванов Иван", Cabinet: "101", ProblemTypeID: 10, Description: "Не работает ПК",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ticket.Deadline == nil {
		t.Fatal("дедлайн не рассчитан")
	}
	if got, want := ticket.Deadline.Format(platform.DateLayout), "2026-11-23"; got != want {
		t.Fatalf("дедлайн = %s, want %s", got, want)
	}
	if ticket.BuildingID != 1 || ticket.Status != StatusNew {
		t.Fatalf("ticket: %+v", ticket)
	}
	if len(f.notifier.events) != 1 || f.notifier.events[0].Type != platform.EventNewTicket {
		t.Fatalf("ожидалось NEW_TICKET: %+v", f.notifier.events)
	}
}

func TestCreate_ValidationAndBuildings(t *testing.T) {
	f := newFixture(t, "2026-11-19 10:00")

	// USER без здания → 409 BUILDING_NOT_ASSIGNED (AUTH-9).
	homeless := &auth.User{ID: 9, Login: "homeless", FullName: "Без здания", Role: auth.RoleUser, IsActive: true}
	_, err := f.service.Create(context.Background(), homeless, CreateInput{
		ApplicantName: "Без здания", Cabinet: "1", ProblemTypeID: 10, Description: "Текст",
	})
	requireAPIError(t, err, 409, apierr.CodeBuildingNotAssigned)

	// USER не может создать заявку в чужом здании (404, TICKET-7).
	_, err = f.service.Create(context.Background(), f.userActor(5), CreateInput{
		BuildingID: int64Ptr(2), ApplicantName: "Иванов", Cabinet: "1", ProblemTypeID: 10, Description: "Текст",
	})
	requireAPIError(t, err, 404, apierr.CodeNotFound)

	// Неактивный вид проблемы → 422.
	_, err = f.service.Create(context.Background(), f.userActor(5), CreateInput{
		ApplicantName: "Иванов", Cabinet: "1", ProblemTypeID: 11, Description: "Текст",
	})
	requireAPIError(t, err, 422, apierr.CodeValidation)

	// ADMIN без building_id → 422.
	_, err = f.service.Create(context.Background(), f.userActor(1), CreateInput{
		ApplicantName: "Иванов", Cabinet: "1", ProblemTypeID: 10, Description: "Текст",
	})
	requireAPIError(t, err, 422, apierr.CodeValidation)

	// Короткое описание → 422.
	_, err = f.service.Create(context.Background(), f.userActor(5), CreateInput{
		ApplicantName: "Иванов", Cabinet: "1", ProblemTypeID: 10, Description: " ",
	})
	requireAPIError(t, err, 422, apierr.CodeValidation)
}

func TestPublicCreate_TokenHashedAndDeadline(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	ticket, token, err := f.service.CreatePublic(context.Background(), PublicCreateInput{
		BuildingID: 1, ApplicantName: "Посетитель", Cabinet: "102", ProblemTypeID: 10, Description: "Сломано",
	})
	if err != nil {
		t.Fatalf("CreatePublic: %v", err)
	}
	if len(token) != 43 {
		t.Fatalf("длина токена = %d, want 43", len(token))
	}
	if ticket.AccessTokenHash == nil || *ticket.AccessTokenHash != auth.HashToken(token) {
		t.Fatal("в репозитории должен храниться SHA-256 токена")
	}
	if ticket.AccessTokenHash != nil && *ticket.AccessTokenHash == token {
		t.Fatal("открытый токен не должен храниться")
	}
	if got := ticket.Deadline.Format(platform.DateLayout); got != "2026-11-23" {
		t.Fatalf("дедлайн = %s", got)
	}
	if len(f.notifier.events) != 1 || f.notifier.events[0].BuildingID == nil || *f.notifier.events[0].BuildingID != 1 {
		t.Fatalf("NEW_TICKET для здания: %+v", f.notifier.events)
	}
}

func TestPublicCreate_InactiveBuilding(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	_, _, err := f.service.CreatePublic(context.Background(), PublicCreateInput{
		BuildingID: 3, ApplicantName: "Гость", Cabinet: "1", ProblemTypeID: 10, Description: "Текст",
	})
	requireAPIError(t, err, 404, apierr.CodeNotFound)

	_, _, err = f.service.CreatePublic(context.Background(), PublicCreateInput{
		BuildingID: 0, ApplicantName: "Гость", Cabinet: "1", ProblemTypeID: 10, Description: "Текст",
	})
	requireAPIError(t, err, 422, apierr.CodeValidation)

	// USER из деактивированного здания не создаёт новые заявки.
	inactiveUser := &auth.User{ID: 8, Login: "old", FullName: "Старый", Role: auth.RoleUser, IsActive: true, BuildingID: int64Ptr(3)}
	f.users.users[8] = inactiveUser
	_, err = f.service.Create(context.Background(), inactiveUser, CreateInput{
		ApplicantName: "Старый", Cabinet: "1", ProblemTypeID: 10, Description: "Текст",
	})
	requireAPIError(t, err, 404, apierr.CodeNotFound)
}

func TestList_UserForcedToOwnBuilding(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A1", Cabinet: "1", ProblemTypeID: 10, Status: StatusNew})
	f.repo.addTicket(&Ticket{BuildingID: 2, ApplicantName: "B1", Cabinet: "2", ProblemTypeID: 10, Status: StatusNew})

	result, err := f.service.List(context.Background(), f.userActor(5), ListFilter{BuildingID: int64Ptr(2)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 1 || result.Items[0].ApplicantName != "A1" {
		t.Fatalf("USER должен видеть только своё здание: %+v", result.Items)
	}

	adminResult, err := f.service.List(context.Background(), f.userActor(1), ListFilter{})
	if err != nil {
		t.Fatalf("List ADMIN: %v", err)
	}
	if adminResult.Total != 2 {
		t.Fatalf("ADMIN должен видеть обе заявки: %d", adminResult.Total)
	}
}

// TICKET-7: чужая заявка → 404.
func TestGet_UserForeignTicketNotFound(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	foreign := f.repo.addTicket(&Ticket{BuildingID: 2, ApplicantName: "B", Cabinet: "2", ProblemTypeID: 10})

	_, err := f.service.Get(context.Background(), f.userActor(5), foreign.ID)
	requireAPIError(t, err, 404, apierr.CodeNotFound)

	if _, err := f.service.Get(context.Background(), f.userActor(1), foreign.ID); err != nil {
		t.Fatalf("ADMIN должен видеть чужую заявку: %v", err)
	}
}

// TICKET-8: USER назначает только себя, снимать исполнителя может ADMIN.
func TestUpdate_AssignRules(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	ticket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})

	other := int64Ptr(6)
	_, _, err := f.service.Update(context.Background(), f.userActor(5), ticket.ID, UpdateInput{
		ExecutorID: OptionalInt64{Set: true, Value: other},
	})
	requireAPIError(t, err, 403, apierr.CodeAssignOtherUser)

	self := int64Ptr(5)
	updated, _, err := f.service.Update(context.Background(), f.userActor(5), ticket.ID, UpdateInput{
		ExecutorID: OptionalInt64{Set: true, Value: self},
	})
	if err != nil {
		t.Fatalf("назначение себя: %v", err)
	}
	if updated.ExecutorID == nil || *updated.ExecutorID != 5 {
		t.Fatalf("executor = %+v", updated.ExecutorID)
	}
	if len(f.notifier.events) != 1 || f.notifier.events[0].Type != platform.EventTicketAssigned {
		t.Fatalf("ожидалось TICKET_ASSIGNED: %+v", f.notifier.events)
	}

	// USER не может снять исполнителя.
	_, _, err = f.service.Update(context.Background(), f.userActor(5), ticket.ID, UpdateInput{
		ExecutorID: OptionalInt64{Set: true, Value: nil},
	})
	requireAPIError(t, err, 403, apierr.CodeAssignOtherUser)

	// ADMIN снимает.
	cleared, _, err := f.service.Update(context.Background(), f.userActor(1), ticket.ID, UpdateInput{
		ExecutorID: OptionalInt64{Set: true, Value: nil},
	})
	if err != nil || cleared.ExecutorID != nil {
		t.Fatalf("ADMIN снял исполнителя: %+v err=%v", cleared.ExecutorID, err)
	}
}

func TestUpdate_AdminExecutorWrongBuilding(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	ticket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})

	wrong := int64Ptr(7) // USER здания 2
	_, _, err := f.service.Update(context.Background(), f.userActor(1), ticket.ID, UpdateInput{
		ExecutorID: OptionalInt64{Set: true, Value: wrong},
	})
	requireAPIError(t, err, 422, apierr.CodeExecutorWrongBuilding)

	admin := int64Ptr(1)
	updated, _, err := f.service.Update(context.Background(), f.userActor(1), ticket.ID, UpdateInput{
		ExecutorID: OptionalInt64{Set: true, Value: admin},
	})
	if err != nil || updated.ExecutorID == nil || *updated.ExecutorID != 1 {
		t.Fatalf("ADMIN-исполнитель: %+v err=%v", updated, err)
	}
}

// TICKET-11: USER правит поля только как исполнитель.
func TestUpdate_UserFieldsOnlyAsExecutor(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	ticket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})
	name := "Новое имя"
	_, _, err := f.service.Update(context.Background(), f.userActor(5), ticket.ID, UpdateInput{
		ApplicantName: &name,
	})
	requireAPIError(t, err, 403, apierr.CodeNotExecutor)

	self := int64Ptr(5)
	f.repo.tickets[ticket.ID].ExecutorID = self
	f.repo.tickets[ticket.ID].ExecutorName = nil
	if _, _, err := f.service.Update(context.Background(), f.userActor(5), ticket.ID, UpdateInput{
		ApplicantName: &name,
	}); err != nil {
		t.Fatalf("исполнитель должен править поля: %v", err)
	}
}

// TICKET-9: смена здания сбрасывает USER-исполнителя и сохраняет ADMIN.
func TestUpdate_BuildingChangeResetsUserExecutor(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	userExecutor := int64Ptr(5)
	ticket := f.repo.addTicket(&Ticket{
		BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10, ExecutorID: userExecutor,
	})
	buildingTwo := int64Ptr(2)
	updated, reset, err := f.service.Update(context.Background(), f.userActor(1), ticket.ID, UpdateInput{
		BuildingID: OptionalInt64{Set: true, Value: buildingTwo},
	})
	if err != nil {
		t.Fatalf("смена здания: %v", err)
	}
	if !reset || updated.ExecutorID != nil {
		t.Fatalf("USER-исполнитель должен сброситься: reset=%v executor=%v", reset, updated.ExecutorID)
	}

	adminExecutor := int64Ptr(1)
	adminTicket := f.repo.addTicket(&Ticket{
		BuildingID: 1, ApplicantName: "B", Cabinet: "2", ProblemTypeID: 10, ExecutorID: adminExecutor,
	})
	updated, reset, err = f.service.Update(context.Background(), f.userActor(1), adminTicket.ID, UpdateInput{
		BuildingID: OptionalInt64{Set: true, Value: buildingTwo},
	})
	if err != nil {
		t.Fatalf("смена здания с ADMIN: %v", err)
	}
	if reset || updated.ExecutorID == nil || *updated.ExecutorID != 1 {
		t.Fatalf("ADMIN-исполнитель должен сохраниться: reset=%v executor=%v", reset, updated.ExecutorID)
	}
}

// TICKET-10: done проставляет completed_at, выход из done обнуляет.
func TestUpdate_StatusCompletedAt(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	ticket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})

	done := StatusDone
	updated, _, err := f.service.Update(context.Background(), f.userActor(5), ticket.ID, UpdateInput{Status: &done})
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if updated.CompletedAt == nil {
		t.Fatal("completed_at должен быть проставлен")
	}

	deferred := StatusDeferred
	updated, _, err = f.service.Update(context.Background(), f.userActor(5), ticket.ID, UpdateInput{Status: &deferred})
	if err != nil {
		t.Fatalf("выход из done: %v", err)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("completed_at должен обнулиться: %v", updated.CompletedAt)
	}
}

func TestUpdate_DeadlineAdminOnly(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	ticket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})

	date := "2020-01-01" // прошедшая дата допустима (TICKET-9)
	_, _, err := f.service.Update(context.Background(), f.userActor(5), ticket.ID, UpdateInput{
		Deadline: OptionalDate{Set: true, Value: &date},
	})
	requireAPIError(t, err, 403, apierr.CodeForbidden)

	updated, _, err := f.service.Update(context.Background(), f.userActor(1), ticket.ID, UpdateInput{
		Deadline: OptionalDate{Set: true, Value: &date},
	})
	if err != nil {
		t.Fatalf("ADMIN deadline: %v", err)
	}
	if got := updated.Deadline.Format(platform.DateLayout); got != date {
		t.Fatalf("deadline = %s, want %s", got, date)
	}

	// Пустой PATCH → 422.
	_, _, err = f.service.Update(context.Background(), f.userActor(1), ticket.ID, UpdateInput{})
	requireAPIError(t, err, 422, apierr.CodeValidation)
}

func TestDelete_SoftAndAdminOnly(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	ticket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})

	err := f.service.Delete(context.Background(), f.userActor(5), ticket.ID)
	requireAPIError(t, err, 403, apierr.CodeForbidden)

	if err := f.service.Delete(context.Background(), f.userActor(1), ticket.ID); err != nil {
		t.Fatalf("ADMIN delete: %v", err)
	}
	if f.repo.tickets[ticket.ID].DeletedAt == nil {
		t.Fatal("заявка должна попасть в корзину")
	}
	if _, err := f.service.Get(context.Background(), f.userActor(1), ticket.ID); err == nil {
		t.Fatal("удалённая заявка не должна читаться")
	}
}

// TICKET-14: пустой текст допустим только с вложением, до 5 файлов.
func TestAddComment_FilesAndValidation(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	ticket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})

	_, err := f.service.AddComment(context.Background(), f.userActor(5), ticket.ID, "   ", nil)
	requireAPIError(t, err, 422, apierr.CodeValidation)

	comment, err := f.service.AddComment(context.Background(), f.userActor(5), ticket.ID, "", []int64{42})
	if err != nil {
		t.Fatalf("комментарий с вложением: %v", err)
	}
	if f.files.validateCalls != 1 || f.files.attachCalls != 1 || len(f.files.attached[0]) != 1 {
		t.Fatalf("FileAttacher: %+v", f.files)
	}
	if comment.AuthorUserID == nil || *comment.AuthorUserID != 5 {
		t.Fatalf("автор комментария: %+v", comment.AuthorUserID)
	}

	_, err = f.service.AddComment(context.Background(), f.userActor(5), ticket.ID, "текст",
		[]int64{1, 2, 3, 4, 5, 6})
	requireAPIError(t, err, 422, apierr.CodeValidation)

	long := strings.Repeat("а", 3001)
	_, err = f.service.AddComment(context.Background(), f.userActor(5), ticket.ID, long, nil)
	requireAPIError(t, err, 422, apierr.CodeValidation)
}

// Ошибка привязки файлов откатывает комментарий.
func TestAddComment_AttachFailureRollsBack(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	ticket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})
	f.files.attachErr = errors.New("attach failed")

	_, err := f.service.AddComment(context.Background(), f.userActor(5), ticket.ID, "текст", []int64{42})
	if err == nil {
		t.Fatal("ожидалась ошибка привязки")
	}
	if len(f.repo.comments) != 0 {
		t.Fatalf("комментарий должен быть откатан: %+v", f.repo.comments)
	}
}

func TestAddPublicComment_AuthorIsApplicant(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	_, token, err := f.service.CreatePublic(context.Background(), PublicCreateInput{
		BuildingID: 1, ApplicantName: "Посетитель", Cabinet: "1", ProblemTypeID: 10, Description: "Текст",
	})
	if err != nil {
		t.Fatalf("CreatePublic: %v", err)
	}
	comment, err := f.service.AddPublicComment(context.Background(), token, "Есть новости?", nil)
	if err != nil {
		t.Fatalf("AddPublicComment: %v", err)
	}
	if comment.AuthorName != "Посетитель" || comment.AuthorUserID != nil {
		t.Fatalf("автор анонимного комментария: %+v", comment)
	}
	// Чужой токен → 404.
	_, err = f.service.AddPublicComment(context.Background(), "wrong-token", "x", nil)
	requireAPIError(t, err, 404, apierr.CodeNotFound)
}

func TestPublicBoardAndBuildings(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	newTicket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10, Status: StatusNew, Description: "секрет"})
	f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "B", Cabinet: "2", ProblemTypeID: 10, Status: StatusInProgress})
	f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "C", Cabinet: "3", ProblemTypeID: 10, Status: StatusDeferred})
	f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "D", Cabinet: "4", ProblemTypeID: 10, Status: StatusDone})
	deleted := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "E", Cabinet: "5", ProblemTypeID: 10, Status: StatusNew})
	now := f.repo.now
	deleted.DeletedAt = &now
	f.repo.addTicket(&Ticket{BuildingID: 3, ApplicantName: "F", Cabinet: "6", ProblemTypeID: 10, Status: StatusNew})

	board, err := f.service.PublicBoard(context.Background(), int64Ptr(1))
	if err != nil {
		t.Fatalf("PublicBoard: %v", err)
	}
	if len(board.Items) != 3 {
		t.Fatalf("публичный дашборд: %d заявок, want 3 (done/удалённые скрыты)", len(board.Items))
	}
	card := f.service.PublicCard(newTicket)
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	if strings.Contains(string(raw), "description") {
		t.Fatalf("публичная карточка не должна содержать description: %s", raw)
	}

	// Неактивное здание → 404.
	_, err = f.service.PublicBoard(context.Background(), int64Ptr(3))
	requireAPIError(t, err, 404, apierr.CodeNotFound)

	counts, err := f.service.PublicBuildings(context.Background())
	if err != nil {
		t.Fatalf("PublicBuildings: %v", err)
	}
	if len(counts) != 2 {
		t.Fatalf("ожидалось 2 активных здания: %+v", counts)
	}
	for _, count := range counts {
		if count.ID != 1 {
			continue
		}
		if count.New != 1 || count.InProgress != 1 || count.Deferred != 1 {
			t.Fatalf("счётчики здания 1: %+v", count)
		}
	}
}

func TestPublicTicket_WrongToken(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	if _, _, err := f.service.PublicTicket(context.Background(), "unknown"); err == nil {
		t.Fatal("чужой токен должен давать 404")
	} else {
		requireAPIError(t, err, 404, apierr.CodeNotFound)
	}
}

func TestIsOverdue(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00") // пятница
	yesterday := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	today := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	tomorrow := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		ticket *Ticket
		want   bool
	}{
		{"вчера new", &Ticket{Status: StatusNew, Deadline: &yesterday}, true},
		{"сегодня", &Ticket{Status: StatusNew, Deadline: &today}, false},
		{"завтра", &Ticket{Status: StatusInProgress, Deadline: &tomorrow}, false},
		{"done не просрочена", &Ticket{Status: StatusDone, Deadline: &yesterday}, false},
		{"без дедлайна", &Ticket{Status: StatusNew}, false},
	}
	for _, tc := range cases {
		if got := f.service.isOverdue(tc.ticket); got != tc.want {
			t.Fatalf("%s: isOverdue=%v, want %v", tc.name, got, tc.want)
		}
	}
}

// NOTIF-2: NEW_COMMENT уходит исполнителю и автору-сотруднику, кроме автора.
func TestNotifyCommentRecipients(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	executor := int64Ptr(6)
	author := int64Ptr(5)
	ticket := f.repo.addTicket(&Ticket{
		BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10,
		ExecutorID: executor, CreatedByUserID: author,
	})
	if _, err := f.service.AddComment(context.Background(), f.userActor(5), ticket.ID, "текст", nil); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	found := false
	for _, event := range f.notifier.events {
		if event.Type != platform.EventNewComment {
			continue
		}
		found = true
		if len(event.UserIDs) != 1 || event.UserIDs[0] != 6 {
			t.Fatalf("получатели NEW_COMMENT: %v", event.UserIDs)
		}
		if event.ActorUserID != 5 {
			t.Fatalf("actor NEW_COMMENT = %d", event.ActorUserID)
		}
	}
	if !found {
		t.Fatalf("событие NEW_COMMENT не отправлено: %+v", f.notifier.events)
	}
}

// Ф2: `executor_id=none` — только заявки без исполнителя (executor_id IS NULL).
func TestList_ExecutorNoneFilter(t *testing.T) {
	f := newFixture(t, "2026-11-20 10:00")
	executor := int64Ptr(5)
	f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "Без исполнителя", Cabinet: "1", ProblemTypeID: 10})
	f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "С исполнителем", Cabinet: "2", ProblemTypeID: 10, ExecutorID: executor})

	result, err := f.service.List(context.Background(), f.userActor(1), ListFilter{ExecutorNone: true})
	if err != nil {
		t.Fatalf("List executor_id=none: %v", err)
	}
	if result.Total != 1 || result.Items[0].ApplicantName != "Без исполнителя" {
		t.Fatalf("фильтр none: %+v", result.Items)
	}

	// Одновременно id и none — противоречие → 422.
	_, err = f.service.List(context.Background(), f.userActor(1), ListFilter{
		ExecutorID: executor, ExecutorNone: true,
	})
	requireAPIError(t, err, 422, apierr.CodeValidation)
}

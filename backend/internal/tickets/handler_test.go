package tickets

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

func newTestHandler(t *testing.T) (*Handler, *fixture) {
	t.Helper()
	f := newFixture(t, "2026-11-20 10:00")
	return NewHandler(f.service, discardLogger()), f
}

func jsonBodyTest(t *testing.T, value any) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return bytes.NewReader(data)
}

func perform(handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func withRouteParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func withActor(req *http.Request, actor *auth.User) *http.Request {
	return req.WithContext(auth.WithUser(req.Context(), actor))
}

// TICKET-5: публичный дашборд не содержит description.
func TestHandler_PublicListHidesDescription(t *testing.T) {
	handler, f := newTestHandler(t)
	f.repo.addTicket(&Ticket{
		BuildingID: 1, ApplicantName: "Иванов", Cabinet: "101",
		ProblemTypeID: 10, Status: StatusNew, Description: "секретное описание",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/public/tickets", nil)
	rec := perform(handler.PublicList, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "description") || strings.Contains(rec.Body.String(), "секретное") {
		t.Fatalf("публичный ответ раскрывает description: %s", rec.Body.String())
	}
	var body PublicTicketsResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].ApplicantName != "Иванов" {
		t.Fatalf("items: %+v", body.Items)
	}
	if body.Items[0].StatusLabel == "" || body.Items[0].StatusColor == "" {
		t.Fatalf("status label/color: %+v", body.Items[0])
	}
}

// TICKET-13: создание анонимной заявки возвращает токен один раз.
func TestHandler_PublicCreateReturnsToken(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/public/tickets", jsonBodyTest(t, map[string]any{
		"building_id": 1, "applicant_name": "Гость", "cabinet": "102",
		"problem_type_id": 10, "description": "Не работает",
	}))
	rec := perform(handler.PublicCreate, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var body PublicTicketCreateResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(body.AccessToken) != 43 || !strings.HasPrefix(body.AccessURL, "/t/") {
		t.Fatalf("токен/ссылка: %q %q", body.AccessToken, body.AccessURL)
	}
	if body.Ticket.Description != "Не работает" {
		t.Fatalf("detail description: %q", body.Ticket.Description)
	}
}

// TICKET-7: USER чужого здания получает 404.
func TestHandler_UserForeignTicket404(t *testing.T) {
	handler, f := newTestHandler(t)
	foreign := f.repo.addTicket(&Ticket{BuildingID: 2, ApplicantName: "B", Cabinet: "2", ProblemTypeID: 10})

	req := httptest.NewRequest(http.MethodGet, "/api/tickets/1", nil)
	req = withRouteParam(req, "id", "1")
	req = withActor(req, f.userActor(5))
	rec := perform(handler.Get, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d (%s), ticket=%d", rec.Code, rec.Body.String(), foreign.ID)
	}
}

// TICKET-9: PATCH здания возвращает executor_reset=true и сбрасывает USER-исполнителя.
func TestHandler_UpdateExecutorResetFlag(t *testing.T) {
	handler, f := newTestHandler(t)
	f.repo.addTicket(&Ticket{
		BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10,
		ExecutorID: int64Ptr(5),
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/tickets/1",
		jsonBodyTest(t, map[string]any{"building_id": 2}))
	req = withRouteParam(req, "id", "1")
	req = withActor(req, f.userActor(1))
	rec := perform(handler.Update, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var body TicketUpdateResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !body.ExecutorReset || body.Ticket.ExecutorID != nil {
		t.Fatalf("executor_reset=%v executor=%v", body.ExecutorReset, body.Ticket.ExecutorID)
	}
}

func TestHandler_InvalidPagination(t *testing.T) {
	handler, f := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/tickets?size=500", nil)
	req = withActor(req, f.userActor(5))
	rec := perform(handler.List, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("тело ошибки: %s", rec.Body.String())
	}
}

func TestHandler_CommentEmptyText422(t *testing.T) {
	handler, f := newTestHandler(t)
	ticket := f.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/1/comments",
		jsonBodyTest(t, map[string]any{"text": "   ", "file_ids": []int{}}))
	req = withRouteParam(req, "id", "1")
	req = withActor(req, f.userActor(5))
	rec := perform(handler.CreateComment, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%s), ticket=%d", rec.Code, rec.Body.String(), ticket.ID)
	}
}

func TestHandler_PublicGetWrongToken404(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/public/tickets/bad-token", nil)
	req = withRouteParam(req, "token", "bad-token")
	rec := perform(handler.PublicGet, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
}

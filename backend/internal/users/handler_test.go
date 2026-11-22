package users

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

func newTestHandler(t *testing.T) (*Handler, *fakeRepo) {
	t.Helper()
	f := newFixture(t)
	return NewHandler(f.service, discardLogger()), f.repo
}

func jsonBody(t *testing.T, value any) *bytes.Reader {
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
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

func withUser(req *http.Request, user *auth.User) *http.Request {
	return req.WithContext(auth.WithUser(req.Context(), user))
}

func decodeJSONBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("некорректный JSON: %v (%s)", err, rec.Body.String())
	}
}

func TestHandler_CreateValidationEnvelope(t *testing.T) {
	handler, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", jsonBody(t, map[string]any{
		"login": "ivanov", "full_name": "Иванов Иван", "role": "USER", "password": "Password123",
	}))
	rec := perform(handler.Create, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	decodeJSONBody(t, rec, &body)
	if body["code"] != "BUILDING_REQUIRED_FOR_USER" {
		t.Fatalf("code = %v", body["code"])
	}
	if _, ok := body["request_id"]; !ok {
		t.Fatal("в ошибке нет request_id")
	}
	if details, ok := body["details"].(map[string]any); !ok || details["building_id"] == nil {
		t.Fatalf("details.building_id: %v", body["details"])
	}
}

func TestHandler_CreateAndList(t *testing.T) {
	handler, repo := newTestHandler(t)
	admin := repo.add("admin", auth.RoleAdmin, nil, true)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", jsonBody(t, map[string]any{
		"login": "ivanov", "full_name": "Иванов Иван", "role": "USER",
		"password": "Password123", "building_id": 1,
	}))
	rec := perform(handler.Create, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password_hash") {
		t.Fatal("ответ не должен содержать password_hash")
	}
	var created auth.UserDTO
	decodeJSONBody(t, rec, &created)
	if created.Login != "ivanov" || !created.MustChangePassword || created.BuildingID == nil {
		t.Fatalf("create response: %+v", created)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/users?role=USER&building_id=1", nil)
	req = withUser(req, admin)
	rec = perform(handler.List, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d (%s)", rec.Code, rec.Body.String())
	}
	var list UserListDTO
	decodeJSONBody(t, rec, &list)
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Login != "ivanov" {
		t.Fatalf("list: %+v", list)
	}
	if list.Page != 1 || list.Size != 20 {
		t.Fatalf("pagination defaults: page=%d size=%d", list.Page, list.Size)
	}
}

func TestHandler_UpdateSelfForbidden(t *testing.T) {
	handler, repo := newTestHandler(t)
	admin := repo.add("admin", auth.RoleAdmin, nil, true)

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/1", jsonBody(t, map[string]any{
		"is_active": false,
	}))
	req = withRouteParam(req, "id", "1")
	req = withUser(req, admin)
	rec := perform(handler.Update, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	decodeJSONBody(t, rec, &body)
	if body["code"] != "CANNOT_MODIFY_SELF" {
		t.Fatalf("code = %v", body["code"])
	}
}

func TestHandler_Assignable(t *testing.T) {
	handler, repo := newTestHandler(t)
	user := repo.add("userA", auth.RoleUser, int64ptr(1), true)

	req := httptest.NewRequest(http.MethodGet, "/api/users/assignable?ticket_id=100", nil)
	req = withUser(req, user)
	rec := perform(handler.Assignable, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var items []AssignableUserDTO
	decodeJSONBody(t, rec, &items)
	if len(items) != 1 || items[0].ID != user.ID {
		t.Fatalf("USER должен получить только себя: %+v", items)
	}

	// Без обязательного ticket_id → 422.
	req = httptest.NewRequest(http.MethodGet, "/api/users/assignable", nil)
	req = withUser(req, user)
	rec = perform(handler.Assignable, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("без ticket_id status = %d, want 422", rec.Code)
	}
}

func TestHandler_InvalidPagination(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?size=500", nil)
	rec := perform(handler.List, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("тело ошибки: %s", rec.Body.String())
	}
}

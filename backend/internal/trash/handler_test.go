package trash

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// newHandlerRouter собирает минимальный роутер корзины (как в httpserver).
func newHandlerRouter(t *testing.T, repo *fakeRepo) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := NewService(repo, nil, platform.FixedClock{Time: time.Now()}, logger)
	handler := NewHandler(service, logger)

	router := chi.NewRouter()
	router.Get("/api/admin/trash", handler.List)
	router.Post("/api/admin/trash/{type}/{id}/restore", handler.Restore)
	router.Delete("/api/admin/trash/{type}/{id}", handler.Purge)
	return router
}

func doRequest(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeMap(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON: %v (%s)", err, recorder.Body.String())
	}
	return body
}

// TRASH-1: список корзины — контрактная форма {items,total,page,size}.
func TestHandlerList(t *testing.T) {
	repo := newFakeRepo()
	repo.items[TypeTicket] = []Item{{
		Type:            TypeTicket,
		ID:              3,
		Title:           "#3 · Иванов · каб. 101 · Здание A",
		DeletedAt:       time.Date(2026, time.November, 20, 10, 0, 0, 0, time.UTC),
		DeletedByUserID: int64Ptr(1),
	}}
	repo.totals[TypeTicket] = 1
	handler := newHandlerRouter(t, repo)

	recorder := doRequest(handler, http.MethodGet, "/api/admin/trash?type=ticket&page=1&size=50")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	body := decodeMap(t, recorder)
	if body["total"] != float64(1) || body["page"] != float64(1) || body["size"] != float64(50) {
		t.Fatalf("body = %v", body)
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %v", body["items"])
	}
	item := items[0].(map[string]any)
	for _, field := range []string{"type", "id", "title", "deleted_at", "deleted_by_user_id",
		"deleted_by_name", "building_id", "building_name"} {
		if _, ok := item[field]; !ok {
			t.Fatalf("в элементе нет поля %q: %v", field, item)
		}
	}
}

// Валидация параметров списка: type обязателен, page/size проверяются.
func TestHandlerListValidation(t *testing.T) {
	handler := newHandlerRouter(t, newFakeRepo())

	tests := []struct {
		target string
		field  string
	}{
		{"/api/admin/trash", "type"},
		{"/api/admin/trash?type=unknown", "type"},
		{"/api/admin/trash?type=ticket&page=0", "page"},
		{"/api/admin/trash?type=ticket&size=101", "size"},
		{"/api/admin/trash?type=ticket&size=abc", "size"},
	}
	for _, tc := range tests {
		recorder := doRequest(handler, http.MethodGet, tc.target)
		if recorder.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status = %d, want 422", tc.target, recorder.Code)
		}
		body := decodeMap(t, recorder)
		if body["code"] != "VALIDATION_ERROR" {
			t.Fatalf("%s: code = %v", tc.target, body["code"])
		}
		details, _ := body["details"].(map[string]any)
		if details[tc.field] == nil {
			t.Fatalf("%s: нет details.%s: %v", tc.target, tc.field, details)
		}
	}
}

// TRASH-2: восстановление — 200, type/id/restored и массив warnings.
func TestHandlerRestore(t *testing.T) {
	repo := newFakeRepo()
	repo.ticketInfo = TicketRestore{BuildingID: 1, BuildingName: "Здание A", BuildingActive: false}
	handler := newHandlerRouter(t, repo)

	recorder := doRequest(handler, http.MethodPost, "/api/admin/trash/ticket/5/restore")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	body := decodeMap(t, recorder)
	if body["type"] != "ticket" || body["id"] != float64(5) || body["restored"] != true {
		t.Fatalf("body = %v", body)
	}
	warnings, ok := body["warnings"].([]any)
	if !ok || len(warnings) != 1 {
		t.Fatalf("warnings = %v", body["warnings"])
	}

	// Неверный тип/id — 404/422.
	if rec := doRequest(handler, http.MethodPost, "/api/admin/trash/unknown/5/restore"); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("неизвестный тип: status = %d", rec.Code)
	}
	if rec := doRequest(handler, http.MethodPost, "/api/admin/trash/ticket/abc/restore"); rec.Code != http.StatusNotFound {
		t.Fatalf("некорректный id: status = %d", rec.Code)
	}
}

// TRASH-3: физическое удаление — 204 без тела.
func TestHandlerPurge(t *testing.T) {
	repo := newFakeRepo()
	repo.purgeFiles = []string{"2026/11/a.png"}
	handler := newHandlerRouter(t, repo)

	recorder := doRequest(handler, http.MethodDelete, "/api/admin/trash/equipment/7")
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("тело ответа 204 должно быть пустым: %q", recorder.Body.String())
	}
	if len(repo.purged[TypeEquipment]) != 1 {
		t.Fatalf("purged = %v", repo.purged)
	}

	repo.purgeErr = ErrNotFound
	recorder = doRequest(handler, http.MethodDelete, "/api/admin/trash/equipment/99")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

// warnings всегда массив, даже когда их нет.
func TestHandlerRestoreNoWarnings(t *testing.T) {
	repo := newFakeRepo()
	repo.ticketInfo = TicketRestore{BuildingID: 1, BuildingName: "Здание", BuildingActive: true}
	handler := newHandlerRouter(t, repo)

	recorder := doRequest(handler, http.MethodPost, "/api/admin/trash/ticket/1/restore")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"warnings":[]`)) {
		t.Fatalf("warnings должен быть пустым массивом: %s", recorder.Body.String())
	}
}

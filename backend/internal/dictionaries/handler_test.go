package dictionaries

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// newHandlerRouter собирает минимальный роутер справочников (как в httpserver).
func newHandlerRouter(t *testing.T, repo *fakeRepo) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(NewService(repo), logger)

	router := chi.NewRouter()
	router.Get("/api/public/problem-types", handler.PublicProblemTypes)
	router.Get("/api/problem-types", handler.ListProblemTypes)
	router.Get("/api/equipment-types", handler.ListEquipmentTypes)
	router.Get("/api/admin/problem-types", handler.ListProblemTypes)
	router.Post("/api/admin/problem-types", handler.CreateProblemType)
	router.Patch("/api/admin/problem-types/{id}", handler.UpdateProblemType)
	router.Delete("/api/admin/problem-types/{id}", handler.DeleteProblemType)
	router.Get("/api/admin/equipment-types", handler.ListEquipmentTypes)
	router.Post("/api/admin/equipment-types", handler.CreateEquipmentType)
	router.Patch("/api/admin/equipment-types/{id}", handler.UpdateEquipmentType)
	router.Delete("/api/admin/equipment-types/{id}", handler.DeleteEquipmentType)
	return router
}

func doJSON(t *testing.T, handler http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, target, reader)
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

// GET /api/public/problem-types: только активные и только id/name/sort_order.
func TestHandlerPublicProblemTypes(t *testing.T) {
	repo := newFakeRepo()
	repo.items[1].IsActive = false
	handler := newHandlerRouter(t, repo)

	recorder := doJSON(t, handler, http.MethodGet, "/api/public/problem-types", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(items) != 1 || items[0]["name"] != "Не работает оборудование" {
		t.Fatalf("items = %v", items)
	}
	if len(items[0]) != 3 {
		t.Fatalf("публичный элемент содержит лишние поля: %v", items[0])
	}
	for _, field := range []string{"id", "name", "sort_order"} {
		if _, ok := items[0][field]; !ok {
			t.Fatalf("нет поля %q: %v", field, items[0])
		}
	}
}

// Без аутентификации include_inactive игнорируется: отдаются только активные.
func TestHandlerListWithoutUserIgnoresIncludeInactive(t *testing.T) {
	repo := newFakeRepo()
	repo.items[1].IsActive = false
	handler := newHandlerRouter(t, repo)

	recorder := doJSON(t, handler, http.MethodGet, "/api/problem-types?include_inactive=true", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var items []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %v", items)
	}
	if len(repo.listAll) == 0 || repo.listAll[len(repo.listAll)-1] {
		t.Fatalf("include_inactive должен игнорироваться: %v", repo.listAll)
	}
}

// Некорректный include_inactive → 422.
func TestHandlerListInvalidIncludeInactive(t *testing.T) {
	handler := newHandlerRouter(t, newFakeRepo())
	recorder := doJSON(t, handler, http.MethodGet, "/api/problem-types?include_inactive=maybe", nil)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := decodeMap(t, recorder)["code"]; code != "VALIDATION_ERROR" {
		t.Fatalf("code = %v", code)
	}
}

// CRUD: 201/422/200/409/204/404.
func TestHandlerDictionaryCRUD(t *testing.T) {
	repo := newFakeRepo()
	handler := newHandlerRouter(t, repo)

	created := doJSON(t, handler, http.MethodPost, "/api/admin/equipment-types", map[string]any{"name": "Принтер", "sort_order": 3})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d (%s)", created.Code, created.Body.String())
	}
	body := decodeMap(t, created)
	if body["name"] != "Принтер" || body["is_active"] != true || body["sort_order"] != float64(3) {
		t.Fatalf("body = %v", body)
	}

	invalid := doJSON(t, handler, http.MethodPost, "/api/admin/equipment-types", map[string]any{"name": "  "})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid status = %d", invalid.Code)
	}
	if details, _ := decodeMap(t, invalid)["details"].(map[string]any); details["name"] == nil {
		t.Fatalf("нет details.name: %v", details)
	}

	conflict := doJSON(t, handler, http.MethodPost, "/api/admin/equipment-types", map[string]any{"name": "Прочее"})
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d", conflict.Code)
	}

	renamed := doJSON(t, handler, http.MethodPatch, "/api/admin/problem-types/1", map[string]any{"name": "Сломалось"})
	if renamed.Code != http.StatusOK {
		t.Fatalf("patch status = %d (%s)", renamed.Code, renamed.Body.String())
	}
	if name := decodeMap(t, renamed)["name"]; name != "Сломалось" {
		t.Fatalf("patch name = %v", name)
	}

	if rec := doJSON(t, handler, http.MethodPatch, "/api/admin/problem-types/999", map[string]any{"name": "Нет"}); rec.Code != http.StatusNotFound {
		t.Fatalf("patch missing status = %d", rec.Code)
	}

	if rec := doJSON(t, handler, http.MethodDelete, "/api/admin/problem-types/1", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}

	// После DELETE элемент остаётся в БД, но деактивирован.
	if repo.items[0].IsActive {
		t.Fatal("DELETE должен деактивировать элемент, а не удалять его")
	}
	if len(repo.items) != 3 {
		t.Fatalf("items = %v", repo.items)
	}
}

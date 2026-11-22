package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// newHandlerRouter собирает минимальный роутер Б7 (как в httpserver) для
// проверки HTTP-контракта настроек.
func newHandlerRouter(t *testing.T, repo *fakeRepo) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	location := yekaterinburg(t)
	service := NewService(repo, platform.FixedClock{Time: time.Date(2026, time.November, 19, 9, 0, 0, 0, location)}, logger)
	if err := service.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	handler := NewHandler(service, logger)

	router := chi.NewRouter()
	router.Get("/api/public/settings", handler.Public)
	router.Get("/api/admin/settings", handler.AdminGet)
	router.Put("/api/admin/settings", handler.Update)
	router.Get("/api/admin/holidays", handler.ListHolidays)
	router.Post("/api/admin/holidays", handler.CreateHoliday)
	router.Patch("/api/admin/holidays/{id}", handler.UpdateHoliday)
	router.Delete("/api/admin/holidays/{id}", handler.DeleteHoliday)
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

// PUT /api/admin/settings: 200 и полный DTO контракта.
func TestHandlerUpdateSettings(t *testing.T) {
	handler := newHandlerRouter(t, newFakeRepo())

	recorder := doJSON(t, handler, http.MethodPut, "/api/admin/settings", validSettingsRequest())
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	body := decodeMap(t, recorder)
	for _, field := range []string{"app_name", "deadline_days", "workdays", "timezone",
		"deadline_reminder_time", "push_enabled", "statuses"} {
		if _, ok := body[field]; !ok {
			t.Fatalf("в ответе нет поля %q: %v", field, body)
		}
	}
	statuses, ok := body["statuses"].([]any)
	if !ok || len(statuses) != 4 {
		t.Fatalf("statuses = %v", body["statuses"])
	}
	first, _ := statuses[0].(map[string]any)
	if first["code"] != "new" || first["label"] == "" || first["color"] == "" {
		t.Fatalf("первый статус = %v", statuses[0])
	}
}

// Ошибки PUT: 422 VALIDATION_ERROR с details по полям.
func TestHandlerUpdateSettingsValidation(t *testing.T) {
	handler := newHandlerRouter(t, newFakeRepo())

	tests := []struct {
		name   string
		mutate func(*SettingsUpdateRequest)
		field  string
	}{
		{"deadline 0", func(r *SettingsUpdateRequest) { r.DeadlineDays = 0 }, "deadline_days"},
		{"timezone", func(r *SettingsUpdateRequest) { r.Timezone = "Europe/Mordor" }, "timezone"},
		{"reminder", func(r *SettingsUpdateRequest) { r.DeadlineReminderTime = "25:00" }, "deadline_reminder_time"},
		{"workdays", func(r *SettingsUpdateRequest) { r.Workdays = []int{} }, "workdays"},
		{"statuses", func(r *SettingsUpdateRequest) { r.Statuses = nil }, "statuses"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := validSettingsRequest()
			tc.mutate(&request)
			recorder := doJSON(t, handler, http.MethodPut, "/api/admin/settings", request)
			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
			}
			body := decodeMap(t, recorder)
			if body["code"] != "VALIDATION_ERROR" {
				t.Fatalf("code = %v", body["code"])
			}
			details, ok := body["details"].(map[string]any)
			if !ok {
				t.Fatalf("details = %v", body["details"])
			}
			if _, ok := details[tc.field]; !ok {
				t.Fatalf("нет details.%s: %v", tc.field, details)
			}
		})
	}
}

// GET /api/admin/settings отдаёт сохранённые значения.
func TestHandlerGetSettings(t *testing.T) {
	handler := newHandlerRouter(t, newFakeRepo())
	recorder := doJSON(t, handler, http.MethodGet, "/api/admin/settings", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := decodeMap(t, recorder)
	if body["deadline_days"] != float64(2) || body["timezone"] != "Asia/Yekaterinburg" {
		t.Fatalf("body = %v", body)
	}
}

// GET /api/public/settings: только app_name и statuses (без календаря/пояса/push).
func TestHandlerPublicSettingsSafeSubset(t *testing.T) {
	handler := newHandlerRouter(t, newFakeRepo())
	recorder := doJSON(t, handler, http.MethodGet, "/api/public/settings", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := decodeMap(t, recorder)
	if len(body) != 2 {
		t.Fatalf("публичный ответ содержит лишние поля: %v", body)
	}
	if _, ok := body["app_name"]; !ok {
		t.Fatalf("нет app_name: %v", body)
	}
	statuses, ok := body["statuses"].([]any)
	if !ok || len(statuses) != 4 {
		t.Fatalf("statuses = %v", body["statuses"])
	}
	for _, forbidden := range []string{"timezone", "deadline_days", "push_enabled", "workdays"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("публичный ответ раскрывает %q", forbidden)
		}
	}
}

// CRUD праздников через HTTP: 201/409/422/200/204/404.
func TestHandlerHolidaysCRUD(t *testing.T) {
	handler := newHandlerRouter(t, newFakeRepo())

	created := doJSON(t, handler, http.MethodPost, "/api/admin/holidays", map[string]string{"date": "2026-11-20"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d (%s)", created.Code, created.Body.String())
	}
	body := decodeMap(t, created)
	if body["date"] != "2026-11-20" {
		t.Fatalf("date = %v", body["date"])
	}
	id := int64(body["id"].(float64))

	duplicate := doJSON(t, handler, http.MethodPost, "/api/admin/holidays", map[string]string{"date": "2026-11-20"})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d", duplicate.Code)
	}
	if code := decodeMap(t, duplicate)["code"]; code != "CONFLICT" {
		t.Fatalf("duplicate code = %v", code)
	}

	badDate := doJSON(t, handler, http.MethodPost, "/api/admin/holidays", map[string]string{"date": "20.11.2026"})
	if badDate.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad date status = %d", badDate.Code)
	}
	if details, _ := decodeMap(t, badDate)["details"].(map[string]any); details["date"] == nil {
		t.Fatalf("нет details.date: %v", details)
	}

	list := doJSON(t, handler, http.MethodGet, "/api/admin/holidays?year=2026", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d", list.Code)
	}
	var items []map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &items); err != nil || len(items) != 1 {
		t.Fatalf("list = %s (%v)", list.Body.String(), err)
	}

	if rec := doJSON(t, handler, http.MethodGet, "/api/admin/holidays?year=1999", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad year status = %d", rec.Code)
	}

	updated := doJSON(t, handler, http.MethodPatch, "/api/admin/holidays/"+strconv.FormatInt(id, 10), map[string]string{"date": "2026-11-21"})
	if updated.Code != http.StatusOK {
		t.Fatalf("patch status = %d (%s)", updated.Code, updated.Body.String())
	}
	if date := decodeMap(t, updated)["date"]; date != "2026-11-21" {
		t.Fatalf("patch date = %v", date)
	}

	if rec := doJSON(t, handler, http.MethodPatch, "/api/admin/holidays/999", map[string]string{"date": "2026-11-22"}); rec.Code != http.StatusNotFound {
		t.Fatalf("patch missing status = %d", rec.Code)
	}

	if rec := doJSON(t, handler, http.MethodDelete, "/api/admin/holidays/"+strconv.FormatInt(id, 10), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if rec := doJSON(t, handler, http.MethodDelete, "/api/admin/holidays/"+strconv.FormatInt(id, 10), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("повторный delete status = %d", rec.Code)
	}
}

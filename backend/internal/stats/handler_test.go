package stats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// newTestHandler собирает handler с фейковым сервисом и подменённым источником
// пользователя (без JWT/MySQL).
func newTestHandler(t *testing.T, repo *fakeRepo, actor *auth.User) *Handler {
	t.Helper()
	handler := NewHandler(newTestService(t, repo), discardLogger())
	handler.UserFrom = func(context.Context) (*auth.User, bool) {
		if actor == nil {
			return nil, false
		}
		return actor, true
	}
	return handler
}

// decodeStats разбирает одиночный ответ статистики.
func decodeStats(t *testing.T, rec *httptest.ResponseRecorder) StatsResponse {
	t.Helper()
	var response StatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, rec.Body.String())
	}
	return response
}

// decodeErrorCode достаёт код ошибки из единого формата.
func decodeErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, rec.Body.String())
	}
	return body.Code
}

// GET /api/stats/me: метрики своей статистики и формат периода (STATS-1).
func TestHandler_Me(t *testing.T) {
	deadline := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	repo := newFakeRepo().withTickets(
		ticketFor(1, 5, time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC), 2*time.Hour, &deadline, "Электрика"),
		ticketFor(2, 5, time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC), 4*time.Hour, nil, "Прочее"),
	)
	handler := newTestHandler(t, repo, user(5, "ivanov", "Иванов", nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/stats/me?from=2026-05-01&to=2026-05-31", nil)
	rec := httptest.NewRecorder()
	handler.Me(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	response := decodeStats(t, rec)
	if response.UserID != 5 || response.Login != "ivanov" || response.FullName != "Иванов" {
		t.Fatalf("response = %+v", response)
	}
	if response.Period.From == nil || *response.Period.From != "2026-05-01" {
		t.Fatalf("period.from = %v", response.Period.From)
	}
	if response.Period.To == nil || *response.Period.To != "2026-05-31" {
		t.Fatalf("period.to = %v", response.Period.To)
	}
	if response.TotalDone != 2 || response.DoneInPeriod != 2 {
		t.Fatalf("total_done=%d done_in_period=%d", response.TotalDone, response.DoneInPeriod)
	}
	if response.AvgResolutionHours == nil || *response.AvgResolutionHours != 3 {
		t.Fatalf("avg = %v, want 3", response.AvgResolutionHours)
	}
	if response.OnTimePercent == nil || *response.OnTimePercent != 100 {
		t.Fatalf("on_time = %v, want 100", response.OnTimePercent)
	}
	if len(response.ByProblemType) != 2 || len(response.ByMonth) != 1 {
		t.Fatalf("разбивки = %+v / %+v", response.ByProblemType, response.ByMonth)
	}
}

// Период не передан — границы null (всё время).
func TestHandler_MeWithoutPeriod(t *testing.T) {
	handler := newTestHandler(t, newFakeRepo(), user(5, "u", "U", nil, nil))

	rec := httptest.NewRecorder()
	handler.Me(rec, httptest.NewRequest(http.MethodGet, "/api/stats/me", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	response := decodeStats(t, rec)
	if response.Period.From != nil || response.Period.To != nil {
		t.Fatalf("period = %+v, want null/null", response.Period)
	}
	// Пустой набор: avg/median/on_time — null, разбивки — пустые массивы.
	raw := rec.Body.String()
	if response.AvgResolutionHours != nil || response.MedianResolutionHours != nil || response.OnTimePercent != nil {
		t.Fatalf("метрики должны быть null: %s", raw)
	}
	if response.ByProblemType == nil || response.ByMonth == nil {
		t.Fatalf("разбивки должны быть массивами: %s", raw)
	}
}

// Некорректные даты периода — 422 VALIDATION_ERROR.
func TestHandler_MeInvalidPeriod(t *testing.T) {
	handler := newTestHandler(t, newFakeRepo(), user(5, "u", "U", nil, nil))

	cases := []string{
		"/api/stats/me?from=2026-13-01",
		"/api/stats/me?to=not-a-date",
		"/api/stats/me?from=2026-05-31&to=2026-05-01",
	}
	for _, target := range cases {
		rec := httptest.NewRecorder()
		handler.Me(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("GET %s = %d, want 422", target, rec.Code)
		}
		if code := decodeErrorCode(t, rec); code != "VALIDATION_ERROR" {
			t.Fatalf("code = %q", code)
		}
	}
}

// GET /api/admin/stats: список сотрудников и фильтр user_id (STATS-1).
func TestHandler_Admin(t *testing.T) {
	buildingID := int64(1)
	buildingName := "Здание А"
	repo := newFakeRepo().
		withEmployees(
			Employee{ID: 1, Login: "u1", FullName: "Первый", Role: auth.RoleUser, BuildingID: &buildingID, BuildingName: &buildingName},
			Employee{ID: 2, Login: "u2", FullName: "Второй", Role: auth.RoleUser, BuildingID: &buildingID, BuildingName: &buildingName},
		).
		withTickets(
			ticketFor(1, 1, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC), time.Hour, nil, "Электрика"),
			ticketFor(2, 2, time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC), 2*time.Hour, nil, "Прочее"),
		)
	handler := newTestHandler(t, repo, admin(100))

	rec := httptest.NewRecorder()
	handler.Admin(rec, httptest.NewRequest(http.MethodGet, "/api/admin/stats?from=2026-05-01&to=2026-05-31", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var list StatsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, rec.Body.String())
	}
	if len(list.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(list.Items))
	}
	if list.Items[0].UserID != 1 || list.Items[1].UserID != 2 {
		t.Fatalf("items = %+v", list.Items)
	}

	// user_id — один сотрудник.
	rec = httptest.NewRecorder()
	handler.Admin(rec, httptest.NewRequest(http.MethodGet, "/api/admin/stats?user_id=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].UserID != 2 {
		t.Fatalf("items = %+v", list.Items)
	}

	// Некорректные фильтры — 422.
	for _, target := range []string{
		"/api/admin/stats?user_id=abc",
		"/api/admin/stats?building_id=0",
		"/api/admin/stats?from=2026-05-31&to=2026-05-01",
	} {
		rec = httptest.NewRecorder()
		handler.Admin(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("GET %s = %d, want 422", target, rec.Code)
		}
	}
}

// USER не получает сводную статистику — 403 (маршрут ADMIN-only, проверка
// дублируется в сервисе для повторного использования в Б11).
func TestHandler_AdminForbiddenForUser(t *testing.T) {
	handler := newTestHandler(t, newFakeRepo(), user(5, "u", "U", nil, nil))

	rec := httptest.NewRecorder()
	handler.Admin(rec, httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if code := decodeErrorCode(t, rec); code != "FORBIDDEN" {
		t.Fatalf("code = %q", code)
	}
}

// Без аутентификации оба обработчика отвечают 401.
func TestHandler_Unauthorized(t *testing.T) {
	handler := newTestHandler(t, newFakeRepo(), nil)

	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"me":    handler.Me,
		"admin": handler.Admin,
	} {
		rec := httptest.NewRecorder()
		call(rec, httptest.NewRequest(http.MethodGet, "/api/stats/me", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: status = %d, want 401", name, rec.Code)
		}
	}
}

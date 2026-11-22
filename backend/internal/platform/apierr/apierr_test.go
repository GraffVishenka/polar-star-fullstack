package apierr_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/reqid"
)

// PLAN §3.13: единый формат ошибки {"code","message","details","request_id"}.
func TestRespond_ErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tickets", nil)
	info := &reqid.Info{RequestID: "req-123"}
	r = r.WithContext(reqid.WithInfo(r.Context(), info))

	apierr.Respond(rec, r, apierr.NotFound("Заявка не найдена"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("некорректный JSON: %v", err)
	}
	for _, key := range []string{"code", "message", "details", "request_id"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("в ответе нет поля %q: %s", key, rec.Body.String())
		}
	}
	if body["code"] != "NOT_FOUND" {
		t.Fatalf("code = %v", body["code"])
	}
	if body["request_id"] != "req-123" {
		t.Fatalf("request_id = %v", body["request_id"])
	}
	if details, ok := body["details"].(map[string]any); !ok || details == nil {
		t.Fatalf("details должен быть объектом, получено %T", body["details"])
	}
}

// Неизвестная ошибка не должна раскрывать внутренние детали клиенту.
func TestRespond_InternalHidesCause(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tickets", nil)

	apierr.Respond(rec, r, errors.New("dial tcp 127.0.0.1:3306: connection refused"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "3306") {
		t.Fatalf("ответ раскрывает внутреннюю ошибку: %s", rec.Body.String())
	}
}

// TooManyRequests проставляет retry_after_seconds в details (AUTH-3).
func TestTooManyRequests_Details(t *testing.T) {
	err := apierr.TooManyRequests(42)
	if err.Status != http.StatusTooManyRequests {
		t.Fatalf("status = %d", err.Status)
	}
	if got := err.Details["retry_after_seconds"]; got != 42 {
		t.Fatalf("retry_after_seconds = %v, want 42", got)
	}
}

func TestValidation_StatusAndCode(t *testing.T) {
	err := apierr.Validation("Поле обязательно", map[string]any{"building_id": "обязательно"})
	if err.Status != http.StatusUnprocessableEntity || err.Code != apierr.CodeValidation {
		t.Fatalf("получено %d %s", err.Status, err.Code)
	}
}

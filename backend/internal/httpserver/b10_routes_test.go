package httpserver_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/httpserver"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// stubTrashAPI подтверждает, что маршруты Б10 подключены к своим handler'ам, а
// не к заглушке 501. Бизнес-логика покрыта unit-тестами пакета trash.
type stubTrashAPI struct{}

func (stubTrashAPI) List(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "trash.List")
}

func (stubTrashAPI) Restore(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "trash.Restore")
}

func (stubTrashAPI) Purge(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// newB10Env собирает роутер Б10 поверх поддельных auth-зависимостей.
func newB10Env(t *testing.T, users ...*auth.User) (*testEnv, http.Handler) {
	t.Helper()
	env := newTestEnv(t, users...)

	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	clock := platform.FixedClock{Time: now}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokens := auth.NewTokenManager(testJWTSecret, "polar-star-service-test", auth.AccessTokenTTL, clock)
	service := auth.NewService(env.repo, tokens, clock, logger, auth.Options{
		RefreshTTL:  7 * 24 * time.Hour,
		LoginLimit:  10,
		LoginWindow: 10 * time.Minute,
	})

	server := httpserver.New(httpserver.Deps{
		Config:        &config.Config{Version: "test", Location: time.UTC, CORSOrigins: []string{"http://localhost:5173"}},
		Logger:        logger,
		Clock:         clock,
		AuthService:   service,
		AuthHandler:   auth.NewHandler(service, logger, false),
		Authenticator: auth.NewAuthenticator(env.repo, tokens),
		TrashHandler:  stubTrashAPI{},
	})
	return env, server.Handler()
}

// Б10: маршруты корзины доступны только ADMIN и подключены к handler'ам.
func TestB10RoutesRegistration(t *testing.T) {
	user := activeUser(7, "user", auth.RoleUser)
	admin := activeUser(1, "admin", auth.RoleAdmin)
	env, handler := newB10Env(t, user, admin)
	userToken := env.token(t, user)
	adminToken := env.token(t, admin)

	tests := []struct {
		name   string
		method string
		target string
		token  string
		status int
		code   string
		stub   string
	}{
		{"USER не видит корзину", http.MethodGet, "/api/admin/trash?type=ticket", userToken,
			http.StatusForbidden, "FORBIDDEN", ""},
		{"USER не восстанавливает", http.MethodPost, "/api/admin/trash/ticket/1/restore", userToken,
			http.StatusForbidden, "FORBIDDEN", ""},
		{"USER не удаляет окончательно", http.MethodDelete, "/api/admin/trash/ticket/1", userToken,
			http.StatusForbidden, "FORBIDDEN", ""},
		{"ADMIN видит корзину", http.MethodGet, "/api/admin/trash?type=ticket", adminToken,
			http.StatusOK, "", "trash.List"},
		{"ADMIN восстанавливает", http.MethodPost, "/api/admin/trash/kb/1/restore", adminToken,
			http.StatusOK, "", "trash.Restore"},
		{"ADMIN удаляет окончательно", http.MethodDelete, "/api/admin/trash/equipment/1", adminToken,
			http.StatusNoContent, "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(handler, tc.method, tc.target, tc.token, nil)
			if rec.Code != tc.status {
				t.Fatalf("%s %s = %d, want %d (%s)", tc.method, tc.target, rec.Code, tc.status, rec.Body.String())
			}
			if tc.code != "" {
				if body := decodeError(t, rec); body["code"] != tc.code {
					t.Fatalf("code = %v, want %s", body["code"], tc.code)
				}
			}
			if tc.stub != "" {
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("stub JSON: %v (%s)", err, rec.Body.String())
				}
				if body["stub"] != tc.stub {
					t.Fatalf("stub = %q, want %q", body["stub"], tc.stub)
				}
			}
		})
	}
}

// Без подключённого handler'а маршруты корзины остаются заглушками 501.
func TestB10RoutesFallbackToStubs(t *testing.T) {
	admin := activeUser(1, "admin", auth.RoleAdmin)
	env := newTestEnv(t, admin)
	adminToken := env.token(t, admin)

	rec := doRequest(env.handler, http.MethodGet, "/api/admin/trash?type=ticket", adminToken, nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("GET /api/admin/trash = %d, want 501", rec.Code)
	}
	rec = doRequest(env.handler, http.MethodDelete, "/api/admin/trash/ticket/1", adminToken, nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("DELETE /api/admin/trash/ticket/1 = %d, want 501", rec.Code)
	}
}

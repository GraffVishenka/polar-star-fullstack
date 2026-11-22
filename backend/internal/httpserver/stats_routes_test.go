package httpserver_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/httpserver"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// stubStats — заглушка StatsAPI: фиксирует вызовы и отдаёт минимальные ответы.
// Нужна, чтобы проверить регистрацию маршрутов Б9 и права ADMIN без MySQL
// (расчёты покрыты тестами internal/stats).
type stubStats struct {
	mu    sync.Mutex
	calls []string
}

func (s *stubStats) record(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, name)
}

func (s *stubStats) called(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, call := range s.calls {
		if call == name {
			return true
		}
	}
	return false
}

func (s *stubStats) Me(w http.ResponseWriter, _ *http.Request) {
	s.record("Me")
	writeStubJSON(w, http.StatusOK, map[string]any{"user_id": 1, "by_problem_type": []any{}, "by_month": []any{}})
}

func (s *stubStats) Admin(w http.ResponseWriter, _ *http.Request) {
	s.record("Admin")
	writeStubJSON(w, http.StatusOK, map[string]any{"items": []any{}})
}

// Маршруты Б9 зарегистрированы: /api/stats/me — USER/ADMIN, /api/admin/stats —
// только ADMIN (STATS-1), выгрузка `.xlsx` остаётся заглушкой Б11 (501).
func TestStatsRoutes_Permissions(t *testing.T) {
	clock := platform.FixedClock{Time: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	admin := activeUser(1, "admin", auth.RoleAdmin)
	user := activeUser(2, "user", auth.RoleUser)

	repo := newFakeRepo()
	repo.users[admin.Login] = admin
	repo.users[user.Login] = user
	repo.byID[admin.ID] = admin
	repo.byID[user.ID] = user

	tokens := auth.NewTokenManager(testJWTSecret, "polar-star-service-test", auth.AccessTokenTTL, clock)
	service := auth.NewService(repo, tokens, clock, logger, auth.Options{
		RefreshTTL:  7 * 24 * time.Hour,
		LoginLimit:  10,
		LoginWindow: 10 * time.Minute,
	})
	statsStub := &stubStats{}
	server := httpserver.New(httpserver.Deps{
		Config:        &config.Config{Version: "test", CORSOrigins: []string{"http://localhost:5173"}, Location: time.UTC},
		Logger:        logger,
		Clock:         clock,
		AuthService:   service,
		AuthHandler:   auth.NewHandler(service, logger, false),
		Authenticator: auth.NewAuthenticator(repo, tokens),
		StatsHandler:  statsStub,
	})
	handler := server.Handler()

	// Без токена — 401, а не 501: маршруты зарегистрированы.
	for _, target := range []string{"/api/stats/me", "/api/admin/stats"} {
		rec := doRequest(handler, http.MethodGet, target, "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s без токена = %d, want 401", target, rec.Code)
		}
	}

	userToken := issueToken(t, tokens, user)
	rec := doRequest(handler, http.MethodGet, "/api/stats/me", userToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("USER GET /api/stats/me = %d, want 200", rec.Code)
	}
	if !statsStub.called("Me") {
		t.Fatalf("Me не вызван: %v", statsStub.calls)
	}

	// USER на сводной статистике — 403 (ADMIN-only route).
	rec = doRequest(handler, http.MethodGet, "/api/admin/stats", userToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("USER GET /api/admin/stats = %d, want 403", rec.Code)
	}
	if statsStub.called("Admin") {
		t.Fatalf("Admin вызван для USER: %v", statsStub.calls)
	}

	adminToken := issueToken(t, tokens, admin)
	rec = doRequest(handler, http.MethodGet, "/api/admin/stats?building_id=1&from=2026-05-01&to=2026-05-31", adminToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ADMIN GET /api/admin/stats = %d, want 200", rec.Code)
	}
	if !statsStub.called("Admin") {
		t.Fatalf("Admin не вызван: %v", statsStub.calls)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if _, ok := body["items"]; !ok {
		t.Fatalf("ответ без items: %s", rec.Body.String())
	}

	// Выгрузка Excel — за Б11, пока 501.
	rec = doRequest(handler, http.MethodGet, "/api/stats/export.xlsx", userToken, nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("GET /api/stats/export.xlsx = %d, want 501 (Б11)", rec.Code)
	}
}

// issueToken выпускает access-токен для тестового пользователя.
func issueToken(t *testing.T, tokens *auth.TokenManager, user *auth.User) string {
	t.Helper()
	token, _, err := tokens.Issue(user)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return token
}

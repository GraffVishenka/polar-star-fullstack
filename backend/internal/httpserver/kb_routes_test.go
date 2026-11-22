package httpserver_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/httpserver"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// stubKB — заглушка KBAPI: фиксирует вызовы и отдаёт минимальные ответы.
// Нужна, чтобы проверить регистрацию маршрутов Б6 и права ADMIN на удаление
// без MySQL (реальная логика покрыта тестами internal/kb).
type stubKB struct {
	mu    sync.Mutex
	calls []string
}

func (s *stubKB) record(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, name)
}

func (s *stubKB) called(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, call := range s.calls {
		if call == name {
			return true
		}
	}
	return false
}

func writeStubJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *stubKB) Tree(w http.ResponseWriter, _ *http.Request) {
	s.record("Tree")
	writeStubJSON(w, http.StatusOK, []any{})
}

func (s *stubKB) PublicTree(w http.ResponseWriter, _ *http.Request) {
	s.record("PublicTree")
	writeStubJSON(w, http.StatusOK, []any{})
}

func (s *stubKB) Get(w http.ResponseWriter, _ *http.Request) {
	s.record("Get")
	writeStubJSON(w, http.StatusOK, map[string]any{"id": 1})
}

func (s *stubKB) PublicGet(w http.ResponseWriter, _ *http.Request) {
	s.record("PublicGet")
	writeStubJSON(w, http.StatusOK, map[string]any{"id": 1})
}

func (s *stubKB) Create(w http.ResponseWriter, _ *http.Request) {
	s.record("Create")
	writeStubJSON(w, http.StatusCreated, map[string]any{"id": 1})
}

func (s *stubKB) Update(w http.ResponseWriter, _ *http.Request) {
	s.record("Update")
	writeStubJSON(w, http.StatusOK, map[string]any{"id": 1})
}

func (s *stubKB) Move(w http.ResponseWriter, _ *http.Request) {
	s.record("Move")
	writeStubJSON(w, http.StatusOK, map[string]any{"id": 1})
}

func (s *stubKB) Delete(w http.ResponseWriter, _ *http.Request) {
	s.record("Delete")
	writeStubJSON(w, http.StatusOK, map[string]any{"deleted_id": 1, "deleted_descendants": 0})
}

func (s *stubKB) SetVisibility(w http.ResponseWriter, _ *http.Request) {
	s.record("SetVisibility")
	writeStubJSON(w, http.StatusOK, map[string]any{"id": 1})
}

// testEnvWithKB — окружение с реальным auth-middleware и заглушкой KBAPI.
type testEnvWithKB struct {
	handler http.Handler
	tokens  *auth.TokenManager
	kb      *stubKB
	admin   *auth.User
	user    *auth.User
}

func newKBTestEnv(t *testing.T) *testEnvWithKB {
	t.Helper()
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
	kbStub := &stubKB{}

	cfg := &config.Config{
		Version:     "test",
		CORSOrigins: []string{"http://localhost:5173"},
		Location:    time.UTC,
		OpenAPIPath: "openapi/openapi.yaml",
	}
	server := httpserver.New(httpserver.Deps{
		Config:        cfg,
		Logger:        logger,
		Clock:         clock,
		AuthService:   service,
		AuthHandler:   auth.NewHandler(service, logger, false),
		Authenticator: auth.NewAuthenticator(repo, tokens),
		KBHandler:     kbStub,
	})
	return &testEnvWithKB{handler: server.Handler(), tokens: tokens, kb: kbStub, admin: admin, user: user}
}

func (e *testEnvWithKB) token(t *testing.T, u *auth.User) string {
	t.Helper()
	token, _, err := e.tokens.Issue(u)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return token
}

// Маршруты Б6 зарегистрированы: публичные — без авторизации, защищённые — 401,
// удаление и видимость — только ADMIN (KB-3, KB-7).
func TestKBRoutes_Permissions(t *testing.T) {
	env := newKBTestEnv(t)

	// Публичное дерево и статья доступны анониму.
	rec := doRequest(env.handler, http.MethodGet, "/api/public/kb/tree", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/public/kb/tree = %d, want 200", rec.Code)
	}
	rec = doRequest(env.handler, http.MethodGet, "/api/public/kb/nodes/1", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/public/kb/nodes/1 = %d, want 200", rec.Code)
	}
	if !env.kb.called("PublicTree") || !env.kb.called("PublicGet") {
		t.Fatalf("публичные методы не вызваны: %v", env.kb.calls)
	}

	// Без токена защищённые маршруты — 401, а не 501.
	for _, target := range []string{"/api/kb/tree", "/api/kb/nodes/1"} {
		rec = doRequest(env.handler, http.MethodGet, target, "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s без токена = %d, want 401", target, rec.Code)
		}
	}

	// USER: чтение/правка/перемещение разрешены (KB-3).
	userToken := env.token(t, env.user)
	rec = doRequest(env.handler, http.MethodGet, "/api/kb/tree", userToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("USER GET /api/kb/tree = %d, want 200", rec.Code)
	}
	rec = doRequest(env.handler, http.MethodPost, "/api/kb/nodes", userToken, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("USER POST /api/kb/nodes = %d, want 201", rec.Code)
	}

	// USER не может удалять и менять видимость — 403 (ADMIN-only).
	rec = doRequest(env.handler, http.MethodDelete, "/api/kb/nodes/1", userToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("USER DELETE = %d, want 403", rec.Code)
	}
	rec = doRequest(env.handler, http.MethodPatch, "/api/kb/nodes/1/visibility", userToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("USER PATCH visibility = %d, want 403", rec.Code)
	}
	if env.kb.called("Delete") || env.kb.called("SetVisibility") {
		t.Fatalf("ADMIN-методы вызваны для USER: %v", env.kb.calls)
	}

	// ADMIN: удаление и видимость работают.
	adminToken := env.token(t, env.admin)
	rec = doRequest(env.handler, http.MethodDelete, "/api/kb/nodes/1", adminToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ADMIN DELETE = %d, want 200", rec.Code)
	}
	rec = doRequest(env.handler, http.MethodPatch, "/api/kb/nodes/1/visibility", adminToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ADMIN PATCH visibility = %d, want 200", rec.Code)
	}
	if !env.kb.called("Delete") || !env.kb.called("SetVisibility") {
		t.Fatalf("ADMIN-методы не вызваны: %v", env.kb.calls)
	}
}

// Без KBHandler маршруты БЗ остаются замороженными заглушками 501.
func TestKBRoutes_StubWithoutHandler(t *testing.T) {
	clock := platform.FixedClock{Time: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newFakeRepo()
	tokens := auth.NewTokenManager(testJWTSecret, "polar-star-service-test", auth.AccessTokenTTL, clock)
	service := auth.NewService(repo, tokens, clock, logger, auth.Options{RefreshTTL: time.Hour, LoginLimit: 10, LoginWindow: time.Minute})
	server := httpserver.New(httpserver.Deps{
		Config:        &config.Config{Version: "test", CORSOrigins: []string{"http://localhost:5173"}, Location: time.UTC},
		Logger:        logger,
		Clock:         clock,
		AuthService:   service,
		AuthHandler:   auth.NewHandler(service, logger, false),
		Authenticator: auth.NewAuthenticator(repo, tokens),
	})
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/kb/tree", nil))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("без KBHandler: %d, want 501", rec.Code)
	}
}

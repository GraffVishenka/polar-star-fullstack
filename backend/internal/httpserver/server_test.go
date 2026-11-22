package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/httpserver"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// fakeRepo — in-memory Repository: тесты проверяют HTTP-слой и auth-правила
// без MySQL (окружение не позволяет запускать сервер и БД).
type fakeRepo struct {
	users   map[string]*auth.User
	byID    map[int64]*auth.User
	refresh map[string]*auth.RefreshToken
	nextID  int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		users:   map[string]*auth.User{},
		byID:    map[int64]*auth.User{},
		refresh: map[string]*auth.RefreshToken{},
	}
}

func (r *fakeRepo) UserByLogin(_ context.Context, login string) (*auth.User, error) {
	if u, ok := r.users[login]; ok {
		return u, nil
	}
	return nil, auth.ErrNotFound
}

func (r *fakeRepo) UserByID(_ context.Context, id int64) (*auth.User, error) {
	if u, ok := r.byID[id]; ok {
		return u, nil
	}
	return nil, auth.ErrNotFound
}

func (r *fakeRepo) UpdatePassword(_ context.Context, id int64, passwordHash string, mustChange bool) error {
	user, ok := r.byID[id]
	if !ok {
		return auth.ErrNotFound
	}
	user.PasswordHash = passwordHash
	user.MustChangePassword = mustChange
	return nil
}

func (r *fakeRepo) CreateRefreshToken(_ context.Context, userID int64, tokenHash string, expiresAt time.Time) (int64, error) {
	r.nextID++
	id := r.nextID
	r.refresh[tokenHash] = &auth.RefreshToken{
		ID:        id,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	return id, nil
}

func (r *fakeRepo) RefreshTokenByHash(_ context.Context, tokenHash string) (*auth.RefreshToken, error) {
	if token, ok := r.refresh[tokenHash]; ok {
		return token, nil
	}
	return nil, auth.ErrNotFound
}

func (r *fakeRepo) RotateRefreshToken(_ context.Context, oldID, userID int64, newHash string, newExpiresAt time.Time) (int64, error) {
	var old *auth.RefreshToken
	for _, token := range r.refresh {
		if token.ID == oldID {
			old = token
			break
		}
	}
	if old == nil || old.RevokedAt != nil {
		return 0, auth.ErrRefreshReuse
	}
	now := time.Now().UTC()
	old.RevokedAt = &now

	r.nextID++
	newID := r.nextID
	old.ReplacedBy = &newID
	r.refresh[newHash] = &auth.RefreshToken{
		ID:        newID,
		UserID:    userID,
		TokenHash: newHash,
		ExpiresAt: newExpiresAt,
		CreatedAt: now,
	}
	return newID, nil
}

func (r *fakeRepo) RevokeRefreshToken(_ context.Context, id int64) error {
	for _, token := range r.refresh {
		if token.ID == id && token.RevokedAt == nil {
			now := time.Now().UTC()
			token.RevokedAt = &now
		}
	}
	return nil
}

func (r *fakeRepo) RevokeAllRefreshTokens(_ context.Context, userID int64, exceptHash string) error {
	now := time.Now().UTC()
	for hash, token := range r.refresh {
		if token.UserID == userID && token.RevokedAt == nil && hash != exceptHash {
			token.RevokedAt = &now
		}
	}
	return nil
}

func (r *fakeRepo) RevokeRefreshTokenChain(_ context.Context, startID int64) error {
	var token *auth.RefreshToken
	for _, candidate := range r.refresh {
		if candidate.ID == startID {
			token = candidate
			break
		}
	}
	for i := 0; token != nil && i < 64; i++ {
		if token.RevokedAt == nil {
			now := time.Now().UTC()
			token.RevokedAt = &now
		}
		if token.ReplacedBy == nil {
			return nil
		}
		var next *auth.RefreshToken
		for _, candidate := range r.refresh {
			if candidate.ID == *token.ReplacedBy {
				next = candidate
				break
			}
		}
		token = next
	}
	return nil
}

const testJWTSecret = "0123456789abcdef0123456789abcdef"

type testEnv struct {
	handler http.Handler
	tokens  *auth.TokenManager
	repo    *fakeRepo
	clock   platform.FixedClock
}

// newTestEnv собирает реальный роутер с поддельными репозиторием и часами.
func newTestEnv(t *testing.T, users ...*auth.User) *testEnv {
	t.Helper()
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	clock := platform.FixedClock{Time: now}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := newFakeRepo()
	for _, u := range users {
		repo.users[u.Login] = u
		repo.byID[u.ID] = u
	}

	tokens := auth.NewTokenManager(testJWTSecret, "polar-star-service-test", auth.AccessTokenTTL, clock)
	service := auth.NewService(repo, tokens, clock, logger, auth.Options{
		RefreshTTL:  7 * 24 * time.Hour,
		LoginLimit:  10,
		LoginWindow: 10 * time.Minute,
	})

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
	})
	return &testEnv{handler: server.Handler(), tokens: tokens, repo: repo, clock: clock}
}

func (e *testEnv) token(t *testing.T, u *auth.User) string {
	t.Helper()
	token, _, err := e.tokens.Issue(u)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return token
}

func doRequest(handler http.Handler, method, target, token string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	req.RemoteAddr = "192.0.2.10:12345"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("некорректный JSON ошибки: %v (%s)", err, rec.Body.String())
	}
	return body
}

func activeUser(id int64, login string, role auth.Role) *auth.User {
	return &auth.User{
		ID:       id,
		Login:    login,
		FullName: "Тестовый " + login,
		Role:     role,
		IsActive: true,
	}
}

// NFR-4: liveness не зависит от БД, readiness без БД отдаёт 503.
func TestHealthAndReadiness(t *testing.T) {
	env := newTestEnv(t)

	health := doRequest(env.handler, http.MethodGet, "/health", "", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("/health = %d, want 200", health.Code)
	}
	var hb httpserver.HealthResponse
	if err := json.Unmarshal(health.Body.Bytes(), &hb); err != nil {
		t.Fatalf("health JSON: %v", err)
	}
	if hb.Status != "ok" {
		t.Fatalf("health.status = %q", hb.Status)
	}

	ready := doRequest(env.handler, http.MethodGet, "/health/ready", "", nil)
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("/health/ready без БД = %d, want 503", ready.Code)
	}
	var rb httpserver.ReadinessResponse
	if err := json.Unmarshal(ready.Body.Bytes(), &rb); err != nil {
		t.Fatalf("readiness JSON: %v", err)
	}
	if rb.Checks.Database != "down" {
		t.Fatalf("checks.database = %q", rb.Checks.Database)
	}
}

// AUTH-2/AUTH-6: без токена защищённый маршрут отдаёт 401 в едином формате
// с request_id, совпадающим с заголовком X-Request-ID.
func TestProtectedRouteWithoutToken(t *testing.T) {
	env := newTestEnv(t)
	rec := doRequest(env.handler, http.MethodGet, "/api/tickets", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	body := decodeError(t, rec)
	if body["code"] != "UNAUTHORIZED" {
		t.Fatalf("code = %v", body["code"])
	}
	headerID := rec.Header().Get("X-Request-ID")
	if headerID == "" {
		t.Fatal("X-Request-ID не проставлен")
	}
	if body["request_id"] != headerID {
		t.Fatalf("request_id в теле (%v) != заголовку (%s)", body["request_id"], headerID)
	}
	if details, ok := body["details"].(map[string]any); !ok || details == nil {
		t.Fatalf("details должен быть объектом: %v", body["details"])
	}
}

// TICKET-7/ADR: USER на ADMIN-маршруте получает 403, а не 404.
func TestRoleEnforcement(t *testing.T) {
	user := activeUser(7, "user", auth.RoleUser)
	admin := activeUser(1, "admin", auth.RoleAdmin)
	env := newTestEnv(t, user, admin)

	userToken := env.token(t, user)
	adminToken := env.token(t, admin)

	rec := doRequest(env.handler, http.MethodDelete, "/api/tickets/1", userToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("USER DELETE /api/tickets/1 = %d, want 403", rec.Code)
	}
	if body := decodeError(t, rec); body["code"] != "FORBIDDEN" {
		t.Fatalf("code = %v", body["code"])
	}

	// Заглушка авторизованного маршрута отдаёт 501, а не 404/405.
	rec = doRequest(env.handler, http.MethodGet, "/api/tickets", userToken, nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("USER GET /api/tickets = %d, want 501", rec.Code)
	}
	if body := decodeError(t, rec); body["code"] != "NOT_IMPLEMENTED" {
		t.Fatalf("code = %v", body["code"])
	}

	// ADMIN проходит requireRole и тоже получает замороженную заглушку.
	rec = doRequest(env.handler, http.MethodDelete, "/api/tickets/1", adminToken, nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("ADMIN DELETE /api/tickets/1 = %d, want 501", rec.Code)
	}
}

// AUTH-4: must_change_password → 428 на всех маршрутах, кроме me/password/logout.
func TestMustChangePasswordGate(t *testing.T) {
	user := activeUser(9, "temp", auth.RoleUser)
	user.MustChangePassword = true
	env := newTestEnv(t, user)
	token := env.token(t, user)

	rec := doRequest(env.handler, http.MethodGet, "/api/tickets", token, nil)
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("GET /api/tickets = %d, want 428", rec.Code)
	}
	if body := decodeError(t, rec); body["code"] != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("code = %v", body["code"])
	}

	me := doRequest(env.handler, http.MethodGet, "/api/auth/me", token, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/me = %d, want 200", me.Code)
	}
	var dto auth.UserDTO
	if err := json.Unmarshal(me.Body.Bytes(), &dto); err != nil {
		t.Fatalf("me JSON: %v", err)
	}
	if dto.Login != "temp" || !dto.MustChangePassword {
		t.Fatalf("me = %+v", dto)
	}
}

// AUTH-1/AUTH-2: успешный вход ставит httpOnly SameSite=Lax cookie на /api/auth,
// неверный пароль — 401 INVALID_CREDENTIALS без раскрытия логина.
func TestLoginSuccessAndInvalidCredentials(t *testing.T) {
	hash, err := auth.HashPassword("CorrectHorse1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := activeUser(3, "alice", auth.RoleUser)
	user.PasswordHash = hash
	env := newTestEnv(t, user)

	bad := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
		jsonBody(map[string]string{"login": "alice", "password": "wrong"}))
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("неверный пароль = %d, want 401", bad.Code)
	}
	if body := decodeError(t, bad); body["code"] != "INVALID_CREDENTIALS" {
		t.Fatalf("code = %v", body["code"])
	}

	unknown := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
		jsonBody(map[string]string{"login": "nobody", "password": "wrong"}))
	if unknown.Code != http.StatusUnauthorized {
		t.Fatalf("неизвестный логин = %d, want 401", unknown.Code)
	}
	if body := decodeError(t, unknown); body["code"] != "INVALID_CREDENTIALS" {
		t.Fatalf("неизвестный логин не должен раскрываться: %v", body["code"])
	}

	ok := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
		jsonBody(map[string]string{"login": "alice", "password": "CorrectHorse1"}))
	if ok.Code != http.StatusOK {
		t.Fatalf("успешный вход = %d (%s)", ok.Code, ok.Body.String())
	}
	var session auth.SessionDTO
	if err := json.Unmarshal(ok.Body.Bytes(), &session); err != nil {
		t.Fatalf("session JSON: %v", err)
	}
	if session.AccessToken == "" || session.TokenType != "Bearer" || session.ExpiresIn <= 0 {
		t.Fatalf("session = %+v", session)
	}
	if session.User.Login != "alice" {
		t.Fatalf("session.user.login = %q", session.User.Login)
	}

	var refresh *http.Cookie
	for _, c := range ok.Result().Cookies() {
		if c.Name == auth.RefreshCookieName {
			refresh = c
		}
	}
	if refresh == nil {
		t.Fatal("refresh-cookie не установлена")
	}
	if !refresh.HttpOnly || refresh.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie: HttpOnly=%v SameSite=%v", refresh.HttpOnly, refresh.SameSite)
	}
	if refresh.Path != auth.RefreshCookiePath {
		t.Fatalf("cookie path = %q", refresh.Path)
	}
	if refresh.Value == "" {
		t.Fatal("пустой refresh-токен")
	}

	me := doRequest(env.handler, http.MethodGet, "/api/auth/me", session.AccessToken, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("me с access-токеном = %d", me.Code)
	}
}

func doRequestWithCookie(handler http.Handler, method, target, cookieValue string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "192.0.2.10:12345"
	req.AddCookie(&http.Cookie{Name: auth.RefreshCookieName, Value: cookieValue})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func refreshCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.RefreshCookieName {
			return c
		}
	}
	return nil
}

// AUTH-2: каждый refresh ротирует токен; в памяти репозитория только SHA-256,
// повторное использование старого токена отзывает всю цепочку.
func TestRefreshRotationAndReuseRevokesChain(t *testing.T) {
	hash, err := auth.HashPassword("CorrectHorse1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := activeUser(4, "bob", auth.RoleUser)
	user.PasswordHash = hash
	env := newTestEnv(t, user)

	login := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
		jsonBody(map[string]string{"login": "bob", "password": "CorrectHorse1"}))
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d", login.Code)
	}
	first := refreshCookieFrom(t, login)
	if first == nil {
		t.Fatal("нет refresh-cookie после login")
	}

	// В репозитории хранится только хеш: открытого токена там быть не должно.
	if _, plainStored := env.repo.refresh[first.Value]; plainStored {
		t.Fatal("refresh-токен хранится в открытом виде")
	}
	if _, hashStored := env.repo.refresh[auth.HashToken(first.Value)]; !hashStored {
		t.Fatal("в репозитории нет SHA-256 хеша refresh-токена")
	}

	rotated := doRequestWithCookie(env.handler, http.MethodPost, "/api/auth/refresh", first.Value)
	if rotated.Code != http.StatusOK {
		t.Fatalf("refresh = %d (%s)", rotated.Code, rotated.Body.String())
	}
	second := refreshCookieFrom(t, rotated)
	if second == nil || second.Value == first.Value {
		t.Fatal("refresh должен выдавать новый refresh-токен")
	}
	var session auth.SessionDTO
	if err := json.Unmarshal(rotated.Body.Bytes(), &session); err != nil || session.AccessToken == "" {
		t.Fatalf("refresh session: %v", err)
	}

	// Повторное использование старой cookie — 401 и отзыв нового токена.
	reuse := doRequestWithCookie(env.handler, http.MethodPost, "/api/auth/refresh", first.Value)
	if reuse.Code != http.StatusUnauthorized {
		t.Fatalf("повтор старого refresh = %d, want 401", reuse.Code)
	}
	cleared := refreshCookieFrom(t, reuse)
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatal("при ошибке refresh cookie должна очищаться (Max-Age<0)")
	}

	afterReuse := doRequestWithCookie(env.handler, http.MethodPost, "/api/auth/refresh", second.Value)
	if afterReuse.Code != http.StatusUnauthorized {
		t.Fatalf("цепочка не отозвана: refresh нового токена = %d, want 401", afterReuse.Code)
	}
}

// AUTH-3: 10 неудачных входов на пару login+IP за 10 минут → 429 + Retry-After.
func TestLoginRateLimit(t *testing.T) {
	hash, err := auth.HashPassword("CorrectHorse1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := activeUser(5, "carol", auth.RoleUser)
	user.PasswordHash = hash
	env := newTestEnv(t, user)

	for attempt := 1; attempt <= 10; attempt++ {
		rec := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
			jsonBody(map[string]string{"login": "carol", "password": "wrong-password"}))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("попытка %d = %d, want 401", attempt, rec.Code)
		}
	}

	blocked := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
		jsonBody(map[string]string{"login": "carol", "password": "wrong-password"}))
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("11-я попытка = %d, want 429", blocked.Code)
	}
	if body := decodeError(t, blocked); body["code"] != "RATE_LIMITED" {
		t.Fatalf("code = %v", body["code"])
	}
	if blocked.Header().Get("Retry-After") == "" {
		t.Fatal("429 без заголовка Retry-After")
	}
}

// AUTH-5: смена своего пароля снимает must_change_password, оставляет текущую
// refresh-сессию и отзывает остальные; временный пароль перестаёт работать.
func TestChangePasswordClearsGateAndRevokesOtherSessions(t *testing.T) {
	hash, err := auth.HashPassword("TempPass123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := activeUser(10, "dave", auth.RoleUser)
	user.PasswordHash = hash
	user.MustChangePassword = true
	env := newTestEnv(t, user)

	first := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
		jsonBody(map[string]string{"login": "dave", "password": "TempPass123"}))
	if first.Code != http.StatusOK {
		t.Fatalf("login (temporary password) = %d (%s)", first.Code, first.Body.String())
	}
	var session auth.SessionDTO
	if err := json.Unmarshal(first.Body.Bytes(), &session); err != nil {
		t.Fatalf("session JSON: %v", err)
	}
	firstCookie := refreshCookieFrom(t, first)

	// Вторая сессия (другое устройство) — её refresh должен быть отозван.
	second := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
		jsonBody(map[string]string{"login": "dave", "password": "TempPass123"}))
	if second.Code != http.StatusOK {
		t.Fatalf("второй login = %d", second.Code)
	}
	secondCookie := refreshCookieFrom(t, second)

	change := httptest.NewRequest(http.MethodPost, "/api/auth/password",
		jsonBody(map[string]string{"current_password": "TempPass123", "new_password": "NewSecret123"}))
	change.RemoteAddr = "192.0.2.10:12345"
	change.Header.Set("Authorization", "Bearer "+session.AccessToken)
	change.AddCookie(firstCookie)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, change)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("смена пароля = %d (%s)", rec.Code, rec.Body.String())
	}

	// Флаг снят: защищённый маршрут доходит до заглушки 501, а не 428.
	afterChange := doRequest(env.handler, http.MethodGet, "/api/tickets", session.AccessToken, nil)
	if afterChange.Code != http.StatusNotImplemented {
		t.Fatalf("после смены пароля GET /api/tickets = %d, want 501", afterChange.Code)
	}

	// Текущая сессия остаётся рабочей, вторая — отозвана.
	if kept := doRequestWithCookie(env.handler, http.MethodPost, "/api/auth/refresh", firstCookie.Value); kept.Code != http.StatusOK {
		t.Fatalf("текущая refresh-сессия = %d, want 200", kept.Code)
	}
	if revoked := doRequestWithCookie(env.handler, http.MethodPost, "/api/auth/refresh", secondCookie.Value); revoked.Code != http.StatusUnauthorized {
		t.Fatalf("вторая refresh-сессия = %d, want 401", revoked.Code)
	}

	// Старый временный пароль больше не принимается, новый — принимается.
	old := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
		jsonBody(map[string]string{"login": "dave", "password": "TempPass123"}))
	if old.Code != http.StatusUnauthorized {
		t.Fatalf("вход со старым паролем = %d, want 401", old.Code)
	}
	fresh := doRequest(env.handler, http.MethodPost, "/api/auth/login", "",
		jsonBody(map[string]string{"login": "dave", "password": "NewSecret123"}))
	if fresh.Code != http.StatusOK {
		t.Fatalf("вход с новым паролем = %d (%s)", fresh.Code, fresh.Body.String())
	}
}

// NFR-2: CORS подтверждает только разрешённый origin.
func TestCORS(t *testing.T) {
	env := newTestEnv(t)

	allowed := httptest.NewRequest(http.MethodOptions, "/api/tickets", nil)
	allowed.Header.Set("Origin", "http://localhost:5173")
	allowed.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, allowed)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Allow-Origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials = %q", got)
	}

	foreign := httptest.NewRequest(http.MethodOptions, "/api/tickets", nil)
	foreign.Header.Set("Origin", "https://evil.example")
	foreign.Header.Set("Access-Control-Request-Method", "GET")
	frec := httptest.NewRecorder()
	env.handler.ServeHTTP(frec, foreign)
	if got := frec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("чужой origin не должен подтверждаться: %q", got)
	}
}

func TestNotFoundEnvelope(t *testing.T) {
	env := newTestEnv(t)
	rec := doRequest(env.handler, http.MethodGet, "/api/does-not-exist", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if body := decodeError(t, rec); body["code"] != "NOT_FOUND" {
		t.Fatalf("code = %v", body["code"])
	}
}

// jsonBody — JSON-тело запроса.
func jsonBody(v any) io.Reader {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(data)
}

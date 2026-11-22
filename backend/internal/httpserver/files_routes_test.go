package httpserver_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/files"
	"github.com/graffvishenka/polar-star-service/backend/internal/httpserver"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// stubFilesRepo — заглушка Repository: маршруты Б3 проверяются без MySQL.
type stubFilesRepo struct{}

func (stubFilesRepo) Insert(context.Context, *files.File) error { return nil }
func (stubFilesRepo) Get(context.Context, int64) (*files.File, error) {
	return nil, files.ErrNotFound
}
func (stubFilesRepo) MarkLinked(context.Context, []int64, time.Time) error { return nil }
func (stubFilesRepo) AttachToComment(context.Context, int64, []int64, time.Time) error {
	return nil
}
func (stubFilesRepo) OrphanFiles(context.Context, time.Time, int) ([]files.File, error) {
	return nil, nil
}
func (stubFilesRepo) Delete(context.Context, int64) error { return nil }

// newFilesTestHandler собирает реальный роутер с подключёнными маршрутами
// файлов (Б3): проверяется именно регистрация маршрутов и их права.
func newFilesTestHandler(t *testing.T) http.Handler {
	t.Helper()
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	clock := platform.FixedClock{Time: now}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	authRepo := newFakeRepo()
	tokens := auth.NewTokenManager(testJWTSecret, "polar-star-service-test", auth.AccessTokenTTL, clock)
	service := auth.NewService(authRepo, tokens, clock, logger, auth.Options{
		RefreshTTL:  7 * 24 * time.Hour,
		LoginLimit:  10,
		LoginWindow: 10 * time.Minute,
	})

	filesService := files.NewService(stubFilesRepo{}, files.NewStorage(t.TempDir(), time.UTC, clock),
		files.Options{Clock: clock, Logger: logger})

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
		Authenticator: auth.NewAuthenticator(authRepo, tokens),
		FilesHandler:  files.NewHandler(filesService, logger),
	})
	return server.Handler()
}

// Маршруты Б3 зарегистрированы с правильными правами: без токена — 401
// (а не 501-заглушка), публичное чтение до Б5 — 404.
func TestFilesRoutes_Registered(t *testing.T) {
	handler := newFilesTestHandler(t)

	rec := doRequest(handler, http.MethodGet, "/api/files/1", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/files/1 без токена = %d, want 401 (маршрут не зарегистрирован?)", rec.Code)
	}

	rec = doRequest(handler, http.MethodGet, "/api/public/files/1?token=abc", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/public/files/1 = %d, want 404 (до подключения Б5)", rec.Code)
	}

	// Публичная загрузка объявлена в контракте (аддитивно): маршрут существует
	// и не отдаёт 501; без multipart-тела — 400 (проверка формы запроса).
	rec = doRequest(handler, http.MethodPost, "/api/public/files?token=abc", "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/public/files = %d, want 400 (маршрут не зарегистрирован?)", rec.Code)
	}
}

// AUTH-3: публичная загрузка ограничена 5 запросами / 10 минут на IP.
func TestFilesPublicUpload_RateLimited(t *testing.T) {
	handler := newFilesTestHandler(t)

	for i := 0; i < 5; i++ {
		rec := doRequest(handler, http.MethodPost, "/api/public/files?token=abc", "", nil)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("запрос %d отклонён лимитом раньше времени", i+1)
		}
	}
	rec := doRequest(handler, http.MethodPost, "/api/public/files?token=abc", "", nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6-й запрос = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Fatal("нет заголовка Retry-After (AUTH-3)")
	}
	if code, _ := decodeError(t, rec)["code"].(string); code != "RATE_LIMITED" {
		t.Fatalf("code = %q", code)
	}
}

// HEAD на /api/files/{id} зарегистрирован (нужен http.ServeContent).
func TestFilesRoutes_HeadRegistered(t *testing.T) {
	handler := newFilesTestHandler(t)
	req := httptest.NewRequest(http.MethodHead, "/api/files/1", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("HEAD /api/files/1 без токена = %d, want 401", rec.Code)
	}
}

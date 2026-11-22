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

// stubSettingsAPI и stubDictionariesAPI подтверждают, что маршруты Б7
// подключены к своим handler'ам, а не к заглушке 501. Бизнес-логика покрыта
// unit-тестами пакетов settings и dictionaries.
type stubSettingsAPI struct{}

func (stubSettingsAPI) AdminGet(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "settings.AdminGet")
}

func (stubSettingsAPI) Update(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "settings.Update")
}

func (stubSettingsAPI) Public(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "settings.Public")
}

func (stubSettingsAPI) ListHolidays(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "settings.ListHolidays")
}

func (stubSettingsAPI) CreateHoliday(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusCreated, "settings.CreateHoliday")
}

func (stubSettingsAPI) UpdateHoliday(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "settings.UpdateHoliday")
}

func (stubSettingsAPI) DeleteHoliday(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

type stubDictionariesAPI struct{}

func (stubDictionariesAPI) ListProblemTypes(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "dictionaries.ListProblemTypes")
}

func (stubDictionariesAPI) CreateProblemType(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusCreated, "dictionaries.CreateProblemType")
}

func (stubDictionariesAPI) UpdateProblemType(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "dictionaries.UpdateProblemType")
}

func (stubDictionariesAPI) DeleteProblemType(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (stubDictionariesAPI) ListEquipmentTypes(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "dictionaries.ListEquipmentTypes")
}

func (stubDictionariesAPI) CreateEquipmentType(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusCreated, "dictionaries.CreateEquipmentType")
}

func (stubDictionariesAPI) UpdateEquipmentType(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "dictionaries.UpdateEquipmentType")
}

func (stubDictionariesAPI) DeleteEquipmentType(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (stubDictionariesAPI) PublicProblemTypes(w http.ResponseWriter, _ *http.Request) {
	writeStub(w, http.StatusOK, "dictionaries.PublicProblemTypes")
}

func writeStub(w http.ResponseWriter, status int, name string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"stub": name})
}

// newB7Env собирает роутер Б7 поверх тех же поддельных auth-зависимостей, что и
// newTestEnv (без MySQL).
func newB7Env(t *testing.T, users ...*auth.User) (*testEnv, http.Handler) {
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
		Config:              &config.Config{Version: "test", Location: time.UTC, CORSOrigins: []string{"http://localhost:5173"}},
		Logger:              logger,
		Clock:               clock,
		AuthService:         service,
		AuthHandler:         auth.NewHandler(service, logger, false),
		Authenticator:       auth.NewAuthenticator(env.repo, tokens),
		SettingsHandler:     stubSettingsAPI{},
		DictionariesHandler: stubDictionariesAPI{},
	})
	return env, server.Handler()
}

// Б7: маршруты настроек/справочников подключены к handler'ам, права по
// матрице PLAN §3.1 (ADMIN — запись, USER/ADMIN — чтение справочников,
// публичные настройки и виды проблем — без токена).
func TestB7RoutesRegistration(t *testing.T) {
	user := activeUser(7, "user", auth.RoleUser)
	admin := activeUser(1, "admin", auth.RoleAdmin)
	env, handler := newB7Env(t, user, admin)
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
		{"публичные настройки", http.MethodGet, "/api/public/settings", "", http.StatusOK, "", "settings.Public"},
		{"публичные виды проблем", http.MethodGet, "/api/public/problem-types", "", http.StatusOK, "", "dictionaries.PublicProblemTypes"},
		{"USER читает виды проблем", http.MethodGet, "/api/problem-types", userToken, http.StatusOK, "", "dictionaries.ListProblemTypes"},
		{"USER читает типы оборудования", http.MethodGet, "/api/equipment-types", userToken, http.StatusOK, "", "dictionaries.ListEquipmentTypes"},
		{"USER не видит настройки", http.MethodGet, "/api/admin/settings", userToken, http.StatusForbidden, "FORBIDDEN", ""},
		{"USER не меняет настройки", http.MethodPut, "/api/admin/settings", userToken, http.StatusForbidden, "FORBIDDEN", ""},
		{"USER не видит праздники", http.MethodGet, "/api/admin/holidays", userToken, http.StatusForbidden, "FORBIDDEN", ""},
		{"USER не создаёт справочник", http.MethodPost, "/api/admin/problem-types", userToken, http.StatusForbidden, "FORBIDDEN", ""},
		{"ADMIN читает настройки", http.MethodGet, "/api/admin/settings", adminToken, http.StatusOK, "", "settings.AdminGet"},
		{"ADMIN меняет настройки", http.MethodPut, "/api/admin/settings", adminToken, http.StatusOK, "", "settings.Update"},
		{"ADMIN читает праздники", http.MethodGet, "/api/admin/holidays", adminToken, http.StatusOK, "", "settings.ListHolidays"},
		{"ADMIN создаёт праздник", http.MethodPost, "/api/admin/holidays", adminToken, http.StatusCreated, "", "settings.CreateHoliday"},
		{"ADMIN переносит праздник", http.MethodPatch, "/api/admin/holidays/1", adminToken, http.StatusOK, "", "settings.UpdateHoliday"},
		{"ADMIN удаляет праздник", http.MethodDelete, "/api/admin/holidays/1", adminToken, http.StatusNoContent, "", ""},
		{"ADMIN читает справочник", http.MethodGet, "/api/admin/problem-types", adminToken, http.StatusOK, "", "dictionaries.ListProblemTypes"},
		{"ADMIN создаёт вид проблемы", http.MethodPost, "/api/admin/problem-types", adminToken, http.StatusCreated, "", "dictionaries.CreateProblemType"},
		{"ADMIN правит тип оборудования", http.MethodPatch, "/api/admin/equipment-types/1", adminToken, http.StatusOK, "", "dictionaries.UpdateEquipmentType"},
		{"ADMIN деактивирует тип", http.MethodDelete, "/api/admin/equipment-types/1", adminToken, http.StatusNoContent, "", ""},
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

// Без подключённых handler'ов Б7 маршруты остаются заглушками 501 (как в
// тестах ранних блоков).
func TestB7RoutesFallbackToStubs(t *testing.T) {
	user := activeUser(7, "user", auth.RoleUser)
	admin := activeUser(1, "admin", auth.RoleAdmin)
	env := newTestEnv(t, user, admin)
	adminToken := env.token(t, admin)

	for _, target := range []string{"/api/public/settings", "/api/public/problem-types"} {
		rec := doRequest(env.handler, http.MethodGet, target, "", nil)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("GET %s = %d, want 501", target, rec.Code)
		}
	}

	rec := doRequest(env.handler, http.MethodGet, "/api/admin/settings", adminToken, nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("GET /api/admin/settings = %d, want 501", rec.Code)
	}
	rec = doRequest(env.handler, http.MethodDelete, "/api/admin/problem-types/1", adminToken, nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("DELETE /api/admin/problem-types/1 = %d, want 501", rec.Code)
	}
}

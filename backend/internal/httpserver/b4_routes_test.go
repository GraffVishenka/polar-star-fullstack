package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/buildings"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/equipment"
	"github.com/graffvishenka/polar-star-service/backend/internal/httpserver"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// stubBuildingsRepo — минимальный репозиторий домена зданий: тест проверяет
// регистрацию маршрутов Б4 и распределение прав, а не бизнес-логику (она
// покрыта unit-тестами пакета buildings).
type stubBuildingsRepo struct{}

func (stubBuildingsRepo) Buildings(context.Context, bool) ([]buildings.Building, error) {
	return nil, nil
}

func (stubBuildingsRepo) BuildingByID(context.Context, int64) (*buildings.Building, error) {
	return nil, buildings.ErrNotFound
}

func (stubBuildingsRepo) CreateBuilding(_ context.Context, name string, address *string) (*buildings.Building, error) {
	return &buildings.Building{ID: 5, Name: name, Address: address, IsActive: true}, nil
}

func (stubBuildingsRepo) UpdateBuilding(context.Context, int64, buildings.BuildingPatch) (*buildings.Building, error) {
	return nil, buildings.ErrNotFound
}

func (stubBuildingsRepo) BuildingUsage(context.Context, int64) (buildings.BuildingUsage, error) {
	return buildings.BuildingUsage{}, nil
}

func (stubBuildingsRepo) DeleteBuilding(context.Context, int64) error { return buildings.ErrNotFound }

func (stubBuildingsRepo) Warehouses(context.Context, *int64) ([]buildings.Warehouse, error) {
	return nil, nil
}

func (stubBuildingsRepo) WarehouseByID(context.Context, int64) (*buildings.Warehouse, error) {
	return nil, buildings.ErrNotFound
}

func (stubBuildingsRepo) CreateWarehouse(context.Context, int64, string) (*buildings.Warehouse, error) {
	return nil, buildings.ErrNotFound
}

func (stubBuildingsRepo) UpdateWarehouse(context.Context, int64, string) (*buildings.Warehouse, error) {
	return nil, buildings.ErrNotFound
}

func (stubBuildingsRepo) DeleteWarehouse(context.Context, int64) error { return buildings.ErrNotFound }

func (stubBuildingsRepo) WarehouseNameTaken(context.Context, int64, string, int64) (bool, error) {
	return false, nil
}

func (stubBuildingsRepo) WarehouseEquipmentCount(context.Context, int64) (int, error) { return 0, nil }

// stubEquipmentRepo — минимальный репозиторий домена оборудования.
type stubEquipmentRepo struct{}

func (stubEquipmentRepo) List(context.Context, equipment.ListFilter) ([]equipment.Equipment, int, error) {
	return nil, 0, nil
}

func (stubEquipmentRepo) ByID(context.Context, int64) (*equipment.Equipment, error) {
	return nil, equipment.ErrNotFound
}

func (stubEquipmentRepo) Create(context.Context, equipment.CreateInput) (*equipment.Equipment, error) {
	return nil, equipment.ErrNotFound
}

func (stubEquipmentRepo) Update(context.Context, int64, equipment.UpdateInput) (*equipment.Equipment, error) {
	return nil, equipment.ErrNotFound
}

func (stubEquipmentRepo) SoftDelete(context.Context, int64) error { return equipment.ErrNotFound }

func (stubEquipmentRepo) RegistrationNumberTaken(context.Context, string, int64) (bool, error) {
	return false, nil
}

func (stubEquipmentRepo) EquipmentTypeExists(context.Context, int64) (bool, error) { return false, nil }

func (stubEquipmentRepo) WarehouseInfo(context.Context, int64) (*equipment.WarehouseInfo, error) {
	return nil, equipment.ErrWarehouseNotFound
}

func (stubEquipmentRepo) Move(context.Context, int64, equipment.MoveInput, equipment.Actor) (*equipment.Equipment, *equipment.Movement, error) {
	return nil, nil, equipment.ErrSameWarehouse
}

func (stubEquipmentRepo) Movements(context.Context, int64, int, int) ([]equipment.Movement, int, error) {
	return nil, 0, nil
}

// newB4Env собирает роутер Б4 поверх тех же поддельных auth-зависимостей, что и
// newTestEnv: доменные handler'ы подключаются без MySQL.
func newB4Env(t *testing.T, users ...*auth.User) (*testEnv, http.Handler) {
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
		Config:           &config.Config{Version: "test", Location: time.UTC, CORSOrigins: []string{"http://localhost:5173"}},
		Logger:           logger,
		Clock:            clock,
		AuthService:      service,
		AuthHandler:      auth.NewHandler(service, logger, false),
		Authenticator:    auth.NewAuthenticator(env.repo, tokens),
		BuildingsHandler: buildings.NewHandler(buildings.NewService(stubBuildingsRepo{}), logger),
		EquipmentHandler: equipment.NewHandler(equipment.NewService(stubEquipmentRepo{}), logger),
	})
	return env, server.Handler()
}

// Б4: маршруты зданий/складов/оборудования подключены к своим handler'ам
// (а не к заглушке 501), а права распределены по матрице PLAN §3.1.
func TestB4RoutesRegistration(t *testing.T) {
	user := activeUser(7, "user", auth.RoleUser)
	admin := activeUser(1, "admin", auth.RoleAdmin)
	env, handler := newB4Env(t, user, admin)
	userToken := env.token(t, user)
	adminToken := env.token(t, admin)

	tests := []struct {
		name   string
		method string
		target string
		token  string
		body   io.Reader
		status int
		code   string
	}{
		{"USER читает здания", http.MethodGet, "/api/buildings", userToken, nil, http.StatusOK, ""},
		{"USER не создаёт здание", http.MethodPost, "/api/buildings", userToken,
			jsonBody(map[string]any{"name": "Новое"}), http.StatusForbidden, "FORBIDDEN"},
		{"ADMIN создаёт здание", http.MethodPost, "/api/buildings", adminToken,
			jsonBody(map[string]any{"name": "Новое"}), http.StatusCreated, ""},
		{"ADMIN с пустым названием", http.MethodPost, "/api/buildings", adminToken,
			jsonBody(map[string]any{"name": "  "}), http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"USER не удаляет здание", http.MethodDelete, "/api/buildings/1", userToken, nil,
			http.StatusForbidden, "FORBIDDEN"},
		{"ADMIN удаляет несуществующее здание", http.MethodDelete, "/api/buildings/1", adminToken, nil,
			http.StatusNotFound, "NOT_FOUND"},
		{"USER читает склады", http.MethodGet, "/api/warehouses?building_id=1", userToken, nil,
			http.StatusOK, ""},
		{"ADMIN создаёт склад без здания", http.MethodPost, "/api/warehouses", adminToken,
			jsonBody(map[string]any{"name": "Склад"}), http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"USER читает оборудование", http.MethodGet, "/api/equipment", userToken, nil,
			http.StatusOK, ""},
		{"USER с некорректной пагинацией", http.MethodGet, "/api/equipment?page=0", userToken, nil,
			http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"USER перемещает (SAME_WAREHOUSE)", http.MethodPost, "/api/equipment/1/move", userToken,
			jsonBody(map[string]any{"to_warehouse_id": 2}), http.StatusConflict, "SAME_WAREHOUSE"},
		{"USER без to_warehouse_id", http.MethodPost, "/api/equipment/1/move", userToken,
			jsonBody(map[string]any{}), http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"USER не удаляет оборудование", http.MethodDelete, "/api/equipment/1", userToken, nil,
			http.StatusForbidden, "FORBIDDEN"},
		{"ADMIN удаляет несуществующее оборудование", http.MethodDelete, "/api/equipment/1", adminToken, nil,
			http.StatusNotFound, "NOT_FOUND"},
		{"журнал перемещений несуществующей единицы", http.MethodGet, "/api/equipment/1/movements", userToken, nil,
			http.StatusNotFound, "NOT_FOUND"},
		{"массовое перемещение — Б12", http.MethodPost, "/api/equipment/bulk-move", adminToken,
			jsonBody(map[string]any{}), http.StatusNotImplemented, "NOT_IMPLEMENTED"},
		{"ведомость помещения — Б6/Б10", http.MethodGet, "/api/warehouses/1/inventory.xlsx", userToken, nil,
			http.StatusNotImplemented, "NOT_IMPLEMENTED"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(handler, tc.method, tc.target, tc.token, tc.body)
			if rec.Code != tc.status {
				t.Fatalf("%s %s = %d, want %d (%s)", tc.method, tc.target, rec.Code, tc.status, rec.Body.String())
			}
			if tc.code != "" {
				body := decodeError(t, rec)
				if body["code"] != tc.code {
					t.Fatalf("code = %v, want %s", body["code"], tc.code)
				}
			}
		})
	}
}

// Форма ответов Б4 совпадает с контрактом: список зданий — массив, страница
// оборудования — объект {items,total,page,size}.
func TestB4ResponseShapes(t *testing.T) {
	user := activeUser(7, "user", auth.RoleUser)
	env, handler := newB4Env(t, user)
	token := env.token(t, user)

	buildingsRec := doRequest(handler, http.MethodGet, "/api/buildings", token, nil)
	var buildingList []buildings.BuildingDTO
	if err := json.Unmarshal(buildingsRec.Body.Bytes(), &buildingList); err != nil {
		t.Fatalf("buildings JSON: %v (%s)", err, buildingsRec.Body.String())
	}
	if len(buildingList) != 0 {
		t.Fatalf("buildings = %v", buildingList)
	}

	equipmentRec := doRequest(handler, http.MethodGet, "/api/equipment?size=50", token, nil)
	var page equipment.ListDTO
	if err := json.Unmarshal(equipmentRec.Body.Bytes(), &page); err != nil {
		t.Fatalf("equipment JSON: %v (%s)", err, equipmentRec.Body.String())
	}
	if page.Items == nil || page.Total != 0 || page.Page != 1 || page.Size != 50 {
		t.Fatalf("page = %+v", page)
	}
}

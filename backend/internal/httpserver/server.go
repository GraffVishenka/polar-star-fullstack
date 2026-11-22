// Package httpserver собирает HTTP-роутер: middleware, health-check'и,
// Swagger UI и маршруты контракта. Реализованные домены подключаются своими
// handler'ами, остальные пока отдают 501 NOT_IMPLEMENTED (контракт заморожен).
package httpserver

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/config"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/ratelimit"
)

// Лимиты AUTH-3 (значения из ТЗ).
const (
	publicTicketsLimit   = 5
	publicBuildingsLimit = 120
	rateLimitWindow      = 10 * time.Minute
)

// UsersAPI — администрирование пользователей и подбор исполнителей (Б2).
// Интерфейс объявлен в httpserver, чтобы пакет не зависел от internal/users
// и оставался тестируемым без БД.
type UsersAPI interface {
	List(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	ResetPassword(w http.ResponseWriter, r *http.Request)
	Assignable(w http.ResponseWriter, r *http.Request)
}

// BuildingsAPI — здания и склады: чтение всем авторизованным, запись,
// деактивация и физическое удаление — ADMIN (Б4, EQ-1, EQ-6).
// Интерфейс объявлен здесь по той же причине, что и UsersAPI: httpserver не
// зависит от internal/buildings и тестируется без БД.
type BuildingsAPI interface {
	ListBuildings(w http.ResponseWriter, r *http.Request)
	CreateBuilding(w http.ResponseWriter, r *http.Request)
	UpdateBuilding(w http.ResponseWriter, r *http.Request)
	DeleteBuilding(w http.ResponseWriter, r *http.Request)
	ListWarehouses(w http.ResponseWriter, r *http.Request)
	CreateWarehouse(w http.ResponseWriter, r *http.Request)
	UpdateWarehouse(w http.ResponseWriter, r *http.Request)
	DeleteWarehouse(w http.ResponseWriter, r *http.Request)
}

// EquipmentAPI — оборудование, перемещение и журнал (Б4, EQ-2, EQ-3, EQ-5,
// EQ-7). Массовое перемещение (EQ-4) реализует Б12.
type EquipmentAPI interface {
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Move(w http.ResponseWriter, r *http.Request)
	Movements(w http.ResponseWriter, r *http.Request)
}

// Deps — зависимости HTTP-слоя. Передаются из main, чтобы сервер не создавал
// подключения сам и оставался тестируемым.
type Deps struct {
	Config        *config.Config
	Logger        *slog.Logger
	DB            *sql.DB
	Clock         platform.Clock
	Calendar      platform.Calendar
	Notifier      platform.Notifier
	AuthService   *auth.Service
	AuthHandler   *auth.Handler
	Authenticator *auth.Authenticator
	UsersHandler  UsersAPI

	// Доменные handler'ы Б4. Если не переданы (unit-тесты HTTP-слоя),
	// соответствующие маршруты остаются заглушками 501.
	BuildingsHandler BuildingsAPI
	EquipmentHandler EquipmentAPI
}

// Server — собранный HTTP-сервер.
type Server struct {
	deps                   Deps
	handler                http.Handler
	startedAt              time.Time
	publicTicketsLimiter   *ratelimit.Limiter
	publicBuildingsLimiter *ratelimit.Limiter
}

// New собирает роутер и middleware.
func New(deps Deps) *Server {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Clock == nil {
		deps.Clock = platform.RealClock{}
	}
	s := &Server{
		deps:                   deps,
		startedAt:              deps.Clock.Now(),
		publicTicketsLimiter:   ratelimit.New(publicTicketsLimit, rateLimitWindow, deps.Clock),
		publicBuildingsLimiter: ratelimit.New(publicBuildingsLimit, rateLimitWindow, deps.Clock),
	}
	s.handler = s.routes()
	return s
}

// Handler возвращает http.Handler для http.Server и тестов.
func (s *Server) Handler() http.Handler { return s.handler }

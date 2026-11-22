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

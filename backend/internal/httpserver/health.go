package httpserver

import (
	"context"
	"net/http"
	"time"
)

// HealthResponse — тело GET /health.
type HealthResponse struct {
	Status        string `json:"status"`
	App           string `json:"app"`
	Version       string `json:"version"`
	Time          string `json:"time"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// ReadinessChecks — состояние зависимостей.
type ReadinessChecks struct {
	Database string `json:"database"`
}

// ReadinessResponse — тело GET /health/ready.
type ReadinessResponse struct {
	Status string          `json:"status"`
	Checks ReadinessChecks `json:"checks"`
}

// health — liveness: процесс жив и принимает запросы (NFR-4). Не зависит от БД.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	loc := s.deps.Config.Location
	if loc == nil {
		loc = time.UTC
	}
	s.writeJSON(w, http.StatusOK, HealthResponse{
		Status:        "ok",
		App:           "polar-star-service",
		Version:       s.deps.Config.Version,
		Time:          s.deps.Clock.Now().In(loc).Format(time.RFC3339),
		UptimeSeconds: int64(s.deps.Clock.Now().Sub(s.startedAt).Seconds()),
	})
}

// ready — readiness: проверяет БД. Без БД отдаёт 503 (критерий Б1).
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if s.deps.DB == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, ReadinessResponse{
			Status: "unavailable",
			Checks: ReadinessChecks{Database: "down"},
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.deps.DB.PingContext(ctx); err != nil {
		s.deps.Logger.Warn("health: база данных недоступна", "error", err)
		s.writeJSON(w, http.StatusServiceUnavailable, ReadinessResponse{
			Status: "unavailable",
			Checks: ReadinessChecks{Database: "down"},
		})
		return
	}
	s.writeJSON(w, http.StatusOK, ReadinessResponse{
		Status: "ready",
		Checks: ReadinessChecks{Database: "ok"},
	})
}

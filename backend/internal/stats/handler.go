package stats

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Handler — HTTP-слой статистики. Тонкий: разбор параметров и вызовы сервиса.
// Права ADMIN на сводную статистику проверяет middleware маршрута
// (`/api/admin/stats`), а также сервис (для повторного использования в Б11).
type Handler struct {
	svc    *Service
	logger *slog.Logger
	// UserFrom — источник аутентифицированного пользователя. В API это
	// auth.UserFrom; unit-тесты подменяют его, чтобы не поднимать JWT/MySQL.
	UserFrom func(context.Context) (*auth.User, bool)
}

// NewHandler создаёт HTTP-handler'ы статистики.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger, UserFrom: auth.UserFrom}
}

// Me — GET /api/stats/me?from&to: личная статистика (STATS-1, USER/ADMIN).
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	query, err := h.parseQuery(r, false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	stats, err := h.svc.MyStats(r.Context(), actor, query)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewStatsDTO(stats, h.svc.Location()))
}

// Admin — GET /api/admin/stats?user_id&building_id&from&to: сводная статистика
// (STATS-1, только ADMIN).
func (h *Handler) Admin(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	query, err := h.parseQuery(r, true)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items, err := h.svc.AdminStats(r.Context(), actor, query)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewStatsListDTO(items, h.svc.Location()))
}

// parseQuery разбирает параметры отчёта. Использует общий ParseQuery, чтобы
// Excel-выгрузка Б11 трактовала период/фильтры ровно так же, как JSON API.
func (h *Handler) parseQuery(r *http.Request, withFilters bool) (Query, error) {
	return ParseQuery(h.svc.Location(), r.URL.Query(), withFilters)
}

// ParseQuery разбирает `from`/`to` (даты `YYYY-MM-DD` в поясе приложения) и, при
// withFilters, `user_id`/`building_id`. Некорректные значения — 422.
// Экспортируется для Excel-выгрузки Б11 (STATS-3).
func ParseQuery(loc *time.Location, values url.Values, withFilters bool) (Query, error) {
	if loc == nil {
		loc = time.UTC
	}
	query := Query{}

	if raw := strings.TrimSpace(values.Get("from")); raw != "" {
		parsed, err := time.ParseInLocation(platform.DateLayout, raw, loc)
		if err != nil {
			return Query{}, apierr.Validation("Некорректная дата периода",
				map[string]any{"from": "ожидается YYYY-MM-DD"})
		}
		query.From = &parsed
	}
	if raw := strings.TrimSpace(values.Get("to")); raw != "" {
		parsed, err := time.ParseInLocation(platform.DateLayout, raw, loc)
		if err != nil {
			return Query{}, apierr.Validation("Некорректная дата периода",
				map[string]any{"to": "ожидается YYYY-MM-DD"})
		}
		query.To = &parsed
	}
	if query.From != nil && query.To != nil && query.From.After(*query.To) {
		return Query{}, apierr.Validation("Некорректный период",
			map[string]any{"to": "должен быть не раньше from"})
	}

	if !withFilters {
		return query, nil
	}

	if raw := strings.TrimSpace(values.Get("user_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return Query{}, apierr.Validation("Некорректный параметр user_id",
				map[string]any{"user_id": "ожидается положительное целое"})
		}
		query.UserID = &id
	}
	if raw := strings.TrimSpace(values.Get("building_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return Query{}, apierr.Validation("Некорректный параметр building_id",
				map[string]any{"building_id": "ожидается положительное целое"})
		}
		query.BuildingID = &id
	}
	return query, nil
}

// actor достаёт авторизованного пользователя (маршруты статистики всегда за
// RequireAuth; проверка нужна для defensive-ответа 401).
func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user, ok := h.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return nil, false
	}
	return user, true
}

// writeJSON отдаёт JSON-ответ (формат API, PLAN §3.13). Статистика содержит
// персональные данные — кеширование запрещено.
func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		h.logger.Error("stats: запись ответа", "error", err)
	}
}

// fail отдаёт ошибку в едином формате и логирует серверные сбои.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierr.Error
	if errors.As(err, &apiErr) && apiErr.Status >= http.StatusInternalServerError {
		h.logger.Error("stats: ошибка запроса",
			"route", r.URL.Path, "status", apiErr.Status, "error", apiErr.Error())
	}
	apierr.Respond(w, r, err)
}

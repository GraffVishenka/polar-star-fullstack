package trash

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Handler — HTTP-слой корзины. Все маршруты доступны только ADMIN (проверяет
// middleware маршрута).
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler создаёт HTTP-handler'ы домена.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger}
}

// List — GET /api/admin/trash?type=&page=&size= (TRASH-1).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	kind, err := parseType(query.Get("type"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page, size, details := parsePagination(query.Get("page"), query.Get("size"))
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте параметры пагинации", fieldDetails(details)))
		return
	}

	result, err := h.svc.List(r.Context(), kind, page, size)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewListDTO(result))
}

// Restore — POST /api/admin/trash/{type}/{id}/restore (TRASH-2).
func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	kind, id, err := h.pathParams(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	result, err := h.svc.Restore(r.Context(), kind, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewRestoreDTO(result))
}

// Purge — DELETE /api/admin/trash/{type}/{id} (TRASH-3): физическое удаление,
// необратимо.
func (h *Handler) Purge(w http.ResponseWriter, r *http.Request) {
	kind, id, err := h.pathParams(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	if err := h.svc.Purge(r.Context(), kind, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pathParams читает {type} и {id} из пути.
func (h *Handler) pathParams(r *http.Request) (Type, int64, error) {
	kind, err := parseType(chi.URLParam(r, "type"))
	if err != nil {
		return "", 0, err
	}
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	id, parseErr := strconv.ParseInt(raw, 10, 64)
	if parseErr != nil || id <= 0 {
		return "", 0, apierr.NotFound("Элемент корзины не найден")
	}
	return kind, id, nil
}

// parseType проверяет параметр type (openapi TrashType). Пустое значение —
// ошибка валидации: фильтр по типу обязателен.
func parseType(raw string) (Type, error) {
	kind := Type(strings.TrimSpace(raw))
	if kind == "" {
		return "", apierr.Validation("Параметр type обязателен",
			map[string]any{"type": "ожидается ticket, kb или equipment"})
	}
	if !kind.Valid() {
		return "", apierr.Validation("Неизвестный тип элемента корзины",
			map[string]any{"type": "ожидается ticket, kb или equipment"})
	}
	return kind, nil
}

// parsePagination разбирает page/size (по умолчанию 1/20, size ≤ 100).
func parsePagination(pageRaw, sizeRaw string) (int, int, map[string]string) {
	page, size := 1, DefaultPageSize
	details := map[string]string{}

	if raw := strings.TrimSpace(pageRaw); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			details["page"] = "ожидается целое число от 1"
		} else {
			page = value
		}
	}
	if raw := strings.TrimSpace(sizeRaw); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > MaxPageSize {
			details["size"] = "ожидается целое число от 1 до 100"
		} else {
			size = value
		}
	}
	return page, size, details
}

// fieldDetails приводит ошибки валидации к виду details в ответе API.
func fieldDetails(details map[string]string) map[string]any {
	out := make(map[string]any, len(details))
	for field, message := range details {
		out[field] = message
	}
	return out
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		h.logger.Error("trash: запись ответа", "error", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierr.Respond(w, r, err)
}

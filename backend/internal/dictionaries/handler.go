package dictionaries

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Handler — HTTP-слой справочников. Один набор методов обслуживает оба
// справочника (вид передаётся константой из маршрута); права проверяет
// middleware маршрута, но include_inactive дополнительно ограничен ADMIN.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler создаёт HTTP-handler'ы.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger}
}

// ListProblemTypes — GET /api/admin/problem-types (ADMIN).
func (h *Handler) ListProblemTypes(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, KindProblemTypes)
}

// CreateProblemType — POST /api/admin/problem-types (ADMIN).
func (h *Handler) CreateProblemType(w http.ResponseWriter, r *http.Request) {
	h.create(w, r, KindProblemTypes)
}

// UpdateProblemType — PATCH /api/admin/problem-types/{id} (ADMIN).
func (h *Handler) UpdateProblemType(w http.ResponseWriter, r *http.Request) {
	h.update(w, r, KindProblemTypes)
}

// DeleteProblemType — DELETE /api/admin/problem-types/{id} (ADMIN):
// деактивация, физического удаления нет.
func (h *Handler) DeleteProblemType(w http.ResponseWriter, r *http.Request) {
	h.deactivate(w, r, KindProblemTypes)
}

// ListEquipmentTypes — GET /api/admin/equipment-types (ADMIN).
func (h *Handler) ListEquipmentTypes(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, KindEquipmentTypes)
}

// CreateEquipmentType — POST /api/admin/equipment-types (ADMIN).
func (h *Handler) CreateEquipmentType(w http.ResponseWriter, r *http.Request) {
	h.create(w, r, KindEquipmentTypes)
}

// UpdateEquipmentType — PATCH /api/admin/equipment-types/{id} (ADMIN).
func (h *Handler) UpdateEquipmentType(w http.ResponseWriter, r *http.Request) {
	h.update(w, r, KindEquipmentTypes)
}

// DeleteEquipmentType — DELETE /api/admin/equipment-types/{id} (ADMIN):
// деактивация, физического удаления нет.
func (h *Handler) DeleteEquipmentType(w http.ResponseWriter, r *http.Request) {
	h.deactivate(w, r, KindEquipmentTypes)
}

// PublicProblemTypes — GET /api/public/problem-types: только активные виды
// проблем для формы анонимной заявки (id, name, sort_order).
func (h *Handler) PublicProblemTypes(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.PublicList(r.Context(), KindProblemTypes)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewPublicProblemTypeDTOs(items))
}

// list читает справочник: активные по умолчанию, include_inactive — только ADMIN.
func (h *Handler) list(w http.ResponseWriter, r *http.Request, kind Kind) {
	includeInactive, valid := includeInactiveParam(r)
	if !valid {
		h.fail(w, r, apierr.Validation("Некорректный параметр include_inactive",
			map[string]any{"include_inactive": "ожидается true или false"}))
		return
	}

	items, err := h.svc.List(r.Context(), kind, includeInactive)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewDTOs(items))
}

// create добавляет элемент справочника.
func (h *Handler) create(w http.ResponseWriter, r *http.Request, kind Kind) {
	var req CreateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateCreate(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля справочника", fieldDetails(details)))
		return
	}

	item, err := h.svc.Create(r.Context(), kind, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, NewDTO(item))
}

// update правит элемент справочника.
func (h *Handler) update(w http.ResponseWriter, r *http.Request, kind Kind) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req UpdateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateUpdate(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля справочника", fieldDetails(details)))
		return
	}

	item, err := h.svc.Update(r.Context(), kind, id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewDTO(item))
}

// deactivate — DELETE: справочники не удаляются физически, а деактивируются
// (история заявок/оборудования должна сохранять названия).
func (h *Handler) deactivate(w http.ResponseWriter, r *http.Request, kind Kind) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), kind, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// includeInactiveParam читает ?include_inactive=. Параметр действует только для
// ADMIN: USER всегда получает активные элементы. Второе значение — false для
// нечитаемого boolean (422).
func includeInactiveParam(r *http.Request) (bool, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("include_inactive"))
	if raw == "" {
		return false, true
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, false
	}
	user, ok := auth.UserFrom(r.Context())
	if !ok {
		return false, true
	}
	return value && user.IsAdmin(), true
}

// pathID читает {id} из пути; неразбираемый id — 404.
func pathID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apierr.NotFound("Элемент справочника не найден")
	}
	return id, nil
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		h.logger.Error("dictionaries: запись ответа", "error", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierr.Respond(w, r, err)
}

// decodeJSON читает тело запроса с лимитом 1 МБ и отвергает мусор после
// объекта.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return apierr.BadRequest(apierr.CodeInvalidJSON, "Тело запроса пусто")
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apierr.New(http.StatusRequestEntityTooLarge, apierr.CodePayloadTooLarge, "Слишком большое тело запроса")
		}
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	return nil
}

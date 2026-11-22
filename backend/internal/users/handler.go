package users

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Handler — HTTP-слой администрирования пользователей (AUTH-4..AUTH-9).
// Тонкий: разбор запроса/параметров и вызовы Service.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler создаёт handler'ы пользователей.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// createRequest — тело POST /api/admin/users.
type createRequest struct {
	Login      string    `json:"login"`
	FullName   string    `json:"full_name"`
	Role       auth.Role `json:"role"`
	Password   string    `json:"password"`
	BuildingID *int64    `json:"building_id"`
}

// updateRequest — тело PATCH /api/admin/users/{id}; BuildingID различает
// отсутствие поля и явный null (очистка здания, AUTH-8).
type updateRequest struct {
	FullName   *string       `json:"full_name"`
	Role       *auth.Role    `json:"role"`
	BuildingID OptionalInt64 `json:"building_id"`
	IsActive   *bool         `json:"is_active"`
}

// resetPasswordRequest — тело POST /api/admin/users/{id}/password.
type resetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// List — GET /api/admin/users (ADMIN).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var filter ListFilter

	if raw := q.Get("role"); raw != "" {
		role := auth.Role(raw)
		if !role.Valid() {
			h.fail(w, r, validation("role", "неизвестная роль"))
			return
		}
		filter.Role = role
	}
	buildingID, err := optionalInt64Query(q, "building_id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	filter.BuildingID = buildingID
	isActive, err := optionalBoolQuery(q, "is_active")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	filter.IsActive = isActive
	filter.Search = q.Get("search")

	page, size, err := parsePagination(q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	filter.Page, filter.Size = page, size

	result, err := h.service.List(r.Context(), filter)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]UserDTO, 0, len(result.Items))
	for _, user := range result.Items {
		items = append(items, auth.NewUserDTO(user))
	}
	h.writeJSON(w, http.StatusOK, UserListDTO{
		Items: items,
		Total: result.Total,
		Page:  result.Page,
		Size:  result.Size,
	})
}

// Create — POST /api/admin/users (ADMIN, AUTH-4).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	user, err := h.service.Create(r.Context(), CreateInput{
		Login:      req.Login,
		FullName:   req.FullName,
		Role:       req.Role,
		Password:   req.Password,
		BuildingID: req.BuildingID,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, auth.NewUserDTO(user))
}

// Update — PATCH /api/admin/users/{id} (ADMIN, AUTH-7/AUTH-8).
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	var req updateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	user, err := h.service.Update(r.Context(), actor.ID, id, UpdateInput{
		FullName:   req.FullName,
		Role:       req.Role,
		BuildingID: req.BuildingID,
		IsActive:   req.IsActive,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, auth.NewUserDTO(user))
}

// ResetPassword — POST /api/admin/users/{id}/password (ADMIN, AUTH-5).
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	var req resetPasswordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.service.ResetPassword(r.Context(), actor.ID, id, req.NewPassword); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Assignable — GET /api/users/assignable?ticket_id= (USER/ADMIN, TICKET-8).
func (h *Handler) Assignable(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	raw := r.URL.Query().Get("ticket_id")
	ticketID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ticketID <= 0 {
		h.fail(w, r, validation("ticket_id", "обязательный параметр, целое число > 0"))
		return
	}
	items, err := h.service.Assignable(r.Context(), actor, ticketID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]AssignableUserDTO, 0, len(items))
	for _, user := range items {
		out = append(out, NewAssignableUserDTO(user))
	}
	h.writeJSON(w, http.StatusOK, out)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		h.logger.Error("users: запись ответа", "error", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierr.Respond(w, r, err)
}

// pathID разбирает {id} из пути.
func pathID(r *http.Request) (int64, error) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, validation("id", "некорректный идентификатор")
	}
	return id, nil
}

func parsePagination(q url.Values) (page, size int, err error) {
	page, size = 1, 20
	if raw := q.Get("page"); raw != "" {
		value, convErr := strconv.Atoi(raw)
		if convErr != nil || value < 1 {
			return 0, 0, validation("page", "целое число ≥ 1")
		}
		page = value
	}
	if raw := q.Get("size"); raw != "" {
		value, convErr := strconv.Atoi(raw)
		if convErr != nil || value < 1 || value > 100 {
			return 0, 0, validation("size", "целое число от 1 до 100")
		}
		size = value
	}
	return page, size, nil
}

func optionalInt64Query(q url.Values, key string) (*int64, error) {
	raw := q.Get(key)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, validation(key, "целое число > 0")
	}
	return &value, nil
}

func optionalBoolQuery(q url.Values, key string) (*bool, error) {
	raw := q.Get(key)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, validation(key, "ожидается true или false")
	}
	return &value, nil
}

// decodeJSON читает тело с лимитом 1 МБ и отклоняет мусор после объекта.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return apierr.BadRequest(apierr.CodeInvalidJSON, "Тело запроса пусто")
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apierr.New(http.StatusRequestEntityTooLarge, apierr.CodePayloadTooLarge,
				"Слишком большое тело запроса")
		}
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	return nil
}

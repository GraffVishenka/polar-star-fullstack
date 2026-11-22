package notifications

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Handler — HTTP-слой push (NOTIF-1, NOTIF-5).
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler создаёт handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// PublicKey — GET /api/push/public-key (USER/ADMIN).
func (h *Handler) PublicKey(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"public_key": h.service.PublicKey()})
}

type subscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	UserAgent string `json:"user_agent"`
}

// Subscribe — POST /api/push/subscriptions (USER/ADMIN).
func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	var req subscribeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	subscription, err := h.service.Subscribe(r.Context(), actor.ID, SubscribeInput{
		Endpoint:  req.Endpoint,
		P256DH:    req.Keys.P256DH,
		Auth:      req.Keys.Auth,
		UserAgent: req.UserAgent,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.service.SubscriptionDTO(subscription))
}

// Delete — DELETE /api/push/subscriptions/{id} (только своя, NOTIF-5).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.fail(w, r, apierr.NotFound("Подписка не найдена"))
		return
	}
	if err := h.service.Delete(r.Context(), actor.ID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return apierr.BadRequest(apierr.CodeInvalidJSON, "Тело запроса пусто")
		}
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON")
	}
	return nil
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		h.logger.Error("notifications: запись ответа", "error", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierr.Error
	if errors.As(err, &apiErr) {
		apierr.Write(w, r, apiErr.Status, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}
	h.logger.Error("notifications: внутренняя ошибка", "error", err)
	apierr.Write(w, r, http.StatusInternalServerError, apierr.CodeInternal,
		"Внутренняя ошибка", nil)
}

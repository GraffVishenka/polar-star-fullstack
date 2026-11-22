package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/middleware"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Cookie с refresh-токеном (AUTH-2): httpOnly, SameSite=Lax, path=/api/auth,
// срок 7 дней. Secure включается в production.
const (
	RefreshCookieName = "refresh_token"
	RefreshCookiePath = "/api/auth"
)

// Handler — HTTP-слой аутентификации. Тонкий: валидация формата + вызовы
// сервиса, вся бизнес-логика в Service.
type Handler struct {
	svc           *Service
	logger        *slog.Logger
	secureCookies bool
}

// NewHandler создаёт HTTP-handler'ы auth. secureCookies=true в production
// (HTTPS обязателен для Web Push, NFR-2).
func NewHandler(svc *Service, logger *slog.Logger, secureCookies bool) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger, secureCookies: secureCookies}
}

// Login — POST /api/auth/login (AUTH-1).
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	session, err := h.svc.Login(r.Context(), req.Login, req.Password, middleware.ClientIP(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.setRefreshCookie(w, session.RefreshToken, h.svc.RefreshTTL())
	h.writeJSON(w, http.StatusOK, NewSessionDTO(session))
}

// Refresh — POST /api/auth/refresh: ротация refresh-токена (AUTH-2).
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(RefreshCookieName)
	if err != nil || cookie.Value == "" {
		h.clearRefreshCookie(w)
		h.fail(w, r, apierr.Unauthorized(apierr.CodeInvalidRefreshToken, "Сессия недействительна, войдите снова"))
		return
	}

	session, err := h.svc.Refresh(r.Context(), cookie.Value)
	if err != nil {
		// Невалидная/повторно использованная cookie больше не нужна клиенту.
		h.clearRefreshCookie(w)
		h.fail(w, r, err)
		return
	}
	h.setRefreshCookie(w, session.RefreshToken, h.svc.RefreshTTL())
	h.writeJSON(w, http.StatusOK, NewSessionDTO(session))
}

// Logout — POST /api/auth/logout: отзывает текущий refresh-токен (идемпотентно).
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(RefreshCookieName); err == nil && cookie.Value != "" {
		if err := h.svc.Logout(r.Context(), cookie.Value); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me — GET /api/auth/me: профиль текущего пользователя (AUTH-6).
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	h.writeJSON(w, http.StatusOK, NewUserDTO(user))
}

// ChangePassword — POST /api/auth/password: смена своего пароля (AUTH-5).
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	user, ok := UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}

	currentRefresh := ""
	if cookie, err := r.Cookie(RefreshCookieName); err == nil {
		currentRefresh = cookie.Value
	}
	if err := h.svc.ChangePassword(r.Context(), user.ID, req.CurrentPassword, req.NewPassword, currentRefresh); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setRefreshCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    token,
		Path:     RefreshCookiePath,
		MaxAge:   int(ttl.Seconds()),
		Expires:  time.Now().Add(ttl),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     RefreshCookiePath,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		h.logger.Error("auth: запись ответа", "error", err)
	}
}

// fail отдаёт ошибку в едином формате; для 429 пробрасывает Retry-After (AUTH-3).
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierr.Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests {
		if seconds, ok := apiErr.Details["retry_after_seconds"]; ok {
			if value, ok := seconds.(int); ok && value > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(value))
			}
		}
	}
	apierr.Respond(w, r, err)
}

// decodeJSON читает тело запроса с лимитом 1 МБ и строгим JSON.
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
	// Отклоняем «мусор» после объекта, чтобы не принимались составные тела.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	return nil
}

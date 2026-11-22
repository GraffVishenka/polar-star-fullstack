package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/reqid"
)

type userContextKey struct{}

// UserFrom достаёт аутентифицированного пользователя из контекста.
func UserFrom(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(userContextKey{}).(*User)
	return user, ok
}

// WithUser кладёт пользователя в контекст. Используется requireAuth, а также
// тестами HTTP-слоя и внутренними вызовами без реального запроса.
func WithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

// Authenticator проверяет access-токены и права ролей.
type Authenticator struct {
	repo   Repository
	tokens *TokenManager
}

// NewAuthenticator создаёт middleware-аутентификатор.
func NewAuthenticator(repo Repository, tokens *TokenManager) *Authenticator {
	return &Authenticator{repo: repo, tokens: tokens}
}

// RequireAuth проверяет Bearer-токен (AUTH-2) и на каждом запросе читает
// пользователя из БД, чтобы блокировка действовала со следующего запроса
// (AUTH-6). Роль берётся из БД, а не из токена.
func (a *Authenticator) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header == "" {
			unauthorized(w, r)
			return
		}
		const bearer = "Bearer "
		if len(header) <= len(bearer) || !strings.EqualFold(header[:len(bearer)], bearer) {
			unauthorized(w, r)
			return
		}
		claims, err := a.tokens.Parse(strings.TrimSpace(header[len(bearer):]))
		if err != nil {
			unauthorized(w, r)
			return
		}
		userID, err := UserIDFromClaims(claims)
		if err != nil {
			unauthorized(w, r)
			return
		}
		user, err := a.repo.UserByID(r.Context(), userID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				unauthorized(w, r)
				return
			}
			apierr.Respond(w, r, apierr.Internal(err))
			return
		}
		if !user.IsActive {
			unauthorized(w, r)
			return
		}

		reqid.SetUserID(r.Context(), user.ID)
		ctx := context.WithValue(r.Context(), userContextKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole пропускает только пользователей с одной из ролей. Используется
// для ADMIN-маршрутов (`requireRole("ADMIN")` в ТЗ).
func (a *Authenticator) RequireRole(roles ...Role) func(http.Handler) http.Handler {
	allowed := make(map[Role]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFrom(r.Context())
			if !ok {
				unauthorized(w, r)
				return
			}
			if _, ok := allowed[user.Role]; !ok {
				apierr.Write(w, r, http.StatusForbidden, apierr.CodeForbidden,
					"Недостаточно прав", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePasswordChanged блокирует все маршруты, кроме me/password/logout,
// если у пользователя стоит must_change_password (AUTH-4): ответ 428.
func RequirePasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFrom(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if user.MustChangePassword && !passwordChangeAllowed(r.Method, r.URL.Path) {
			apierr.Write(w, r, http.StatusPreconditionRequired, apierr.CodePasswordChangeRequired,
				"Требуется смена временного пароля", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// passwordChangeAllowed — маршруты, доступные до смены временного пароля.
func passwordChangeAllowed(method, path string) bool {
	switch {
	case method == http.MethodGet && path == "/api/auth/me":
		return true
	case method == http.MethodPost && path == "/api/auth/password":
		return true
	case method == http.MethodPost && path == "/api/auth/logout":
		return true
	default:
		return false
	}
}

// unauthorized отдаёт 401 в едином формате, не раскрывая причину (битый токен /
// несуществующий пользователь / блокировка).
func unauthorized(w http.ResponseWriter, r *http.Request) {
	apierr.Write(w, r, http.StatusUnauthorized, apierr.CodeUnauthorized,
		"Требуется авторизация", nil)
}

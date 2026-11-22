package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/ratelimit"
)

// RateLimit применяет token bucket к запросам с ключом key.
// При превышении отдаёт 429 RATE_LIMITED и Retry-After (AUTH-3).
func RateLimit(limiter *ratelimit.Limiter, key func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := limiter.Take(key(r))
			if !allowed {
				seconds := ratelimit.RetryAfterSeconds(retryAfter)
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				apierr.Respond(w, r, apierr.TooManyRequests(seconds))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS разрешает запросы только с настроенных origin (NFR-2). При списке
// origin'ов возвращается конкретный origin и Allow-Credentials: true; при "*"
// (только для локальной разработки) origin эхом повторяется, потому что браузер
// запрещает credentials с wildcard.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	wildcard := false
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "*" {
			wildcard = true
			continue
		}
		if o != "" {
			allowed[strings.TrimSuffix(o, "/")] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSuffix(r.Header.Get("Origin"), "/")
			if origin != "" {
				_, ok := allowed[origin]
				if ok || wildcard {
					h := w.Header()
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Access-Control-Allow-Credentials", "true")
					h.Set("Vary", "Origin")
					if r.Method == http.MethodOptions {
						// Preflight: разрешаем методы и заголовки, используемые API.
						h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
						h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
						h.Set("Access-Control-Max-Age", "600")
						w.WriteHeader(http.StatusNoContent)
						return
					}
				} else if r.Method == http.MethodOptions {
					// Чужой origin: не подтверждаем CORS, preflight без заголовков.
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders добавляет базовые защитные заголовки, безопасные для API и
// Swagger UI.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

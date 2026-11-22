// Package middleware — HTTP-обвязка: request_id, recover, CORS, JSON-логи,
// rate limit и общие заголовки безопасности (NFR-2, NFR-4, AUTH-3).
package middleware

import (
	"net/http"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/reqid"
)

// RequestID назначает каждому запросу request_id (внешний санитизируется),
// кладёт Info в контекст и возвращает заголовок клиенту. Info намеренно
// создаётся здесь: access-log и requireAuth дописывают в него UserID.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := reqid.SanitizeHeader(r.Header.Get(reqid.Header))
		if id == "" {
			id = reqid.New()
		}
		info := &reqid.Info{RequestID: id}
		ctx := reqid.WithInfo(r.Context(), info)
		reqid.SetHeader(w, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/reqid"
)

// statusWriter перехватывает статус и объём ответа для access-лога и не ломает
// интерфейсы http.Flusher/http.Pusher, нужные для стриминга файлов (Б3).
type statusWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.status = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Flush пробрасывает flush (SSE/большие файлы).
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap нужен http.ResponseController (Go 1.20+) для доступа к оригиналу.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// AccessLog пишет JSON-лог запроса: request_id, user_id, route, status,
// duration_ms, method, path, ip (NFR-4).
func AccessLog(logger *slog.Logger, clock platform.Clock) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := clock.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			duration := clock.Now().Sub(start)

			route := ""
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				route = rctx.RoutePattern()
			}
			attrs := []any{
				"request_id", reqid.From(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"route", route,
				"status", sw.status,
				"duration_ms", float64(duration.Microseconds()) / 1000.0,
				"bytes", sw.bytes,
				"ip", ClientIP(r),
			}
			if uid := reqid.UserID(r.Context()); uid > 0 {
				attrs = append(attrs, "user_id", uid)
			}
			switch {
			case sw.status >= 500:
				logger.Error("http request failed", attrs...)
			case sw.status >= 400:
				logger.Warn("http request rejected", attrs...)
			default:
				logger.Info("http request", attrs...)
			}
		})
	}
}

// Recover перехватывает паники handler'ов: логирует стек и отдаёт 500 в едином
// формате, не роняя процесс.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				logger.Error("panic recovered",
					"request_id", reqid.From(r.Context()),
					"panic", rec,
					"stack", string(debug.Stack()),
					"method", r.Method,
					"path", r.URL.Path,
				)
				apierr.Write(w, r, http.StatusInternalServerError, apierr.CodeInternal,
					"Внутренняя ошибка сервера", nil)
			}()
			next.ServeHTTP(w, r)
		})
	}
}

package middleware

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP определяет IP клиента для rate limit. API работает за reverse proxy
// (PLAN §4, п. 21), поэтому доверяем X-Real-IP / X-Forwarded-For; при прямом
// доступе используется RemoteAddr. В проде прокси обязан перезаписывать эти
// заголовки, иначе клиент сможет подделать IP (NFR-2).
func ClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		if parsed := net.ParseIP(ip); parsed != nil {
			return parsed.String()
		}
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Первый адрес — исходный клиент.
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if parsed := net.ParseIP(first); parsed != nil {
			return parsed.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ClientIPKey — ключ лимитера по IP клиента.
func ClientIPKey(r *http.Request) string { return ClientIP(r) }

// Package reqid хранит идентификатор запроса и данные аутентификации в
// context.Context. Отдельный пакет нужен, чтобы middleware, apierr и handler'ы
// могли пользоваться request_id без циклических импортов.
package reqid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

// Header — HTTP-заголовок с идентификатором запроса (NFR-4).
const Header = "X-Request-ID"

type ctxKey struct{}

// Info — данные запроса, доступные всем слоям. Указатель намеренно общий:
// access-log middleware создаёт его до вызова handler'а, а requireAuth
// дописывает UserID, поэтому лог видит пользователя после обработки.
type Info struct {
	RequestID string
	UserID    int64
}

// New генерирует новый request_id (16 случайных байт в hex).
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand не должен падать; резервный вариант — пустой id.
		return ""
	}
	return hex.EncodeToString(b[:])
}

// WithInfo кладёт Info в контекст.
func WithInfo(ctx context.Context, info *Info) context.Context {
	return context.WithValue(ctx, ctxKey{}, info)
}

// InfoFrom достаёт Info из контекста (nil, если middleware не установил).
func InfoFrom(ctx context.Context) *Info {
	info, _ := ctx.Value(ctxKey{}).(*Info)
	return info
}

// From возвращает request_id из контекста или пустую строку.
func From(ctx context.Context) string {
	if info := InfoFrom(ctx); info != nil {
		return info.RequestID
	}
	return ""
}

// SetUserID записывает id пользователя в общий Info (вызывается requireAuth).
func SetUserID(ctx context.Context, userID int64) {
	if info := InfoFrom(ctx); info != nil {
		info.UserID = userID
	}
}

// UserID возвращает id аутентифицированного пользователя или 0.
func UserID(ctx context.Context) int64 {
	if info := InfoFrom(ctx); info != nil {
		return info.UserID
	}
	return 0
}

// SanitizeHeader проверяет внешний X-Request-ID: только безопасные символы и
// ограниченная длина, иначе значение игнорируется и генерируется новое.
func SanitizeHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return ""
		}
	}
	return value
}

// SetHeader проставляет request_id в ответ (используется middleware).
func SetHeader(w http.ResponseWriter, id string) {
	if id != "" {
		w.Header().Set(Header, id)
	}
}

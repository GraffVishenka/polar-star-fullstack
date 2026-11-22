package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// RefreshTokenLength — 32 случайных байта (AUTH-2, TICKET-13 — тот же размер для
// токена анонимной заявки).
const RefreshTokenLength = 32

// NewRefreshToken генерирует открытый refresh-токен (base64url без padding) и
// его SHA-256 хеш для хранения в БД.
func NewRefreshToken() (plain, hash string, err error) {
	buf := make([]byte, RefreshTokenLength)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("auth: генерация refresh-токена: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	return plain, HashToken(plain), nil
}

// HashToken считает SHA-256 в hex. Используется для refresh-токенов и токенов
// доступа анонимов (TICKET-13): в БД не должно быть открытых токенов.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

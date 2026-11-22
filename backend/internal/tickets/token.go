package tickets

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// accessTokenBytes — 32 случайных байта для токена анонимной заявки (TICKET-13).
const accessTokenBytes = 32

// newAccessToken генерирует открытый токен (base64url) и его SHA-256 для БД.
// Открытый токен показывается пользователю один раз и в БД не хранится.
func newAccessToken() (plain string, hash string, err error) {
	buf := make([]byte, accessTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("tickets: генерация токена доступа: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	return plain, auth.HashToken(plain), nil
}

package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// AccessTokenTTL — TTL access-токена (AUTH-2): 15 минут.
const AccessTokenTTL = 15 * time.Minute

// Claims — полезная нагрузка access-токена. Роль кладётся в токен для быстрых
// проверок, но requireAuth всё равно читает пользователя из БД (AUTH-6).
type Claims struct {
	Role Role `json:"role"`
	jwt.RegisteredClaims
}

// TokenManager выпускает и проверяет access-токены JWT HS256.
type TokenManager struct {
	secret []byte
	issuer string
	ttl    time.Duration
	clock  platform.Clock
}

// NewTokenManager создаёт менеджер токенов. secret должен быть ≥32 символов —
// это проверяет config (NFR-2).
func NewTokenManager(secret, issuer string, ttl time.Duration, clock platform.Clock) *TokenManager {
	if ttl <= 0 {
		ttl = AccessTokenTTL
	}
	if clock == nil {
		clock = platform.RealClock{}
	}
	return &TokenManager{secret: []byte(secret), issuer: issuer, ttl: ttl, clock: clock}
}

// TTL возвращает время жизни access-токена (для expires_in в ответе логина).
func (m *TokenManager) TTL() time.Duration { return m.ttl }

// Issue выпускает access-токен для пользователя.
func (m *TokenManager) Issue(user *User) (string, time.Time, error) {
	now := m.clock.Now()
	expiresAt := now.Add(m.ttl)
	claims := Claims{
		Role: user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(user.ID, 10),
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: подпись access-токена: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse проверяет подпись, срок действия, issuer и алгоритм HS256.
func (m *TokenManager) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		// Защита от подмены алгоритма (alg=none / RS256).
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: неожиданный алгоритм подписи %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(), jwt.WithLeeway(15*time.Second),
		// Время проверки берётся из инжектированных часов: тесты и бизнес-логика
		// не зависят от системного времени (NFR-7).
		jwt.WithTimeFunc(func() time.Time { return m.clock.Now() }))
	if err != nil {
		return nil, err
	}
	if claims.Subject == "" || !claims.Role.Valid() {
		return nil, fmt.Errorf("auth: некорректные claims")
	}
	return claims, nil
}

// UserIDFromClaims достаёт числовой id пользователя из subject.
func UserIDFromClaims(claims *Claims) (int64, error) {
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("auth: некорректный subject %q", claims.Subject)
	}
	return id, nil
}

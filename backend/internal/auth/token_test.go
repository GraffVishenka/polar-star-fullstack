package auth

import (
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

func testTokenManager(t *testing.T, now time.Time) *TokenManager {
	t.Helper()
	return NewTokenManager("0123456789abcdef0123456789abcdef", "polar-star-service-test",
		AccessTokenTTL, platform.FixedClock{Time: now})
}

// AUTH-2: access-токен HS256 с TTL 15 минут, subject = id пользователя.
func TestTokenManager_IssueAndParse(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	manager := testTokenManager(t, now)

	user := &User{ID: 42, Role: RoleAdmin}
	token, expiresAt, err := manager.Issue(user)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if want := now.Add(AccessTokenTTL); !expiresAt.Equal(want) {
		t.Fatalf("expiresAt = %v, want %v", expiresAt, want)
	}

	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	id, err := UserIDFromClaims(claims)
	if err != nil {
		t.Fatalf("UserIDFromClaims: %v", err)
	}
	if id != 42 {
		t.Fatalf("user id = %d, want 42", id)
	}
	if claims.Role != RoleAdmin {
		t.Fatalf("role = %q, want ADMIN", claims.Role)
	}
}

// Чужой секрет и истёкший токен должны отклоняться.
func TestTokenManager_RejectsForeignAndExpired(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	manager := testTokenManager(t, now)
	token, _, err := manager.Issue(&User{ID: 1, Role: RoleUser})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	foreign := NewTokenManager("another-secret-another-secret-32", "polar-star-service-test",
		AccessTokenTTL, platform.FixedClock{Time: now})
	if _, err := foreign.Parse(token); err == nil {
		t.Fatal("токен с чужим секретом должен отклоняться")
	}

	// Тот же секрет, но время ушло за exp + leeway.
	later := NewTokenManager("0123456789abcdef0123456789abcdef", "polar-star-service-test",
		AccessTokenTTL, platform.FixedClock{Time: now.Add(AccessTokenTTL + time.Minute)})
	if _, err := later.Parse(token); err == nil {
		t.Fatal("истёкший токен должен отклоняться")
	}
}

func TestTokenManager_RejectsWrongIssuer(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	manager := testTokenManager(t, now)
	token, _, err := manager.Issue(&User{ID: 1, Role: RoleUser})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	otherIssuer := NewTokenManager("0123456789abcdef0123456789abcdef", "other-issuer",
		AccessTokenTTL, platform.FixedClock{Time: now})
	if _, err := otherIssuer.Parse(token); err == nil {
		t.Fatal("токен с чужим issuer должен отклоняться")
	}
}

// AUTH-2: refresh-токен — 32 случайных байта base64url, в БД хранится SHA-256.
func TestNewRefreshToken(t *testing.T) {
	plain, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	if len(plain) != 43 { // 32 байта base64url без padding = 43 символа
		t.Fatalf("длина refresh-токена = %d, want 43", len(plain))
	}
	if len(hash) != 64 { // sha256 hex
		t.Fatalf("длина хеша = %d, want 64", len(hash))
	}
	if HashToken(plain) != hash {
		t.Fatal("HashToken должен соответствовать хешу из NewRefreshToken")
	}

	other, _, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	if other == plain {
		t.Fatal("refresh-токены должны быть уникальными")
	}
}

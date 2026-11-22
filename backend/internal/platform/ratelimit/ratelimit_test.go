package ratelimit_test

import (
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/ratelimit"
)

// AUTH-3: 5 публичных заявок за 10 минут на IP.
func TestLimiter_TakeAndRefill(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	limiter := ratelimit.New(5, 10*time.Minute, clock)

	for i := 0; i < 5; i++ {
		allowed, retry := limiter.Take("1.2.3.4")
		if !allowed {
			t.Fatalf("попытка %d должна проходить, retry=%v", i+1, retry)
		}
	}
	allowed, retry := limiter.Take("1.2.3.4")
	if allowed {
		t.Fatal("6-я попытка должна блокироваться")
	}
	if retry <= 0 || retry > 10*time.Minute {
		t.Fatalf("некорректный Retry-After: %v", retry)
	}

	// Другой IP имеет собственный bucket.
	if allowed, _ := limiter.Take("5.6.7.8"); !allowed {
		t.Fatal("другой IP не должен быть заблокирован")
	}

	// Через 2 минуты восстанавливается ~1 токен (5 токенов / 10 минут).
	clock.now = now.Add(2 * time.Minute)
	if allowed, _ := limiter.Take("1.2.3.4"); !allowed {
		t.Fatal("после пополнения запрос должен проходить")
	}
}

// Peek не расходует токен (нужно логину: успешный вход не считается).
func TestLimiter_PeekDoesNotConsume(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	limiter := ratelimit.New(2, time.Minute, clock)

	for i := 0; i < 10; i++ {
		if allowed, _ := limiter.Peek("k"); !allowed {
			t.Fatalf("Peek не должен расходовать токены (итерация %d)", i)
		}
	}
	if allowed, _ := limiter.Take("k"); !allowed {
		t.Fatal("Take после Peek должен проходить")
	}
}

// Reset освобождает ключ после успешного входа.
func TestLimiter_Reset(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	limiter := ratelimit.New(1, time.Minute, clock)

	if allowed, _ := limiter.Take("login|ip"); !allowed {
		t.Fatal("первый запрос должен проходить")
	}
	if allowed, _ := limiter.Take("login|ip"); allowed {
		t.Fatal("второй запрос должен блокироваться")
	}
	limiter.Reset("login|ip")
	if allowed, _ := limiter.Take("login|ip"); !allowed {
		t.Fatal("после Reset лимит должен быть снят")
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want int
	}{
		{0, 1},
		{-time.Second, 1},
		{time.Millisecond, 1},
		{time.Second, 1},
		{time.Second + time.Millisecond, 2},
		{90 * time.Second, 90},
	}
	for _, tc := range cases {
		if got := ratelimit.RetryAfterSeconds(tc.in); got != tc.want {
			t.Fatalf("RetryAfterSeconds(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

type mutableClock struct{ now time.Time }

func (c *mutableClock) Now() time.Time { return c.now }

var _ platform.Clock = (*mutableClock)(nil)

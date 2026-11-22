// Package ratelimit — потокобезопасный in-memory token bucket для ограничения
// частоты запросов (AUTH-3). Реализация не требует Redis: один инстанс API
// (PLAN §4, допущение 1), поэтому состояние в памяти допустимо.
package ratelimit

import (
	"sync"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// Limiter — token bucket с ёмкостью limit и пополнением limit токенов за window.
type Limiter struct {
	mu       sync.Mutex
	capacity float64
	window   time.Duration
	rate     float64 // токенов в секунду
	clock    platform.Clock
	buckets  map[string]*bucket

	lastSweep time.Time
}

type bucket struct {
	tokens    float64
	updatedAt time.Time
}

// New создаёт лимитер. limit — число запросов за window (например, 10 за 10 мин).
func New(limit int, window time.Duration, clock platform.Clock) *Limiter {
	if limit < 1 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	if clock == nil {
		clock = platform.RealClock{}
	}
	return &Limiter{
		capacity: float64(limit),
		window:   window,
		rate:     float64(limit) / window.Seconds(),
		clock:    clock,
		buckets:  make(map[string]*bucket),
	}
}

// Peek сообщает, доступен ли запрос, не расходуя токен. Нужен логину: заранее
// неизвестно, будет ли попытка неуспешной (AUTH-3 — лимит на неудачные входы).
// Если запрос запрещён, возвращается время до восстановления одного токена.
func (l *Limiter) Peek(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	l.maybeSweep(now)
	b := l.bucketLocked(key, now)
	l.refill(b, now)
	if b.tokens >= 1 {
		return true, 0
	}
	return false, l.retryAfter(b)
}

// Take пытается израсходовать один токен.
func (l *Limiter) Take(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	l.maybeSweep(now)
	b := l.bucketLocked(key, now)
	l.refill(b, now)
	if b.tokens < 1 {
		return false, l.retryAfter(b)
	}
	b.tokens--
	return true, 0
}

// Reset удаляет состояние ключа (например, после успешного входа — успешные
// попытки не должны расходовать лимит неудачных).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

// RetryAfterSeconds возвращает ceil(Retry-After) для HTTP-заголовка.
func RetryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	sec := int(d / time.Second)
	if d%time.Second != 0 {
		sec++
	}
	if sec < 1 {
		sec = 1
	}
	return sec
}

func (l *Limiter) bucketLocked(key string, now time.Time) *bucket {
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, updatedAt: now}
		l.buckets[key] = b
	}
	return b
}

func (l *Limiter) refill(b *bucket, now time.Time) {
	if now.Before(b.updatedAt) {
		// Часы «откатились» (например, смена системного времени) — не начисляем.
		b.updatedAt = now
		return
	}
	elapsed := now.Sub(b.updatedAt).Seconds()
	if elapsed <= 0 {
		return
	}
	b.tokens += elapsed * l.rate
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.updatedAt = now
}

func (l *Limiter) retryAfter(b *bucket) time.Duration {
	missing := 1 - b.tokens
	if missing < 0 {
		missing = 0
	}
	return time.Duration(missing / l.rate * float64(time.Second))
}

// maybeSweep раз в окно удаляет «полные» бакеты, к которым давно не обращались:
// иначе карта росла бы бесконечно при переборе ключей (login+IP).
func (l *Limiter) maybeSweep(now time.Time) {
	if !l.lastSweep.IsZero() && now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	ttl := 2 * l.window
	for key, b := range l.buckets {
		if now.Sub(b.updatedAt) > ttl && b.tokens >= l.capacity {
			delete(l.buckets, key)
		}
	}
}

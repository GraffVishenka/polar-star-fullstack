package notifications

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Service — подписки на push и выдача VAPID public key.
type Service struct {
	repo      Repository
	publicKey string
	enabled   bool
	clock     platform.Clock
	logger    *slog.Logger
}

// NewService создаёт сервис подписок. publicKey пустой в no-op режиме
// (VAPID-ключи не заданы): фронт получает пустой ключ и не подписывается.
func NewService(repo Repository, publicKey string, enabled bool, clock platform.Clock, logger *slog.Logger) *Service {
	if clock == nil {
		clock = platform.RealClock{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, publicKey: publicKey, enabled: enabled, clock: clock, logger: logger}
}

// PublicKey возвращает VAPID public key (NOTIF-1).
func (s *Service) PublicKey() string { return s.publicKey }

// Enabled сообщает, настроен ли транспорт.
func (s *Service) Enabled() bool { return s.enabled }

// SubscribeInput — тело POST /api/push/subscriptions.
type SubscribeInput struct {
	Endpoint  string
	P256DH    string
	Auth      string
	UserAgent string
}

// Subscribe сохраняет/обновляет подписку устройства текущего пользователя.
func (s *Service) Subscribe(ctx context.Context, userID int64, input SubscribeInput) (*Subscription, error) {
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.P256DH = strings.TrimSpace(input.P256DH)
	input.Auth = strings.TrimSpace(input.Auth)
	input.UserAgent = strings.TrimSpace(input.UserAgent)

	if input.Endpoint == "" || len(input.Endpoint) > 500 {
		return nil, apierr.Validation("Некорректный endpoint подписки",
			map[string]any{"endpoint": "обязателен, не более 500 символов"})
	}
	parsedEndpoint, err := url.ParseRequestURI(input.Endpoint)
	if err != nil || parsedEndpoint.Host == "" ||
		(parsedEndpoint.Scheme != "https" && parsedEndpoint.Scheme != "http") {
		return nil, apierr.Validation("Некорректный endpoint подписки",
			map[string]any{"endpoint": "ожидается абсолютный http(s) URL"})
	}
	if input.P256DH == "" || len(input.P256DH) > 255 {
		return nil, apierr.Validation("Некорректный ключ p256dh",
			map[string]any{"keys.p256dh": "обязателен, не более 255 символов"})
	}
	if input.Auth == "" || len(input.Auth) > 255 {
		return nil, apierr.Validation("Некорректный ключ auth",
			map[string]any{"keys.auth": "обязателен, не более 255 символов"})
	}
	if len(input.UserAgent) > 512 {
		input.UserAgent = input.UserAgent[:512]
	}

	subscription, err := s.repo.UpsertSubscription(ctx, &Subscription{
		UserID:    userID,
		Endpoint:  input.Endpoint,
		P256DH:    input.P256DH,
		Auth:      input.Auth,
		UserAgent: input.UserAgent,
		CreatedAt: s.clock.Now().UTC(),
		IsActive:  true,
	})
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return subscription, nil
}

// Delete удаляет подписку текущего пользователя; чужая — 404 (NOTIF-5).
func (s *Service) Delete(ctx context.Context, userID, subscriptionID int64) error {
	if subscriptionID <= 0 {
		return apierr.NotFound("Подписка не найдена")
	}
	deleted, err := s.repo.DeleteSubscription(ctx, subscriptionID, userID)
	if err != nil {
		return apierr.Internal(err)
	}
	if !deleted {
		return apierr.NotFound("Подписка не найдена")
	}
	return nil
}

// SubscriptionDTO — ответ API (без ключей шифрования).
type SubscriptionDTO struct {
	ID        int64     `json:"id"`
	Endpoint  string    `json:"endpoint"`
	CreatedAt time.Time `json:"created_at"`
	IsActive  bool      `json:"is_active"`
}

// SubscriptionDTO собирает ответ по подписке.
func (s *Service) SubscriptionDTO(subscription *Subscription) SubscriptionDTO {
	return SubscriptionDTO{
		ID:        subscription.ID,
		Endpoint:  subscription.Endpoint,
		CreatedAt: subscription.CreatedAt,
		IsActive:  subscription.IsActive,
	}
}

package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Sender — транспорт доставки одного payload на одно устройство.
//
// Реализация по умолчанию — WebPushSender; в тестах используется fake.
// Enabled=false означает no-op режим (VAPID-ключи не заданы): dispatcher не
// создаёт события, worker не отправляет.
type Sender interface {
	Enabled() bool
	Send(ctx context.Context, subscription Subscription, payload Payload) error
}

// DeliveryError — ответ push-сервиса с HTTP-статусом.
//
// Gone=true для 404/410: подписка больше не существует и должна быть
// деактивирована (NOTIF-6).
type DeliveryError struct {
	StatusCode int
	Gone       bool
	Err        error
}

// Error реализует error.
func (e *DeliveryError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("push: статус %d: %v", e.StatusCode, e.Err)
	}
	return fmt.Sprintf("push: статус %d", e.StatusCode)
}

// Unwrap позволяет errors.Is/As доходить до причины.
func (e *DeliveryError) Unwrap() error { return e.Err }

// WebPushSender отправляет зашифрованные Web Push сообщения по VAPID.
type WebPushSender struct {
	publicKey  string
	privateKey string
	subject    string
	client     *http.Client
}

// NewWebPushSender создаёт отправитель. Пустые ключи переводят его в no-op:
// сервис продолжает работать, но уведомления не создаются (NOTIF-1).
//
// VAPID_SUBJECT в конфигурации задаётся как mailto:...; webpush-go сам
// добавляет префикс mailto:, поэтому здесь он снимается, чтобы не получить
// "mailto:mailto:...".
func NewWebPushSender(publicKey, privateKey, subject string) *WebPushSender {
	subject = strings.TrimSpace(subject)
	if strings.HasPrefix(subject, "mailto:") {
		subject = strings.TrimPrefix(subject, "mailto:")
	}
	if subject == "" {
		subject = "admin@example.com"
	}
	return &WebPushSender{
		publicKey:  publicKey,
		privateKey: privateKey,
		subject:    subject,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled сообщает, настроены ли VAPID-ключи.
func (s *WebPushSender) Enabled() bool {
	return s != nil && s.publicKey != "" && s.privateKey != ""
}

// PublicKey возвращает VAPID public key (пусто в no-op).
func (s *WebPushSender) PublicKey() string {
	if s == nil || !s.Enabled() {
		return ""
	}
	return s.publicKey
}

// Send шифрует payload и отправляет его на endpoint подписки.
func (s *WebPushSender) Send(ctx context.Context, subscription Subscription, payload Payload) error {
	if !s.Enabled() {
		return errors.New("push: VAPID-ключи не настроены")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("push: payload: %w", err)
	}
	response, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
		Endpoint: subscription.Endpoint,
		Keys: webpush.Keys{
			Auth:   subscription.Auth,
			P256dh: subscription.P256DH,
		},
	}, &webpush.Options{
		Subscriber:      s.subject,
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
		TTL:             300,
		Urgency:         webpush.UrgencyNormal,
		HTTPClient:      s.client,
	})
	if err != nil {
		return fmt.Errorf("push: отправка: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		return &DeliveryError{StatusCode: response.StatusCode, Gone: true}
	}
	if response.StatusCode >= http.StatusMultipleChoices {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return &DeliveryError{
			StatusCode: response.StatusCode,
			Err:        fmt.Errorf("ответ push-сервиса: %s", string(snippet)),
		}
	}
	return nil
}

// NoopSender — отправитель без VAPID; нужен для явного no-op в тестах/дев.
type NoopSender struct{}

// Enabled всегда false.
func (NoopSender) Enabled() bool { return false }

// Send ничего не делает.
func (NoopSender) Send(context.Context, Subscription, Payload) error { return nil }

var (
	_ Sender = (*WebPushSender)(nil)
	_ Sender = NoopSender{}
)

// Package notifications реализует Web Push уведомления (NOTIF-1..NOTIF-7):
// подписки устройств, outbox с ретраями, выбор получателей событий заявок и
// ежедневные напоминания о дедлайнах. Отправка изолирована интерфейсом Sender,
// поэтому домен и тесты не зависят от сети.
package notifications

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound — подписка не найдена (или принадлежит другому пользователю).
var ErrNotFound = errors.New("notifications: подписка не найдена")

// Subscription — Web Push подписка устройства (таблица push_subscriptions).
type Subscription struct {
	ID            int64
	UserID        int64
	Endpoint      string
	P256DH        string
	Auth          string
	UserAgent     string
	CreatedAt     time.Time
	LastSuccessAt *time.Time
	IsActive      bool
}

// Payload — тело push-сообщения (payload в notification_outbox).
//
// Ограничения NOTIF-4: title ≤ 60, body ≤ 120 символов. Лимиты применяются при
// постановке в outbox (Dispatcher), а не при отправке.
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
	Icon  string `json:"icon"`
}

// MarshalJSON возвращает канонический JSON для колонки payload.
func (p Payload) MarshalJSON() ([]byte, error) {
	type alias Payload
	return json.Marshal(alias(p))
}

// OutboxItem — строка notification_outbox.
type OutboxItem struct {
	ID             int64
	EventType      string
	UserID         int64
	Payload        []byte
	Status         string
	Attempts       int
	NextAttemptAt  time.Time
	IdempotencyKey *string
	CreatedAt      time.Time
	SentAt         *time.Time
	LastError      *string
}

// ReminderTicket — заявка для ежедневной джобы напоминаний (NOTIF-7).
type ReminderTicket struct {
	ID            int64
	BuildingID    int64
	ExecutorID    *int64
	ApplicantName string
	Cabinet       string
	Deadline      time.Time
	IsToday       bool
}

// TicketInfo — минимум данных заявки для адресации и payload события.
type TicketInfo struct {
	ID              int64
	BuildingID      int64
	ExecutorID      *int64
	CreatedByUserID *int64
	ApplicantName   string
	Cabinet         string
}

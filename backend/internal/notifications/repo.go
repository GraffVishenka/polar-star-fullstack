package notifications

import (
	"context"
	"time"
)

// Repository — хранилище подписок и outbox, а также запросы адресации.
// Интерфейс позволяет тестировать dispatcher/worker без MySQL.
type Repository interface {
	// UpsertSubscription создаёт подписку или обновляет её по endpoint,
	// привязывая к текущему пользователю (NOTIF-1).
	UpsertSubscription(ctx context.Context, subscription *Subscription) (*Subscription, error)
	// DeleteSubscription удаляет подписку пользователя. false — нет такой
	// подписки или она чужая (NOTIF-5).
	DeleteSubscription(ctx context.Context, id, userID int64) (bool, error)
	// ActiveSubscriptionsByUser — активные устройства пользователя.
	ActiveSubscriptionsByUser(ctx context.Context, userID int64) ([]*Subscription, error)
	// DeactivateSubscription выключает подписку после 404/410 (NOTIF-6).
	DeactivateSubscription(ctx context.Context, id int64) error
	// TouchSubscription фиксирует успешную доставку.
	TouchSubscription(ctx context.Context, id int64, at time.Time) error

	// EnqueueOutbox вставляет пачку событий; дубли по idempotency_key
	// игнорируются. Возвращает число реально вставленных строк.
	EnqueueOutbox(ctx context.Context, items []*OutboxItem) (int, error)
	// PendingOutbox — pending-события, срок которых наступил.
	PendingOutbox(ctx context.Context, limit int) ([]*OutboxItem, error)
	MarkOutboxSent(ctx context.Context, id int64, at time.Time) error
	MarkOutboxRetry(ctx context.Context, id int64, attempts int, nextAt time.Time, lastErr string) error
	MarkOutboxFailed(ctx context.Context, id int64, attempts int, lastErr string) error

	// BuildingUserIDs — активные USER здания (адресаты NEW_TICKET).
	BuildingUserIDs(ctx context.Context, buildingID int64) ([]int64, error)
	// AdminUserIDs — все активные ADMIN.
	AdminUserIDs(ctx context.Context) ([]int64, error)
	// ReminderTickets — заявки для DEADLINE_TODAY/DEADLINE_OVERDUE на дату today.
	ReminderTickets(ctx context.Context, today time.Time) ([]*ReminderTicket, error)
	// TicketInfo — данные заявки для адресации/payload; ErrNotFound если нет.
	TicketInfo(ctx context.Context, id int64) (*TicketInfo, error)
}

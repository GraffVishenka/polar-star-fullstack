package platform

import (
	"context"
	"log/slog"
)

// EventType — тип доменного события для push-уведомлений (NOTIF-2).
type EventType string

const (
	EventNewTicket       EventType = "NEW_TICKET"
	EventTicketAssigned  EventType = "TICKET_ASSIGNED"
	EventNewComment      EventType = "NEW_COMMENT"
	EventDeadlineToday   EventType = "DEADLINE_TODAY"
	EventDeadlineOverdue EventType = "DEADLINE_OVERDUE"
)

// NotificationEvent — событие, которое домены публикуют через Notifier.
// Конкретная доставка (outbox + Web Push) реализуется в Б8; до тех пор
// используется LogNotifier, чтобы не блокировать домены.
type NotificationEvent struct {
	Type EventType
	// TicketID — заявка-источник события (для deep link и идемпотентности).
	TicketID int64
	// BuildingID — здание заявки (адресация NEW_TICKET, NOTIF-2).
	BuildingID *int64
	// UserIDs — явные получатели (для TICKET_ASSIGNED/NEW_COMMENT).
	UserIDs []int64
	// ActorUserID — автор действия: получатели не должны получать уведомление
	// о собственном действии (NOTIF-2: «кроме автора»). 0 — аноним.
	ActorUserID int64
	Title       string
	Body        string
	URL         string
	Tag         string
	// DedupKey — ключ идемпотентности событий напоминаний (NOTIF-7).
	DedupKey string
	Payload  map[string]any
}

// Notifier — интерфейс публикации push-событий. Домены (заявки, комментарии,
// напоминания) зависят только от него, реализация — internal/notifications (Б8).
type Notifier interface {
	Publish(ctx context.Context, event NotificationEvent) error
}

// LogNotifier — временная реализация Notifier: пишет событие в лог.
// Используется до готовности Б8 и в тестах.
type LogNotifier struct {
	Logger *slog.Logger
}

// Publish логирует событие, не выполняя реальную доставку.
func (n *LogNotifier) Publish(_ context.Context, event NotificationEvent) error {
	if n.Logger == nil {
		return nil
	}
	n.Logger.Debug("notifier stub: event accepted",
		"event_type", event.Type,
		"ticket_id", event.TicketID,
		"recipients", len(event.UserIDs),
		"dedup_key", event.DedupKey,
	)
	return nil
}

var _ Notifier = (*LogNotifier)(nil)

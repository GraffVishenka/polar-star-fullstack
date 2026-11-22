package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

const (
	// NOTIF-4: ограничения payload.
	maxTitleRunes = 60
	maxBodyRunes  = 120
	defaultIcon   = "/icons/icon-192.png"
)

// Dispatcher реализует platform.Notifier: выбирает получателей события,
// собирает payload и ставит по строке outbox на каждого получателя.
//
// deliveryEnabled=false (VAPID не настроен) переводит уведомления в no-op, как
// и push_enabled=false в настройках (NOTIF-1).
type Dispatcher struct {
	repo            Repository
	settings        PushSettings
	clock           platform.Clock
	logger          *slog.Logger
	deliveryEnabled bool
}

// NewDispatcher создаёт dispatcher.
func NewDispatcher(repo Repository, settings PushSettings, clock platform.Clock, logger *slog.Logger, deliveryEnabled bool) *Dispatcher {
	if clock == nil {
		clock = platform.RealClock{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		repo: repo, settings: settings, clock: clock, logger: logger,
		deliveryEnabled: deliveryEnabled,
	}
}

// Publish реализует platform.Notifier.
func (d *Dispatcher) Publish(ctx context.Context, event platform.NotificationEvent) error {
	if !d.deliveryEnabled {
		return nil
	}
	if d.settings != nil && !d.settings.Enabled() {
		return nil
	}

	var info *TicketInfo
	loadInfo := func() (*TicketInfo, error) {
		if info != nil {
			return info, nil
		}
		loaded, err := d.repo.TicketInfo(ctx, event.TicketID)
		if err != nil {
			return nil, err
		}
		info = loaded
		return info, nil
	}

	var recipients []int64
	switch event.Type {
	case platform.EventNewTicket:
		buildingID := int64(0)
		if event.BuildingID != nil {
			buildingID = *event.BuildingID
		} else {
			loaded, err := loadInfo()
			if err != nil {
				return err
			}
			buildingID = loaded.BuildingID
		}
		users, err := d.repo.BuildingUserIDs(ctx, buildingID)
		if err != nil {
			return err
		}
		admins, err := d.repo.AdminUserIDs(ctx)
		if err != nil {
			return err
		}
		recipients = append(recipients, users...)
		recipients = append(recipients, admins...)
	case platform.EventTicketAssigned:
		recipients = append(recipients, event.UserIDs...)
		if len(recipients) == 0 {
			loaded, err := loadInfo()
			if err != nil {
				return err
			}
			if loaded.ExecutorID != nil {
				recipients = append(recipients, *loaded.ExecutorID)
			}
		}
	case platform.EventNewComment:
		recipients = append(recipients, event.UserIDs...)
		if len(recipients) == 0 {
			loaded, err := loadInfo()
			if err != nil {
				return err
			}
			if loaded.ExecutorID != nil {
				recipients = append(recipients, *loaded.ExecutorID)
			}
			if loaded.CreatedByUserID != nil {
				recipients = append(recipients, *loaded.CreatedByUserID)
			}
		}
	default:
		recipients = append(recipients, event.UserIDs...)
	}

	// NOTIF-2: автор действия не получает уведомление о своём действии.
	if event.Type == platform.EventNewTicket || event.Type == platform.EventNewComment {
		recipients = excludeUser(recipients, event.ActorUserID)
	}
	recipients = uniquePositive(recipients)
	if len(recipients) == 0 {
		return nil
	}

	payload, err := d.buildPayload(ctx, event, loadInfo)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("notifications: payload: %w", err)
	}

	now := d.clock.Now().UTC()
	items := make([]*OutboxItem, 0, len(recipients))
	for _, userID := range recipients {
		items = append(items, &OutboxItem{
			EventType:      string(event.Type),
			UserID:         userID,
			Payload:        body,
			NextAttemptAt:  now,
			CreatedAt:      now,
			IdempotencyKey: idempotencyKey(event.DedupKey, userID),
		})
	}
	if _, err := d.repo.EnqueueOutbox(ctx, items); err != nil {
		return err
	}
	d.logger.Debug("notifications: событие поставлено в outbox",
		"event_type", event.Type, "ticket_id", event.TicketID, "recipients", len(items))
	return nil
}

// buildPayload собирает payload с лимитами NOTIF-4. Если данных заявки нет,
// используются дефолты по типу события: уведомление не должно теряться из-за
// недоступной карточки.
func (d *Dispatcher) buildPayload(ctx context.Context, event platform.NotificationEvent, loadInfo func() (*TicketInfo, error)) (Payload, error) {
	title := event.Title
	body := event.Body
	var info *TicketInfo
	if title == "" || body == "" {
		loaded, err := loadInfo()
		if err == nil {
			info = loaded
		}
	}
	if title == "" {
		title = defaultTitle(event.Type)
	}
	if body == "" {
		if info != nil {
			body = fmt.Sprintf("%s · %s", info.ApplicantName, info.Cabinet)
		} else {
			body = fmt.Sprintf("Заявка #%d", event.TicketID)
		}
	}
	url := event.URL
	if url == "" {
		url = fmt.Sprintf("/tickets/%d", event.TicketID)
	}
	tag := event.Tag
	if tag == "" {
		tag = fmt.Sprintf("ticket-%d", event.TicketID)
	}
	icon := defaultIcon
	if raw, ok := event.Payload["icon"].(string); ok && raw != "" {
		icon = raw
	}
	return Payload{
		Title: truncateRunes(title, maxTitleRunes),
		Body:  truncateRunes(body, maxBodyRunes),
		URL:   url,
		Tag:   tag,
		Icon:  icon,
	}, nil
}

// EnqueueReminders ставит напоминания DEADLINE_TODAY/DEADLINE_OVERDUE на
// локальную дату now. Получатели — исполнитель (если есть) и все ADMIN.
// Повторный запуск в ту же дату не создаёт дублей (NOTIF-7).
func (d *Dispatcher) EnqueueReminders(ctx context.Context, now time.Time) (int, error) {
	if !d.deliveryEnabled {
		return 0, nil
	}
	if d.settings != nil && !d.settings.Enabled() {
		return 0, nil
	}
	location := time.UTC
	if d.settings != nil && d.settings.Timezone() != nil {
		location = d.settings.Timezone()
	}
	localNow := now.In(location)
	date := localNow.Format("2006-01-02")

	tickets, err := d.repo.ReminderTickets(ctx, localNow)
	if err != nil {
		return 0, err
	}
	if len(tickets) == 0 {
		return 0, nil
	}
	admins, err := d.repo.AdminUserIDs(ctx)
	if err != nil {
		return 0, err
	}

	var items []*OutboxItem
	for _, ticket := range tickets {
		eventType := platform.EventDeadlineOverdue
		title := "Заявка просрочена"
		if ticket.IsToday {
			eventType = platform.EventDeadlineToday
			title = "Дедлайн сегодня"
		}
		body := fmt.Sprintf("%s · %s · %s", ticket.ApplicantName, ticket.Cabinet,
			ticket.Deadline.Format("02.01.2006"))
		payload, err := json.Marshal(Payload{
			Title: truncateRunes(title, maxTitleRunes),
			Body:  truncateRunes(body, maxBodyRunes),
			URL:   fmt.Sprintf("/tickets/%d", ticket.ID),
			Tag:   fmt.Sprintf("deadline-%d", ticket.ID),
			Icon:  defaultIcon,
		})
		if err != nil {
			return 0, fmt.Errorf("notifications: payload напоминания: %w", err)
		}
		recipients := append([]int64{}, admins...)
		if ticket.ExecutorID != nil {
			recipients = append(recipients, *ticket.ExecutorID)
		}
		recipients = uniquePositive(recipients)
		baseKey := fmt.Sprintf("%s:%d:%s", eventType, ticket.ID, date)
		for _, userID := range recipients {
			items = append(items, &OutboxItem{
				EventType:      string(eventType),
				UserID:         userID,
				Payload:        payload,
				NextAttemptAt:  now.UTC(),
				CreatedAt:      now.UTC(),
				IdempotencyKey: idempotencyKey(baseKey, userID),
			})
		}
	}
	if len(items) == 0 {
		return 0, nil
	}
	inserted, err := d.repo.EnqueueOutbox(ctx, items)
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

func defaultTitle(eventType platform.EventType) string {
	switch eventType {
	case platform.EventNewTicket:
		return "Новая заявка"
	case platform.EventTicketAssigned:
		return "Вам назначена заявка"
	case platform.EventNewComment:
		return "Новый комментарий"
	case platform.EventDeadlineToday:
		return "Дедлайн сегодня"
	case platform.EventDeadlineOverdue:
		return "Заявка просрочена"
	default:
		return "Уведомление"
	}
}

// idempotencyKey добавляет userID: outbox хранит строку на получателя, поэтому
// уникальный ключ должен различать пользователей (NOTIF-7).
func idempotencyKey(base string, userID int64) *string {
	if base == "" {
		return nil
	}
	key := base + ":" + strconv.FormatInt(userID, 10)
	return &key
}

func uniquePositive(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func excludeUser(ids []int64, userID int64) []int64 {
	if userID <= 0 {
		return ids
	}
	result := ids[:0]
	for _, id := range ids {
		if id != userID {
			result = append(result, id)
		}
	}
	return result
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

var _ platform.Notifier = (*Dispatcher)(nil)

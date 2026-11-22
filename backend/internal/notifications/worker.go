package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

const (
	// MaxAttempts — NOTIF-6: до 3 попыток доставки.
	MaxAttempts = 3
	// outboxBatch — размер пачки за один тик.
	outboxBatch = 100
	// retryBase — база экспоненциальной задержки: 30s, 60s.
	retryBase = 30 * time.Second
	// tickInterval — период worker'а (outbox 30s, проверка напоминаний тоже).
	tickInterval = 30 * time.Second
)

// ReminderEnqueuer — постановка ежедневных напоминаний (реализует Dispatcher).
type ReminderEnqueuer interface {
	EnqueueReminders(ctx context.Context, now time.Time) (int, error)
}

// Worker — фоновый отправитель outbox и планировщик напоминаний.
type Worker struct {
	repo             Repository
	sender           Sender
	settings         PushSettings
	reminders        ReminderEnqueuer
	clock            platform.Clock
	logger           *slog.Logger
	lastReminderDate string
}

// NewWorker создаёт worker. reminders может быть nil — тогда напоминания не
// ставятся (используется в тестах outbox).
func NewWorker(repo Repository, sender Sender, settings PushSettings, reminders ReminderEnqueuer, clock platform.Clock, logger *slog.Logger) *Worker {
	if clock == nil {
		clock = platform.RealClock{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		repo: repo, sender: sender, settings: settings, reminders: reminders,
		clock: clock, logger: logger,
	}
}

// Run запускает цикл до отмены контекста.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	if w.sender == nil || !w.sender.Enabled() {
		return
	}
	if _, err := w.ProcessOutbox(ctx); err != nil {
		w.logger.Error("notifications: обработка outbox", "error", err)
	}
	if _, err := w.RunRemindersIfDue(ctx, w.clock.Now()); err != nil {
		w.logger.Error("notifications: напоминания", "error", err)
	}
}

// ProcessOutbox отправляет пачку pending-событий. Возвращает число обработанных.
func (w *Worker) ProcessOutbox(ctx context.Context) (int, error) {
	if w.sender == nil || !w.sender.Enabled() {
		return 0, nil
	}
	if w.settings != nil && !w.settings.Enabled() {
		return 0, nil
	}
	items, err := w.repo.PendingOutbox(ctx, outboxBatch)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range items {
		if err := w.processOutboxItem(ctx, item); err != nil {
			w.logger.Error("notifications: событие outbox",
				"error", err, "outbox_id", item.ID, "user_id", item.UserID)
			continue
		}
		processed++
	}
	return processed, nil
}

func (w *Worker) processOutboxItem(ctx context.Context, item *OutboxItem) error {
	var payload Payload
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return w.repo.MarkOutboxFailed(ctx, item.ID, item.Attempts, "некорректный payload: "+err.Error())
	}

	subscriptions, err := w.repo.ActiveSubscriptionsByUser(ctx, item.UserID)
	if err != nil {
		return err
	}
	if len(subscriptions) == 0 {
		return w.repo.MarkOutboxFailed(ctx, item.ID, item.Attempts, "нет активных подписок")
	}

	now := w.clock.Now().UTC()
	delivered := false
	allGone := true
	lastErr := ""
	for _, subscription := range subscriptions {
		sendErr := w.sender.Send(ctx, *subscription, payload)
		if sendErr == nil {
			delivered = true
			if err := w.repo.TouchSubscription(ctx, subscription.ID, now); err != nil {
				w.logger.Warn("notifications: last_success_at",
					"error", err, "subscription_id", subscription.ID)
			}
			continue
		}

		var deliveryErr *DeliveryError
		if errors.As(sendErr, &deliveryErr) && deliveryErr.Gone {
			if err := w.repo.DeactivateSubscription(ctx, subscription.ID); err != nil {
				w.logger.Warn("notifications: деактивация подписки",
					"error", err, "subscription_id", subscription.ID)
			}
			lastErr = sendErr.Error()
			continue
		}
		allGone = false
		lastErr = sendErr.Error()
	}

	if delivered {
		return w.repo.MarkOutboxSent(ctx, item.ID, now)
	}
	if allGone {
		return w.repo.MarkOutboxFailed(ctx, item.ID, item.Attempts, lastErr)
	}

	attempts := item.Attempts + 1
	if attempts >= MaxAttempts {
		return w.repo.MarkOutboxFailed(ctx, item.ID, attempts, lastErr)
	}
	return w.repo.MarkOutboxRetry(ctx, item.ID, attempts, now.Add(retryDelay(attempts)), lastErr)
}

// retryDelay — экспоненциальная задержка после неудачной попытки.
func retryDelay(attempts int) time.Duration {
	delay := retryBase
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay > 10*time.Minute {
			return 10 * time.Minute
		}
	}
	return delay
}

// RunRemindersIfDue запускает дневную джобу, если локальное время достигло
// deadline_reminder_time и сегодня она ещё не выполнялась (NOTIF-7).
func (w *Worker) RunRemindersIfDue(ctx context.Context, now time.Time) (int, error) {
	if w.sender == nil || !w.sender.Enabled() || w.reminders == nil {
		return 0, nil
	}
	if w.settings != nil && !w.settings.Enabled() {
		return 0, nil
	}
	location := time.UTC
	reminder := "10:00"
	if w.settings != nil {
		if w.settings.Timezone() != nil {
			location = w.settings.Timezone()
		}
		reminder = w.settings.ReminderTime()
	}
	hour, minute, ok := ParseReminderTime(reminder)
	if !ok {
		hour, minute = 10, 0
	}
	local := now.In(location)
	if local.Hour() < hour || (local.Hour() == hour && local.Minute() < minute) {
		return 0, nil
	}
	date := local.Format("2006-01-02")
	if w.lastReminderDate == date {
		return 0, nil
	}
	count, err := w.reminders.EnqueueReminders(ctx, now)
	if err != nil {
		return 0, err
	}
	w.lastReminderDate = date
	w.logger.Info("notifications: напоминания поставлены",
		"date", date, "count", count)
	return count, nil
}

var _ = fmt.Sprintf

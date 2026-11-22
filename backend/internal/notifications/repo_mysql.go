package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MySQLRepository — реализация Repository поверх MySQL.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий уведомлений.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// UpsertSubscription вставляет подписку или обновляет её по endpoint.
//
// NOTIF-1: endpoint уникален; повторная подписка того же устройства обновляет
// ключи и возвращает подписку текущему пользователю (last write wins).
func (r *MySQLRepository) UpsertSubscription(ctx context.Context, subscription *Subscription) (*Subscription, error) {
	createdAt := subscription.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, user_agent, created_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE
			id = LAST_INSERT_ID(id),
			user_id = VALUES(user_id),
			p256dh = VALUES(p256dh),
			auth = VALUES(auth),
			user_agent = VALUES(user_agent),
			is_active = 1`,
		subscription.UserID, subscription.Endpoint, subscription.P256DH, subscription.Auth,
		nullIfEmpty(subscription.UserAgent), createdAt)
	if err != nil {
		return nil, fmt.Errorf("notifications: сохранение подписки: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("notifications: id подписки: %w", err)
	}
	return r.subscriptionByID(ctx, id)
}

func (r *MySQLRepository) subscriptionByID(ctx context.Context, id int64) (*Subscription, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, endpoint, p256dh, auth, COALESCE(user_agent, ''),
		       created_at, last_success_at, is_active
		FROM push_subscriptions WHERE id = ?`, id)
	return scanSubscription(row)
}

func scanSubscription(row interface{ Scan(...any) error }) (*Subscription, error) {
	var (
		subscription Subscription
		lastSuccess  sql.NullTime
	)
	if err := row.Scan(&subscription.ID, &subscription.UserID, &subscription.Endpoint,
		&subscription.P256DH, &subscription.Auth, &subscription.UserAgent,
		&subscription.CreatedAt, &lastSuccess, &subscription.IsActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("notifications: чтение подписки: %w", err)
	}
	if lastSuccess.Valid {
		subscription.LastSuccessAt = &lastSuccess.Time
	}
	return &subscription, nil
}

// DeleteSubscription удаляет только подписку указанного пользователя.
func (r *MySQLRepository) DeleteSubscription(ctx context.Context, id, userID int64) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, fmt.Errorf("notifications: удаление подписки: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("notifications: удаление подписки: %w", err)
	}
	return affected > 0, nil
}

// ActiveSubscriptionsByUser возвращает устройства пользователя.
func (r *MySQLRepository) ActiveSubscriptionsByUser(ctx context.Context, userID int64) ([]*Subscription, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, endpoint, p256dh, auth, COALESCE(user_agent, ''),
		       created_at, last_success_at, is_active
		FROM push_subscriptions
		WHERE user_id = ? AND is_active = 1
		ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("notifications: подписки пользователя: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*Subscription
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, subscription)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notifications: подписки пользователя: %w", err)
	}
	return result, nil
}

// DeactivateSubscription выключает мёртвую подписку (404/410).
func (r *MySQLRepository) DeactivateSubscription(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE push_subscriptions SET is_active = 0 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("notifications: деактивация подписки: %w", err)
	}
	return nil
}

// TouchSubscription фиксирует last_success_at.
func (r *MySQLRepository) TouchSubscription(ctx context.Context, id int64, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE push_subscriptions SET last_success_at = ? WHERE id = ?`, at, id); err != nil {
		return fmt.Errorf("notifications: отметка успешной доставки: %w", err)
	}
	return nil
}

// EnqueueOutbox вставляет события, игнорируя дубли по idempotency_key.
func (r *MySQLRepository) EnqueueOutbox(ctx context.Context, items []*OutboxItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("notifications: outbox: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inserted := 0
	fallback := time.Now().UTC()
	for _, item := range items {
		var key any
		if item.IdempotencyKey != nil && *item.IdempotencyKey != "" {
			key = *item.IdempotencyKey
		}
		nextAttempt := item.NextAttemptAt
		if nextAttempt.IsZero() {
			nextAttempt = fallback
		}
		createdAt := item.CreatedAt
		if createdAt.IsZero() {
			createdAt = fallback
		}
		result, err := tx.ExecContext(ctx, `
			INSERT IGNORE INTO notification_outbox
				(event_type, user_id, payload, status, attempts, next_attempt_at, idempotency_key, created_at)
			VALUES (?, ?, ?, 'pending', 0, ?, ?, ?)`,
			item.EventType, item.UserID, string(item.Payload), nextAttempt, key, createdAt)
		if err != nil {
			return 0, fmt.Errorf("notifications: постановка в outbox: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("notifications: постановка в outbox: %w", err)
		}
		inserted += int(affected)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("notifications: outbox: %w", err)
	}
	return inserted, nil
}

// PendingOutbox возвращает pending-события, срок которых наступил.
func (r *MySQLRepository) PendingOutbox(ctx context.Context, limit int) ([]*OutboxItem, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, event_type, user_id, payload, status, attempts, next_attempt_at,
		       idempotency_key, created_at, sent_at, last_error
		FROM notification_outbox
		WHERE status = 'pending' AND next_attempt_at <= ?
		ORDER BY id
		LIMIT ?`, time.Now().UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("notifications: выборка outbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*OutboxItem
	for rows.Next() {
		var (
			item  OutboxItem
			key   sql.NullString
			sent  sql.NullTime
			lastE sql.NullString
		)
		if err := rows.Scan(&item.ID, &item.EventType, &item.UserID, &item.Payload, &item.Status,
			&item.Attempts, &item.NextAttemptAt, &key, &item.CreatedAt, &sent, &lastE); err != nil {
			return nil, fmt.Errorf("notifications: чтение outbox: %w", err)
		}
		if key.Valid {
			item.IdempotencyKey = &key.String
		}
		if sent.Valid {
			item.SentAt = &sent.Time
		}
		if lastE.Valid {
			item.LastError = &lastE.String
		}
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notifications: выборка outbox: %w", err)
	}
	return items, nil
}

// MarkOutboxSent фиксирует успешную доставку хотя бы на одно устройство.
func (r *MySQLRepository) MarkOutboxSent(ctx context.Context, id int64, at time.Time) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE notification_outbox
		SET status = 'sent', sent_at = ?, last_error = NULL
		WHERE id = ?`, at, id); err != nil {
		return fmt.Errorf("notifications: outbox sent: %w", err)
	}
	return nil
}

// MarkOutboxRetry планирует повторную попытку.
func (r *MySQLRepository) MarkOutboxRetry(ctx context.Context, id int64, attempts int, nextAt time.Time, lastErr string) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE notification_outbox
		SET status = 'pending', attempts = ?, next_attempt_at = ?, last_error = ?
		WHERE id = ?`, attempts, nextAt, lastErr, id); err != nil {
		return fmt.Errorf("notifications: outbox retry: %w", err)
	}
	return nil
}

// MarkOutboxFailed фиксирует окончательный провал.
func (r *MySQLRepository) MarkOutboxFailed(ctx context.Context, id int64, attempts int, lastErr string) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE notification_outbox
		SET status = 'failed', attempts = ?, last_error = ?
		WHERE id = ?`, attempts, lastErr, id); err != nil {
		return fmt.Errorf("notifications: outbox failed: %w", err)
	}
	return nil
}

// BuildingUserIDs — активные USER здания.
func (r *MySQLRepository) BuildingUserIDs(ctx context.Context, buildingID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM users
		WHERE building_id = ? AND role = 'USER' AND is_active = 1
		ORDER BY id`, buildingID)
	if err != nil {
		return nil, fmt.Errorf("notifications: пользователи здания: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanIDs(rows)
}

// AdminUserIDs — активные ADMIN.
func (r *MySQLRepository) AdminUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM users WHERE role = 'ADMIN' AND is_active = 1 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("notifications: администраторы: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanIDs(rows)
}

func scanIDs(rows *sql.Rows) ([]int64, error) {
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("notifications: чтение id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notifications: чтение id: %w", err)
	}
	return ids, nil
}

// ReminderTickets выбирает заявки для напоминаний на локальную дату today.
//
// DEADLINE_TODAY — deadline = today; DEADLINE_OVERDUE — deadline < today и
// статус не done (NOTIF-7). Удалённые заявки исключены.
func (r *MySQLRepository) ReminderTickets(ctx context.Context, today time.Time) ([]*ReminderTicket, error) {
	date := today.Format("2006-01-02")
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, building_id, executor_id, applicant_name, cabinet, deadline,
		       (deadline = ?) AS is_today
		FROM tickets
		WHERE deleted_at IS NULL
		  AND deadline IS NOT NULL
		  AND (deadline = ? OR (deadline < ? AND status <> 'done'))
		ORDER BY deadline, id`, date, date, date)
	if err != nil {
		return nil, fmt.Errorf("notifications: заявки для напоминаний: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*ReminderTicket
	for rows.Next() {
		var (
			ticket   ReminderTicket
			executor sql.NullInt64
			deadline time.Time
			isToday  bool
		)
		if err := rows.Scan(&ticket.ID, &ticket.BuildingID, &executor, &ticket.ApplicantName,
			&ticket.Cabinet, &deadline, &isToday); err != nil {
			return nil, fmt.Errorf("notifications: чтение напоминания: %w", err)
		}
		if executor.Valid {
			ticket.ExecutorID = &executor.Int64
		}
		ticket.Deadline = deadline
		ticket.IsToday = isToday
		result = append(result, &ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notifications: заявки для напоминаний: %w", err)
	}
	return result, nil
}

// TicketInfo возвращает данные заявки для адресации/payload.
func (r *MySQLRepository) TicketInfo(ctx context.Context, id int64) (*TicketInfo, error) {
	var (
		info     TicketInfo
		executor sql.NullInt64
		created  sql.NullInt64
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id, building_id, executor_id, created_by_user_id, applicant_name, cabinet
		FROM tickets WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&info.ID, &info.BuildingID, &executor, &created, &info.ApplicantName, &info.Cabinet)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("notifications: заявка %d: %w", id, err)
	}
	if executor.Valid {
		info.ExecutorID = &executor.Int64
	}
	if created.Valid {
		info.CreatedByUserID = &created.Int64
	}
	return &info, nil
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

var _ Repository = (*MySQLRepository)(nil)

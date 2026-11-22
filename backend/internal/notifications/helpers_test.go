package notifications

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// fakeRepo — in-memory Repository для unit-тестов без MySQL.
type fakeRepo struct {
	mu sync.Mutex

	subscriptions map[int64]*Subscription
	endpoints     map[string]int64
	nextSubID     int64

	outbox     []*OutboxItem
	nextOutbox int64
	keys       map[string]bool

	buildingUsers map[int64][]int64
	admins        []int64
	tickets       map[int64]*TicketInfo
	reminders     []*ReminderTicket
	now           time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		subscriptions: map[int64]*Subscription{},
		endpoints:     map[string]int64{},
		keys:          map[string]bool{},
		buildingUsers: map[int64][]int64{},
		admins:        []int64{},
		tickets:       map[int64]*TicketInfo{},
	}
}

func (r *fakeRepo) addSub(userID int64) *Subscription {
	r.nextSubID++
	id := r.nextSubID
	sub := &Subscription{
		ID: id, UserID: userID,
		Endpoint:  "https://push.example/" + strconv.FormatInt(id, 10),
		P256DH:    "p256dh",
		Auth:      "auth",
		IsActive:  true,
		CreatedAt: r.now,
	}
	r.subscriptions[sub.ID] = sub
	r.endpoints[sub.Endpoint] = sub.ID
	return sub
}

func (r *fakeRepo) addOutbox(userID int64, eventType string, payload Payload) *OutboxItem {
	r.nextOutbox++
	body, _ := payload.MarshalJSON()
	item := &OutboxItem{
		ID: r.nextOutbox, EventType: eventType, UserID: userID, Payload: body,
		Status: "pending", NextAttemptAt: r.now, CreatedAt: r.now,
	}
	r.outbox = append(r.outbox, item)
	return item
}

func (r *fakeRepo) UpsertSubscription(_ context.Context, subscription *Subscription) (*Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.endpoints[subscription.Endpoint]; ok {
		existing := r.subscriptions[id]
		existing.UserID = subscription.UserID
		existing.P256DH = subscription.P256DH
		existing.Auth = subscription.Auth
		existing.UserAgent = subscription.UserAgent
		existing.IsActive = true
		return copySubscription(existing), nil
	}
	r.nextSubID++
	sub := *subscription
	sub.ID = r.nextSubID
	sub.IsActive = true
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = r.now
	}
	r.subscriptions[sub.ID] = &sub
	r.endpoints[sub.Endpoint] = sub.ID
	return copySubscription(&sub), nil
}

func copySubscription(sub *Subscription) *Subscription {
	cp := *sub
	if sub.LastSuccessAt != nil {
		value := *sub.LastSuccessAt
		cp.LastSuccessAt = &value
	}
	return &cp
}

func (r *fakeRepo) DeleteSubscription(_ context.Context, id, userID int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub, ok := r.subscriptions[id]
	if !ok || sub.UserID != userID {
		return false, nil
	}
	delete(r.subscriptions, id)
	delete(r.endpoints, sub.Endpoint)
	return true, nil
}

func (r *fakeRepo) ActiveSubscriptionsByUser(_ context.Context, userID int64) ([]*Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*Subscription
	for _, sub := range r.subscriptions {
		if sub.UserID == userID && sub.IsActive {
			result = append(result, copySubscription(sub))
		}
	}
	return result, nil
}

func (r *fakeRepo) DeactivateSubscription(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sub, ok := r.subscriptions[id]; ok {
		sub.IsActive = false
	}
	return nil
}

func (r *fakeRepo) TouchSubscription(_ context.Context, id int64, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sub, ok := r.subscriptions[id]; ok {
		value := at
		sub.LastSuccessAt = &value
	}
	return nil
}

func (r *fakeRepo) EnqueueOutbox(_ context.Context, items []*OutboxItem) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inserted := 0
	for _, item := range items {
		if item.IdempotencyKey != nil {
			if r.keys[*item.IdempotencyKey] {
				continue
			}
			r.keys[*item.IdempotencyKey] = true
		}
		r.nextOutbox++
		cp := *item
		cp.ID = r.nextOutbox
		if cp.Status == "" {
			cp.Status = "pending"
		}
		r.outbox = append(r.outbox, &cp)
		inserted++
	}
	return inserted, nil
}

func (r *fakeRepo) PendingOutbox(_ context.Context, limit int) ([]*OutboxItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*OutboxItem
	for _, item := range r.outbox {
		if item.Status == "pending" && !item.NextAttemptAt.After(r.now) {
			result = append(result, item)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (r *fakeRepo) MarkOutboxSent(_ context.Context, id int64, at time.Time) error {
	return r.updateOutbox(id, func(item *OutboxItem) {
		item.Status = "sent"
		item.SentAt = &at
	})
}

func (r *fakeRepo) MarkOutboxRetry(_ context.Context, id int64, attempts int, nextAt time.Time, lastErr string) error {
	return r.updateOutbox(id, func(item *OutboxItem) {
		item.Status = "pending"
		item.Attempts = attempts
		item.NextAttemptAt = nextAt
		item.LastError = &lastErr
	})
}

func (r *fakeRepo) MarkOutboxFailed(_ context.Context, id int64, attempts int, lastErr string) error {
	return r.updateOutbox(id, func(item *OutboxItem) {
		item.Status = "failed"
		item.Attempts = attempts
		item.LastError = &lastErr
	})
}

func (r *fakeRepo) updateOutbox(id int64, mutate func(*OutboxItem)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.outbox {
		if item.ID == id {
			mutate(item)
			return nil
		}
	}
	return nil
}

func (r *fakeRepo) BuildingUserIDs(_ context.Context, buildingID int64) ([]int64, error) {
	return append([]int64{}, r.buildingUsers[buildingID]...), nil
}

func (r *fakeRepo) AdminUserIDs(context.Context) ([]int64, error) {
	return append([]int64{}, r.admins...), nil
}

func (r *fakeRepo) ReminderTickets(context.Context, time.Time) ([]*ReminderTicket, error) {
	return r.reminders, nil
}

func (r *fakeRepo) TicketInfo(_ context.Context, id int64) (*TicketInfo, error) {
	if info, ok := r.tickets[id]; ok {
		return info, nil
	}
	return nil, ErrNotFound
}

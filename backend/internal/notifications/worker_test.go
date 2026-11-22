package notifications

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

type fakeSender struct {
	enabled bool
	err     error
	calls   int
}

func (s *fakeSender) Enabled() bool { return s.enabled }

func (s *fakeSender) Send(context.Context, Subscription, Payload) error {
	s.calls++
	return s.err
}

type fakeReminders struct {
	count int
	calls int
	err   error
}

func (r *fakeReminders) EnqueueReminders(context.Context, time.Time) (int, error) {
	r.calls++
	return r.count, r.err
}

func newTestWorker(repo *fakeRepo, sender Sender, reminders ReminderEnqueuer, now time.Time) *Worker {
	return NewWorker(repo, sender,
		NewStaticPushSettings(true, "10:00", time.UTC), reminders,
		platform.FixedClock{Time: now}, nil)
}

func TestWorker_Success(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.now = now
	sub := repo.addSub(1)
	item := repo.addOutbox(1, "NEW_TICKET", Payload{Title: "T", Body: "B"})
	sender := &fakeSender{enabled: true}
	worker := newTestWorker(repo, sender, nil, now)

	processed, err := worker.ProcessOutbox(context.Background())
	if err != nil || processed != 1 {
		t.Fatalf("ProcessOutbox = %d, %v", processed, err)
	}
	if item.Status != "sent" || item.SentAt == nil {
		t.Fatalf("item = %+v", item)
	}
	if sub.LastSuccessAt == nil || !sub.LastSuccessAt.Equal(now) {
		t.Fatalf("last_success_at = %v", sub.LastSuccessAt)
	}
	if sender.calls != 1 {
		t.Fatalf("calls = %d", sender.calls)
	}
}

func TestWorker_GoneDeactivates(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.now = now
	sub := repo.addSub(1)
	item := repo.addOutbox(1, "NEW_TICKET", Payload{Title: "T", Body: "B"})
	sender := &fakeSender{enabled: true, err: &DeliveryError{StatusCode: 410, Gone: true}}
	worker := newTestWorker(repo, sender, nil, now)

	if _, err := worker.ProcessOutbox(context.Background()); err != nil {
		t.Fatalf("ProcessOutbox: %v", err)
	}
	if item.Status != "failed" {
		t.Fatalf("status = %s", item.Status)
	}
	if sub.IsActive {
		t.Fatal("подписка должна быть деактивирована после 410")
	}
}

func TestWorker_RetryThenFail(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.now = now
	repo.addSub(1)
	item := repo.addOutbox(1, "NEW_TICKET", Payload{Title: "T", Body: "B"})
	sender := &fakeSender{enabled: true, err: errors.New("push service unavailable")}
	worker := newTestWorker(repo, sender, nil, now)
	ctx := context.Background()

	if _, err := worker.ProcessOutbox(ctx); err != nil {
		t.Fatalf("attempt 1: %v", err)
	}
	if item.Status != "pending" || item.Attempts != 1 {
		t.Fatalf("after attempt 1: %+v", item)
	}
	repo.now = item.NextAttemptAt
	if _, err := worker.ProcessOutbox(ctx); err != nil {
		t.Fatalf("attempt 2: %v", err)
	}
	if item.Status != "pending" || item.Attempts != 2 {
		t.Fatalf("after attempt 2: %+v", item)
	}
	repo.now = item.NextAttemptAt
	if _, err := worker.ProcessOutbox(ctx); err != nil {
		t.Fatalf("attempt 3: %v", err)
	}
	if item.Status != "failed" || item.Attempts != MaxAttempts {
		t.Fatalf("after attempt 3: %+v", item)
	}
	if item.LastError == nil || !strings.Contains(*item.LastError, "unavailable") {
		t.Fatalf("last_error = %v", item.LastError)
	}
}

func TestWorker_NoSubscriptions(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.now = now
	item := repo.addOutbox(2, "NEW_TICKET", Payload{Title: "T", Body: "B"})
	worker := newTestWorker(repo, &fakeSender{enabled: true}, nil, now)

	if _, err := worker.ProcessOutbox(context.Background()); err != nil {
		t.Fatalf("ProcessOutbox: %v", err)
	}
	if item.Status != "failed" {
		t.Fatalf("status = %s", item.Status)
	}
}

func TestWorker_ReminderSchedule(t *testing.T) {
	repo := newFakeRepo()
	sender := &fakeSender{enabled: true}
	reminders := &fakeReminders{count: 4}
	worker := newTestWorker(repo, sender, reminders, time.Date(2026, 11, 22, 9, 59, 0, 0, time.UTC))

	if count, err := worker.RunRemindersIfDue(context.Background(), time.Date(2026, 11, 22, 9, 59, 0, 0, time.UTC)); err != nil || count != 0 {
		t.Fatalf("до времени: count=%d err=%v", count, err)
	}
	if count, err := worker.RunRemindersIfDue(context.Background(), time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)); err != nil || count != 4 {
		t.Fatalf("в 10:00: count=%d err=%v", count, err)
	}
	if count, err := worker.RunRemindersIfDue(context.Background(), time.Date(2026, 11, 22, 10, 30, 0, 0, time.UTC)); err != nil || count != 0 {
		t.Fatalf("повтор в тот же день: count=%d err=%v", count, err)
	}
	if count, err := worker.RunRemindersIfDue(context.Background(), time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)); err != nil || count != 4 {
		t.Fatalf("следующий день: count=%d err=%v", count, err)
	}
}

func TestWorker_DisabledSenderSkips(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.now = now
	repo.addSub(1)
	item := repo.addOutbox(1, "NEW_TICKET", Payload{Title: "T", Body: "B"})
	worker := newTestWorker(repo, &fakeSender{enabled: false}, nil, now)

	processed, err := worker.ProcessOutbox(context.Background())
	if err != nil || processed != 0 {
		t.Fatalf("no-op: %d, %v", processed, err)
	}
	if item.Status != "pending" {
		t.Fatalf("item status = %s", item.Status)
	}
}

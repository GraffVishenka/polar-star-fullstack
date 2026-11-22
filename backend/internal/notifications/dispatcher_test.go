package notifications

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

func testClock(now time.Time) *testClockValue { return &testClockValue{now: now} }

type testClockValue struct {
	now time.Time
}

func (c *testClockValue) Now() time.Time { return c.now }

func int64Ptr(value int64) *int64 { return &value }

func newTestDispatcher(repo *fakeRepo, enabled bool) *Dispatcher {
	return NewDispatcher(repo, NewStaticPushSettings(enabled, "10:00", time.UTC),
		testClock(repo.now), nil, true)
}

func TestDispatcher_NewTicketRecipients(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.now = now
	repo.buildingUsers[1] = []int64{10, 11}
	// Другое здание не должно попадать в адресаты NEW_TICKET.
	repo.buildingUsers[2] = []int64{12}
	repo.admins = []int64{1, 2}
	repo.addSub(1)
	repo.addSub(2)
	repo.addSub(10)
	repo.addSub(11)
	repo.addSub(12)

	dispatcher := newTestDispatcher(repo, true)
	err := dispatcher.Publish(context.Background(), platform.NotificationEvent{
		Type:        platform.EventNewTicket,
		TicketID:    5,
		BuildingID:  int64Ptr(1),
		ActorUserID: 10,
		Title:       "Новая заявка",
		Body:        "Гость · 101",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	assertRecipients(t, repo, []int64{1, 2, 11})

	// Пустое здание: остаются только ADMIN.
	repo.outbox = nil
	repo.buildingUsers[3] = nil
	err = dispatcher.Publish(context.Background(), platform.NotificationEvent{
		Type:       platform.EventNewTicket,
		TicketID:   6,
		BuildingID: int64Ptr(3),
	})
	if err != nil {
		t.Fatalf("Publish empty building: %v", err)
	}
	assertRecipients(t, repo, []int64{1, 2})
}

func TestDispatcher_AssignedAndComment(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.now = now
	repo.addSub(7)
	repo.addSub(10)
	repo.addSub(2)
	dispatcher := newTestDispatcher(repo, true)

	if err := dispatcher.Publish(context.Background(), platform.NotificationEvent{
		Type:        platform.EventTicketAssigned,
		TicketID:    9,
		UserIDs:     []int64{7},
		ActorUserID: 1,
	}); err != nil {
		t.Fatalf("assigned: %v", err)
	}
	assertRecipients(t, repo, []int64{7})

	// NEW_COMMENT: исполнитель + created_by, автор исключён.
	repo.outbox = nil
	if err := dispatcher.Publish(context.Background(), platform.NotificationEvent{
		Type:        platform.EventNewComment,
		TicketID:    9,
		UserIDs:     []int64{10, 2},
		ActorUserID: 10,
	}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	assertRecipients(t, repo, []int64{2})
}

func TestDispatcher_Disabled(t *testing.T) {
	repo := newFakeRepo()
	repo.now = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo.admins = []int64{1}
	repo.addSub(1)
	dispatcher := newTestDispatcher(repo, false)
	if err := dispatcher.Publish(context.Background(), platform.NotificationEvent{
		Type: platform.EventNewTicket, TicketID: 1, BuildingID: int64Ptr(1),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(repo.outbox) != 0 {
		t.Fatalf("push_enabled=false создал outbox: %+v", repo.outbox)
	}
}

func TestDispatcher_NoVAPIDNoEvents(t *testing.T) {
	repo := newFakeRepo()
	repo.now = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo.admins = []int64{1}
	dispatcher := NewDispatcher(repo, NewStaticPushSettings(true, "10:00", time.UTC),
		testClock(repo.now), nil, false)
	if err := dispatcher.Publish(context.Background(), platform.NotificationEvent{
		Type: platform.EventNewTicket, TicketID: 1, BuildingID: int64Ptr(1),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(repo.outbox) != 0 {
		t.Fatalf("no-op VAPID создал outbox: %+v", repo.outbox)
	}
}

func TestDispatcher_PayloadLimits(t *testing.T) {
	repo := newFakeRepo()
	repo.now = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo.admins = []int64{1}
	repo.addSub(1)
	dispatcher := newTestDispatcher(repo, true)
	title := strings.Repeat("Ж", 100)
	body := strings.Repeat("Я", 300)
	if err := dispatcher.Publish(context.Background(), platform.NotificationEvent{
		Type:       platform.EventNewTicket,
		TicketID:   1,
		BuildingID: int64Ptr(1),
		Title:      title,
		Body:       body,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(repo.outbox) != 1 {
		t.Fatalf("outbox = %d", len(repo.outbox))
	}
	var payload Payload
	if err := json.Unmarshal(repo.outbox[0].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if utf8.RuneCountInString(payload.Title) != maxTitleRunes {
		t.Fatalf("title runes = %d", utf8.RuneCountInString(payload.Title))
	}
	if utf8.RuneCountInString(payload.Body) != maxBodyRunes {
		t.Fatalf("body runes = %d", utf8.RuneCountInString(payload.Body))
	}
}

func TestDispatcher_RemindersIdempotent(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.now = now
	repo.admins = []int64{1}
	repo.addSub(1)
	repo.addSub(10)
	executor := int64(10)
	adminExecutor := int64(1)
	repo.reminders = []*ReminderTicket{
		{ID: 5, ApplicantName: "Иванов", Cabinet: "101", Deadline: now, IsToday: true, ExecutorID: &executor},
		// Исполнитель совпадает с ADMIN: получатель не должен задвоиться.
		{ID: 6, ApplicantName: "Петров", Cabinet: "102", Deadline: now.Add(-24 * time.Hour), IsToday: false, ExecutorID: &adminExecutor},
	}
	dispatcher := newTestDispatcher(repo, true)

	count, err := dispatcher.EnqueueReminders(context.Background(), now)
	if err != nil {
		t.Fatalf("EnqueueReminders: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3 (2 админа/исполнитель + 1 админ)", count)
	}
	second, err := dispatcher.EnqueueReminders(context.Background(), now)
	if err != nil || second != 0 {
		t.Fatalf("повторный запуск: count=%d err=%v", second, err)
	}
	types := map[string]int{}
	for _, item := range repo.outbox {
		types[item.EventType]++
		if item.IdempotencyKey == nil || *item.IdempotencyKey == "" {
			t.Fatalf("нет idempotency key: %+v", item)
		}
	}
	if types[string(platform.EventDeadlineToday)] != 2 || types[string(platform.EventDeadlineOverdue)] != 1 {
		t.Fatalf("types = %v", types)
	}
}

func assertRecipients(t *testing.T, repo *fakeRepo, want []int64) {
	t.Helper()
	got := make([]int64, 0, len(repo.outbox))
	for _, item := range repo.outbox {
		got = append(got, item.UserID)
	}
	if len(got) != len(want) {
		t.Fatalf("recipients = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("recipients = %v, want %v", got, want)
		}
	}
}

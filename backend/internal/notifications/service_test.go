package notifications

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

func newTestService(repo *fakeRepo, publicKey string, enabled bool) *Service {
	return NewService(repo, publicKey, enabled, platform.FixedClock{Time: repo.now}, nil)
}

func TestService_SubscribeUpsertAndValidate(t *testing.T) {
	repo := newFakeRepo()
	repo.now = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	service := newTestService(repo, "public-key", true)
	ctx := context.Background()

	input := SubscribeInput{Endpoint: "https://push.example/a", P256DH: "p", Auth: "a", UserAgent: "ua"}
	first, err := service.Subscribe(ctx, 1, input)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	second, err := service.Subscribe(ctx, 2, input)
	if err != nil {
		t.Fatalf("Subscribe upsert: %v", err)
	}
	if first.ID != second.ID || second.UserID != 2 {
		t.Fatalf("upsert: first=%+v second=%+v", first, second)
	}
	if len(repo.subscriptions) != 1 {
		t.Fatalf("подписок = %d, want 1", len(repo.subscriptions))
	}

	for name, bad := range map[string]SubscribeInput{
		"endpoint": {Endpoint: "", P256DH: "p", Auth: "a"},
		"p256dh":   {Endpoint: "https://push.example/b", P256DH: "", Auth: "a"},
		"auth":     {Endpoint: "https://push.example/c", P256DH: "p", Auth: ""},
	} {
		_, err := service.Subscribe(ctx, 1, bad)
		var apiErr *apierr.Error
		if !errors.As(err, &apiErr) || apiErr.Status != 422 {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestService_DeleteForeignIs404(t *testing.T) {
	repo := newFakeRepo()
	repo.now = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	sub := repo.addSub(2)
	service := newTestService(repo, "public-key", true)

	err := service.Delete(context.Background(), 1, sub.ID)
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("чужая подписка: %v", err)
	}
	if _, ok := repo.subscriptions[sub.ID]; !ok {
		t.Fatal("чужая подписка не должна удаляться")
	}
	if err := service.Delete(context.Background(), 2, sub.ID); err != nil {
		t.Fatalf("своя подписка: %v", err)
	}
	if _, ok := repo.subscriptions[sub.ID]; ok {
		t.Fatal("своя подписка должна удалиться")
	}
}

func TestSender_NoVAPIDIsNoop(t *testing.T) {
	sender := NewWebPushSender("", "", "")
	if sender.Enabled() {
		t.Fatal("без ключей sender должен быть disabled")
	}
	if sender.PublicKey() != "" {
		t.Fatalf("public key = %q", sender.PublicKey())
	}
	err := sender.Send(context.Background(), Subscription{Endpoint: "https://push.example/a"},
		Payload{Title: "T", Body: "B"})
	if err == nil {
		t.Fatal("disabled sender должен возвращать ошибку")
	}

	repo := newFakeRepo()
	service := newTestService(repo, sender.PublicKey(), sender.Enabled())
	if service.Enabled() || service.PublicKey() != "" {
		t.Fatalf("service no-op: enabled=%v key=%q", service.Enabled(), service.PublicKey())
	}
}

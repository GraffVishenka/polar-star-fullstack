package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

func newTestHandler(repo *fakeRepo, publicKey string) *Handler {
	service := NewService(repo, publicKey, publicKey != "",
		platform.FixedClock{Time: repo.now}, nil)
	return NewHandler(service, nil)
}

func TestHandler_PublicKey(t *testing.T) {
	repo := newFakeRepo()
	repo.now = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	handler := newTestHandler(repo, "BPublicKey")

	req := httptest.NewRequest(http.MethodGet, "/api/push/public-key", nil)
	rec := httptest.NewRecorder()
	handler.PublicKey(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if body["public_key"] != "BPublicKey" {
		t.Fatalf("public_key = %q", body["public_key"])
	}
}

func TestHandler_Subscribe(t *testing.T) {
	repo := newFakeRepo()
	repo.now = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	handler := newTestHandler(repo, "BPublicKey")

	body := `{"endpoint":"https://push.example/device-1","keys":{"p256dh":"p","auth":"a"},"user_agent":"ua"}`
	req := httptest.NewRequest(http.MethodPost, "/api/push/subscriptions", strings.NewReader(body))
	req = req.WithContext(auth.WithUser(req.Context(), &auth.User{ID: 5, Role: auth.RoleUser, IsActive: true}))
	rec := httptest.NewRecorder()
	handler.Subscribe(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if len(repo.subscriptions) != 1 {
		t.Fatalf("подписок = %d", len(repo.subscriptions))
	}
}

func TestHandler_DeleteForeign404(t *testing.T) {
	repo := newFakeRepo()
	repo.now = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	foreign := repo.addSub(2)
	handler := newTestHandler(repo, "BPublicKey")

	req := httptest.NewRequest(http.MethodDelete, "/api/push/subscriptions/"+strconv.FormatInt(foreign.ID, 10), nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", strconv.FormatInt(foreign.ID, 10))
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	ctx = auth.WithUser(ctx, &auth.User{ID: 1, Role: auth.RoleUser, IsActive: true})
	rec := httptest.NewRecorder()
	handler.Delete(rec, req.WithContext(ctx))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if _, ok := repo.subscriptions[foreign.ID]; !ok {
		t.Fatal("чужая подписка удалена")
	}
}

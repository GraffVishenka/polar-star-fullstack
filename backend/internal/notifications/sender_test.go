package notifications

import (
	"context"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// TestWebPushSender_AgainstFakePushServer проверяет полный путь шифрования и
// VAPID-подписи на локальном httptest-сервере (без внешней сети): 201 — успех,
// 410 — DeliveryError с Gone для деактивации подписки (NOTIF-6).
func TestWebPushSender_AgainstFakePushServer(t *testing.T) {
	// js/wasm использует fetch вместо net/http, а тестовый listener httptest
	// недоступен; в CI (нативный Go) тест выполняется полностью.
	if runtime.GOOS == "js" {
		t.Skip("httptest push server недоступен в js/wasm")
	}
	privateVAPID, publicVAPID, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("VAPID: %v", err)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.Header.Get("Content-Encoding"); got != "aes128gcm" {
			t.Errorf("Content-Encoding = %q", got)
		}
		if r.Header.Get("Authorization") == "" || r.Header.Get("TTL") != "300" {
			t.Errorf("VAPID/TTL headers: %v", r.Header)
		}
		if r.URL.Path == "/gone" {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	curve := elliptic.P256()
	_, x, y, err := elliptic.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatalf("subscription key: %v", err)
	}
	p256dh := base64.RawURLEncoding.EncodeToString(elliptic.Marshal(curve, x, y))
	authBytes := make([]byte, 16)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("auth: %v", err)
	}
	auth := base64.RawURLEncoding.EncodeToString(authBytes)

	sender := NewWebPushSender(publicVAPID, privateVAPID, "mailto:test@example.com")
	payload := Payload{Title: "Заявка", Body: "Текст", URL: "/tickets/1", Tag: "ticket-1", Icon: "/icon.png"}

	err = sender.Send(context.Background(), Subscription{
		Endpoint: server.URL + "/ok", P256DH: p256dh, Auth: auth,
	}, payload)
	if err != nil {
		t.Fatalf("Send ok: %v", err)
	}

	err = sender.Send(context.Background(), Subscription{
		Endpoint: server.URL + "/gone", P256DH: p256dh, Auth: auth,
	}, payload)
	var deliveryErr *DeliveryError
	if !errors.As(err, &deliveryErr) || !deliveryErr.Gone || deliveryErr.StatusCode != http.StatusGone {
		t.Fatalf("gone: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
}

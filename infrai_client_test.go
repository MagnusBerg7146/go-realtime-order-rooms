package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPublishRetries429WithSameIdempotencyKey(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/realtime/publish" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "update:ord_42:checkout" {
			t.Fatalf("Idempotency-Key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"ok":false,"data":null,"error":{"code":"RATE_LIMITED","message":"retry later"},"metadata":{}}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"data":{"published":true},"error":null,"metadata":{}}`)
	}))
	defer upstream.Close()

	client := NewInfraiClient("test-key")
	client.baseURL = upstream.URL
	client.httpClient = upstream.Client()
	client.sleep = func(context.Context, time.Duration) error { return nil }
	update := OrderUpdate{OrderID: "ord_42", Stage: StageCheckout, Message: "payment accepted", AccountID: "acct_7"}
	if err := client.Publish(context.Background(), update, "update:ord_42:checkout"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

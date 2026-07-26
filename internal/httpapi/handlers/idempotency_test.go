package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/repo"
)

// End-to-end through the real RegisterRoutes chain: a mutation replayed with the
// same Idempotency-Key returns the stored response, does not re-run the handler
// (only one context is created), and — because IdempotencyMiddleware is
// registered before PublishMiddleware and short-circuits on replay — does not
// re-emit an SSE event.
func TestIdempotency_ReplayNoDuplicateSSEOrRecord(t *testing.T) {
	e := setupAPIEnv(t)
	ch, cancel := e.eventsHub.Subscribe(1)
	defer cancel()

	const key = "e2e-idem-key-123456"
	body := map[string]any{"name": "Home", "color": "blue"}

	req1 := e.authedReq(t, http.MethodPost, "/api/v1/contexts/", body)
	req1.Header.Set("Idempotency-Key", key)
	resp1, b1 := doReq(t, e.app, req1)
	if resp1.StatusCode != 201 && resp1.StatusCode != 200 {
		t.Fatalf("create context: got %d, body %s", resp1.StatusCode, b1)
	}
	if resp1.Header.Get("X-Idempotent-Replay") != "" {
		t.Fatalf("first call must not be flagged as a replay")
	}
	if got := drain(ch, 200*time.Millisecond); len(got) == 0 {
		t.Fatal("first mutation should publish an SSE event")
	}

	req2 := e.authedReq(t, http.MethodPost, "/api/v1/contexts/", body)
	req2.Header.Set("Idempotency-Key", key)
	resp2, b2 := doReq(t, e.app, req2)
	if resp2.StatusCode != resp1.StatusCode {
		t.Fatalf("replay status: got %d, want %d", resp2.StatusCode, resp1.StatusCode)
	}
	if resp2.Header.Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("replay must set X-Idempotent-Replay: true")
	}
	if string(b1) != string(b2) {
		t.Fatalf("replay body mismatch:\n first:  %s\n replay: %s", b1, b2)
	}
	if got := drain(ch, 200*time.Millisecond); len(got) != 0 {
		t.Fatalf("replay must not re-publish SSE, got %v", got)
	}

	_, total, err := e.ctxs.List(context.Background(), repo.Page{})
	if err != nil {
		t.Fatalf("list contexts: %v", err)
	}
	if total != 1 {
		t.Fatalf("replay must not create a second context, total=%d", total)
	}
}

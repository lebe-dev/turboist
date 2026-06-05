package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

// TestFederationDeadLetter_ListsParkedEvents asserts the JWT admin endpoint
// surfaces parked dead-letter rows newest-first for the diagnostics view, with
// only metadata (no payload) (Federation v1 F4.4, US-4.4 AC3).
func TestFederationDeadLetter_ListsParkedEvents(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederation(t, e, p.ID)

	for _, ev := range []struct{ id, at string }{
		{"e1", "2026-06-03T10:00:00.000Z"},
		{"e2", "2026-06-03T10:00:05.000Z"},
	} {
		if _, err := e.db.Exec(
			`INSERT INTO federation_dead_letter (event_id, peer_instance_url, local_project_id, payload, status_code, reason, failed_at)
			 VALUES (?, 'https://peer.example', ?, '{"event_id":"x"}', 403, 'federation_read_only', ?)`,
			ev.id, p.ID, ev.at,
		); err != nil {
			t.Fatalf("seed dead-letter %s: %v", ev.id, err)
		}
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/federation/dead-letter", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dead-letter status: got %d, want 200; body %s", resp.StatusCode, body)
	}
	var out []dto.DeadLetterDTO
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v; body %s", err, body)
	}
	if len(out) != 2 {
		t.Fatalf("dead-letter entries: got %d, want 2", len(out))
	}
	if out[0].EventId != "e2" || out[1].EventId != "e1" {
		t.Errorf("dead-letter order: got [%s, %s], want [e2, e1]", out[0].EventId, out[1].EventId)
	}
	if out[0].PeerInstanceUrl != "https://peer.example" || out[0].StatusCode != 403 || out[0].Reason != "federation_read_only" {
		t.Errorf("dead-letter entry: got %+v", out[0])
	}
	if out[0].ProjectId != p.ID {
		t.Errorf("dead-letter project id: got %d, want %d", out[0].ProjectId, p.ID)
	}
}

// TestFederationDeadLetter_EmptyArray asserts the endpoint returns a stable empty
// JSON array when nothing has been parked.
func TestFederationDeadLetter_EmptyArray(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/federation/dead-letter", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dead-letter status: got %d, want 200; body %s", resp.StatusCode, body)
	}
	var out []dto.DeadLetterDTO
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v; body %s", err, body)
	}
	if len(out) != 0 {
		t.Errorf("empty dead-letter: got %d entries, want 0", len(out))
	}
}

// TestEvents_RateLimited429WithRetryAfter asserts an inbound batch from a peer
// that has exceeded its rate is rejected 429 with a Retry-After header, and that
// the rejected batch writes ZERO inbox/enqueue rows — no partial accept (Federation
// v1 F4.4, US-8.3 AC1).
func TestEvents_RateLimited429WithRetryAfter(t *testing.T) {
	env := newFedEventsEnv(t, func(e *fedEventsEnv) {
		// Allow nothing: the very first batch is throttled with a 17s Retry-After.
		e.rateLimiter = &stubRateLimiter{allowCount: 0, retryAfterSecs: 17}
	})
	evt := env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0))

	resp := env.postEvents(t, events.Batch{Events: []events.Event{evt}})
	if resp.StatusCode != 429 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("rate-limited status: got %d, body %s", resp.StatusCode, b)
	}
	if ra := resp.Header.Get("Retry-After"); ra != "17" {
		t.Errorf("Retry-After header: got %q, want 17", ra)
	}
	if n := inboxCount(t, env.db); n != 0 {
		t.Errorf("rate-limited batch must write no inbox rows: got %d", n)
	}
	if len(env.enqueued) != 0 {
		t.Errorf("rate-limited batch must not enqueue: got %d", len(env.enqueued))
	}
}

// TestEvents_RateLimitAllowsUnderLimit asserts a peer under its rate limit is
// served normally (the limiter does not break the happy path).
func TestEvents_RateLimitAllowsUnderLimit(t *testing.T) {
	env := newFedEventsEnv(t, func(e *fedEventsEnv) {
		e.rateLimiter = &stubRateLimiter{allowCount: 5, retryAfterSecs: 10}
	})
	evt := env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0))

	resp := env.postEvents(t, events.Batch{Events: []events.Event{evt}})
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("under-limit status: got %d, body %s", resp.StatusCode, b)
	}
	if n := inboxCount(t, env.db); n != 1 {
		t.Errorf("under-limit batch should be accepted: inbox rows got %d, want 1", n)
	}
}

// TestEvents_OversizedBatch413NoPartialApply asserts a batch exceeding the max
// events-per-batch limit is rejected 413 and applied to NOTHING — no partial
// accept (Federation v1 F4.4, US-8.3 AC3). The 413 is checked BEFORE any
// validation / inbox write so an oversized batch is cheap to reject.
func TestEvents_OversizedBatch413NoPartialApply(t *testing.T) {
	env := newFedEventsEnv(t, func(e *fedEventsEnv) {
		e.maxBatchEvents = 2 // cap at 2 events so a 3-event batch is oversized.
	})
	batch := events.Batch{Events: []events.Event{
		env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0)),
		env.signedEvent(t, events.OpUpdate, "task-2", hlcNow(1)),
		env.signedEvent(t, events.OpUpdate, "task-3", hlcNow(2)),
	}}

	resp := env.postEvents(t, batch)
	if resp.StatusCode != 413 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("oversized batch status: got %d, want 413, body %s", resp.StatusCode, b)
	}
	if n := inboxCount(t, env.db); n != 0 {
		t.Errorf("oversized batch must not partially apply: inbox rows got %d, want 0", n)
	}
	if len(env.enqueued) != 0 {
		t.Errorf("oversized batch must not enqueue: got %d", len(env.enqueued))
	}
}

// TestEvents_BatchAtLimitAccepted asserts a batch exactly at the max-events cap
// is accepted (the cap is inclusive — only a STRICTLY larger batch is 413).
func TestEvents_BatchAtLimitAccepted(t *testing.T) {
	env := newFedEventsEnv(t, func(e *fedEventsEnv) {
		e.maxBatchEvents = 2
	})
	batch := events.Batch{Events: []events.Event{
		env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0)),
		env.signedEvent(t, events.OpUpdate, "task-2", hlcNow(1)),
	}}

	resp := env.postEvents(t, batch)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("at-limit batch status: got %d, body %s", resp.StatusCode, b)
	}
	if n := inboxCount(t, env.db); n != 2 {
		t.Errorf("at-limit batch should be accepted whole: inbox rows got %d, want 2", n)
	}
}

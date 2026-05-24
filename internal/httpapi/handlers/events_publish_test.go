package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/service/events"
)

// drain consumes all currently buffered events from ch (up to wait), returning
// them in order. Used to assert that PublishMiddleware emitted (or did not
// emit) events for a given request.
func drain(ch <-chan events.Event, wait time.Duration) []events.Event {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	var out []events.Event
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline.C:
			return out
		}
	}
}

func TestPublishMiddleware_EmitsOnContextCreate(t *testing.T) {
	e := setupAPIEnv(t)
	ch, cancel := e.eventsHub.Subscribe(1)
	defer cancel()

	req := e.authedReq(t, http.MethodPost, "/api/v1/contexts/", map[string]any{
		"name":  "Home",
		"color": "blue",
	})
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		t.Fatalf("create context: got %d, body %s", resp.StatusCode, body)
	}

	got := drain(ch, 200*time.Millisecond)
	if len(got) == 0 {
		t.Fatal("expected at least one event")
	}
	if got[0].Scope != events.ScopeContexts {
		t.Fatalf("scope: want %q, got %q", events.ScopeContexts, got[0].Scope)
	}
}

func TestPublishMiddleware_NoEmitOnGet(t *testing.T) {
	e := setupAPIEnv(t)
	ch, cancel := e.eventsHub.Subscribe(1)
	defer cancel()

	req := e.authedReq(t, http.MethodGet, "/api/v1/contexts/", nil)
	resp, _ := doReq(t, e.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("list contexts: got %d", resp.StatusCode)
	}

	if got := drain(ch, 100*time.Millisecond); len(got) != 0 {
		t.Fatalf("GET should not publish, got %v", got)
	}
}

func TestPublishMiddleware_NoEmitOnValidationError(t *testing.T) {
	e := setupAPIEnv(t)
	ch, cancel := e.eventsHub.Subscribe(1)
	defer cancel()

	// Missing title → 400.
	req := e.authedReq(t, http.MethodPost, "/api/v1/contexts/", map[string]any{
		"color": "blue",
	})
	resp, _ := doReq(t, e.app, req)
	if resp.StatusCode < 400 {
		t.Fatalf("expected 4xx, got %d", resp.StatusCode)
	}

	if got := drain(ch, 100*time.Millisecond); len(got) != 0 {
		t.Fatalf("failed mutation should not publish, got %v", got)
	}
}

func TestPublishMiddleware_NoEmitOnTicketIssue(t *testing.T) {
	e := setupAPIEnv(t)
	ch, cancel := e.eventsHub.Subscribe(1)
	defer cancel()

	req := e.authedReq(t, http.MethodPost, "/api/v1/events/ticket", nil)
	resp, _ := doReq(t, e.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("issue ticket: got %d", resp.StatusCode)
	}

	if got := drain(ch, 100*time.Millisecond); len(got) != 0 {
		t.Fatalf("/events/ticket should not publish, got %v", got)
	}
}

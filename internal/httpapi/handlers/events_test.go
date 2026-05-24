package handlers_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/service/events"
)

func TestEvents_IssueTicket_RequiresAuth(t *testing.T) {
	e := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/ticket", nil)
	resp, _ := doReq(t, e.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestEvents_IssueTicket_Success(t *testing.T) {
	e := setupAPIEnv(t)
	req := e.authedReq(t, http.MethodPost, "/api/v1/events/ticket", nil)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d; body: %s", resp.StatusCode, body)
	}
	var r struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(r.Ticket) < 32 {
		t.Fatalf("ticket too short: %q", r.Ticket)
	}
	if r.ExpiresIn <= 0 {
		t.Fatalf("expiresIn: want >0, got %d", r.ExpiresIn)
	}
}

func TestEvents_Stream_RejectsMissingTicket(t *testing.T) {
	e := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	resp, _ := doReq(t, e.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestEvents_Stream_RejectsBadTicket(t *testing.T) {
	e := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?ticket=garbage", nil)
	resp, _ := doReq(t, e.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

// openStream opens the SSE stream with the given ticket and returns the
// scanner over the response body plus a cleanup function. The caller must
// fully drain or close the body to release the underlying fasthttp goroutine.
func openStream(t *testing.T, app *fiber.App, ticket string) (*bufio.Reader, io.Closer) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?ticket="+ticket, nil)
	// app.Test with a long timeout returns after headers + first chunk; for
	// streaming the body is read incrementally by us.
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if resp.StatusCode != 200 {
		_ = resp.Body.Close()
		t.Fatalf("stream status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		_ = resp.Body.Close()
		t.Fatalf("content-type: want text/event-stream, got %q", ct)
	}
	return bufio.NewReader(resp.Body), resp.Body
}

func readUntil(t *testing.T, r *bufio.Reader, prefix string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for line with prefix %q", prefix)
		}
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
}

func TestEvents_Stream_DeliversConnectedAndEvent(t *testing.T) {
	e := setupAPIEnv(t)
	tok, err := e.eventsTix.Issue(1)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	// Publish an event slightly after we open the stream so it definitely
	// arrives after the connected comment.
	go func() {
		time.Sleep(50 * time.Millisecond)
		e.eventsHub.Publish(context.Background(), 1, events.ScopeTasks)
	}()

	r, body := openStream(t, e.app, tok)
	defer func() { _ = body.Close() }()

	connected := readUntil(t, r, ": connected", 2*time.Second)
	if !strings.Contains(connected, "connected") {
		t.Fatalf("expected connected comment, got %q", connected)
	}
	evLine := readUntil(t, r, "event: ", 2*time.Second)
	if !strings.Contains(evLine, "tasks") {
		t.Fatalf("expected tasks event, got %q", evLine)
	}
}

func TestEvents_Stream_TicketIsOneShot(t *testing.T) {
	e := setupAPIEnv(t)
	tok, _ := e.eventsTix.Issue(1)

	// First use must succeed (we don't need to drain — just check status).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?ticket="+tok, nil)
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("first: want 200, got %d", resp.StatusCode)
	}

	// Second use of the same ticket must be rejected.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/events?ticket="+tok, nil)
	resp2, _ := doReq(t, e.app, req2)
	if resp2.StatusCode != 401 {
		t.Fatalf("second: want 401, got %d", resp2.StatusCode)
	}
}

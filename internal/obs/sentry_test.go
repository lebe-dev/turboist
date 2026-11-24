package obs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	sentry "github.com/getsentry/sentry-go"
)

// captureTransport records events in memory so tests can assert what was sent.
type captureTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *captureTransport) Configure(sentry.ClientOptions) {}
func (t *captureTransport) SendEvent(e *sentry.Event) {
	t.mu.Lock()
	t.events = append(t.events, e)
	t.mu.Unlock()
}
func (t *captureTransport) Flush(time.Duration) bool              { return true }
func (t *captureTransport) FlushWithContext(context.Context) bool { return true }
func (t *captureTransport) Close()                                {}

func (t *captureTransport) all() []*sentry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*sentry.Event(nil), t.events...)
}

func bindSentry(t *testing.T) *captureTransport {
	t.Helper()
	tr := &captureTransport{}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:       "https://test@localhost/1",
		Transport: tr,
	}); err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })
	return tr
}

func TestFailureEvent_CarriesChainAndMessage(t *testing.T) {
	cause := errors.New("database is locked")
	err := fmt.Errorf("complete task: %w", cause)
	event := FailureEvent(err, []string{"error", "route"})

	if event.Message != err.Error() {
		t.Errorf("message: got %q, want %q", event.Message, err.Error())
	}
	if len(event.Exception) != 2 {
		t.Fatalf("exception chain: got %d entries, want 2", len(event.Exception))
	}
	if event.Exception[0].Value != "database is locked" {
		t.Errorf("innermost exception: got %q", event.Exception[0].Value)
	}
	if event.Exception[1].Value != err.Error() {
		t.Errorf("outermost exception: got %q, want %q", event.Exception[1].Value, err.Error())
	}
	if len(event.Fingerprint) != 2 || event.Fingerprint[0] != "error" {
		t.Errorf("fingerprint: got %v", event.Fingerprint)
	}
	if event.Level != sentry.LevelError {
		t.Errorf("level: got %q, want error", event.Level)
	}
}

func TestFailureEvent_NilError(t *testing.T) {
	if event := FailureEvent(nil, nil); event != nil {
		t.Errorf("FailureEvent(nil): got %v, want nil", event)
	}
}

func TestCaptureError_GroupsByOperationAndType(t *testing.T) {
	tr := bindSentry(t)
	CaptureError(errors.New("cleanup failed"), map[string]string{"op": "auth.SessionCleanup"})

	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("captured events: got %d, want 1", len(events))
	}
	event := events[0]
	if event.Tags["op"] != "auth.SessionCleanup" {
		t.Errorf("op tag: got %q", event.Tags["op"])
	}
	if event.Message != "cleanup failed" {
		t.Errorf("message: got %q", event.Message)
	}
	want := []string{"background", "auth.SessionCleanup", "*errors.errorString"}
	if fmt.Sprint(event.Fingerprint) != fmt.Sprint(want) {
		t.Errorf("fingerprint: got %v, want %v", event.Fingerprint, want)
	}
}

func TestCaptureError_IgnoresNil(t *testing.T) {
	tr := bindSentry(t)
	CaptureError(nil, nil)
	if n := len(tr.all()); n != 0 {
		t.Errorf("captured events: got %d, want 0", n)
	}
}

func TestDropEmptyEvents(t *testing.T) {
	if got := dropEmptyEvents(&sentry.Event{}, nil); got != nil {
		t.Error("an event with no message and no exception should be dropped")
	}
	if got := dropEmptyEvents(&sentry.Event{Message: "boom"}, nil); got == nil {
		t.Error("an event with a message should be kept")
	}
	withException := &sentry.Event{Exception: []sentry.Exception{{Type: "boom"}}}
	if got := dropEmptyEvents(withException, nil); got == nil {
		t.Error("an event with an exception should be kept")
	}
}

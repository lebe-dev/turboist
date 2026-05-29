package events

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func recvWithin(t *testing.T, ch <-chan Event, d time.Duration) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		return ev, ok
	case <-time.After(d):
		return Event{}, false
	}
}

func TestHub_PublishDeliversToSubscriber(t *testing.T) {
	h := NewHub(slog.Default())
	ch, cancel := h.Subscribe(1)
	defer cancel()

	h.Publish(context.Background(), 1, ScopeTasks)
	ev, ok := recvWithin(t, ch, time.Second)
	if !ok {
		t.Fatal("expected event, got none")
	}
	if ev.Scope != ScopeTasks {
		t.Fatalf("scope: want %q, got %q", ScopeTasks, ev.Scope)
	}
}

func TestHub_PublishFansOutToAllSubscribersOfSameUser(t *testing.T) {
	h := NewHub(slog.Default())
	ch1, cancel1 := h.Subscribe(1)
	defer cancel1()
	ch2, cancel2 := h.Subscribe(1)
	defer cancel2()

	h.Publish(context.Background(), 1, ScopeCalendar)
	if _, ok := recvWithin(t, ch1, time.Second); !ok {
		t.Fatal("sub1: expected event")
	}
	if _, ok := recvWithin(t, ch2, time.Second); !ok {
		t.Fatal("sub2: expected event")
	}
}

func TestHub_PublishSkipsMatchingOrigin(t *testing.T) {
	h := NewHub(slog.Default())
	self, cancelSelf := h.Subscribe(1, "tab-self")
	defer cancelSelf()
	other, cancelOther := h.Subscribe(1, "tab-other")
	defer cancelOther()
	plain, cancelPlain := h.Subscribe(1) // no origin
	defer cancelPlain()

	h.Publish(context.Background(), 1, ScopeTasks, "tab-self")

	if _, ok := recvWithin(t, self, 100*time.Millisecond); ok {
		t.Fatal("origin tab-self: should not receive echo of its own mutation")
	}
	if _, ok := recvWithin(t, other, time.Second); !ok {
		t.Fatal("origin tab-other: expected event")
	}
	if _, ok := recvWithin(t, plain, time.Second); !ok {
		t.Fatal("no-origin subscriber: expected event")
	}
}

func TestHub_PublishEmptyOriginDeliversToAll(t *testing.T) {
	h := NewHub(slog.Default())
	tagged, cancel := h.Subscribe(1, "tab-self")
	defer cancel()

	// An empty publishing origin must never suppress (back-compat with clients
	// that do not send X-Client-Origin).
	h.Publish(context.Background(), 1, ScopeTasks)
	if _, ok := recvWithin(t, tagged, time.Second); !ok {
		t.Fatal("expected event when publish origin is empty")
	}
}

func TestHub_PublishDoesNotCrossUsers(t *testing.T) {
	h := NewHub(slog.Default())
	ch1, cancel1 := h.Subscribe(1)
	defer cancel1()
	ch2, cancel2 := h.Subscribe(2)
	defer cancel2()

	h.Publish(context.Background(), 1, ScopeInbox)
	if _, ok := recvWithin(t, ch1, time.Second); !ok {
		t.Fatal("user 1: expected event")
	}
	if _, ok := recvWithin(t, ch2, 100*time.Millisecond); ok {
		t.Fatal("user 2: should not have received event for user 1")
	}
}

func TestHub_CancelStopsDelivery(t *testing.T) {
	h := NewHub(slog.Default())
	ch, cancel := h.Subscribe(1)
	cancel()

	h.Publish(context.Background(), 1, ScopeTasks)
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after cancel")
	}
	if got := h.SubscriberCount(1); got != 0 {
		t.Fatalf("subscriber count after cancel: want 0, got %d", got)
	}
}

func TestHub_CancelIsIdempotent(t *testing.T) {
	h := NewHub(slog.Default())
	_, cancel := h.Subscribe(1)
	cancel()
	cancel() // should not panic
}

func TestHub_PublishNoSubscribersIsNoop(t *testing.T) {
	h := NewHub(slog.Default())
	h.Publish(context.Background(), 42, ScopeTasks) // must not panic
}

func TestHub_SlowSubscriberDoesNotBlockPublisher(t *testing.T) {
	h := NewHub(slog.Default())
	_, cancel := h.Subscribe(1) // never drain ch
	defer cancel()

	// Fill the buffer plus a few extra to force the drop path.
	done := make(chan struct{})
	go func() {
		for range subscriberBufferSize + 5 {
			h.Publish(context.Background(), 1, ScopeTasks)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}
}

func TestHub_SubscriberCount(t *testing.T) {
	h := NewHub(slog.Default())
	if got := h.SubscriberCount(1); got != 0 {
		t.Fatalf("initial: want 0, got %d", got)
	}
	_, c1 := h.Subscribe(1)
	_, c2 := h.Subscribe(1)
	if got := h.SubscriberCount(1); got != 2 {
		t.Fatalf("after 2 subscribes: want 2, got %d", got)
	}
	c1()
	if got := h.SubscriberCount(1); got != 1 {
		t.Fatalf("after 1 cancel: want 1, got %d", got)
	}
	c2()
	if got := h.SubscriberCount(1); got != 0 {
		t.Fatalf("after both cancel: want 0, got %d", got)
	}
}

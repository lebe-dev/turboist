package events

import (
	"context"
	"log/slog"
	"testing"
)

func TestHub_CloseClosesAllSubscribers(t *testing.T) {
	h := NewHub(slog.Default())
	ch1, _ := h.Subscribe(1)
	ch2, _ := h.Subscribe(2)
	h.Close()
	if _, ok := <-ch1; ok {
		t.Fatal("ch1 should be closed")
	}
	if _, ok := <-ch2; ok {
		t.Fatal("ch2 should be closed")
	}
}

func TestHub_CloseIsIdempotent(t *testing.T) {
	h := NewHub(slog.Default())
	_, _ = h.Subscribe(1)
	h.Close()
	h.Close() // must not panic
}

func TestHub_SubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	h := NewHub(slog.Default())
	h.Close()
	ch, cancel := h.Subscribe(1)
	defer cancel()
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed when subscribing after Close")
	}
}

func TestHub_CancelAfterCloseDoesNotPanic(t *testing.T) {
	h := NewHub(slog.Default())
	_, cancel := h.Subscribe(1)
	h.Close()
	cancel() // must not panic (channel already closed by Close)
}

func TestHub_PublishAfterCloseIsNoop(t *testing.T) {
	h := NewHub(slog.Default())
	h.Close()
	h.Publish(context.Background(), 1, ScopeTasks) // must not panic
}

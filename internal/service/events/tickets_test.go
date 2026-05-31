package events

import (
	"errors"
	"testing"
	"time"
)

func newTestStore(now func() time.Time) *TicketStore {
	s := NewTicketStore()
	s.now = now
	return s
}

func TestTickets_IssueAndConsume(t *testing.T) {
	s := NewTicketStore()
	tok, err := s.Issue(42)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	uid, origin, err := s.Consume(tok)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if uid != 42 {
		t.Fatalf("user id: want 42, got %d", uid)
	}
	if origin != "" {
		t.Fatalf("origin: want empty, got %q", origin)
	}
}

func TestTickets_IssueAndConsume_Origin(t *testing.T) {
	s := NewTicketStore()
	tok, err := s.Issue(42, "tab-abc")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	uid, origin, err := s.Consume(tok)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if uid != 42 {
		t.Fatalf("user id: want 42, got %d", uid)
	}
	if origin != "tab-abc" {
		t.Fatalf("origin: want %q, got %q", "tab-abc", origin)
	}
}

func TestTickets_ConsumeIsOneShot(t *testing.T) {
	s := NewTicketStore()
	tok, _ := s.Issue(1)
	if _, _, err := s.Consume(tok); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, _, err := s.Consume(tok); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("second consume: want ErrTicketInvalid, got %v", err)
	}
}

func TestTickets_ConsumeUnknownInvalid(t *testing.T) {
	s := NewTicketStore()
	if _, _, err := s.Consume("nope"); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("want ErrTicketInvalid, got %v", err)
	}
	if _, _, err := s.Consume(""); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("empty: want ErrTicketInvalid, got %v", err)
	}
}

func TestTickets_ConsumeExpired(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	s := newTestStore(func() time.Time { return now })
	tok, _ := s.Issue(7)

	now = now.Add(TicketTTL + time.Second)
	if _, _, err := s.Consume(tok); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("expired: want ErrTicketInvalid, got %v", err)
	}
}

func TestTickets_IssueSweepsExpired(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	s := newTestStore(func() time.Time { return now })
	_, _ = s.Issue(1)
	_, _ = s.Issue(2)

	now = now.Add(TicketTTL + time.Second)
	_, _ = s.Issue(3) // triggers sweep

	s.mu.Lock()
	got := len(s.items)
	s.mu.Unlock()
	if got != 1 {
		t.Fatalf("after sweep: want 1 ticket, got %d", got)
	}
}

func TestTickets_TokensAreUniqueAndLong(t *testing.T) {
	s := NewTicketStore()
	seen := make(map[string]struct{})
	for range 100 {
		tok, err := s.Issue(1)
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		if len(tok) < 32 {
			t.Fatalf("token too short: %d", len(tok))
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token: %s", tok)
		}
		seen[tok] = struct{}{}
	}
}

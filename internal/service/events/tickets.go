package events

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// TicketTTL is how long an issued SSE handshake ticket remains valid.
const TicketTTL = 30 * time.Second

// ErrTicketInvalid is returned by TicketStore.Consume when the ticket is
// unknown, already consumed, or expired.
var ErrTicketInvalid = errors.New("events: invalid or expired ticket")

type ticket struct {
	userID    int64
	origin    string
	expiresAt time.Time
}

// TicketStore issues short-lived one-shot tokens for SSE handshake. The
// EventSource API cannot send Authorization headers, so the client first
// obtains a ticket under normal JWT auth and uses it as a query parameter on
// the streaming endpoint.
type TicketStore struct {
	mu      sync.Mutex
	items   map[string]ticket
	now     func() time.Time
	ttl     time.Duration
	randHex func() (string, error)
}

// NewTicketStore returns a TicketStore using the default TTL and crypto/rand.
func NewTicketStore() *TicketStore {
	return &TicketStore{
		items:   make(map[string]ticket),
		now:     time.Now,
		ttl:     TicketTTL,
		randHex: defaultRandHex,
	}
}

// Issue creates a fresh ticket bound to userID. An optional origin identifies
// the client opening the stream; it is returned by Consume so the hub can
// suppress that client's own mutation echoes (see Hub.Publish).
func (s *TicketStore) Issue(userID int64, origin ...string) (string, error) {
	token, err := s.randHex()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	s.items[token] = ticket{userID: userID, origin: firstOrigin(origin), expiresAt: s.now().Add(s.ttl)}
	return token, nil
}

// Consume validates and removes the ticket, returning the bound user id and the
// origin supplied at Issue time (empty when none was given). Returns
// ErrTicketInvalid for unknown, expired, or already-consumed tickets.
func (s *TicketStore) Consume(token string) (int64, string, error) {
	if token == "" {
		return 0, "", ErrTicketInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.items[token]
	if !ok {
		return 0, "", ErrTicketInvalid
	}
	delete(s.items, token)
	if s.now().After(t.expiresAt) {
		return 0, "", ErrTicketInvalid
	}
	return t.userID, t.origin, nil
}

func (s *TicketStore) sweepLocked() {
	now := s.now()
	for k, v := range s.items {
		if now.After(v.expiresAt) {
			delete(s.items, k)
		}
	}
}

func defaultRandHex() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

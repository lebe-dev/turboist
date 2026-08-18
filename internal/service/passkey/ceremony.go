// Package passkey implements WebAuthn (passkey) registration and login on top
// of github.com/go-webauthn/webauthn.
//
// A passkey is an ADDITIONAL login method: the password stays in place as the
// recovery path, and a successful passkey assertion satisfies both factors at
// once (the authenticator performs user verification itself), so it skips the
// TOTP step the password flow may require.
package passkey

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// ceremonyTTL bounds how long a begin→finish pair may stay open. WebAuthn
// prompts are interactive (biometrics, a security key tap), so this is
// deliberately generous, while still expiring abandoned challenges.
const ceremonyTTL = 5 * time.Minute

// ceremonyStore keeps the per-ceremony SessionData server-side, addressed by an
// opaque id handed to the client.
//
// The challenge must never round-trip through the client unprotected, and it is
// single-use: Take() removes it, so a replayed finish request finds nothing.
// Keeping it in memory (rather than in SQLite) is deliberate — Turboist is a
// single-user, single-process instance, the data is worthless after five
// minutes, and a restart mid-ceremony just means the user taps the button
// again.
type ceremonyStore struct {
	mu    sync.Mutex
	items map[string]ceremonyEntry
}

type ceremonyEntry struct {
	session   webauthn.SessionData
	userID    int64
	expiresAt time.Time
}

func newCeremonyStore() *ceremonyStore {
	return &ceremonyStore{items: make(map[string]ceremonyEntry)}
}

// Put stores session data and returns its opaque id. userID is 0 for a
// discoverable login, where the account is only known once the authenticator
// answers.
func (s *ceremonyStore) Put(session *webauthn.SessionData, userID int64, now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	s.items[id] = ceremonyEntry{session: *session, userID: userID, expiresAt: now.Add(ceremonyTTL)}
	return id, nil
}

// Take consumes the entry for id. The second result is false when the id is
// unknown or expired.
func (s *ceremonyStore) Take(id string, now time.Time) (ceremonyEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	entry, ok := s.items[id]
	if !ok {
		return ceremonyEntry{}, false
	}
	delete(s.items, id)
	if now.After(entry.expiresAt) {
		return ceremonyEntry{}, false
	}
	return entry, true
}

// pruneLocked drops expired entries. Called on every access, which is enough
// for a store that sees a handful of entries a day — no janitor goroutine.
func (s *ceremonyStore) pruneLocked(now time.Time) {
	for id, entry := range s.items {
		if now.After(entry.expiresAt) {
			delete(s.items, id)
		}
	}
}

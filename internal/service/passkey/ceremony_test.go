package passkey

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestCeremonyStore_TakeIsSingleUse(t *testing.T) {
	store := newCeremonyStore()
	now := time.Now()

	id, err := store.Put(&webauthn.SessionData{Challenge: "abc"}, 7, now)
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	entry, ok := store.Take(id, now)
	if !ok {
		t.Fatal("first take: got miss, want hit")
	}
	if entry.userID != 7 || entry.session.Challenge != "abc" {
		t.Errorf("entry: got userID=%d challenge=%q, want 7/abc", entry.userID, entry.session.Challenge)
	}

	// A replayed finish request must not find the challenge again.
	if _, ok := store.Take(id, now); ok {
		t.Error("second take: got hit, want miss")
	}
}

func TestCeremonyStore_TakeExpired(t *testing.T) {
	store := newCeremonyStore()
	now := time.Now()

	id, err := store.Put(&webauthn.SessionData{Challenge: "abc"}, 0, now)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, ok := store.Take(id, now.Add(ceremonyTTL+time.Second)); ok {
		t.Error("expired take: got hit, want miss")
	}
}

func TestCeremonyStore_TakeUnknown(t *testing.T) {
	store := newCeremonyStore()
	if _, ok := store.Take("nope", time.Now()); ok {
		t.Error("unknown id: got hit, want miss")
	}
}

func TestCeremonyStore_PutPrunesExpiredEntries(t *testing.T) {
	store := newCeremonyStore()
	now := time.Now()

	if _, err := store.Put(&webauthn.SessionData{Challenge: "old"}, 0, now); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := store.Put(&webauthn.SessionData{Challenge: "new"}, 0, now.Add(ceremonyTTL+time.Second)); err != nil {
		t.Fatalf("put: %v", err)
	}
	if got := len(store.items); got != 1 {
		t.Errorf("stored entries: got %d, want 1 (the expired one should be pruned)", got)
	}
}

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"blank falls back", "   ", defaultCredentialName},
		{"trimmed", "  iPhone  ", "iPhone"},
		{"kept", "YubiKey 5C", "YubiKey 5C"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeName(tc.in); got != tc.want {
				t.Errorf("normalizeName(%q): got %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeName_Truncated(t *testing.T) {
	long := make([]rune, maxNameLength+50)
	for i := range long {
		long[i] = 'ä'
	}
	got := normalizeName(string(long))
	if len([]rune(got)) != maxNameLength {
		t.Errorf("length: got %d runes, want %d", len([]rune(got)), maxNameLength)
	}
}

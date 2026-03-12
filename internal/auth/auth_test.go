package auth

import (
	"testing"
)

func TestSessionStore(t *testing.T) {
	store := NewSessionStore()

	token, err := store.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if len(token) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(token))
	}

	if !store.ValidateSession(token) {
		t.Fatal("expected token to be valid after create")
	}

	store.DeleteSession(token)

	if store.ValidateSession(token) {
		t.Fatal("expected token to be invalid after delete")
	}
}

func TestSessionStore_UniqueTokens(t *testing.T) {
	store := NewSessionStore()

	t1, _ := store.CreateSession()
	t2, _ := store.CreateSession()

	if t1 == t2 {
		t.Fatal("expected unique tokens")
	}
}

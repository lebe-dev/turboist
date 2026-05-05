package auth

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestGenerateRefreshToken_FormatAndUniqueness(t *testing.T) {
	tok1, hash1, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("gen 1: %v", err)
	}
	tok2, hash2, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("gen 2: %v", err)
	}
	if tok1 == tok2 {
		t.Errorf("tokens must differ")
	}
	if hash1 == hash2 {
		t.Errorf("hashes must differ")
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok1)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if len(raw) != RefreshTokenBytes {
		t.Errorf("token bytes: got %d, want %d", len(raw), RefreshTokenBytes)
	}
}

func TestHashRefreshToken_Deterministic(t *testing.T) {
	h1 := HashRefreshToken("hello")
	h2 := HashRefreshToken("hello")
	if h1 != h2 {
		t.Errorf("hash must be deterministic")
	}
	if HashRefreshToken("hello") == HashRefreshToken("Hello") {
		t.Errorf("hash must be case-sensitive")
	}
}

func TestRefreshExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	exp := RefreshExpiry(now)
	want := now.Add(30 * 24 * time.Hour)
	if !exp.Equal(want) {
		t.Errorf("expiry: got %v, want %v", exp, want)
	}
}

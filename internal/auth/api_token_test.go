package auth

import "testing"

func TestGenerateAPIToken_LengthAndUniqueness(t *testing.T) {
	a, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if a == "" || b == "" {
		t.Fatalf("got empty token")
	}
	if a == b {
		t.Fatalf("two generated tokens collided: %q", a)
	}
	// 32 random bytes -> base64.RawURLEncoding length is 43.
	if len(a) != 43 {
		t.Errorf("encoded length: got %d, want 43", len(a))
	}
}

func TestHashAPIToken_DeterministicSameSalt(t *testing.T) {
	salt := []byte("salt-salt-salt-salt-salt-salt-salt-salt-")
	got1 := HashAPIToken("abc", salt)
	got2 := HashAPIToken("abc", salt)
	if got1 != got2 {
		t.Fatalf("HMAC must be deterministic: %q vs %q", got1, got2)
	}
	// HMAC-SHA256 hex => 64 chars.
	if len(got1) != 64 {
		t.Errorf("hash length: got %d, want 64", len(got1))
	}
}

func TestHashAPIToken_DiffersWithSalt(t *testing.T) {
	a := HashAPIToken("abc", []byte("salt-a-salt-a-salt-a-salt-a-salt-a-salt-a"))
	b := HashAPIToken("abc", []byte("salt-b-salt-b-salt-b-salt-b-salt-b-salt-b"))
	if a == b {
		t.Fatalf("hash must depend on salt")
	}
}

func TestHashAPIToken_DiffersWithToken(t *testing.T) {
	salt := []byte("salt-salt-salt-salt-salt-salt-salt-salt-")
	if HashAPIToken("a", salt) == HashAPIToken("b", salt) {
		t.Fatalf("hash must depend on token")
	}
}

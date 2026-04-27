package auth

import (
	"strings"
	"testing"
)

func TestHashPassword_PHCFormat(t *testing.T) {
	h, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Errorf("unexpected PHC prefix: %q", h)
	}
	parts := strings.Split(h, "$")
	if len(parts) != 6 {
		t.Errorf("parts: got %d, want 6", len(parts))
	}
}

func TestVerifyPassword_RoundTrip(t *testing.T) {
	h, err := HashPassword("p@ssw0rd!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := VerifyPassword("p@ssw0rd!", h); err != nil {
		t.Errorf("verify correct: %v", err)
	}
	if err := VerifyPassword("wrong", h); err == nil {
		t.Errorf("verify wrong: expected error")
	}
}

func TestVerifyPassword_DifferentSalts(t *testing.T) {
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	if h1 == h2 {
		t.Errorf("hashes must differ due to random salt")
	}
}

func TestVerifyPassword_InvalidFormat(t *testing.T) {
	if err := VerifyPassword("p", "not-a-hash"); err == nil {
		t.Errorf("expected error on invalid format")
	}
	if err := VerifyPassword("p", "$bcrypt$v=1$m=1,t=1,p=1$YQ$Yg"); err == nil {
		t.Errorf("expected error on unsupported algorithm")
	}
}

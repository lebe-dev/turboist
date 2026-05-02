package auth

import (
	"fmt"
	"strings"
	"testing"
)

func TestHashPassword_PHCFormat(t *testing.T) {
	h, err := HashPassword("secret123", DefaultArgon2Params())
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	p := DefaultArgon2Params()
	wantPrefix := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$", p.Memory, p.Time, p.Threads)
	if !strings.HasPrefix(h, wantPrefix) {
		t.Errorf("PHC prefix: got %q, want prefix %q", h, wantPrefix)
	}
	parts := strings.Split(h, "$")
	if len(parts) != 6 {
		t.Errorf("parts: got %d, want 6", len(parts))
	}
}

func TestHashPassword_CustomParamsEncoded(t *testing.T) {
	custom := Argon2Params{Memory: 8 * 1024, Time: 1, Threads: 2}
	h, err := HashPassword("secret123", custom)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=8192,t=1,p=2$") {
		t.Errorf("custom params not encoded: %q", h)
	}
}

func TestVerifyPassword_RoundTrip(t *testing.T) {
	h, err := HashPassword("p@ssw0rd!", DefaultArgon2Params())
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

func TestVerifyPassword_AcceptsLegacyHashParams(t *testing.T) {
	// Hashes produced with stronger legacy parameters must still verify so existing
	// users aren't locked out after we lower the default cost.
	legacy := Argon2Params{Memory: 64 * 1024, Time: 3, Threads: 4}
	h, err := HashPassword("legacy-pwd", legacy)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := VerifyPassword("legacy-pwd", h); err != nil {
		t.Errorf("verify legacy hash: %v", err)
	}
}

func TestVerifyPassword_DifferentSalts(t *testing.T) {
	h1, _ := HashPassword("same", DefaultArgon2Params())
	h2, _ := HashPassword("same", DefaultArgon2Params())
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

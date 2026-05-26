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

func TestHashToken_DeterministicAndHexEncoded(t *testing.T) {
	got := HashToken("tok-abc")
	again := HashToken("tok-abc")
	if got != again {
		t.Errorf("HashToken must be deterministic: %q vs %q", got, again)
	}
	if len(got) != 64 {
		t.Errorf("hex sha256 length: got %d, want 64", len(got))
	}
	for _, c := range got {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			t.Errorf("non-hex char %q in %q", c, got)
			break
		}
	}
}

func TestHashToken_DifferentInputsDifferentHashes(t *testing.T) {
	cases := []string{"", "a", "b", "tok-abc", "tok-abcd", strings.Repeat("x", 1024)}
	seen := make(map[string]string, len(cases))
	for _, in := range cases {
		h := HashToken(in)
		if prev, ok := seen[h]; ok {
			t.Errorf("collision: HashToken(%q) == HashToken(%q)", in, prev)
		}
		seen[h] = in
	}
}

func TestVerifyToken_Match(t *testing.T) {
	tok := "01HXYZAPIKEY00000000000000"
	if !VerifyToken(tok, HashToken(tok)) {
		t.Errorf("matching token must verify")
	}
}

func TestVerifyToken_Mismatch(t *testing.T) {
	if VerifyToken("a", HashToken("b")) {
		t.Errorf("non-matching token must not verify")
	}
}

func TestVerifyToken_EmptyToken(t *testing.T) {
	if !VerifyToken("", HashToken("")) {
		t.Errorf("empty token must verify against hash of empty")
	}
	if VerifyToken("", HashToken("non-empty")) {
		t.Errorf("empty token must not verify against hash of non-empty")
	}
}

func TestVerifyToken_LengthMismatchHash(t *testing.T) {
	if VerifyToken("tok", "deadbeef") {
		t.Errorf("hash of wrong length must not verify")
	}
}

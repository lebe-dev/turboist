package crypto

import "testing"

func TestTokenCipherRoundTrip(t *testing.T) {
	c := NewTokenCipher("01234567890123456789012345678901")
	encrypted, err := c.Encrypt("secret-token")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if encrypted == "secret-token" {
		t.Fatal("token was not encrypted")
	}
	decrypted, err := c.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != "secret-token" {
		t.Fatalf("decrypted = %q; want secret-token", decrypted)
	}
}

func TestTokenCipherAllowsLegacyPlaintext(t *testing.T) {
	c := NewTokenCipher("01234567890123456789012345678901")
	decrypted, err := c.Decrypt("legacy-token")
	if err != nil {
		t.Fatalf("Decrypt legacy token: %v", err)
	}
	if decrypted != "legacy-token" {
		t.Fatalf("decrypted = %q; want legacy-token", decrypted)
	}
}

func TestTokenCipherEmpty(t *testing.T) {
	c := NewTokenCipher("key")
	enc, err := c.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	if enc != "" {
		t.Fatalf("encrypted empty = %q; want empty", enc)
	}
	dec, err := c.Decrypt("")
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if dec != "" {
		t.Fatalf("decrypted empty = %q; want empty", dec)
	}
}

func TestIsEncrypted(t *testing.T) {
	if !IsEncrypted("") {
		t.Error("empty should be considered encrypted")
	}
	if !IsEncrypted(EncryptedTokenPrefix + "something") {
		t.Error("prefixed value should be considered encrypted")
	}
	if IsEncrypted("plain-token") {
		t.Error("plain token should not be considered encrypted")
	}
}

func TestTokenCipherWireFormatStable(t *testing.T) {
	if EncryptedTokenPrefix != "enc:v1:" {
		t.Fatalf("wire prefix changed: got %q, want enc:v1:", EncryptedTokenPrefix)
	}
}

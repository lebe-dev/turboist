package crypto

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

const testFedKey = "federation-cipher-key-32-bytes-min-padding!"

func TestGenerateInstanceKeypair_RoundTrips(t *testing.T) {
	cipher := NewTokenCipher(testFedKey)

	kp, err := GenerateInstanceKeypair(cipher)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// PublicKey is base64-std and decodes to a valid Ed25519 public key.
	pubBytes, err := base64.StdEncoding.DecodeString(kp.PublicKey)
	if err != nil {
		t.Fatalf("decode public key: %v", err)
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		t.Fatalf("public key size: got %d, want %d", len(pubBytes), ed25519.PublicKeySize)
	}

	// The stored seed is encrypted at rest (TokenCipher prefix).
	if !IsEncrypted(kp.PrivateSeedEnc) || kp.PrivateSeedEnc == "" {
		t.Fatalf("private seed not encrypted at rest: %q", kp.PrivateSeedEnc)
	}

	// LoadInstanceKeypair decrypts the seed and yields a private key whose
	// signature verifies under the stored public key.
	priv, pub, err := LoadInstanceKeypair(cipher, kp.PublicKey, kp.PrivateSeedEnc)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	msg := []byte("federation canonical string")
	sig := ed25519.Sign(priv, msg)
	if !ed25519.Verify(pub, msg, sig) {
		t.Errorf("loaded keypair signature did not verify")
	}
	// And it verifies under the publicly-published key too (decoded above).
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), msg, sig) {
		t.Errorf("published public key did not verify loaded private key signature")
	}
}

func TestLoadInstanceKeypair_WrongCipherFails(t *testing.T) {
	kp, err := GenerateInstanceKeypair(NewTokenCipher(testFedKey))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// A different key must not be able to decrypt the seed.
	if _, _, err := LoadInstanceKeypair(NewTokenCipher("a-totally-different-key-32-bytes-padding!"), kp.PublicKey, kp.PrivateSeedEnc); err == nil {
		t.Errorf("expected decryption failure with wrong cipher key")
	}
}

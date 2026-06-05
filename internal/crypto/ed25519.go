package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// InstanceKeypair is the at-rest representation of this instance's federation
// trust-plane Ed25519 keypair (Federation v1 F0.3). The public key is stored
// and published verbatim (base64-std); the private seed is encrypted with the
// shared TokenCipher (FEDERATION_KEY env) so the on-disk DB never holds the raw
// signing material — mirroring how totp_secret is stored (019).
//
// This is a SEPARATE trust plane from the HS256 JWT issuer and the HMAC API
// token hashing; it exists solely to sign/verify peer-to-peer federation
// requests.
type InstanceKeypair struct {
	// PublicKey is the base64-std encoded 32-byte Ed25519 public key.
	PublicKey string
	// PrivateSeedEnc is the TokenCipher-encrypted base64-std encoded 32-byte
	// Ed25519 seed (ed25519.PrivateKey.Seed()).
	PrivateSeedEnc string
}

// GenerateInstanceKeypair creates a fresh Ed25519 keypair, encrypting the seed
// with cipher for at-rest storage. The seed (not the full 64-byte expanded
// private key) is persisted so the private key can be deterministically
// reconstructed via ed25519.NewKeyFromSeed.
func GenerateInstanceKeypair(cipher *TokenCipher) (*InstanceKeypair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 keypair: %w", err)
	}
	seed := priv.Seed()
	seedB64 := base64.StdEncoding.EncodeToString(seed)
	enc, err := cipher.Encrypt(seedB64)
	if err != nil {
		return nil, fmt.Errorf("encrypt ed25519 seed: %w", err)
	}
	return &InstanceKeypair{
		PublicKey:      base64.StdEncoding.EncodeToString(pub),
		PrivateSeedEnc: enc,
	}, nil
}

// LoadInstanceKeypair decrypts the stored seed and reconstructs the Ed25519
// private and public keys. The returned public key is derived from the seed and
// must equal the decoded publicKeyB64 — a mismatch indicates tampered storage.
func LoadInstanceKeypair(cipher *TokenCipher, publicKeyB64, privateSeedEnc string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	seedB64, err := cipher.Decrypt(privateSeedEnc)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt ed25519 seed: %w", err)
	}
	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil {
		return nil, nil, fmt.Errorf("decode ed25519 seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, nil, fmt.Errorf("ed25519 seed size: got %d, want %d", len(seed), ed25519.SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("ed25519 derive public key")
	}
	stored, err := DecodePublicKey(publicKeyB64)
	if err != nil {
		return nil, nil, err
	}
	if !pub.Equal(stored) {
		return nil, nil, fmt.Errorf("ed25519 stored public key does not match seed")
	}
	return priv, pub, nil
}

// DecodePublicKey parses a base64-std encoded Ed25519 public key. Used when
// verifying a peer's signature against its published .well-known key.
func DecodePublicKey(publicKeyB64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode ed25519 public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("ed25519 public key size: got %d, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

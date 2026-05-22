package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// EncryptedTokenPrefix is the prefix used to identify encrypted token values.
const EncryptedTokenPrefix = "enc:v1:"

// TokenCipher encrypts and decrypts secrets using AES-256-GCM.
type TokenCipher struct {
	key [32]byte
}

// NewTokenCipher creates a TokenCipher from the given key material.
// The key is derived via SHA-256 so any non-empty string is accepted.
func NewTokenCipher(keyMaterial string) *TokenCipher {
	return &TokenCipher{key: sha256.Sum256([]byte(keyMaterial))}
}

// Encrypt encrypts plain using AES-256-GCM and returns the ciphertext
// prefixed with EncryptedTokenPrefix. Returns empty string for empty input.
func (c *TokenCipher) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", fmt.Errorf("create token cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create token gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("create token nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return EncryptedTokenPrefix + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a value previously returned by Encrypt.
// Plain-text values (no prefix) are returned as-is for backward compatibility.
// Returns empty string for empty input.
func (c *TokenCipher) Decrypt(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, EncryptedTokenPrefix) {
		return stored, nil
	}
	raw := strings.TrimPrefix(stored, EncryptedTokenPrefix)
	ciphertext, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("decode token ciphertext: %w", err)
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", fmt.Errorf("create token cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create token gcm: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("token ciphertext is too short")
	}
	nonce, sealed := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt token: %w", err)
	}
	return string(plain), nil
}

// IsEncrypted reports whether value is either empty or already encrypted.
func IsEncrypted(value string) bool {
	return value == "" || strings.HasPrefix(value, EncryptedTokenPrefix)
}

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	RefreshTokenBytes = 32
	RefreshTokenTTL   = 30 * 24 * time.Hour
)

// GenerateRefreshToken returns (token, sha256hex(token)) where token is base64url(32 random bytes).
func GenerateRefreshToken() (string, string, error) {
	buf := make([]byte, RefreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("read random: %w", err)
	}
	tok := base64.RawURLEncoding.EncodeToString(buf)
	return tok, HashRefreshToken(tok), nil
}

// HashRefreshToken returns hex-encoded sha256 of the token.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RefreshExpiry returns now + RefreshTokenTTL.
func RefreshExpiry(now time.Time) time.Time {
	return now.Add(RefreshTokenTTL)
}

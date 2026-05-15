package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const APITokenBytes = 32

// GenerateAPIToken returns a base64url-encoded 32-byte random token.
func GenerateAPIToken() (string, error) {
	buf := make([]byte, APITokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashAPIToken returns hex-encoded HMAC-SHA256 of the token using the server salt.
func HashAPIToken(token string, salt []byte) string {
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

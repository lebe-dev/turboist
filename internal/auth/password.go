package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MB in KiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

var (
	ErrInvalidHash         = errors.New("auth: invalid argon2 hash format")
	ErrUnsupportedHashAlgo = errors.New("auth: unsupported hash algorithm")
)

// HashPassword returns a PHC-formatted argon2id hash.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(hash)), nil
}

// VerifyPassword returns nil if password matches the PHC-formatted hash.
func VerifyPassword(password, encoded string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return ErrInvalidHash
	}
	if parts[1] != "argon2id" {
		return ErrUnsupportedHashAlgo
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return ErrInvalidHash
	}
	if version != argon2.Version {
		return ErrUnsupportedHashAlgo
	}
	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return ErrInvalidHash
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return ErrInvalidHash
	}
	expected, err := b64.DecodeString(parts[5])
	if err != nil {
		return ErrInvalidHash
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expected)))
	if subtle.ConstantTimeCompare(expected, got) != 1 {
		return errors.New("auth: password mismatch")
	}
	return nil
}

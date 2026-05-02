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
	argonKeyLen  = 32
	argonSaltLen = 16
)

// Argon2Params controls the cost of password hashing.
//
// VerifyPassword reads parameters from the encoded hash itself, so changing
// these values affects only newly hashed passwords; existing hashes keep
// verifying against the parameters they were created with.
type Argon2Params struct {
	// Memory in KiB.
	Memory uint32
	// Number of iterations.
	Time uint32
	// Parallelism (number of lanes).
	Threads uint8
}

// DefaultArgon2Params returns the OWASP recommended minimum profile for
// argon2id (m=19 MiB, t=2, p=1). See:
// https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html#argon2id
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		Memory:  19 * 1024,
		Time:    2,
		Threads: 1,
	}
}

var (
	ErrInvalidHash         = errors.New("auth: invalid argon2 hash format")
	ErrUnsupportedHashAlgo = errors.New("auth: unsupported hash algorithm")
)

// HashPassword returns a PHC-formatted argon2id hash using the given parameters.
func HashPassword(password string, p Argon2Params) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Time, p.Threads,
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

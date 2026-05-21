// Package totp implements RFC 6238 TOTP two-factor authentication.
//
// Secrets are encrypted at rest with the shared TokenCipher; recovery codes are
// stored as Argon2id hashes and consumed exactly once.
package totp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/repo"
)

// Issuer is the label shown in authenticator apps next to the account.
const Issuer = "Turboist"

// RecoveryCodeCount is the number of single-use recovery codes generated per
// confirmed enrollment.
const RecoveryCodeCount = 8

// recoveryCodeLength is the number of base32 characters per recovery code.
const recoveryCodeLength = 10

// qrSize is the side length in pixels of the generated PNG QR code.
const qrSize = 256

// Errors returned by the service.
var (
	ErrInvalidCode     = errors.New("totp: invalid code")
	ErrAlreadyEnabled  = errors.New("totp: already enabled")
	ErrNotEnabled      = errors.New("totp: not enabled")
	ErrNoPendingSetup  = errors.New("totp: no pending setup")
	ErrFeatureDisabled = errors.New("totp: feature disabled")
)

// Service wires the cipher and repositories together. It is safe for concurrent
// use; all state lives in the database.
type Service struct {
	cipher       *crypto.TokenCipher
	users        *repo.UserRepo
	recovery     *repo.TOTPRecoveryRepo
	argon2Params auth.Argon2Params

	now func() time.Time
}

// NewService constructs a TOTP Service. argon2Params are used for hashing
// recovery codes; passing zero-value params disables Argon2id (only useful in
// tests).
func NewService(
	cipher *crypto.TokenCipher,
	users *repo.UserRepo,
	recovery *repo.TOTPRecoveryRepo,
	argon2Params auth.Argon2Params,
) *Service {
	return &Service{
		cipher:       cipher,
		users:        users,
		recovery:     recovery,
		argon2Params: argon2Params,
		now:          time.Now,
	}
}

// SetNowFunc overrides the clock used for code validation (tests only).
func (s *Service) SetNowFunc(fn func() time.Time) { s.now = fn }

// SetupInfo carries the data returned by BeginSetup.
type SetupInfo struct {
	// Secret is the base32-encoded TOTP secret (no padding) — displayed to the
	// user as a fallback when QR scanning is unavailable.
	Secret string
	// OtpauthURL is the full otpauth:// URI suitable for QR encoding.
	OtpauthURL string
	// QRPNG holds the PNG-encoded QR code bytes.
	QRPNG []byte
}

// BeginSetup generates a fresh TOTP secret for the user, encrypts and persists
// it (without enabling 2FA), and returns the otpauth:// URL plus a QR PNG.
//
// Calling BeginSetup again before ConfirmSetup overwrites the pending secret.
// Calling it when 2FA is already enabled returns ErrAlreadyEnabled.
func (s *Service) BeginSetup(ctx context.Context, userID int64, username string) (*SetupInfo, error) {
	state, err := s.users.GetTOTPState(ctx, userID)
	if err != nil {
		return nil, err
	}
	if state.Enabled {
		return nil, ErrAlreadyEnabled
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      Issuer,
		AccountName: username,
	})
	if err != nil {
		return nil, fmt.Errorf("generate totp: %w", err)
	}
	encrypted, err := s.cipher.Encrypt(key.Secret())
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}
	if err := s.users.SetTOTPSecret(ctx, userID, encrypted); err != nil {
		return nil, err
	}
	img, err := key.Image(qrSize, qrSize)
	if err != nil {
		return nil, fmt.Errorf("render qr: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return &SetupInfo{
		Secret:     key.Secret(),
		OtpauthURL: key.URL(),
		QRPNG:      buf.Bytes(),
	}, nil
}

// ConfirmSetup validates the user-supplied TOTP code against the pending
// secret, enables 2FA on success, regenerates the set of recovery codes, and
// returns the plaintext recovery codes (the only time they are visible).
func (s *Service) ConfirmSetup(ctx context.Context, userID int64, code string) ([]string, error) {
	state, err := s.users.GetTOTPState(ctx, userID)
	if err != nil {
		return nil, err
	}
	if state.Enabled {
		return nil, ErrAlreadyEnabled
	}
	if state.Secret == "" {
		return nil, ErrNoPendingSetup
	}
	secret, err := s.cipher.Decrypt(state.Secret)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	if !totp.Validate(code, secret) {
		// Note: pquerna/otp's plain Validate uses skew=1; we replicate it via
		// ValidateCustom to inject the test clock when needed.
		if !s.validateAt(code, secret, s.now()) {
			return nil, ErrInvalidCode
		}
	}
	codes, hashes, err := s.generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := s.recovery.Replace(ctx, userID, hashes); err != nil {
		return nil, err
	}
	if err := s.users.EnableTOTP(ctx, userID); err != nil {
		return nil, err
	}
	return codes, nil
}

// Verify checks code against the user's enabled TOTP secret. Returns
// ErrInvalidCode when the code does not match and ErrNotEnabled when 2FA is
// not active.
func (s *Service) Verify(ctx context.Context, userID int64, code string) error {
	state, err := s.users.GetTOTPState(ctx, userID)
	if err != nil {
		return err
	}
	if !state.Enabled {
		return ErrNotEnabled
	}
	secret, err := s.cipher.Decrypt(state.Secret)
	if err != nil {
		return fmt.Errorf("decrypt secret: %w", err)
	}
	if !s.validateAt(code, secret, s.now()) {
		return ErrInvalidCode
	}
	return nil
}

// ConsumeRecoveryCode finds an unused recovery code matching the plaintext
// input and marks it used. Returns ErrInvalidCode when no match is found.
func (s *Service) ConsumeRecoveryCode(ctx context.Context, userID int64, code string) error {
	code = normalizeRecoveryCode(code)
	if code == "" {
		return ErrInvalidCode
	}
	state, err := s.users.GetTOTPState(ctx, userID)
	if err != nil {
		return err
	}
	if !state.Enabled {
		return ErrNotEnabled
	}
	codes, err := s.recovery.ListUnused(ctx, userID)
	if err != nil {
		return err
	}
	for _, rc := range codes {
		if err := auth.VerifyPassword(code, rc.CodeHash); err == nil {
			if err := s.recovery.MarkUsed(ctx, rc.ID); err != nil {
				if errors.Is(err, repo.ErrNotFound) {
					return ErrInvalidCode
				}
				return err
			}
			return nil
		}
	}
	return ErrInvalidCode
}

// Disable turns off 2FA after verifying the supplied code (either a TOTP code
// or an unused recovery code).
func (s *Service) Disable(ctx context.Context, userID int64, code string) error {
	state, err := s.users.GetTOTPState(ctx, userID)
	if err != nil {
		return err
	}
	if !state.Enabled {
		return ErrNotEnabled
	}
	verr := s.Verify(ctx, userID, code)
	if verr != nil {
		if !errors.Is(verr, ErrInvalidCode) {
			return verr
		}
		// Fall back to recovery code.
		if rerr := s.ConsumeRecoveryCode(ctx, userID, code); rerr != nil {
			return rerr
		}
	}
	if err := s.recovery.DeleteAll(ctx, userID); err != nil {
		return err
	}
	return s.users.DisableTOTP(ctx, userID)
}

// validateAt validates a code at a specific time with skew=1.
func (s *Service) validateAt(code, secret string, t time.Time) bool {
	ok, err := totp.ValidateCustom(code, secret, t, totp.ValidateOpts{
		Period: 30,
		Skew:   1,
		Digits: 6,
	})
	if err != nil {
		return false
	}
	return ok
}

// generateRecoveryCodes returns a fresh batch of plaintext codes and their
// Argon2id hashes.
func (s *Service) generateRecoveryCodes() (plaintexts []string, hashes []string, err error) {
	plaintexts = make([]string, RecoveryCodeCount)
	hashes = make([]string, RecoveryCodeCount)
	for i := range RecoveryCodeCount {
		code, gerr := randomRecoveryCode()
		if gerr != nil {
			return nil, nil, gerr
		}
		hash, herr := auth.HashPassword(code, s.argon2Params)
		if herr != nil {
			return nil, nil, fmt.Errorf("hash recovery code: %w", herr)
		}
		plaintexts[i] = code
		hashes[i] = hash
	}
	return plaintexts, hashes, nil
}

// randomRecoveryCode returns a base32 (Crockford-style, no padding) string of
// length recoveryCodeLength. Uses 7 random bytes (~56 bits) which encode to
// ≥11 base32 chars; truncated to recoveryCodeLength for readability.
func randomRecoveryCode() (string, error) {
	const need = recoveryCodeLength
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	// 7 bytes -> 12 chars, plenty to slice to need.
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	s := enc.EncodeToString(buf)
	if len(s) < need {
		return "", fmt.Errorf("encoded length %d < %d", len(s), need)
	}
	return s[:need], nil
}

// normalizeRecoveryCode strips whitespace/hyphens and uppercases so that codes
// can be entered with spaces or dashes for readability.
func normalizeRecoveryCode(code string) string {
	var b strings.Builder
	b.Grow(len(code))
	for _, r := range code {
		switch {
		case r == ' ' || r == '-' || r == '\t':
			continue
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - ('a' - 'A'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

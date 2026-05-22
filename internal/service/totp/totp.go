// Package totp implements RFC 6238 TOTP two-factor authentication.
//
// Secrets are encrypted at rest with the shared TokenCipher; recovery codes are
// stored as Argon2id hashes and consumed exactly once.
package totp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"fmt"
	"image/png"
	"log/slog"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/repo"
)

// totpPeriodSeconds is the RFC 6238 step interval used to derive 6-digit codes.
const totpPeriodSeconds = 30

// totpSkew is the number of adjacent steps (before and after the current one)
// also accepted, to tolerate clock drift between the server and authenticator.
const totpSkew = 1

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
		// The CAS in SetTOTPSecret refuses to overwrite once totp_enabled = 1.
		// Reaching here despite the pre-check above means another request
		// completed ConfirmSetup in the meantime — surface that to the caller
		// instead of silently dropping the live secret.
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrAlreadyEnabled
		}
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
	step, ok := s.matchedStep(code, secret, s.now())
	if !ok {
		return nil, ErrInvalidCode
	}
	// Burn the matched step before issuing recovery codes. The atomic CAS in
	// AdvanceTOTPLastUsedStep also serializes concurrent ConfirmSetup calls so
	// the recovery codes returned to the winning caller are the ones persisted.
	advanced, err := s.users.AdvanceTOTPLastUsedStep(ctx, userID, step)
	if err != nil {
		return nil, err
	}
	if !advanced {
		return nil, ErrInvalidCode
	}
	codes, hashes, err := s.generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	// Enable and recovery-code replacement run in one transaction so the user
	// never ends up totp_enabled=1 without a fresh set of recovery codes — a
	// partial commit would lock them out because ConfirmSetup rejects with
	// ErrAlreadyEnabled on retry. The CAS on state.Secret inside the tx
	// preserves the original guard against a concurrent BeginSetup, and the
	// CAS on totp_enabled=0 serializes concurrent ConfirmSetup calls so two
	// requests using codes from adjacent skew steps can't both succeed and
	// race to overwrite each other's recovery-code set.
	if err := s.users.EnableTOTPWithRecoveryCodes(ctx, userID, state.Secret, hashes); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			// CAS lost: either totp_secret changed (concurrent BeginSetup) or
			// totp_enabled was already 1 (concurrent ConfirmSetup). Re-read to
			// surface the precise reason so the caller can show the right hint.
			if cur, gerr := s.users.GetTOTPState(ctx, userID); gerr == nil && cur.Enabled {
				return nil, ErrAlreadyEnabled
			}
			return nil, ErrNoPendingSetup
		}
		return nil, err
	}
	return codes, nil
}

// Verify checks code against the user's enabled TOTP secret. Returns
// ErrInvalidCode when the code does not match, when it has already been used
// (replay protection within the validity window), and ErrNotEnabled when 2FA
// is not active.
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
		// Treat decryption failure as an invalid code so callers naturally fall
		// back to the recovery-code path — the user must not be locked out of
		// their account when the stored secret becomes unreadable (e.g. a bad
		// TOTP_SECRET_KEY rotation). Log so operators can detect the condition.
		slog.ErrorContext(ctx, "totp: decrypt stored secret failed",
			slog.Int64("user_id", userID),
			slog.String("err", err.Error()),
		)
		return ErrInvalidCode
	}
	step, ok := s.matchedStep(code, secret, s.now())
	if !ok {
		return ErrInvalidCode
	}
	if step <= state.LastUsedStep {
		return ErrInvalidCode
	}
	advanced, err := s.users.AdvanceTOTPLastUsedStep(ctx, userID, step)
	if err != nil {
		return err
	}
	if !advanced {
		// Concurrent verifier already consumed this (or a newer) step.
		return ErrInvalidCode
	}
	return nil
}

// ConsumeRecoveryCode finds an unused recovery code matching the plaintext
// input and marks it used. Returns ErrInvalidCode when no match is found.
func (s *Service) ConsumeRecoveryCode(ctx context.Context, userID int64, code string) error {
	rc, err := s.findRecoveryCode(ctx, userID, code)
	if err != nil {
		return err
	}
	if err := s.recovery.MarkUsed(ctx, rc.ID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrInvalidCode
		}
		return err
	}
	return nil
}

// findRecoveryCode locates the unused recovery code row matching the
// plaintext input without mutating any state. Returns ErrInvalidCode when no
// match exists. Used by callers that want to verify a recovery code before
// performing an operation that will delete all recovery codes anyway (e.g.
// Disable), so a transient failure between match and follow-up write doesn't
// burn the code.
func (s *Service) findRecoveryCode(ctx context.Context, userID int64, code string) (repo.TOTPRecoveryCode, error) {
	code = normalizeRecoveryCode(code)
	// Fast-fail anything that isn't shaped like a recovery code so a mistyped
	// TOTP (or random garbage) doesn't trigger N Argon2id verifications.
	if len(code) != recoveryCodeLength || !isRecoveryCodeAlphabet(code) {
		return repo.TOTPRecoveryCode{}, ErrInvalidCode
	}
	state, err := s.users.GetTOTPState(ctx, userID)
	if err != nil {
		return repo.TOTPRecoveryCode{}, err
	}
	if !state.Enabled {
		return repo.TOTPRecoveryCode{}, ErrNotEnabled
	}
	codes, err := s.recovery.ListUnused(ctx, userID)
	if err != nil {
		return repo.TOTPRecoveryCode{}, err
	}
	for _, rc := range codes {
		if auth.VerifyToken(code, rc.CodeHash) {
			return rc, nil
		}
	}
	return repo.TOTPRecoveryCode{}, ErrInvalidCode
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
		// Fall back to recovery code, but only verify the match — DisableTOTP +
		// DeleteAll below wipes every recovery row anyway, so marking the
		// matched code "used" here is redundant and would burn it if the
		// downstream writes failed (a transient lockout when the user is on
		// their last code with no working TOTP device).
		if _, rerr := s.findRecoveryCode(ctx, userID, code); rerr != nil {
			return rerr
		}
	}
	// Disable the user flag first. If DeleteAll then fails, any leftover
	// recovery rows are inert because ConsumeRecoveryCode checks state.Enabled.
	// The reverse order risks leaving the account 2FA-enabled with no recovery
	// codes available, locking the user out.
	if err := s.users.DisableTOTP(ctx, userID); err != nil {
		return err
	}
	return s.recovery.DeleteAll(ctx, userID)
}

// matchedStep returns the TOTP time-step matched by code (within ±totpSkew) at
// time t, or (0, false) when no step in the skew window produces the code.
//
// Unlike pquerna/otp's ValidateCustom, this also reports which step matched so
// callers can persist it for replay protection.
func (s *Service) matchedStep(code, secret string, t time.Time) (int64, bool) {
	counter := t.Unix() / int64(totpPeriodSeconds)
	for delta := int64(-totpSkew); delta <= int64(totpSkew); delta++ {
		step := counter + delta
		stepTime := time.Unix(step*int64(totpPeriodSeconds), 0)
		expected, err := totp.GenerateCode(secret, stepTime)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
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
		plaintexts[i] = code
		hashes[i] = auth.HashToken(code)
	}
	return plaintexts, hashes, nil
}

// randomRecoveryCode returns a base32 (RFC 4648, no padding) string of length
// recoveryCodeLength. Uses 8 random bytes which encode to 13 base32 chars;
// truncated to recoveryCodeLength (≈50 bits retained after Argon2id hashing).
func randomRecoveryCode() (string, error) {
	const need = recoveryCodeLength
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
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

// isRecoveryCodeAlphabet reports whether s contains only RFC 4648 base32
// characters (A–Z, 2–7). normalizeRecoveryCode has already stripped spaces and
// dashes and uppercased letters, so this is a cheap final shape check.
func isRecoveryCodeAlphabet(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '2' && r <= '7':
		default:
			return false
		}
	}
	return true
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

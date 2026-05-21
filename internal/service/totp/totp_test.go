package totp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/repo"
)

const testCipherKey = "test-cipher-key-32-bytes-min-padding!"

func setupService(t *testing.T) (*Service, *repo.UserRepo, *repo.TOTPRecoveryRepo) {
	t.Helper()
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "totp.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.RunMigrations(context.Background(), sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := repo.NewUserRepo(sqlDB)
	rec := repo.NewTOTPRecoveryRepo(sqlDB)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// Use the smallest argon profile so tests stay fast.
	params := auth.Argon2Params{Memory: 8 * 1024, Time: 1, Threads: 1}
	svc := NewService(crypto.NewTokenCipher(testCipherKey), users, rec, params)
	return svc, users, rec
}

func TestService_BeginSetup_PersistsEncryptedSecret(t *testing.T) {
	svc, users, _ := setupService(t)
	ctx := context.Background()
	info, err := svc.BeginSetup(ctx, 1, "admin")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if info.Secret == "" {
		t.Errorf("secret: got empty")
	}
	if !strings.HasPrefix(info.OtpauthURL, "otpauth://totp/") {
		t.Errorf("otpauth url: got %q", info.OtpauthURL)
	}
	if !strings.Contains(info.OtpauthURL, "issuer="+Issuer) {
		t.Errorf("otpauth url missing issuer=%s: %q", Issuer, info.OtpauthURL)
	}
	if len(info.QRPNG) < 8 || string(info.QRPNG[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Errorf("qr png: missing PNG signature, got %d bytes", len(info.QRPNG))
	}
	st, err := users.GetTOTPState(ctx, 1)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if st.Enabled {
		t.Errorf("enabled after BeginSetup: got true, want false")
	}
	if !strings.HasPrefix(st.Secret, crypto.EncryptedTokenPrefix) {
		t.Errorf("stored secret not encrypted: %q", st.Secret)
	}
}

func TestService_BeginSetup_FailsWhenAlreadyEnabled(t *testing.T) {
	svc, users, _ := setupService(t)
	ctx := context.Background()
	if _, err := svc.BeginSetup(ctx, 1, "admin"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	// Force-enable by calling EnableTOTP directly to bypass code check.
	if err := users.EnableTOTP(ctx, 1); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if _, err := svc.BeginSetup(ctx, 1, "admin"); !errors.Is(err, ErrAlreadyEnabled) {
		t.Errorf("got %v, want ErrAlreadyEnabled", err)
	}
}

func TestService_ConfirmSetup_HappyPath_GeneratesRecoveryCodes(t *testing.T) {
	svc, users, rec := setupService(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	svc.SetNowFunc(func() time.Time { return now })

	info, err := svc.BeginSetup(ctx, 1, "admin")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	code, err := totp.GenerateCode(info.Secret, now)
	if err != nil {
		t.Fatalf("gen code: %v", err)
	}
	codes, err := svc.ConfirmSetup(ctx, 1, code)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Errorf("codes: got %d, want %d", len(codes), RecoveryCodeCount)
	}
	for i, c := range codes {
		if len(c) != 10 {
			t.Errorf("code[%d] len: got %d, want 10 (%q)", i, len(c), c)
		}
	}
	st, _ := users.GetTOTPState(ctx, 1)
	if !st.Enabled {
		t.Errorf("enabled after confirm: got false, want true")
	}
	unused, _ := rec.ListUnused(ctx, 1)
	if len(unused) != RecoveryCodeCount {
		t.Errorf("stored unused: got %d, want %d", len(unused), RecoveryCodeCount)
	}
}

func TestService_ConfirmSetup_InvalidCode(t *testing.T) {
	svc, _, _ := setupService(t)
	ctx := context.Background()
	if _, err := svc.BeginSetup(ctx, 1, "admin"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := svc.ConfirmSetup(ctx, 1, "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("got %v, want ErrInvalidCode", err)
	}
}

func TestService_ConfirmSetup_RequiresPendingSecret(t *testing.T) {
	svc, _, _ := setupService(t)
	ctx := context.Background()
	if _, err := svc.ConfirmSetup(ctx, 1, "123456"); !errors.Is(err, ErrNoPendingSetup) {
		t.Errorf("got %v, want ErrNoPendingSetup", err)
	}
}

func TestService_Verify_ReplayWindow(t *testing.T) {
	svc, _, _ := setupService(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	svc.SetNowFunc(func() time.Time { return now })

	info, err := svc.BeginSetup(ctx, 1, "admin")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	code, err := totp.GenerateCode(info.Secret, now)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	if _, err := svc.ConfirmSetup(ctx, 1, code); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	// Same code, same time -> still valid (the service does not track used
	// TOTP codes; replay protection within the period is the caller's job).
	if err := svc.Verify(ctx, 1, code); err != nil {
		t.Errorf("verify same period: got %v, want nil", err)
	}
	// Move time far outside the skew window -> rejected.
	svc.SetNowFunc(func() time.Time { return now.Add(5 * time.Minute) })
	if err := svc.Verify(ctx, 1, code); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("verify after window: got %v, want ErrInvalidCode", err)
	}
}

func TestService_Verify_NotEnabled(t *testing.T) {
	svc, _, _ := setupService(t)
	if err := svc.Verify(context.Background(), 1, "123456"); !errors.Is(err, ErrNotEnabled) {
		t.Errorf("got %v, want ErrNotEnabled", err)
	}
}

func TestService_ConsumeRecoveryCode_SingleUse(t *testing.T) {
	svc, _, _ := setupService(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	svc.SetNowFunc(func() time.Time { return now })

	info, err := svc.BeginSetup(ctx, 1, "admin")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	code, _ := totp.GenerateCode(info.Secret, now)
	recovery, err := svc.ConfirmSetup(ctx, 1, code)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	// Use the first code (case-insensitive + dash tolerance).
	rc := recovery[0][:5] + "-" + recovery[0][5:]
	rc = strings.ToLower(rc)
	if err := svc.ConsumeRecoveryCode(ctx, 1, rc); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if err := svc.ConsumeRecoveryCode(ctx, 1, recovery[0]); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("replay: got %v, want ErrInvalidCode", err)
	}
	// Wrong code -> invalid.
	if err := svc.ConsumeRecoveryCode(ctx, 1, "AAAAAAAAAA"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("wrong code: got %v, want ErrInvalidCode", err)
	}
}

func TestService_Disable_WithTOTPCode(t *testing.T) {
	svc, users, rec := setupService(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	svc.SetNowFunc(func() time.Time { return now })

	info, _ := svc.BeginSetup(ctx, 1, "admin")
	code, _ := totp.GenerateCode(info.Secret, now)
	if _, err := svc.ConfirmSetup(ctx, 1, code); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := svc.Disable(ctx, 1, code); err != nil {
		t.Fatalf("disable: %v", err)
	}
	st, _ := users.GetTOTPState(ctx, 1)
	if st.Enabled || st.Secret != "" {
		t.Errorf("state after disable: %+v, want disabled+empty", st)
	}
	rem, _ := rec.ListUnused(ctx, 1)
	if len(rem) != 0 {
		t.Errorf("recovery codes after disable: got %d, want 0", len(rem))
	}
}

func TestService_Disable_WithRecoveryCode(t *testing.T) {
	svc, users, _ := setupService(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	svc.SetNowFunc(func() time.Time { return now })

	info, _ := svc.BeginSetup(ctx, 1, "admin")
	code, _ := totp.GenerateCode(info.Secret, now)
	recovery, err := svc.ConfirmSetup(ctx, 1, code)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := svc.Disable(ctx, 1, recovery[2]); err != nil {
		t.Fatalf("disable with recovery: %v", err)
	}
	st, _ := users.GetTOTPState(ctx, 1)
	if st.Enabled {
		t.Errorf("enabled after disable: got true, want false")
	}
}

func TestService_Disable_NotEnabled(t *testing.T) {
	svc, _, _ := setupService(t)
	if err := svc.Disable(context.Background(), 1, "123456"); !errors.Is(err, ErrNotEnabled) {
		t.Errorf("got %v, want ErrNotEnabled", err)
	}
}

func TestService_Disable_InvalidCode(t *testing.T) {
	svc, _, _ := setupService(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	svc.SetNowFunc(func() time.Time { return now })

	info, _ := svc.BeginSetup(ctx, 1, "admin")
	code, _ := totp.GenerateCode(info.Secret, now)
	if _, err := svc.ConfirmSetup(ctx, 1, code); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := svc.Disable(ctx, 1, "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("got %v, want ErrInvalidCode", err)
	}
}

func TestNormalizeRecoveryCode(t *testing.T) {
	cases := map[string]string{
		"abcde-fghij": "ABCDEFGHIJ",
		"ABC 12 XYZ":  "ABC12XYZ",
		"  a-b-c  ":   "ABC",
		"":            "",
	}
	for in, want := range cases {
		if got := normalizeRecoveryCode(in); got != want {
			t.Errorf("normalize(%q): got %q, want %q", in, got, want)
		}
	}
}

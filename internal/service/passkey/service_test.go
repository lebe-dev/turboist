package passkey

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/repo"
)

func setupService(t *testing.T) (*Service, *repo.WebAuthnRepo) {
	t.Helper()
	d := openTestDB(t)
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	creds := repo.NewWebAuthnRepo(d)
	svc, err := NewService(Config{
		RPID:          "example.com",
		RPDisplayName: "Turboist",
		Origins:       []string{"https://example.com"},
	}, creds, users)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, creds
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func TestNewService_RejectsMissingOrigins(t *testing.T) {
	if _, err := NewService(Config{RPID: "example.com"}, nil, nil); err == nil {
		t.Fatal("new service: got nil error, want a config rejection when no origin is allowed")
	}
}

func TestService_BeginRegistration_Options(t *testing.T) {
	svc, creds := setupService(t)
	ctx := context.Background()

	begin, err := svc.BeginRegistration(ctx, 1)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}
	if begin.CeremonyID == "" {
		t.Error("ceremonyId: got empty string, want an opaque id")
	}
	opts := begin.Options.Response
	if opts.RelyingParty.ID != "example.com" {
		t.Errorf("rp id: got %q, want example.com", opts.RelyingParty.ID)
	}
	if opts.AuthenticatorSelection.ResidentKey != "required" {
		t.Errorf("residentKey: got %q, want required — discoverable credentials make the login usernameless",
			opts.AuthenticatorSelection.ResidentKey)
	}

	// The handle handed to the authenticator must be the persisted one, so a
	// later discoverable login resolves back to this account.
	handle, err := creds.EnsureHandle(ctx, 1)
	if err != nil {
		t.Fatalf("ensure handle: %v", err)
	}
	gotHandle, ok := opts.User.ID.(protocol.URLEncodedBase64)
	if !ok {
		t.Fatalf("user handle: got %T, want protocol.URLEncodedBase64", opts.User.ID)
	}
	if string(gotHandle) != handle {
		t.Errorf("user handle: got %q, want %q", gotHandle, handle)
	}
}

func TestService_BeginRegistration_ExcludesRegisteredCredentials(t *testing.T) {
	svc, creds := setupService(t)
	ctx := context.Background()
	seedCredential(t, creds, "cred-1")

	begin, err := svc.BeginRegistration(ctx, 1)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}
	if got := len(begin.Options.Response.CredentialExcludeList); got != 1 {
		t.Errorf("excludeCredentials: got %d entries, want 1 so the platform offers to replace, not duplicate", got)
	}
}

func TestService_BeginRegistration_LimitReached(t *testing.T) {
	svc, creds := setupService(t)
	ctx := context.Background()
	for i := 0; i < MaxCredentialsPerUser; i++ {
		seedCredential(t, creds, string(rune('a'+i)))
	}
	if _, err := svc.BeginRegistration(ctx, 1); !errors.Is(err, ErrLimitReached) {
		t.Errorf("begin registration: got %v, want ErrLimitReached", err)
	}
}

func TestService_FinishRegistration_UnknownCeremony(t *testing.T) {
	svc, _ := setupService(t)
	_, err := svc.FinishRegistration(context.Background(), 1, "nope", "Key", []byte(`{}`))
	if !errors.Is(err, ErrCeremonyExpired) {
		t.Errorf("finish: got %v, want ErrCeremonyExpired", err)
	}
}

func TestService_FinishRegistration_ForeignCeremony(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	begin, err := svc.BeginRegistration(ctx, 1)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}
	// Another account must not be able to complete a ceremony it did not start.
	_, err = svc.FinishRegistration(ctx, 2, begin.CeremonyID, "Key", []byte(`{}`))
	if !errors.Is(err, ErrCeremonyExpired) {
		t.Errorf("finish: got %v, want ErrCeremonyExpired", err)
	}
}

func TestService_FinishRegistration_GarbageResponse(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	begin, err := svc.BeginRegistration(ctx, 1)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}
	_, err = svc.FinishRegistration(ctx, 1, begin.CeremonyID, "Key", []byte(`{"nope":true}`))
	if !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("finish: got %v, want ErrInvalidResponse", err)
	}
}

func TestService_BeginLogin_Discoverable(t *testing.T) {
	svc, _ := setupService(t)

	begin, err := svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	if begin.CeremonyID == "" {
		t.Error("ceremonyId: got empty string, want an opaque id")
	}
	if got := len(begin.Options.Response.AllowedCredentials); got != 0 {
		t.Errorf("allowCredentials: got %d entries, want 0 for a discoverable login", got)
	}
	if begin.Options.Response.RelyingPartyID != "example.com" {
		t.Errorf("rp id: got %q, want example.com", begin.Options.Response.RelyingPartyID)
	}
}

func TestService_FinishLogin_UnknownCeremony(t *testing.T) {
	svc, _ := setupService(t)
	if _, err := svc.FinishLogin(context.Background(), "nope", []byte(`{}`)); !errors.Is(err, ErrCeremonyExpired) {
		t.Errorf("finish login: got %v, want ErrCeremonyExpired", err)
	}
}

func TestService_FinishLogin_GarbageResponse(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	begin, err := svc.BeginLogin(ctx)
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	if _, err := svc.FinishLogin(ctx, begin.CeremonyID, []byte(`{"nope":true}`)); !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("finish login: got %v, want ErrInvalidResponse", err)
	}
}

func TestService_ListRenameDelete(t *testing.T) {
	svc, creds := setupService(t)
	ctx := context.Background()
	seedCredential(t, creds, "cred-1")

	rows, err := svc.List(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("list: got %d rows, want 1", len(rows))
	}

	renamed, err := svc.Rename(ctx, rows[0].ID, 1, "  iPhone  ")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name != "iPhone" {
		t.Errorf("name: got %q, want iPhone (trimmed)", renamed.Name)
	}

	if _, err := svc.Rename(ctx, 999, 1, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename missing: got %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, 999, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: got %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, rows[0].ID, 1); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestService_AnyRegistered(t *testing.T) {
	svc, creds := setupService(t)
	ctx := context.Background()

	any, err := svc.AnyRegistered(ctx)
	if err != nil {
		t.Fatalf("any registered: %v", err)
	}
	if any {
		t.Error("any registered: got true, want false on a fresh instance")
	}

	seedCredential(t, creds, "cred-1")
	any, err = svc.AnyRegistered(ctx)
	if err != nil {
		t.Fatalf("any registered: %v", err)
	}
	if !any {
		t.Error("any registered: got false, want true once a passkey exists")
	}
}

// seedCredential stores a syntactically valid (but cryptographically empty)
// credential record, which is all the non-ceremony paths need.
func seedCredential(t *testing.T, creds *repo.WebAuthnRepo, credentialID string) {
	t.Helper()
	blob, err := json.Marshal(map[string]any{"id": []byte(credentialID)})
	if err != nil {
		t.Fatalf("marshal credential: %v", err)
	}
	if _, err := creds.Create(context.Background(), repo.CreateCredentialParams{
		UserID:       1,
		CredentialID: credentialID,
		Name:         "Key",
		Credential:   blob,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
}

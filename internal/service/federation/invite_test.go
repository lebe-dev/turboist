package federation_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// newInviteSvc builds a federation service with the invites repo wired (and the
// id=1 context seeded so seedProject succeeds) and returns it alongside the
// project + invites repos for assertions.
func newInviteSvc(t *testing.T, instanceURL string) (*fedsvc.Service, *repo.ProjectRepo, *repo.FederationInviteRepo) {
	t.Helper()
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	invites := repo.NewFederationInviteRepo(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, invites, repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), instanceURL)
	return svc, projects, invites
}

// enableProject creates a context+project and enables federation on it.
func enableProject(t *testing.T, svc *fedsvc.Service, projects *repo.ProjectRepo) int64 {
	t.Helper()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(context.Background(), pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	return pid
}

// TestCreateInvite_PastExpiryRejected asserts a caller-supplied past expires_at
// is rejected up front (ErrInviteExpiryInPast) rather than minting a born-dead
// invite that would only fail confusingly at the Phase-2 handshake, while a
// future expiry still succeeds (US-1.2; F1.2 follow-up).
func TestCreateInvite_PastExpiryRejected(t *testing.T) {
	svc, projects, _ := newInviteSvc(t, "https://me.example")
	pid := enableProject(t, svc, projects)

	past := time.Now().Add(-time.Hour)
	if _, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
		ExpiresAt:   &past,
	}); !errors.Is(err, fedsvc.ErrInviteExpiryInPast) {
		t.Fatalf("past expiry: got %v, want ErrInviteExpiryInPast", err)
	}

	future := time.Now().Add(time.Hour)
	if _, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
		ExpiresAt:   &future,
	}); err != nil {
		t.Fatalf("future expiry: unexpected error %v", err)
	}
}

// TestCreateInvite_DefaultsAndHash asserts the default 7d expiry + max_uses=1
// (US-1.2 AC1, AC4) and that the returned plaintext secret is stored only as its
// SHA-256 hash (US-1.2 AC2).
func TestCreateInvite_DefaultsAndHash(t *testing.T) {
	svc, projects, invites := newInviteSvc(t, "https://me.example")
	pid := enableProject(t, svc, projects)

	before := time.Now()
	out, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	after := time.Now()

	if out.InviteID == "" {
		t.Errorf("invite id is empty")
	}
	if out.Secret == "" {
		t.Errorf("plaintext secret is empty")
	}

	// Stored row carries hash, defaults.
	stored, err := invites.Get(context.Background(), out.InviteID)
	if err != nil {
		t.Fatalf("get stored invite: %v", err)
	}
	sum := sha256.Sum256([]byte(out.Secret))
	wantHash := hex.EncodeToString(sum[:])
	if stored.SecretHash != wantHash {
		t.Errorf("stored secret_hash does not match SHA-256(secret): got %q", stored.SecretHash)
	}
	if stored.SecretHash == out.Secret {
		t.Errorf("stored secret_hash equals plaintext — not hashed")
	}
	if stored.MaxUses != 1 {
		t.Errorf("default max_uses: got %d, want 1", stored.MaxUses)
	}
	if stored.ExpiresAt == nil {
		t.Fatalf("default expires_at is nil, want now+7d")
	}
	wantMin := before.Add(7 * 24 * time.Hour).Add(-2 * time.Minute)
	wantMax := after.Add(7 * 24 * time.Hour).Add(2 * time.Minute)
	if stored.ExpiresAt.Before(wantMin) || stored.ExpiresAt.After(wantMax) {
		t.Errorf("default expires_at %v not within ~7d window [%v, %v]", stored.ExpiresAt, wantMin, wantMax)
	}
	if stored.Permissions != model.FederationPermissionWrite {
		t.Errorf("permissions: got %q, want write", stored.Permissions)
	}
}

// TestCreateInvite_CustomExpiryAndMaxUses asserts explicit overrides win.
func TestCreateInvite_CustomExpiryAndMaxUses(t *testing.T) {
	svc, projects, invites := newInviteSvc(t, "https://me.example")
	pid := enableProject(t, svc, projects)

	exp := time.Now().Add(time.Hour)
	out, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionRead,
		MaxUses:     5,
		ExpiresAt:   &exp,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	stored, err := invites.Get(context.Background(), out.InviteID)
	if err != nil {
		t.Fatalf("get stored: %v", err)
	}
	if stored.MaxUses != 5 {
		t.Errorf("max_uses: got %d, want 5", stored.MaxUses)
	}
	if stored.ExpiresAt == nil || stored.ExpiresAt.Sub(exp).Abs() > time.Second {
		t.Errorf("expires_at: got %v, want ~%v", stored.ExpiresAt, exp)
	}
}

// TestCreateInvite_NotEnabled asserts that creating an invite for a project that
// has NOT been enabled for federation returns ErrFederationNotEnabled (US-1.1
// AC3 → 400), and that no invite row is written.
func TestCreateInvite_NotEnabled(t *testing.T) {
	svc, projects, invites := newInviteSvc(t, "https://me.example")
	pid := seedProject(t, projects) // NOT enabled

	if _, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
	}); !errors.Is(err, fedsvc.ErrFederationNotEnabled) {
		t.Fatalf("expected ErrFederationNotEnabled, got %v", err)
	}
	rows, err := invites.ListByProject(context.Background(), pid)
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("invite rows after not-enabled create: got %d, want 0", len(rows))
	}
}

// TestCreateInvite_ProjectNotFound asserts ErrProjectNotFound for an unknown id.
func TestCreateInvite_ProjectNotFound(t *testing.T) {
	svc, _, _ := newInviteSvc(t, "https://me.example")
	if _, err := svc.CreateInvite(context.Background(), 99999, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
	}); !errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

// TestCreateInvite_DBErrorNotMaskedAsNotFound asserts a non-ErrNotFound
// project-Get failure (closed *sql.DB) surfaces as a wrapped error rather than
// being collapsed into ErrProjectNotFound, so the handler returns 500 instead
// of a misleading 404.
func TestCreateInvite_DBErrorNotMaskedAsNotFound(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), "https://me.example")

	if err := d.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	// Permissions are valid so the guard runs the project lookup (which now hits
	// the closed DB) instead of returning ErrInvalidPermissions first.
	_, err := svc.CreateInvite(context.Background(), 1, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
	})
	if err == nil {
		t.Fatal("expected an error from CreateInvite on a closed DB, got nil")
	}
	if errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("DB failure masked as ErrProjectNotFound: %v", err)
	}
}

// TestCreateInvite_InvalidPermissions rejects a permission outside read/write/admin.
func TestCreateInvite_InvalidPermissions(t *testing.T) {
	svc, projects, _ := newInviteSvc(t, "https://me.example")
	pid := enableProject(t, svc, projects)
	if _, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermission("owner"),
	}); !errors.Is(err, fedsvc.ErrInvalidPermissions) {
		t.Fatalf("expected ErrInvalidPermissions, got %v", err)
	}
}

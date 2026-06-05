package federation_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

const fedSvcKey = "federation-service-cipher-key-32-bytes!!"

func setup(t *testing.T) (*sql.DB, *repo.ProjectRepo, *repo.FederatedProjectRepo, *repo.FederationKeysRepo) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "fedsvc.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	return d, projects, repo.NewFederatedProjectRepo(d), repo.NewFederationKeysRepo(d)
}

func seedProject(t *testing.T, projects *repo.ProjectRepo) int64 {
	t.Helper()
	p, err := projects.Create(context.Background(), repo.CreateProject{
		ContextID: 1, Title: "Shared", Color: "blue",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p.ID
}

func seedContext(t *testing.T, d *sql.DB) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, client_id, created_at, updated_at)
		 VALUES (1, 'c', 'blue', 'svc-ctx-1', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed context: %v", err)
	}
}

func TestEnableForProject_Success(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	pid := seedProject(t, projects)

	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), "https://me.example")
	got, err := svc.EnableForProject(context.Background(), pid)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !got.IsFederated {
		t.Errorf("returned project isFederated: got false, want true")
	}

	self, err := fedProjects.SelfRow(context.Background(), pid)
	if err != nil {
		t.Fatalf("self-row: %v", err)
	}
	if self.OriginInstanceURL != "https://me.example" {
		t.Errorf("origin: got %q, want https://me.example", self.OriginInstanceURL)
	}

	// Keypair was generated (US-1.1 AC4).
	if _, err := keys.Get(context.Background()); err != nil {
		t.Errorf("keypair not generated: %v", err)
	}
}

func TestEnableForProject_NotFound(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), "https://me.example")
	if _, err := svc.EnableForProject(context.Background(), 99999); !errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

func TestEnableForProject_KeyMissingWhenNoCipher(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	pid := seedProject(t, projects)

	// nil cipher = FEDERATION_KEY not configured.
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), nil, "https://me.example")
	if _, err := svc.EnableForProject(context.Background(), pid); !errors.Is(err, fedsvc.ErrKeyMissing) {
		t.Fatalf("expected ErrKeyMissing, got %v", err)
	}
	// The flag must NOT have been flipped when keys are unavailable.
	p, err := projects.Get(context.Background(), pid)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if p.IsFederated {
		t.Errorf("project should remain non-federated when key is missing")
	}
}

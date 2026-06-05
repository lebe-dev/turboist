package federation_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// newStatusSvcReusingDB builds a federation service over an EXISTING migrated DB
// with a chosen instanceURL, simulating a restore of the same DB under a (possibly
// different) BASE_URL (Federation v1 F6.5, US-8.5 AC2).
func newStatusSvcReusingDB(t *testing.T, d *sql.DB, instanceURL string) *fedsvc.Service {
	t.Helper()
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	return fedsvc.NewService(d, projects, repo.NewFederatedProjectRepo(d), repo.NewFederationKeysRepo(d),
		repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), instanceURL).
		WithSyncStore(store.New(d))
}

// TestCheckRestoreIdentity_UnchangedURLKeepsIdentity asserts that when the current
// instance_url matches the persisted owner self-row URL, the check is a no-op and
// the federation mappings stay live (Federation v1 F6.5, US-8.5 — restore under the
// same BASE_URL keeps identity, no re-handshake).
func TestCheckRestoreIdentity_UnchangedURLKeepsIdentity(t *testing.T) {
	svc, _, projects, fp, instances, _ := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", nil, false, false)

	res, err := svc.CheckRestoreIdentity(ctx)
	if err != nil {
		t.Fatalf("CheckRestoreIdentity: %v", err)
	}
	if res.Changed {
		t.Errorf("identity changed=true for matching URL; want false")
	}
	// The peer mapping is NOT lost.
	row, err := fp.Get(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("get peer row: %v", err)
	}
	if row.Lost {
		t.Errorf("peer row marked lost on unchanged URL")
	}
}

// TestCheckRestoreIdentity_ChangedURLPreservesAsHistory asserts that when the DB is
// restored under a NEW BASE_URL, the federation mappings are marked
// lost=instance_url_changed (read-only history) — NOT deleted — and the keypair is
// preserved (Federation v1 F6.5, US-8.5 AC2, R27).
func TestCheckRestoreIdentity_ChangedURLPreservesAsHistory(t *testing.T) {
	// Enable under the OLD url so the owner self-row records origin=old url.
	svc, d, projects, fp, instances, _ := newStatusSvc(t, "https://old.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", nil, false, false)

	// Seed a keypair so we can prove it survives the history-marking.
	keys := repo.NewFederationKeysRepo(d)
	before, err := keys.Get(ctx)
	if err != nil {
		t.Fatalf("get keys before: %v", err)
	}

	// Re-open the service AS IF restored under a NEW url.
	restored := newStatusSvcReusingDB(t, d, "https://new.example")
	res, err := restored.CheckRestoreIdentity(ctx)
	if err != nil {
		t.Fatalf("CheckRestoreIdentity: %v", err)
	}
	if !res.Changed {
		t.Fatalf("identity changed=false on a changed URL; want true")
	}
	if res.PriorInstanceURL != "https://old.example" {
		t.Errorf("priorInstanceURL: got %q, want https://old.example", res.PriorInstanceURL)
	}
	if res.RowsMarked == 0 {
		t.Errorf("rowsMarked: got 0, want > 0 (mappings marked history)")
	}

	// The peer mapping is kept as READ-ONLY history (not deleted).
	row, err := fp.Get(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("get peer row after: %v (rows must be kept as history, not deleted)", err)
	}
	if !row.Lost || row.LostReason != model.FederationLostInstanceURLChanged {
		t.Errorf("peer row: lost=%v reason=%q, want lost + instance_url_changed", row.Lost, row.LostReason)
	}
	if !row.LostReason.IsReadOnly() {
		t.Errorf("instance_url_changed must render read-only")
	}

	// The keypair is preserved (no key regen, US-8.5).
	after, err := keys.Get(ctx)
	if err != nil {
		t.Fatalf("get keys after: %v", err)
	}
	if before == nil || after == nil || after.PublicKey != before.PublicKey {
		t.Errorf("keypair changed across restore: before=%v after=%v (must be preserved)", before, after)
	}
}

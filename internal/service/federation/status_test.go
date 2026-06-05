package federation_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// newStatusSvc builds a federation service wired with the instance directory + the
// sync store so Status can read both peer health and the outbox overdue signal.
func newStatusSvc(t *testing.T, instanceURL string) (*fedsvc.Service, *sql.DB, *repo.ProjectRepo, *repo.FederatedProjectRepo, *repo.FederatedInstanceRepo, *store.Store) {
	t.Helper()
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	instances := repo.NewFederatedInstanceRepo(d)
	st := store.New(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), instances, crypto.NewTokenCipher(fedSvcKey), instanceURL).
		WithSyncStore(st)
	return svc, d, projects, fedProjects, instances, st
}

// TestStatus_SyncedWhenCleanAndFresh asserts a federated project with a fresh
// peer and an empty outbox reports "synced" (US-4.3 AC1).
func TestStatus_SyncedWhenCleanAndFresh(t *testing.T) {
	svc, _, projects, fp, instances, _ := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	statuses, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	got := byProject(statuses)
	if got[pid].Status != model.SyncStatusSynced {
		t.Errorf("status: got %q, want synced (US-4.3 AC1)", got[pid].Status)
	}
}

// TestStatus_PendingWhenUndeliveredOverdue asserts an undelivered outbox event
// older than 5 minutes flips the project to "pending" (US-4.3 AC2).
func TestStatus_PendingWhenUndeliveredOverdue(t *testing.T) {
	svc, d, projects, fp, instances, st := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	// An undelivered event created 10 minutes ago → overdue (>5min).
	old := model.FormatUTC(time.Now().Add(-10 * time.Minute))
	tx, _ := d.BeginTx(ctx, nil)
	if err := st.InsertOutboxTx(ctx, tx, "e-overdue", pid, `{}`, 1, old); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	_ = tx.Commit()

	statuses, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	got := byProject(statuses)
	if got[pid].Status != model.SyncStatusPending {
		t.Errorf("status: got %q, want pending (US-4.3 AC2)", got[pid].Status)
	}
	if got[pid].PendingCount != 1 {
		t.Errorf("pendingCount: got %d, want 1", got[pid].PendingCount)
	}
}

// TestStatus_UnreachableWhenPeerStale asserts a peer not contacted in >24h flips
// the project to "unreachable" and beats a merely-pending event (US-4.3 AC3).
func TestStatus_UnreachableWhenPeerStale(t *testing.T) {
	svc, d, projects, fp, instances, st := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	stale := time.Now().Add(-48 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &stale, false, false)

	// Even with an overdue outbox event, unreachable wins.
	old := model.FormatUTC(time.Now().Add(-10 * time.Minute))
	tx, _ := d.BeginTx(ctx, nil)
	_ = st.InsertOutboxTx(ctx, tx, "e-overdue", pid, `{}`, 1, old)
	_ = tx.Commit()

	statuses, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	got := byProject(statuses)
	if got[pid].Status != model.SyncStatusUnreachable {
		t.Errorf("status: got %q, want unreachable (US-4.3 AC3)", got[pid].Status)
	}
	if got[pid].UnreachablePeer != "https://bob.example" {
		t.Errorf("unreachablePeer: got %q, want bob", got[pid].UnreachablePeer)
	}
}

// TestStatus_KeyMismatchBeatsAll asserts a sticky key-mismatch marker flips the
// project to "key_mismatch" red and outranks unreachable + pending (US-4.3 AC4).
func TestStatus_KeyMismatchBeatsAll(t *testing.T) {
	svc, _, projects, fp, instances, _ := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	stale := time.Now().Add(-48 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &stale, false, false)

	// First mismatch transitions; status goes red.
	transitioned, err := svc.MarkPeerKeyMismatch(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("MarkPeerKeyMismatch: %v", err)
	}
	if !transitioned {
		t.Errorf("first mark: got transitioned=false, want true")
	}

	statuses, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	got := byProject(statuses)
	if got[pid].Status != model.SyncStatusKeyMismatch {
		t.Errorf("status: got %q, want key_mismatch (US-4.3 AC4)", got[pid].Status)
	}
	if got[pid].KeyMismatchPeer != "https://bob.example" {
		t.Errorf("keyMismatchPeer: got %q, want bob", got[pid].KeyMismatchPeer)
	}

	// A second mismatch is sticky → no transition (so no duplicate SSE).
	transitioned, err = svc.MarkPeerKeyMismatch(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("second MarkPeerKeyMismatch: %v", err)
	}
	if transitioned {
		t.Errorf("second mark: got transitioned=true, want false (sticky)")
	}
}

// TestStatus_ExcludesNonOwnedProjects asserts Status only reports federation-
// enabled (owner self-row) projects (US-4.3 — hidden for non-federated).
func TestStatus_ExcludesNonOwnedProjects(t *testing.T) {
	svc, _, projects, _, _, _ := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	// A plain non-federated project.
	_ = seedProject(t, projects)

	statuses, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("statuses: got %d, want 0 (no federated project)", len(statuses))
	}
}

func byProject(s []fedsvc.ProjectSyncStatus) map[int64]fedsvc.ProjectSyncStatus {
	out := map[int64]fedsvc.ProjectSyncStatus{}
	for _, st := range s {
		out[st.LocalProjectID] = st
	}
	return out
}

package repo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// TestFederationReads_ExcludeSoftDeletedProject asserts that once the parent
// project is soft-deleted (ProjectRepo.Delete sets deleted_at, leaving the
// federation rows in place for Phase-3 delete-propagation), the UI/surface read
// paths stop returning its federation rows: SelfRow → ErrNotFound, the peers list
// is empty, and the surface map omits the project (item 7 — no ghost peers, no
// editable read-only surface).
func TestFederationReads_ExcludeSoftDeletedProject(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	instRepo := NewFederatedInstanceRepo(d)
	projects := NewProjectRepo(d, NewProjectLabelsRepo(d))
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Enable federation: owner self-row (is_owner=1) via UpsertSelfRowTx in a tx.
	if err := withTx(ctx, d, func(tx *sql.Tx) error {
		return r.UpsertSelfRowTx(ctx, tx, model.FederatedProject{
			LocalProjectID:    1,
			PeerInstanceURL:   "https://me.example",
			OriginInstanceURL: "https://me.example",
			IsOwner:           true,
			Permissions:       model.FederationPermissionAdmin,
			ProtocolVersion:   1,
			JoinedAt:          now,
		})
	}); err != nil {
		t.Fatalf("seed self-row: %v", err)
	}

	// Remote peer row + its directory entry.
	lc := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	seedPeerRow(t, r, instRepo, "https://bob.example", "Bob's Box", &lc, false, false)

	// Sanity: before the soft-delete, all three reads see the federation rows.
	if _, err := r.SelfRow(ctx, 1); err != nil {
		t.Fatalf("SelfRow before delete: %v", err)
	}
	if peers, err := r.ListPeersByProject(ctx, 1); err != nil || len(peers) != 1 {
		t.Fatalf("ListPeersByProject before delete: got %d peers, err %v; want 1", len(peers), err)
	}
	if surf, err := r.FederationSurfaceByProjectIDs(ctx, []int64{1}); err != nil {
		t.Fatalf("surface before delete: %v", err)
	} else if _, ok := surf[1]; !ok {
		t.Fatalf("surface before delete: project 1 missing")
	}

	// Soft-delete the parent project (federation rows survive in the DB).
	if err := projects.Delete(ctx, 1); err != nil {
		t.Fatalf("soft-delete project: %v", err)
	}

	// SelfRow now reports ErrNotFound.
	if _, err := r.SelfRow(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("SelfRow after delete: got %v, want ErrNotFound", err)
	}

	// The peers list is empty (no ghost peers).
	peers, err := r.ListPeersByProject(ctx, 1)
	if err != nil {
		t.Fatalf("ListPeersByProject after delete: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("ListPeersByProject after delete: got %d peers, want 0 (ghost peers)", len(peers))
	}

	// The surface map omits the soft-deleted project.
	surf, err := r.FederationSurfaceByProjectIDs(ctx, []int64{1})
	if err != nil {
		t.Fatalf("surface after delete: %v", err)
	}
	if _, ok := surf[1]; ok {
		t.Errorf("surface after delete: project 1 present; want absent (read-only surface leak)")
	}

	// The federation rows are still physically present (Phase-3 needs them).
	var fpCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federated_projects WHERE local_project_id = 1`).Scan(&fpCount); err != nil {
		t.Fatalf("count federated_projects: %v", err)
	}
	if fpCount != 2 {
		t.Errorf("federated_projects rows after soft-delete: got %d, want 2 (rows must survive)", fpCount)
	}
}

// TestFederatedProjectGet_ExcludesSoftDeletedProject pins item 8: Get JOINs
// projects (deleted_at IS NULL) like SelfRow, so a tombstoned-but-still-federated
// project's mapping row is NOT returned by Get (it must report ErrNotFound rather
// than be fetched as a live row).
func TestFederatedProjectGet_ExcludesSoftDeletedProject(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	instRepo := NewFederatedInstanceRepo(d)
	projects := NewProjectRepo(d, NewProjectLabelsRepo(d))
	ctx := context.Background()

	// A remote peer mapping row + its directory entry.
	lc := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	seedPeerRow(t, r, instRepo, "https://bob.example", "Bob's Box", &lc, false, false)

	// Sanity: before the soft-delete, Get returns the mapping row.
	if _, err := r.Get(ctx, 1, "https://bob.example"); err != nil {
		t.Fatalf("Get before delete: %v", err)
	}

	// Soft-delete the parent project (the federation row survives in the DB).
	if err := projects.Delete(ctx, 1); err != nil {
		t.Fatalf("soft-delete project: %v", err)
	}

	// Get now reports ErrNotFound even though the row is still physically present.
	if _, err := r.Get(ctx, 1, "https://bob.example"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: got %v, want ErrNotFound", err)
	}

	// The row is still physically present (Phase-3 delete-propagation needs it).
	var fpCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federated_projects WHERE local_project_id = 1 AND peer_instance_url = 'https://bob.example'`).Scan(&fpCount); err != nil {
		t.Fatalf("count federated_projects: %v", err)
	}
	if fpCount != 1 {
		t.Errorf("federated_projects peer row after soft-delete: got %d, want 1 (row must survive)", fpCount)
	}
}

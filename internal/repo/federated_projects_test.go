package repo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/model"
)

// seedFederatedProjectRow inserts a context + project (id=1) so federated rows
// have a local project FK target.
func seedFederatedProjectRow(t *testing.T, d *sql.DB) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'c', 'blue', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed context: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO projects (id, context_id, title, description, color, status, is_pinned, client_id, created_at, updated_at)
		 VALUES (1, 1, 'p', '', 'blue', 'open', 0, 'fp-cid-1', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

func TestFederatedProjectRepo_UpsertSelfRowIdempotent(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)

	row := model.FederatedProject{
		LocalProjectID:    1,
		PeerInstanceURL:   "https://me.example",
		OriginInstanceURL: "https://me.example",
		IsOwner:           true,
		Permissions:       model.FederationPermissionAdmin,
		ProtocolVersion:   1,
		JoinedAt:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	insert := func() {
		t.Helper()
		err := db.WithTx(context.Background(), d, func(tx *sql.Tx) error {
			return r.UpsertSelfRowTx(context.Background(), tx, row)
		})
		if err != nil {
			t.Fatalf("upsert self-row: %v", err)
		}
	}
	insert()
	// A second upsert with a DIFFERENT joined_at must be a no-op (ON CONFLICT DO
	// NOTHING): the row count stays 1 and joined_at is unchanged.
	row.JoinedAt = time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
	insert()

	rows, err := r.ListByProject(context.Background(), 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows after double upsert: got %d, want 1", len(rows))
	}
	self, err := r.SelfRow(context.Background(), 1)
	if err != nil {
		t.Fatalf("self-row: %v", err)
	}
	if !self.IsOwner {
		t.Errorf("is_owner: got false, want true")
	}
	if self.Permissions != model.FederationPermissionAdmin {
		t.Errorf("permissions: got %q, want admin", self.Permissions)
	}
	if got := self.JoinedAt.UTC().Format("2006-01-02"); got != "2024-01-01" {
		t.Errorf("joined_at overwritten: got %s, want 2024-01-01", got)
	}
}

func TestFederatedProjectRepo_SelfRowNotFound(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	if _, err := r.SelfRow(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestFederatedProjectRepo_SetPaused asserts SetPaused flips the paused flag on a
// single (project, peer) row, leaves the row otherwise intact (non-destructive,
// Federation v1 F5.3, US-6.1 AC1), is idempotent, and reports the affected-row
// count so the service can map a missing peer to a 404.
func TestFederatedProjectRepo_SetPaused(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	ctx := context.Background()

	const peer = "https://bob.example"
	if err := r.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    1,
		PeerInstanceURL:   peer,
		RemoteProjectID:   "remote-cid",
		OriginInstanceURL: "https://me.example",
		Permissions:       model.FederationPermissionWrite,
		ProtocolVersion:   1,
		LastSentHLC:       "0000000000000-00000-node",
		JoinedAt:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// Pause: flips paused=1 and reports one affected row (US-6.1 AC1).
	n, err := r.SetPaused(ctx, 1, peer, true)
	if err != nil {
		t.Fatalf("SetPaused(true): %v", err)
	}
	if n != 1 {
		t.Fatalf("affected rows on pause: got %d, want 1", n)
	}
	fp, err := r.Get(ctx, 1, peer)
	if err != nil {
		t.Fatalf("get after pause: %v", err)
	}
	if !fp.Paused {
		t.Errorf("paused: got false, want true (US-6.1 AC1)")
	}
	// Non-destructive: the link / permission / cursor survive the pause.
	if fp.Revoked {
		t.Errorf("revoked: got true, want false (pause must not revoke)")
	}
	if fp.Permissions != model.FederationPermissionWrite {
		t.Errorf("permissions: got %q, want write (pause must not change grant)", fp.Permissions)
	}
	if fp.LastSentHLC != "0000000000000-00000-node" {
		t.Errorf("lastSentHlc: got %q, want preserved cursor", fp.LastSentHLC)
	}

	// Resume: flips paused back to 0 (US-6.1 AC2).
	if _, err := r.SetPaused(ctx, 1, peer, false); err != nil {
		t.Fatalf("SetPaused(false): %v", err)
	}
	fp, err = r.Get(ctx, 1, peer)
	if err != nil {
		t.Fatalf("get after resume: %v", err)
	}
	if fp.Paused {
		t.Errorf("paused after resume: got true, want false (US-6.1 AC2)")
	}

	// Unknown peer: zero affected rows so the service returns ErrPeerNotFound (404).
	n, err = r.SetPaused(ctx, 1, "https://nobody.example", true)
	if err != nil {
		t.Fatalf("SetPaused unknown peer: %v", err)
	}
	if n != 0 {
		t.Errorf("affected rows for unknown peer: got %d, want 0", n)
	}
}

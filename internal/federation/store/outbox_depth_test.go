package store_test

import (
	"context"
	"database/sql"
	"testing"
)

// TestOutboxDepth counts outbox rows still pending delivery to at least one
// active (non-revoked, non-owner) peer (Federation v1 F6.5, US-8.2 AC1 — the
// gauge source). A row fully delivered to every active peer is not pending; a
// row whose only undelivered target is the owner self-row or a revoked peer is
// not pending either.
func TestOutboxDepth(t *testing.T) {
	d, st := openMigratedDB(t)
	ctx := context.Background()

	pid := seedProjectRow(t, d, "depth")
	// Owner self-row + one active peer + one revoked peer.
	seedDepthMapping(t, d, pid, "https://owner.example", "https://owner.example", true, false)
	seedDepthMapping(t, d, pid, "https://owner.example", "https://peer-a.example", false, false)
	seedDepthMapping(t, d, pid, "https://owner.example", "https://peer-b.example", false, true /* revoked */)

	// Row 1: delivered to nobody → pending (peer-a still owed).
	seedDepthOutbox(t, d, "evt-1", pid, "", "2026-06-04T00:00:00.000Z")
	// Row 2: delivered to peer-a (the only active peer) → NOT pending.
	seedDepthOutbox(t, d, "evt-2", pid, `["https://peer-a.example"]`, "2026-06-04T00:00:01.000Z")
	// Row 3: delivered only to the revoked peer-b → still owes the active peer-a → pending.
	seedDepthOutbox(t, d, "evt-3", pid, `["https://peer-b.example"]`, "2026-06-04T00:00:02.000Z")

	got, err := st.OutboxDepth(ctx)
	if err != nil {
		t.Fatalf("OutboxDepth: %v", err)
	}
	if got != 2 {
		t.Errorf("OutboxDepth: got %d, want 2 (evt-1, evt-3 pending; evt-2 delivered)", got)
	}
}

// TestOutboxDepth_Empty returns 0 with no outbox rows.
func TestOutboxDepth_Empty(t *testing.T) {
	_, st := openMigratedDB(t)
	got, err := st.OutboxDepth(context.Background())
	if err != nil {
		t.Fatalf("OutboxDepth: %v", err)
	}
	if got != 0 {
		t.Errorf("OutboxDepth empty: got %d, want 0", got)
	}
}

func seedDepthMapping(t *testing.T, d *sql.DB, projectID int64, ownerURL, peerURL string, isOwner, revoked bool) {
	t.Helper()
	owner, rev := 0, 0
	perm := "write"
	if isOwner {
		owner = 1
		perm = "admin"
	}
	if revoked {
		rev = 1
	}
	if _, err := d.Exec(`INSERT INTO federated_projects
		(local_project_id, peer_instance_url, origin_instance_url, is_owner, permissions, revoked, joined_at)
		VALUES (?, ?, ?, ?, ?, ?, '2026-01-01T00:00:00.000Z')`,
		projectID, peerURL, ownerURL, owner, perm, rev); err != nil {
		t.Fatalf("seed mapping %s: %v", peerURL, err)
	}
}

func seedDepthOutbox(t *testing.T, d *sql.DB, eventID string, projectID int64, deliveredTo, createdAt string) {
	t.Helper()
	if _, err := d.Exec(`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, protocol_version, created_at)
		VALUES (?, ?, ?, ?, 1, ?)`, eventID, projectID, `{"event_id":"`+eventID+`"}`, deliveredTo, createdAt); err != nil {
		t.Fatalf("seed outbox %s: %v", eventID, err)
	}
}

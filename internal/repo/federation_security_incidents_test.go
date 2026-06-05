package repo

import (
	"context"
	"testing"
	"time"
)

// seedIncidentProject inserts a context + project (id=1) so incident FK inserts
// have a local project target.
func seedIncidentProject(t *testing.T, r *FederationSecurityIncidentRepo) {
	t.Helper()
	ctx := context.Background()
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'c', 'blue', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed context: %v", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO projects (id, context_id, title, description, color, status, is_pinned, client_id, created_at, updated_at)
		 VALUES (1, 1, 'p', '', 'blue', 'open', 0, 'inc-p1', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

// TestSecurityIncidentRepo_RecordKeyChangeIsIdempotentWhileOpen asserts that a
// flood of rejected events under one key rotation records exactly ONE open
// incident (Federation v1 F5.6b, US-6.4 AC2; §7 "incident write non-blocking
// under key-mismatch flood"). RecordKeyChange returns whether a NEW incident
// opened so the caller can audit-log only on the transition.
func TestSecurityIncidentRepo_RecordKeyChangeIsIdempotentWhileOpen(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationSecurityIncidentRepo(d)
	seedIncidentProject(t, r)
	ctx := context.Background()
	at := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)

	opened, err := r.RecordKeyChange(ctx, 1, "https://peer.example", "oldkey", at)
	if err != nil {
		t.Fatalf("record first: %v", err)
	}
	if !opened {
		t.Errorf("first RecordKeyChange opened: got false, want true")
	}

	// A second mismatch while the incident is open records NOTHING new.
	opened2, err := r.RecordKeyChange(ctx, 1, "https://peer.example", "oldkey", at.Add(time.Minute))
	if err != nil {
		t.Fatalf("record second: %v", err)
	}
	if opened2 {
		t.Errorf("second RecordKeyChange opened: got true, want false (idempotent while open)")
	}

	inc, err := r.OpenIncident(ctx, 1, "https://peer.example")
	if err != nil {
		t.Fatalf("open incident: %v", err)
	}
	if inc == nil {
		t.Fatalf("expected an open incident, got nil")
	}
	if inc.OldPublicKey != "oldkey" {
		t.Errorf("incident old key: got %q, want oldkey", inc.OldPublicKey)
	}
	if inc.DetectedAt != "2026-06-03T10:00:00.000Z" {
		t.Errorf("incident detected_at: got %q, want 2026-06-03T10:00:00.000Z (not moved by the flood)", inc.DetectedAt)
	}
}

// TestSecurityIncidentRepo_OpenIncidentNilWhenNone asserts OpenIncident returns
// nil (not an error) when no open incident exists — the common, healthy case.
func TestSecurityIncidentRepo_OpenIncidentNilWhenNone(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationSecurityIncidentRepo(d)
	seedIncidentProject(t, r)
	ctx := context.Background()

	inc, err := r.OpenIncident(ctx, 1, "https://peer.example")
	if err != nil {
		t.Fatalf("open incident: %v", err)
	}
	if inc != nil {
		t.Errorf("expected no open incident, got %+v", inc)
	}
}

// TestSecurityIncidentRepo_ResolveStampsNewKeyAndFreesForReopen asserts that
// resolving the open incident stamps resolved_at + the newly-trusted key (the
// audit trail, US-6.4 AC3) and frees the partial index so a LATER rotation opens
// a fresh incident — the history is append-only and survives the resolve.
func TestSecurityIncidentRepo_ResolveStampsNewKeyAndFreesForReopen(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationSecurityIncidentRepo(d)
	seedIncidentProject(t, r)
	ctx := context.Background()
	at := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)

	if _, err := r.RecordKeyChange(ctx, 1, "https://peer.example", "oldkey", at); err != nil {
		t.Fatalf("record: %v", err)
	}

	resolved := at.Add(time.Hour)
	n, err := r.ResolveKeyChange(ctx, 1, "https://peer.example", "newkey", resolved)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if n != 1 {
		t.Fatalf("resolve affected rows: got %d, want 1", n)
	}

	// The incident is no longer open.
	if inc, err := r.OpenIncident(ctx, 1, "https://peer.example"); err != nil {
		t.Fatalf("open after resolve: %v", err)
	} else if inc != nil {
		t.Errorf("expected no open incident after resolve, got %+v", inc)
	}

	// A later rotation opens a fresh incident (append-only history).
	opened, err := r.RecordKeyChange(ctx, 1, "https://peer.example", "newkey", at.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("record after resolve: %v", err)
	}
	if !opened {
		t.Errorf("post-resolve RecordKeyChange opened: got false, want true (resolve frees the index)")
	}

	// Two history rows total: the resolved one + the fresh open one.
	var count int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM federation_security_incidents WHERE local_project_id = 1 AND peer_instance_url = 'https://peer.example'`,
	).Scan(&count); err != nil {
		t.Fatalf("count incidents: %v", err)
	}
	if count != 2 {
		t.Errorf("incident history rows: got %d, want 2", count)
	}
}

// TestSecurityIncidentRepo_ResolveNoOpenIsZeroRows asserts ResolveKeyChange on a
// peer with no open incident is a no-op (0 rows, nil error) — trust-key on a peer
// whose marker was never set must not error.
func TestSecurityIncidentRepo_ResolveNoOpenIsZeroRows(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationSecurityIncidentRepo(d)
	seedIncidentProject(t, r)
	ctx := context.Background()

	n, err := r.ResolveKeyChange(ctx, 1, "https://peer.example", "newkey", time.Now())
	if err != nil {
		t.Fatalf("resolve with no open incident: %v", err)
	}
	if n != 0 {
		t.Errorf("resolve affected rows: got %d, want 0", n)
	}
}

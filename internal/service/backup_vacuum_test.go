package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lebe-dev/turboist/internal/db"
)

// TestVacuumInto_IncludesFederationAndKeypair asserts the VACUUM INTO backup is a
// byte-for-byte physical copy that preserves the federation bookkeeping AND the
// instance keypair (Federation v1 F6.5, US-8.5) — the logical JSON Export, by
// contrast, captures NEITHER.
func TestVacuumInto_IncludesFederationAndKeypair(t *testing.T) {
	f := setupBackupFixtures(t)
	ctx := context.Background()

	// Seed an instance keypair + a federation mapping so the backup has federation
	// state to preserve.
	if _, err := f.db.Exec(
		`INSERT INTO federation_keys (id, public_key, private_seed_enc, node_id, display_name, created_at)
		 VALUES (1, 'PUBKEY-XYZ', 'enc', 'node-uuid-1', 'me.example', '2026-06-04T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed keys: %v", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'c', 'blue', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed context: %v", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO projects (id, context_id, title, description, color, status, is_pinned, is_federated, client_id, created_at, updated_at)
		 VALUES (5, 1, 'Shared', '', 'blue', 'open', 0, 1, 'cid-5', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO federated_projects (local_project_id, peer_instance_url, origin_instance_url, is_owner, permissions, joined_at)
		 VALUES (5, 'https://me.example', 'https://me.example', 1, 'admin', '2026-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed fed project: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "vacuum-backup.db")
	if err := f.svc.VacuumInto(ctx, dest); err != nil {
		t.Fatalf("VacuumInto: %v", err)
	}

	// Open the produced file and assert the federation rows + keypair survived.
	restored, err := db.Open(dest)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = restored.Close() })

	var pubKey string
	if err := restored.QueryRow(`SELECT public_key FROM federation_keys WHERE id = 1`).Scan(&pubKey); err != nil {
		t.Fatalf("read keypair from backup: %v (VACUUM INTO must include federation_keys)", err)
	}
	if pubKey != "PUBKEY-XYZ" {
		t.Errorf("backup public_key: got %q, want PUBKEY-XYZ", pubKey)
	}

	var fedRows int
	if err := restored.QueryRow(`SELECT COUNT(1) FROM federated_projects WHERE local_project_id = 5`).Scan(&fedRows); err != nil {
		t.Fatalf("read fed rows from backup: %v", err)
	}
	if fedRows != 1 {
		t.Errorf("backup federated_projects rows: got %d, want 1 (VACUUM INTO must include federation tables)", fedRows)
	}
}

// TestVacuumInto_RefusesExistingDest asserts VacuumInto refuses to overwrite an
// existing file (SQLite VACUUM INTO semantics, surfaced as a clear error).
func TestVacuumInto_RefusesExistingDest(t *testing.T) {
	f := setupBackupFixtures(t)
	dest := filepath.Join(t.TempDir(), "exists.db")
	// Pre-create the destination.
	if err := os.WriteFile(dest, []byte("x"), 0o600); err != nil {
		t.Fatalf("precreate: %v", err)
	}
	if err := f.svc.VacuumInto(context.Background(), dest); err == nil {
		t.Errorf("VacuumInto overwrote an existing file; want error")
	}
}

// TestVacuumIntoBytes_ReturnsReadableBackup asserts VacuumIntoBytes returns a
// non-empty SQLite file payload (the download path).
func TestVacuumIntoBytes_ReturnsReadableBackup(t *testing.T) {
	f := setupBackupFixtures(t)
	b, err := f.svc.VacuumIntoBytes(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("VacuumIntoBytes: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("empty backup bytes")
	}
	// SQLite files begin with the "SQLite format 3\000" magic header.
	if string(b[:16]) != "SQLite format 3\x00" {
		t.Errorf("backup is not a SQLite file (bad magic header)")
	}
}

package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/store"
)

// openMigrated opens a fresh migrated SQLite DB for the federation store tests.
func openMigrated(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "store.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.New(d)
}

// TestCASFieldHLC_NewerApplies asserts a higher incoming HLC wins the CAS:
// the stored HLC advances and the writer is told the field is newer (US-3.3,
// US-3.4 server resolution — the per-field LWW compare).
func TestCASFieldHLC_NewerApplies(t *testing.T) {
	s := openMigrated(t)
	ctx := context.Background()

	applied, err := s.CASFieldHLC(ctx, "task", "t-1", "title", "00000000000100-0000-nodeA")
	if err != nil {
		t.Fatalf("first cas: %v", err)
	}
	if !applied {
		t.Fatalf("first write should apply (no prior HLC)")
	}

	applied, err = s.CASFieldHLC(ctx, "task", "t-1", "title", "00000000000200-0000-nodeA")
	if err != nil {
		t.Fatalf("second cas: %v", err)
	}
	if !applied {
		t.Errorf("higher HLC should apply: got applied=false")
	}

	got, err := s.GetFieldHLC(ctx, "task", "t-1", "title")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "00000000000200-0000-nodeA" {
		t.Errorf("stored HLC: got %q, want the higher one", got)
	}
}

// TestCASFieldHLC_StaleNoOp asserts a lower-or-equal incoming HLC does NOT win:
// the stored HLC is left untouched and the writer is told the field is stale so
// the field value is not applied (US-3.3 AC2 — stale field ignored).
func TestCASFieldHLC_StaleNoOp(t *testing.T) {
	s := openMigrated(t)
	ctx := context.Background()

	if _, err := s.CASFieldHLC(ctx, "task", "t-1", "title", "00000000000300-0000-nodeA"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	applied, err := s.CASFieldHLC(ctx, "task", "t-1", "title", "00000000000200-0000-nodeA")
	if err != nil {
		t.Fatalf("stale cas: %v", err)
	}
	if applied {
		t.Errorf("stale (lower) HLC must not apply: got applied=true")
	}

	// An equal HLC is also a no-op (idempotent redelivery / tie on identical write).
	applied, err = s.CASFieldHLC(ctx, "task", "t-1", "title", "00000000000300-0000-nodeA")
	if err != nil {
		t.Fatalf("equal cas: %v", err)
	}
	if applied {
		t.Errorf("equal HLC must not re-apply: got applied=true")
	}

	got, err := s.GetFieldHLC(ctx, "task", "t-1", "title")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "00000000000300-0000-nodeA" {
		t.Errorf("stored HLC must be unchanged: got %q", got)
	}
}

// TestCASFieldHLC_TieBreakNodeID asserts that on an identical physical+logical
// HLC the node_id tie-break decides the winner (US-3.4): a lexically-greater
// node_id wins, a lesser one is a no-op. This is the per-field total order the
// design pins (§5.3).
func TestCASFieldHLC_TieBreakNodeID(t *testing.T) {
	s := openMigrated(t)
	ctx := context.Background()

	if _, err := s.CASFieldHLC(ctx, "task", "t-1", "title", "00000000000100-0000-nodeB"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// nodeA < nodeB lexically -> loses the tie.
	applied, err := s.CASFieldHLC(ctx, "task", "t-1", "title", "00000000000100-0000-nodeA")
	if err != nil {
		t.Fatalf("lower-node cas: %v", err)
	}
	if applied {
		t.Errorf("lower node_id on tie must not win: got applied=true")
	}

	// nodeC > nodeB lexically -> wins the tie.
	applied, err = s.CASFieldHLC(ctx, "task", "t-1", "title", "00000000000100-0000-nodeC")
	if err != nil {
		t.Fatalf("higher-node cas: %v", err)
	}
	if !applied {
		t.Errorf("higher node_id on tie must win: got applied=false")
	}
}

// TestCASFieldHLC_MalformedRejected asserts the CAS rejects an unparseable
// incoming HLC with ErrMalformedHLC and never stores it. This is the
// defense-in-depth gate (the F3.2a skew validator is the primary one): garbage
// that sorts above well-formed HLCs would otherwise win the lexical CAS and
// permanently poison per-field LWW for the field. The error must be a permanent
// (do-not-retry) classification so the apply worker drops the event.
func TestCASFieldHLC_MalformedRejected(t *testing.T) {
	s := openMigrated(t)
	ctx := context.Background()

	// Seed a valid prior HLC.
	if _, err := s.CASFieldHLC(ctx, "task", "t-1", "title", "00000000000100-0000-nodeA"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A garbage HLC that lexically sorts ABOVE the valid prior ("~" = 0x7E > '0').
	applied, err := s.CASFieldHLC(ctx, "task", "t-1", "title", "~garbage")
	if !errors.Is(err, store.ErrMalformedHLC) {
		t.Fatalf("malformed incoming HLC must be ErrMalformedHLC, got applied=%v err=%v", applied, err)
	}
	if applied {
		t.Errorf("malformed HLC must not be applied: got applied=true")
	}

	// The stored HLC must be unchanged — the garbage never poisoned the field.
	got, err := s.GetFieldHLC(ctx, "task", "t-1", "title")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "00000000000100-0000-nodeA" {
		t.Errorf("stored HLC must be unchanged after a malformed CAS: got %q", got)
	}
}

// TestGetFieldHLC_Missing returns the empty string for a field that has no HLC
// row yet (the create-on-missing path treats this as "always newer").
func TestGetFieldHLC_Missing(t *testing.T) {
	s := openMigrated(t)
	got, err := s.GetFieldHLC(context.Background(), "task", "absent", "title")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "" {
		t.Errorf("missing field HLC: got %q, want empty", got)
	}
}

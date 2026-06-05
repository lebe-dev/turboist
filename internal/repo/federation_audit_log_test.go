package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// TestFederationAuditRepo_InsertAndListRoundTrips asserts a recorded audit entry
// round-trips with every required AC1 field (timestamp, peer, kind, outcome) and
// that List returns rows newest-first (Federation v1 F6.3, US-7.4 AC1).
func TestFederationAuditRepo_InsertAndListRoundTrips(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationAuditLogRepo(d)
	ctx := context.Background()
	base := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)

	if err := r.Insert(ctx, AuditEntry{
		Kind:            AuditKindHandshake,
		Outcome:         AuditOutcomeAccepted,
		PeerInstanceURL: "https://alice.example",
		Detail:          "handshake accepted",
		CreatedAt:       base,
	}); err != nil {
		t.Fatalf("insert first: %v", err)
	}
	if err := r.Insert(ctx, AuditEntry{
		Kind:            AuditKindSignatureInvalid,
		Outcome:         AuditOutcomeRejected,
		PeerInstanceURL: "https://bob.example",
		Detail:          "event signature invalid",
		CreatedAt:       base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("insert second: %v", err)
	}

	rows, err := r.List(ctx, AuditFilter{}, Page{Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("list rows: got %d, want 2", len(rows))
	}
	// Newest-first.
	if rows[0].Kind != string(AuditKindSignatureInvalid) {
		t.Errorf("rows[0].Kind: got %q, want %q (newest-first)", rows[0].Kind, AuditKindSignatureInvalid)
	}
	if rows[0].PeerInstanceURL != "https://bob.example" {
		t.Errorf("rows[0].PeerInstanceURL: got %q, want %q", rows[0].PeerInstanceURL, "https://bob.example")
	}
	if rows[0].Outcome != string(AuditOutcomeRejected) {
		t.Errorf("rows[0].Outcome: got %q, want %q", rows[0].Outcome, AuditOutcomeRejected)
	}
	if rows[0].Detail != "event signature invalid" {
		t.Errorf("rows[0].Detail: got %q, want %q", rows[0].Detail, "event signature invalid")
	}
	if rows[0].CreatedAt == "" {
		t.Errorf("rows[0].CreatedAt is empty, want an ISO-8601 timestamp")
	}
}

// TestFederationAuditRepo_ListFiltersByPeer asserts the per-peer filter narrows
// the result to one peer (Federation v1 F6.3, US-7.4 AC1 owner audit view).
func TestFederationAuditRepo_ListFiltersByPeer(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationAuditLogRepo(d)
	ctx := context.Background()
	base := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)

	for i, peer := range []string{"https://alice.example", "https://bob.example", "https://alice.example"} {
		if err := r.Insert(ctx, AuditEntry{
			Kind:            AuditKindReplay,
			Outcome:         AuditOutcomeRejected,
			PeerInstanceURL: peer,
			CreatedAt:       base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	rows, err := r.List(ctx, AuditFilter{PeerInstanceURL: "https://alice.example"}, Page{Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("filtered rows: got %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.PeerInstanceURL != "https://alice.example" {
			t.Errorf("filtered row peer: got %q, want only https://alice.example", row.PeerInstanceURL)
		}
	}
}

// TestFederationAuditRepo_ListPaginates asserts the limit/offset page bounds the
// result set so the JWT audit endpoint can paginate (Federation v1 F6.3, US-7.4).
func TestFederationAuditRepo_ListPaginates(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationAuditLogRepo(d)
	ctx := context.Background()
	base := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		if err := r.Insert(ctx, AuditEntry{
			Kind:      AuditKindReplay,
			Outcome:   AuditOutcomeRejected,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	page1, err := r.List(ctx, AuditFilter{}, Page{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 rows: got %d, want 2", len(page1))
	}
	page3, err := r.List(ctx, AuditFilter{}, Page{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page3 rows: got %d, want 1 (last page)", len(page3))
	}
}

// TestFederationAuditRepo_CountSignatureFailures asserts the per-peer aggregation
// counts ONLY the signature-failure kinds in the recent window, ignoring other
// kinds and other peers (Federation v1 F6.3, US-7.4 AC3 "possible attack" alert).
func TestFederationAuditRepo_CountSignatureFailures(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationAuditLogRepo(d)
	ctx := context.Background()
	base := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)

	// alice + bob are KNOWN federated peers (rows in federated_instances); the alert
	// counts only known peers, so a stranger spoofing the X-Federation-Instance
	// header cannot raise a bogus "attack on peer X" (F6.3 spoof mitigation).
	inst := NewFederatedInstanceRepo(d)
	for _, url := range []string{"https://alice.example", "https://bob.example"} {
		if err := inst.Upsert(ctx, model.FederatedInstance{InstanceURL: url, PublicKey: "pk", DisplayName: url, CreatedAt: base, UpdatedAt: base}); err != nil {
			t.Fatalf("seed instance %s: %v", url, err)
		}
	}

	// Three signature failures for alice within the window.
	for i := 0; i < 3; i++ {
		if err := r.Insert(ctx, AuditEntry{
			Kind:            AuditKindSignatureInvalid,
			Outcome:         AuditOutcomeRejected,
			PeerInstanceURL: "https://alice.example",
			CreatedAt:       base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("insert alice sig fail %d: %v", i, err)
		}
	}
	// A non-signature kind for alice (must NOT count).
	if err := r.Insert(ctx, AuditEntry{
		Kind:            AuditKindHandshake,
		Outcome:         AuditOutcomeAccepted,
		PeerInstanceURL: "https://alice.example",
		CreatedAt:       base.Add(4 * time.Minute),
	}); err != nil {
		t.Fatalf("insert alice handshake: %v", err)
	}
	// A signature failure for bob (must NOT count under alice's filter).
	if err := r.Insert(ctx, AuditEntry{
		Kind:            AuditKindSignatureInvalid,
		Outcome:         AuditOutcomeRejected,
		PeerInstanceURL: "https://bob.example",
		CreatedAt:       base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("insert bob sig fail: %v", err)
	}
	// An old signature failure for alice OUTSIDE the window (must NOT count).
	if err := r.Insert(ctx, AuditEntry{
		Kind:            AuditKindSignatureInvalid,
		Outcome:         AuditOutcomeRejected,
		PeerInstanceURL: "https://alice.example",
		CreatedAt:       base.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("insert alice old sig fail: %v", err)
	}
	// Two in-window signature failures CLAIMING to be an UNKNOWN peer (a spoofed
	// X-Federation-Instance header): must NOT raise the alert (F6.3 known-peer filter).
	for i := 0; i < 2; i++ {
		if err := r.Insert(ctx, AuditEntry{
			Kind:            AuditKindSignatureInvalid,
			Outcome:         AuditOutcomeRejected,
			PeerInstanceURL: "https://stranger.example",
			CreatedAt:       base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("insert stranger sig fail %d: %v", i, err)
		}
	}

	since := base.Add(-10 * time.Minute)
	counts, err := r.CountSignatureFailures(ctx, since)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if counts["https://alice.example"] != 3 {
		t.Errorf("alice signature failures: got %d, want 3", counts["https://alice.example"])
	}
	if counts["https://bob.example"] != 1 {
		t.Errorf("bob signature failures: got %d, want 1", counts["https://bob.example"])
	}
	// The spoofed unknown peer must be ABSENT from the alert counts (de-noised).
	if n, ok := counts["https://stranger.example"]; ok {
		t.Errorf("unknown (spoofed) peer raised the attack alert: got count %d, want absent (F6.3 known-peer filter)", n)
	}
}

// TestFederationAuditRepo_DeleteOlderThan asserts the 1-year retention GC drops
// rows whose created_at predates the cutoff while keeping fresh ones, using the
// fixed-width TEXT lexical compare (Federation v1 F6.3, US-7.4 AC2).
func TestFederationAuditRepo_DeleteOlderThan(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationAuditLogRepo(d)
	ctx := context.Background()
	now := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)

	if err := r.Insert(ctx, AuditEntry{Kind: AuditKindReplay, Outcome: AuditOutcomeRejected, CreatedAt: now.Add(-400 * 24 * time.Hour)}); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := r.Insert(ctx, AuditEntry{Kind: AuditKindReplay, Outcome: AuditOutcomeRejected, CreatedAt: now.Add(-10 * 24 * time.Hour)}); err != nil {
		t.Fatalf("insert fresh: %v", err)
	}

	cutoff := now.Add(-365 * 24 * time.Hour)
	n, err := r.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted rows: got %d, want 1", n)
	}
	rows, err := r.List(ctx, AuditFilter{}, Page{Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("remaining rows: got %d, want 1 (only the fresh row)", len(rows))
	}
}

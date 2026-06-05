package inbox_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/model"
)

// gatedApply mirrors the F3.2 endpoint flow: it runs the F3.2a per-event payload
// validator FIRST and only calls Apply when validation passes. The end-to-end ACs
// (US-7.2 AC1 "inbox/domain count 0" on a rejected event) are asserted against a
// REAL migrated DB so a tampered/invalid event provably leaves the domain row and
// the per-field HLC untouched.
func gatedApply(t *testing.T, env *applyEnv, v *inbox.Validator, e events.Event, peerURL string) error {
	t.Helper()
	ctx := context.Background()
	if _, err := v.Validate(ctx, e, peerURL); err != nil {
		return err
	}
	_, err := env.applier.Apply(ctx, e, peerURL)
	return err
}

// dbValidator builds a Validator wired against the real applyEnv: the author key
// is resolved from a static keypair, membership from the seeded federated_projects
// row, and the clock is pinned.
func dbValidator(env *applyEnv, pub ed25519.PublicKey, now time.Time) *inbox.Validator {
	resolve := func(_ context.Context, _ string) (ed25519.PublicKey, error) { return pub, nil }
	member := func(ctx context.Context, _ /*projectClientID*/, peerURL string) (*model.FederatedProject, error) {
		fp := &model.FederatedProject{
			LocalProjectID:    env.projectID,
			PeerInstanceURL:   peerURL,
			OriginInstanceURL: peerURL,
			Permissions:       model.FederationPermissionWrite,
		}
		return fp, nil
	}
	return inbox.NewValidator(resolve, member, func() time.Time { return now })
}

// TestValidateThenApply_TamperedEventLeavesZeroRows asserts a tampered event is
// rejected by the validator BEFORE Apply, so the live task is unchanged and no
// per-field HLC advanced — the "inbox/domain count 0" half of US-7.2 AC1 / §15.5.
func TestValidateThenApply_TamperedEventLeavesZeroRows(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	v := dbValidator(env, pub, now)

	e := updateEvent(env, map[string]events.Field{
		"title": {Value: "Should Not Land", HLC: hlcAt(now)},
	})
	e.EventID = eventID("tampered")
	signed, err := events.Sign(e, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// Tamper AFTER signing: the canonical bytes no longer match the signature.
	signed.Fields["title"] = events.Field{Value: "Hijacked", HLC: hlcAt(now)}

	err = gatedApply(t, env, v, signed, "https://alice.example")
	if !errors.Is(err, inbox.ErrEventSignatureInvalid) {
		t.Fatalf("tampered event must be rejected by the validator, got %v", err)
	}

	// Domain row untouched.
	tk, err := env.tasks.Get(ctx, env.localTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if tk.Title != "Original" {
		t.Errorf("domain row must be untouched: got %q, want Original", tk.Title)
	}
	// No per-field HLC advanced.
	got, err := env.store.GetFieldHLC(ctx, "task", env.taskClientID, "title")
	if err != nil {
		t.Fatalf("get title hlc: %v", err)
	}
	if got != "" {
		t.Errorf("title field HLC must not advance on a rejected event: got %q", got)
	}
}

// TestValidateThenApply_AuthorMismatchLeavesZeroRows asserts an author/origin
// mismatch is rejected before Apply, leaving the domain row untouched (US-7.2 AC3).
func TestValidateThenApply_AuthorMismatchLeavesZeroRows(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	v := dbValidator(env, pub, now)

	e := updateEvent(env, map[string]events.Field{
		"title": {Value: "Spoofed", HLC: hlcAt(now)},
	})
	e.EventID = eventID("author-mismatch")
	e.OriginInstance = "https://eve.example" // author stays alice, origin differs
	signed, err := events.Sign(e, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	err = gatedApply(t, env, v, signed, "https://alice.example")
	if !errors.Is(err, inbox.ErrAuthorOriginMismatch) {
		t.Fatalf("author/origin mismatch must be rejected, got %v", err)
	}
	tk, err := env.tasks.Get(ctx, env.localTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if tk.Title != "Original" {
		t.Errorf("domain row must be untouched: got %q", tk.Title)
	}
}

// TestValidateThenApply_ValidEventApplies asserts the happy path: a correctly
// signed, in-window, write-peer event passes validation and Apply lands it —
// proving the validator does not over-reject legitimate traffic.
func TestValidateThenApply_ValidEventApplies(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	v := dbValidator(env, pub, now)

	e := updateEvent(env, map[string]events.Field{
		"title": {Value: "Legit Update", HLC: hlcAt(now)},
	})
	e.EventID = eventID("valid")
	signed, err := events.Sign(e, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if err := gatedApply(t, env, v, signed, "https://alice.example"); err != nil {
		t.Fatalf("valid event must apply: %v", err)
	}
	tk, err := env.tasks.Get(ctx, env.localTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if tk.Title != "Legit Update" {
		t.Errorf("valid event must land: got %q, want Legit Update", tk.Title)
	}
}

// TestValidate_SkewBoundaryAgainstParsedHLC is a regression guard tying the skew
// window to the HLC physical_ms: an event exactly at +10min boundary is the edge
// of the future window and must be accepted; just past it must be rejected.
func TestValidate_SkewBoundaryAgainstParsedHLC(t *testing.T) {
	env := newApplyEnv(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	v := dbValidator(env, pub, now)

	atBoundary := events.Field{Value: "Edge", HLC: hlc.HLC{PhysicalMS: now.Add(10 * time.Minute).UnixMilli(), NodeID: "nodeA"}.String()}
	e := updateEvent(env, map[string]events.Field{"title": atBoundary})
	e.EventID = eventID("boundary")
	signed, err := events.Sign(e, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := v.Validate(context.Background(), signed, "https://alice.example"); err != nil {
		t.Fatalf("event exactly at +10min boundary must be accepted: %v", err)
	}
}

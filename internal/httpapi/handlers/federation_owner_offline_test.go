package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// setOwnerLastContact stamps the owner instance directory row's last_contact_at so
// the joiner's owner-offline derivation (Federation v1 F5.6a, US-6.5 AC1) can be
// placed deterministically inside or outside the owner-timeout window relative to
// the harness clock (testNow).
func setOwnerLastContact(t *testing.T, e *apiEnv, ownerURL string, at time.Time) {
	t.Helper()
	if err := e.fedInstances.Upsert(context.Background(), model.FederatedInstance{
		InstanceURL:   ownerURL,
		PublicKey:     "owner-pk",
		DisplayName:   "Owner Box",
		LastContactAt: &at,
		CreatedAt:     at,
		UpdatedAt:     at,
	}); err != nil {
		t.Fatalf("set owner last_contact_at: %v", err)
	}
}

// TestProjectGet_OwnerOffline_FreshOwner asserts a joined project whose owner was
// contacted within the owner-timeout window is NOT flagged owner-offline
// (Federation v1 F5.6a, US-6.5 AC1 negative leg).
func TestProjectGet_OwnerOffline_FreshOwner(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Joined fresh")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionWrite)
	// Contacted 1 day ago — well within the 30-day window.
	setOwnerLastContact(t, e, "https://owner.example", testNow.Add(-24*time.Hour))

	got := getProjectDTO(t, e, p.ID)
	if got.OwnerOffline {
		t.Errorf("ownerOffline: got true, want false (owner contacted within window)")
	}
}

// TestProjectGet_OwnerOffline_DeadOwner asserts a joined project whose owner has
// not been contacted past the owner-timeout window IS flagged owner-offline
// (Federation v1 F5.6a, US-6.5 AC1). It is NOT marked read-only/lost — owner-death
// in v1 is a transient queued state, not a permanent read-only one.
func TestProjectGet_OwnerOffline_DeadOwner(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Joined dead-owner")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionWrite)
	// Contacted 40 days ago — beyond the default 30-day owner timeout.
	setOwnerLastContact(t, e, "https://owner.example", testNow.Add(-40*24*time.Hour))

	got := getProjectDTO(t, e, p.ID)
	if !got.OwnerOffline {
		t.Errorf("ownerOffline: got false, want true (owner unreachable >30d, US-6.5 AC1)")
	}
	if got.FederationLost {
		t.Errorf("federationLost: got true, want false (owner-offline is transient, not lost)")
	}
}

// TestProjectMutations_OwnerOffline_EditsAllowed asserts that an owner-offline
// joined WRITE copy is still locally editable — edits are queued, not blocked
// (Federation v1 F5.6a, US-6.5 AC2). The owner-offline badge is informational; it
// must NOT engage the read-only guard.
func TestProjectMutations_OwnerOffline_EditsAllowed(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Editable while owner offline")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionWrite)
	setOwnerLastContact(t, e, "https://owner.example", testNow.Add(-40*24*time.Hour))

	// The project surfaces owner-offline …
	if got := getProjectDTO(t, e, p.ID); !got.OwnerOffline {
		t.Fatalf("precondition: ownerOffline should be true")
	}
	// … yet a write still succeeds (queued not blocked, US-6.5 AC2).
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", p.ID), map[string]any{"title": "renamed while offline"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("write while owner offline: got %d, want 200 (edits queued not blocked); body: %s", resp.StatusCode, body)
	}
}

// TestProjectGet_OwnerOffline_ReadOnlyStillReadOnly asserts owner-offline does not
// flip a read-only grant into editable: a joined READ copy stays read-only whether
// or not its owner is offline (the read grant is the binding constraint).
func TestProjectGet_OwnerOffline_ReadGrantStillBlocks(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Read-only dead owner")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionRead)
	setOwnerLastContact(t, e, "https://owner.example", testNow.Add(-40*24*time.Hour))

	got := getProjectDTO(t, e, p.ID)
	if !got.OwnerOffline {
		t.Errorf("ownerOffline: got false, want true")
	}
	// A read-only copy's mutation is still rejected 403 federation_read_only — the
	// read grant is enforced independently of owner reachability.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", p.ID), map[string]any{"title": "nope"}))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read-only patch while owner offline: got %d, want 403; body: %s", resp.StatusCode, body)
	}
}

// TestProjectGet_OwnerOffline_OwnerSelfRowNeverOffline asserts the owner's OWN
// federated project is never owner-offline regardless of any contact recency — the
// owner-offline notion only applies to a joined copy (Federation v1 F5.6a).
func TestProjectGet_OwnerOffline_OwnerSelfRowNeverOffline(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Mine")
	enableFederationOn(t, e, p.ID) // is_owner=1 self-row

	got := getProjectDTO(t, e, p.ID)
	if !got.IsOwner {
		t.Fatalf("precondition: isOwner should be true")
	}
	if got.OwnerOffline {
		t.Errorf("ownerOffline: got true, want false (owner's own project)")
	}
}

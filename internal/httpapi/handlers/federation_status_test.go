package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
)

// seedStatusPeer enables federation on a project and inserts a peer mapping +
// instance directory row so the status endpoint has something to roll up.
func seedStatusPeer(t *testing.T, e *apiEnv, projectID int64, peerURL string, lastContact *time.Time) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := e.fedInstances.Upsert(ctx, model.FederatedInstance{
		InstanceURL: peerURL, PublicKey: "pk", DisplayName: "Peer",
		LastContactAt: lastContact, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	if err := e.fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    projectID,
		PeerInstanceURL:   peerURL,
		OriginInstanceURL: testBaseURL,
		Permissions:       model.FederationPermissionWrite,
		ProtocolVersion:   1,
		JoinedAt:          now,
	}); err != nil {
		t.Fatalf("seed peer row: %v", err)
	}
}

func getStatus(t *testing.T, e *apiEnv) []dto.SyncStatusDTO {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/federation/status", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var out []dto.SyncStatusDTO
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v; body: %s", err, body)
	}
	return out
}

func enableFederation(t *testing.T, e *apiEnv, projectID int64) {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/enable", projectID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enable: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// TestFederationStatus_Synced asserts a federated project with a fresh peer and
// an empty outbox reports synced (US-4.3 AC1).
func TestFederationStatus_Synced(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederation(t, e, p.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedStatusPeer(t, e, p.ID, "https://bob.example", &recent)

	out := getStatus(t, e)
	if len(out) != 1 {
		t.Fatalf("statuses: got %d, want 1", len(out))
	}
	if out[0].ProjectId != p.ID || out[0].Status != string(model.SyncStatusSynced) {
		t.Errorf("status: got %+v, want project %d synced (US-4.3 AC1)", out[0], p.ID)
	}
}

// TestFederationStatus_Pending asserts an undelivered outbox event older than 5
// minutes flips the project to pending with a count (US-4.3 AC2).
func TestFederationStatus_Pending(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederation(t, e, p.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedStatusPeer(t, e, p.ID, "https://bob.example", &recent)

	old := model.FormatUTC(time.Now().Add(-10 * time.Minute))
	if _, err := e.db.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
		 VALUES ('e-overdue', ?, '{}', '', ?)`, p.ID, old,
	); err != nil {
		t.Fatalf("seed outbox: %v", err)
	}

	out := getStatus(t, e)
	if len(out) != 1 || out[0].Status != string(model.SyncStatusPending) {
		t.Fatalf("status: got %+v, want pending (US-4.3 AC2)", out)
	}
	if out[0].PendingCount != 1 {
		t.Errorf("pendingCount: got %d, want 1", out[0].PendingCount)
	}
}

// TestFederationStatus_Unreachable asserts a peer not contacted in >24h reports
// unreachable with the offending peer URL (US-4.3 AC3).
func TestFederationStatus_Unreachable(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederation(t, e, p.ID)
	stale := time.Now().Add(-48 * time.Hour)
	seedStatusPeer(t, e, p.ID, "https://bob.example", &stale)

	out := getStatus(t, e)
	if len(out) != 1 || out[0].Status != string(model.SyncStatusUnreachable) {
		t.Fatalf("status: got %+v, want unreachable (US-4.3 AC3)", out)
	}
	if out[0].UnreachablePeer != "https://bob.example" {
		t.Errorf("unreachablePeer: got %q, want bob", out[0].UnreachablePeer)
	}
}

// TestFederationStatus_KeyMismatch asserts a sticky key-mismatch marker reports
// key_mismatch red and outranks unreachable/pending (US-4.3 AC4).
func TestFederationStatus_KeyMismatch(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederation(t, e, p.ID)
	stale := time.Now().Add(-48 * time.Hour)
	seedStatusPeer(t, e, p.ID, "https://bob.example", &stale)

	// Stamp the sticky marker directly (the inbox signature check writes it via
	// MarkKeyMismatch in production).
	if _, err := e.fedProjects.MarkKeyMismatch(context.Background(), p.ID, "https://bob.example", model.FormatUTC(time.Now())); err != nil {
		t.Fatalf("mark key mismatch: %v", err)
	}

	out := getStatus(t, e)
	if len(out) != 1 || out[0].Status != string(model.SyncStatusKeyMismatch) {
		t.Fatalf("status: got %+v, want key_mismatch (US-4.3 AC4)", out)
	}
	if out[0].KeyMismatchPeer != "https://bob.example" {
		t.Errorf("keyMismatchPeer: got %q, want bob", out[0].KeyMismatchPeer)
	}
}

// TestFederationStatus_ExcludesNonFederated asserts a non-federated project
// produces no status row (hidden for non-federated, US-4.3).
func TestFederationStatus_ExcludesNonFederated(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	_ = createTestProject(t, e, ctx.ID, "Plain")

	out := getStatus(t, e)
	if len(out) != 0 {
		t.Errorf("statuses: got %d, want 0 (no federated project)", len(out))
	}
}

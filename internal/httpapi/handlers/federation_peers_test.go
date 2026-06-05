package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
)

// seedPeerVia inserts a federated_instances + federated_projects peer row through
// the env repos so the GET /peers handler can join and render them.
func seedPeerVia(t *testing.T, e *apiEnv, projectID int64, peerURL, displayName string, lastContact *time.Time, paused, revoked bool) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	instances := e.fedInstances
	if err := instances.Upsert(ctx, model.FederatedInstance{
		InstanceURL:   peerURL,
		PublicKey:     "pk",
		DisplayName:   displayName,
		LastContactAt: lastContact,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	if err := e.fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    projectID,
		PeerInstanceURL:   peerURL,
		RemoteProjectID:   "remote-cid",
		IsOwner:           false,
		OriginInstanceURL: "http://test",
		Permissions:       model.FederationPermissionWrite,
		Paused:            paused,
		Revoked:           revoked,
		ProtocolVersion:   1,
		LastSentHLC:       "0000000000000-00000-node",
		JoinedAt:          now,
	}); err != nil {
		t.Fatalf("seed peer row: %v", err)
	}
}

// TestListPeers_ExcludesSelfJoinsDisplayName asserts GET .../federation/peers
// returns the joined display_name + status for each remote peer and excludes the
// owner self-row (US-1.4 AC1, AC2, AC3). pendingDelivery is present and 0
// (US-1.4 AC4 partial).
func TestListPeers_ExcludesSelfJoinsDisplayName(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID) // creates the owner self-row

	stale := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)
	seedPeerVia(t, e, p.ID, "https://bob.example", "Bob's Box", &recent, false, false)
	seedPeerVia(t, e, p.ID, "https://carol.example", "Carol", &stale, false, false)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers", p.ID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list peers: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	var peers []dto.PeerDTO
	if err := json.Unmarshal(body, &peers); err != nil {
		t.Fatalf("parse peers: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("peer count: got %d, want 2 (self-row excluded, US-1.4 AC1)", len(peers))
	}

	byURL := map[string]dto.PeerDTO{}
	for _, pr := range peers {
		byURL[pr.InstanceURL] = pr
		if pr.InstanceURL == "http://test" {
			t.Errorf("self-row leaked into peers response")
		}
		if pr.PendingDelivery != 0 {
			t.Errorf("%s pendingDelivery: got %d, want 0 (US-1.4 AC4 partial)", pr.InstanceURL, pr.PendingDelivery)
		}
	}

	bob := byURL["https://bob.example"]
	if bob.DisplayName != "Bob's Box" {
		t.Errorf("bob displayName: got %q, want Bob's Box (US-1.4 AC2)", bob.DisplayName)
	}
	if bob.Status != "active" {
		t.Errorf("bob status: got %q, want active", bob.Status)
	}
	if bob.Permissions != "write" {
		t.Errorf("bob permissions: got %q, want write", bob.Permissions)
	}
	if bob.LastContactAt == "" {
		t.Errorf("bob lastContactAt: got empty, want a timestamp (US-1.4 AC1)")
	}
	if byURL["https://carol.example"].Status != "stale" {
		t.Errorf("carol status: got %q, want stale (US-1.4 AC3)", byURL["https://carol.example"].Status)
	}
}

// TestListPeers_EmptyWhenNoPeers asserts a federated project with only the owner
// self-row returns an empty array (not the self-row).
func TestListPeers_EmptyWhenNoPeers(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers", p.ID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list peers: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var peers []dto.PeerDTO
	if err := json.Unmarshal(body, &peers); err != nil {
		t.Fatalf("parse peers: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("peer count: got %d, want 0", len(peers))
	}
	// Defense-in-depth: the self-row instance_url must not appear in the body.
	if strings.Contains(string(body), "http://test") {
		t.Errorf("self-row instance_url leaked: %s", body)
	}
}

// TestListPeers_ProjectNotFound asserts an unknown project → 404.
func TestListPeers_ProjectNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		"/api/v1/projects/99999/federation/peers", nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("list peers unknown project: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

// peerStatus loads the current paused/derived status of one peer from the
// GET .../federation/peers list (Federation v1 F5.3, US-6.1 AC3).
func peerStatus(t *testing.T, e *apiEnv, projectID int64, peerURL string) string {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers", projectID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list peers: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var peers []dto.PeerDTO
	if err := json.Unmarshal(body, &peers); err != nil {
		t.Fatalf("parse peers: %v", err)
	}
	for _, p := range peers {
		if p.InstanceURL == peerURL {
			return p.Status
		}
	}
	t.Fatalf("peer %q not found in list", peerURL)
	return ""
}

// TestPausePeer_SetsPausedStatus asserts POST .../peers/pause flips the peer's
// status to paused (Federation v1 F5.3, US-6.1 AC1) and the peers list reflects
// it (US-6.1 AC3). The peer URL rides in the body, not the path.
func TestPausePeer_SetsPausedStatus(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedPeerVia(t, e, p.ID, "https://bob.example", "Bob", &recent, false, false)

	if got := peerStatus(t, e, p.ID, "https://bob.example"); got != "active" {
		t.Fatalf("status before pause: got %q, want active", got)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers/pause", p.ID),
		dto.PausePeerRequest{InstanceURL: "https://bob.example"}))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("pause peer: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	if got := peerStatus(t, e, p.ID, "https://bob.example"); got != "paused" {
		t.Errorf("status after pause: got %q, want paused (US-6.1 AC1/AC3)", got)
	}
}

// TestResumePeer_ClearsPausedStatus asserts POST .../peers/resume flips a paused
// peer back to active (Federation v1 F5.3, US-6.1 AC2).
func TestResumePeer_ClearsPausedStatus(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedPeerVia(t, e, p.ID, "https://bob.example", "Bob", &recent, true /*paused*/, false)

	if got := peerStatus(t, e, p.ID, "https://bob.example"); got != "paused" {
		t.Fatalf("status before resume: got %q, want paused", got)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers/resume", p.ID),
		dto.PausePeerRequest{InstanceURL: "https://bob.example"}))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("resume peer: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	if got := peerStatus(t, e, p.ID, "https://bob.example"); got != "active" {
		t.Errorf("status after resume: got %q, want active (US-6.1 AC2)", got)
	}
}

// TestPausePeer_UnknownPeer asserts pausing a peer that is not joined → 404.
func TestPausePeer_UnknownPeer(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers/pause", p.ID),
		dto.PausePeerRequest{InstanceURL: "https://nobody.example"}))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("pause unknown peer: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

// TestPausePeer_ProjectNotFound asserts pause on an unknown project → 404.
func TestPausePeer_ProjectNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/projects/99999/federation/peers/pause",
		dto.PausePeerRequest{InstanceURL: "https://bob.example"}))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("pause unknown project: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

// TestPausePeer_MissingInstanceURL asserts a pause body with no instanceUrl → 400.
func TestPausePeer_MissingInstanceURL(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers/pause", p.ID),
		dto.PausePeerRequest{InstanceURL: ""}))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("pause empty instanceUrl: got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

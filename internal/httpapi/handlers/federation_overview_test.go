package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
)

func getOverview(t *testing.T, e *apiEnv) dto.OverviewResponseDTO {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/federation/overview", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overview: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var out dto.OverviewResponseDTO
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v; body: %s", err, body)
	}
	return out
}

// TestFederationOverview_RoleAndPeerList asserts GET /federation/overview returns
// each federated project's role + named peer list, and excludes non-federated
// projects (Federation v1 F6.4, US-7.1 AC1).
func TestFederationOverview_RoleAndPeerList(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	shared := createTestProject(t, e, ctx.ID, "Shared")
	enableFederation(t, e, shared.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedStatusPeer(t, e, shared.ID, "https://bob.example", &recent)

	// A second, plain project must NOT appear in the overview.
	_ = createTestProject(t, e, ctx.ID, "Local Only")

	out := getOverview(t, e)
	if len(out.Projects) != 1 {
		t.Fatalf("overview projects: got %d, want 1 (non-federated excluded, US-7.1 AC1); body=%+v", len(out.Projects), out)
	}
	row := out.Projects[0]
	if row.ProjectId != shared.ID {
		t.Errorf("project id: got %d, want %d", row.ProjectId, shared.ID)
	}
	if row.Role != string(model.FederationRoleOwner) {
		t.Errorf("role: got %q, want owner (US-7.1 AC1)", row.Role)
	}
	if len(row.Peers) != 1 || row.Peers[0].InstanceUrl != "https://bob.example" {
		t.Fatalf("peers: got %+v, want one bob peer (US-7.1 AC1)", row.Peers)
	}
	if row.Peers[0].DisplayName != "Peer" {
		t.Errorf("peer displayName: got %q, want Peer (US-7.1 AC1)", row.Peers[0].DisplayName)
	}
}

// TestProjectDTO_PeerInstancesExposed asserts the project DTO carries the per-
// project peerInstances array (named instances, not a bare count) so the new-task
// editor hint and the "visible to N peers" task badge can read it locally without
// an extra round-trip (Federation v1 F6.4, US-7.1 AC3 data contract). Resolved
// once at bootstrap; the count badge reads peerInstances.length.
func TestProjectDTO_PeerInstancesExposed(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	shared := createTestProject(t, e, ctx.ID, "Shared")
	enableFederation(t, e, shared.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedStatusPeer(t, e, shared.ID, "https://bob.example", &recent)
	seedStatusPeer(t, e, shared.ID, "https://carol.example", &recent)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d", shared.ID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get project: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var got dto.ProjectDTO
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.PeerInstances) != 2 {
		t.Fatalf("peerInstances: got %d, want 2 (US-7.1 AC3), got=%+v", len(got.PeerInstances), got.PeerInstances)
	}
	urls := map[string]bool{}
	for _, pi := range got.PeerInstances {
		urls[pi.InstanceUrl] = true
		if pi.DisplayName == "" {
			t.Errorf("peer %q has empty displayName (US-7.1 AC3 needs the named list)", pi.InstanceUrl)
		}
	}
	if !urls["https://bob.example"] || !urls["https://carol.example"] {
		t.Errorf("peerInstances urls: got %+v, want bob + carol", urls)
	}
}

// TestProjectDTO_NonFederatedNoPeers asserts a non-federated project exposes an
// empty peerInstances array (Federation v1 F6.4): the badge/hint then render
// nothing.
func TestProjectDTO_NonFederatedNoPeers(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	plain := createTestProject(t, e, ctx.ID, "Local")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d", plain.ID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get project: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var got dto.ProjectDTO
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.PeerInstances) != 0 {
		t.Errorf("non-federated peerInstances: got %d, want 0", len(got.PeerInstances))
	}
}

// TestTaskDTO_FederatedAndVisibleToPeers asserts a task in a federated project
// carries federated=true + visibleToPeers=N (the audience count), so the
// "federated, visible to N peers" header badge renders (Federation v1 F6.4,
// US-7.1 AC2). A task in a plain project carries federated=false / 0.
func TestTaskDTO_FederatedAndVisibleToPeers(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	shared := createTestProject(t, e, ctx.ID, "Shared")
	enableFederation(t, e, shared.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedStatusPeer(t, e, shared.ID, "https://bob.example", &recent)

	created := createTaskInProject(t, e, shared.ID, "Shared task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/tasks/%d", created.ID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get task: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var got dto.TaskDTO
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.Federated {
		t.Errorf("federated: got false, want true (US-7.1 AC2)")
	}
	if got.VisibleToPeers != 1 {
		t.Errorf("visibleToPeers: got %d, want 1 (US-7.1 AC2)", got.VisibleToPeers)
	}
}

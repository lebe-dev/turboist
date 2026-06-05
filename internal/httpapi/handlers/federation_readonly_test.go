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

// seedJoinedProject marks a local project as a read-only / write joined federated
// project: it flips is_federated and inserts a federated_projects row mapping the
// local project to its owner instance with is_owner=0 and the granted permission
// (Federation v1 F2.4). This is the joiner-side state the F2.3 bootstrap leaves
// behind, reproduced here so the F2.4 surface + guard can be exercised without a
// full two-instance handshake.
func seedJoinedProject(t *testing.T, e *apiEnv, projectID int64, ownerURL string, perm model.FederationPermission) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := e.db.ExecContext(ctx,
		`UPDATE projects SET is_federated = 1 WHERE id = ?`, projectID); err != nil {
		t.Fatalf("mark federated: %v", err)
	}
	if err := e.fedInstances.Upsert(ctx, model.FederatedInstance{
		InstanceURL: ownerURL,
		PublicKey:   "owner-pk",
		DisplayName: "Owner Box",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed owner instance: %v", err)
	}
	if err := e.fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    projectID,
		PeerInstanceURL:   ownerURL,
		RemoteProjectID:   "owner-cid",
		IsOwner:           false,
		OriginInstanceURL: ownerURL,
		Permissions:       perm,
		ProtocolVersion:   1,
		LastReceivedHLC:   "00000000000000-0000-node",
		JoinedAt:          now,
	}); err != nil {
		t.Fatalf("seed joiner row: %v", err)
	}
}

func getProjectDTO(t *testing.T, e *apiEnv, projectID int64) dto.ProjectDTO {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d", projectID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get project: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var p dto.ProjectDTO
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("parse project: %v", err)
	}
	return p
}

// TestProjectGet_FederationFieldsJoinerReadOnly asserts the project DTO carries
// the federation surface (originInstance, federationPermissions, isOwner) for a
// joined read-only project (Federation v1 F2.4, US-2.4 AC1, AC2).
func TestProjectGet_FederationFieldsJoinerReadOnly(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared from owner")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionRead)

	got := getProjectDTO(t, e, p.ID)
	if !got.IsFederated {
		t.Errorf("isFederated: got false, want true")
	}
	if got.IsOwner {
		t.Errorf("isOwner: got true, want false (joiner)")
	}
	if got.OriginInstance == nil || *got.OriginInstance != "https://owner.example" {
		t.Errorf("originInstance: got %v, want https://owner.example", got.OriginInstance)
	}
	if got.FederationPermissions == nil || *got.FederationPermissions != "read" {
		t.Errorf("federationPermissions: got %v, want read", got.FederationPermissions)
	}
}

// TestProjectList_FederationFields asserts the list endpoint enriches every
// project with its federation surface in one round-trip (Federation v1 F2.4,
// US-2.4 AC1). A non-federated project leaves the federation fields null.
func TestProjectList_FederationFields(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	plain := createTestProject(t, e, ctx.ID, "Plain")
	joined := createTestProject(t, e, ctx.ID, "Joined")
	seedJoinedProject(t, e, joined.ID, "https://owner.example", model.FederationPermissionRead)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/projects/", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var page dto.PagedResponse[dto.ProjectDTO]
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	byID := map[int64]dto.ProjectDTO{}
	for _, p := range page.Items {
		byID[p.ID] = p
	}
	if pj := byID[joined.ID]; pj.OriginInstance == nil || *pj.OriginInstance != "https://owner.example" || pj.FederationPermissions == nil || *pj.FederationPermissions != "read" {
		t.Errorf("joined project: got origin=%v perms=%v, want owner.example/read", pj.OriginInstance, pj.FederationPermissions)
	}
	if pp := byID[plain.ID]; pp.OriginInstance != nil || pp.FederationPermissions != nil {
		t.Errorf("plain project: got origin=%v perms=%v, want both nil", pp.OriginInstance, pp.FederationPermissions)
	}
}

// TestProjectMutations_ReadOnlyFederated403 asserts that every project mutation
// against a joined read-only federated project is rejected 403
// federation_read_only — the authoritative backend enforcement seam (Federation
// v1 F2.4, US-2.4 AC4, §9.2). UI disabling is insufficient.
func TestProjectMutations_ReadOnlyFederated403(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Read only")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionRead)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"patch", http.MethodPatch, fmt.Sprintf("/api/v1/projects/%d", p.ID), map[string]any{"title": "renamed"}},
		{"delete", http.MethodDelete, fmt.Sprintf("/api/v1/projects/%d", p.ID), nil},
		{"createTask", http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/tasks", p.ID), map[string]any{"title": "t"}},
		{"createSection", http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/sections", p.ID), map[string]any{"title": "s"}},
		{"pin", http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/pin", p.ID), nil},
		{"unpin", http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/unpin", p.ID), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doReq(t, e.app, e.authedReq(t, tc.method, tc.path, tc.body))
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("%s: got %d, want 403; body: %s", tc.name, resp.StatusCode, body)
			}
			var env struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if env.Error.Code != "federation_read_only" {
				t.Errorf("%s code: got %q, want federation_read_only", tc.name, env.Error.Code)
			}
		})
	}
}

// TestProjectMutations_WriteFederatedSucceeds asserts a joined WRITE-permission
// federated project is NOT locked: its mutations go through (Federation v1 F2.4,
// US-2.4 AC3 write leg). Only read-only peers are blocked.
func TestProjectMutations_WriteFederatedSucceeds(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Writable")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionWrite)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", p.ID), map[string]any{"title": "renamed"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("write-project patch: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// TestProjectMutations_OwnerNotLocked asserts the owner's own federated project
// (is_owner=1, admin) is never read-only — owner controls stay enabled
// (Federation v1 F2.4 risk note; US-2.4 AC3/AC4 don't disable owner).
func TestProjectMutations_OwnerNotLocked(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Mine")
	enableFederationOn(t, e, p.ID) // writes the is_owner=1 self-row

	got := getProjectDTO(t, e, p.ID)
	if !got.IsOwner {
		t.Errorf("isOwner: got false, want true (owner self-row)")
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", p.ID), map[string]any{"title": "renamed"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner patch: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// TestProjectMutations_NonFederatedSucceeds asserts the guard is a no-op for a
// plain project (Federation v1 F2.4): the read-only seam must never affect the
// non-federated 99% of usage.
func TestProjectMutations_NonFederatedSucceeds(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Plain")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", p.ID), map[string]any{"title": "renamed"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plain patch: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

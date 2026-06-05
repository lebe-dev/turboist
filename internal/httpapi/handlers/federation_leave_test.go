package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
)

// TestLeaveProject_MarksLocalCopyLeft asserts POST .../federation/leave on a
// JOINED federated copy marks the local copy federation_lost with reason="left",
// so it becomes a plain editable local project (Federation v1 F5.5, US-6.3
// AC1/AC3). The project DTO reflects the lost-left surface afterwards.
func TestLeaveProject_MarksLocalCopyLeft(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Joined")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionWrite)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/leave", p.ID), nil))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("leave: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	dtoP := getProjectDTO(t, e, p.ID)
	if !dtoP.FederationLost {
		t.Errorf("federationLost after leave: got false, want true (US-6.3 AC1)")
	}
	if dtoP.FederationLostReason == nil || *dtoP.FederationLostReason != string(model.FederationLostLeft) {
		got := "<nil>"
		if dtoP.FederationLostReason != nil {
			got = *dtoP.FederationLostReason
		}
		t.Errorf("federationLostReason: got %q, want left (US-6.3 AC1)", got)
	}
}

// TestLeaveProject_Idempotent asserts re-leaving an already-left project is a
// no-op success (204) — the reason stays "left" and nothing else changes.
func TestLeaveProject_Idempotent(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Joined")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionRead)

	for i := 0; i < 2; i++ {
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
			fmt.Sprintf("/api/v1/projects/%d/federation/leave", p.ID), nil))
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("leave #%d: got %d, want 204; body: %s", i, resp.StatusCode, body)
		}
	}
	dtoP := getProjectDTO(t, e, p.ID)
	if dtoP.FederationLostReason == nil || *dtoP.FederationLostReason != string(model.FederationLostLeft) {
		t.Errorf("federationLostReason after double leave: want left")
	}
}

// TestLeaveProject_ReadOnlyCopyBecomesEditable asserts a READ-only joined copy that
// is left is no longer read-only on the federation axis (US-6.3 AC3 — "available
// for local editing"). The project surface drops the read-only flag because a
// lost-left copy is a plain local project (isReadOnly is false for reason="left").
func TestLeaveProject_ReadOnlyCopyBecomesEditable(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "ReadOnly joined")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionRead)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/leave", p.ID), nil))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("leave: got %d, want 204; body: %s", resp.StatusCode, body)
	}
	dtoP := getProjectDTO(t, e, p.ID)
	// The local copy is now lost-left; the read-only reason is "left" which is NOT a
	// read-only reason — the project is editable again.
	if !dtoP.FederationLost || dtoP.FederationLostReason == nil || *dtoP.FederationLostReason != string(model.FederationLostLeft) {
		t.Errorf("after leave the copy must be lost-left (editable): got lost=%v reason=%v", dtoP.FederationLost, dtoP.FederationLostReason)
	}
}

// TestLeaveProject_OwnerOwnProjectRejected asserts the owner of a project cannot
// "leave" their OWN federated project (only peers leave; the owner revokes peers).
// It maps to a 409 conflict (not-joined).
func TestLeaveProject_OwnerOwnProjectRejected(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Owned")
	enableFederationOn(t, e, p.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/leave", p.ID), nil))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("leave own project: got %d, want 409; body: %s", resp.StatusCode, body)
	}
}

// TestLeaveProject_NonFederatedRejected asserts leaving a plain non-federated
// project is rejected as not-joined (409).
func TestLeaveProject_NonFederatedRejected(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Local")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/leave", p.ID), nil))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("leave non-federated: got %d, want 409; body: %s", resp.StatusCode, body)
	}
}

// TestLeaveProject_UnknownProject asserts leaving an unknown project → 404.
func TestLeaveProject_UnknownProject(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/projects/99999/federation/leave", nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("leave unknown project: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

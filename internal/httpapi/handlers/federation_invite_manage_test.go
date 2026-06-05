package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

// enableFederationOn enables federation on a project via the API and fails the
// test if the call does not return 200.
func enableFederationOn(t *testing.T, e *apiEnv, projectID int64) {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/enable", projectID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enable federation: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// createInviteVia creates an invite through the API and returns the response.
func createInviteVia(t *testing.T, e *apiEnv, projectID int64, perms string) dto.CreateInviteResponse {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/invites", projectID),
		map[string]any{"permissions": perms}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create invite: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var got dto.CreateInviteResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse create invite: %v", err)
	}
	return got
}

// TestListInvites_ReturnsStatusAndNoSecret asserts the list endpoint returns the
// invite metadata + derived status and NEVER serves the secret (US-1.3 AC1, AC5).
func TestListInvites_ReturnsStatusAndNoSecret(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	created := createInviteVia(t, e, p.ID, "write")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d/invites", p.ID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list invites: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	var list []dto.InviteDTO
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("invite count: got %d, want 1", len(list))
	}
	got := list[0]
	if got.InviteID != created.InviteID {
		t.Errorf("inviteId: got %q, want %q", got.InviteID, created.InviteID)
	}
	if got.Status != "active" {
		t.Errorf("status: got %q, want active", got.Status)
	}
	if got.Permissions != "write" {
		t.Errorf("permissions: got %q, want write", got.Permissions)
	}
	if got.MaxUses != 1 {
		t.Errorf("maxUses: got %d, want 1", got.MaxUses)
	}

	// US-1.3 AC5: the secret and its hash must NOT appear anywhere in the list
	// response. Assert against the raw JSON to catch any leaked field.
	raw := string(body)
	if strings.Contains(raw, created.Secret) {
		t.Errorf("list response leaked the plaintext secret")
	}
	if strings.Contains(strings.ToLower(raw), "secret") {
		t.Errorf("list response contains a secret field: %s", raw)
	}
}

// TestRevokeInvite_Idempotent asserts POST .../revoke flips the derived status to
// revoked and a second revoke is a no-op 204 (US-1.3 AC2).
func TestRevokeInvite_Idempotent(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)
	created := createInviteVia(t, e, p.ID, "write")

	for i := 0; i < 2; i++ {
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
			fmt.Sprintf("/api/v1/projects/%d/invites/%s/revoke", p.ID, created.InviteID), nil))
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("revoke #%d: got %d, want 204; body: %s", i, resp.StatusCode, body)
		}
	}

	// The derived status is now revoked.
	stored, err := e.fedInvites.Get(context.Background(), created.InviteID)
	if err != nil {
		t.Fatalf("get stored: %v", err)
	}
	if stored.RevokedAt == nil {
		t.Errorf("revoked_at not set after revoke")
	}
}

// TestRevokeInvite_NotFound asserts revoking an unknown invite returns 404.
func TestRevokeInvite_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/invites/does-not-exist/revoke", p.ID), nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoke unknown: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

// TestDeleteInvite_RemovesRow asserts DELETE .../invites/:id hard-removes the
// invite and that the row is gone from the list afterwards (US-1.3 AC3).
func TestDeleteInvite_RemovesRow(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)
	created := createInviteVia(t, e, p.ID, "write")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%d/invites/%s", p.ID, created.InviteID), nil))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d/invites", p.ID), nil))
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list after delete: got %d, want 200; body: %s", resp2.StatusCode, body2)
	}
	var list []dto.InviteDTO
	if err := json.Unmarshal(body2, &list); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("invite count after delete: got %d, want 0", len(list))
	}
}

// TestDeleteInvite_NotFound asserts deleting an unknown invite returns 404.
func TestDeleteInvite_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%d/invites/missing", p.ID), nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete unknown: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

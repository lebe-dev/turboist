package handlers_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

// TestRevokePeer_SetsRevokedStatus asserts DELETE .../federation/peers flips the
// peer's status to revoked (Federation v1 F5.4, US-6.2 AC1) and the peers list
// reflects it. The peer URL rides in the body, not the path.
func TestRevokePeer_SetsRevokedStatus(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedPeerVia(t, e, p.ID, "https://bob.example", "Bob", &recent, false, false)

	if got := peerStatus(t, e, p.ID, "https://bob.example"); got != "active" {
		t.Fatalf("status before revoke: got %q, want active", got)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers", p.ID),
		dto.RevokePeerRequest{InstanceURL: "https://bob.example"}))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke peer: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	if got := peerStatus(t, e, p.ID, "https://bob.example"); got != "revoked" {
		t.Errorf("status after revoke: got %q, want revoked (US-6.2 AC1)", got)
	}
}

// TestRevokePeer_Idempotent asserts re-revoking an already-revoked peer is a
// no-op success (204) — the revoke event is deduped and the flag stays set.
func TestRevokePeer_Idempotent(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedPeerVia(t, e, p.ID, "https://bob.example", "Bob", &recent, false, true /*already revoked*/)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers", p.ID),
		dto.RevokePeerRequest{InstanceURL: "https://bob.example"}))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("re-revoke peer: got %d, want 204; body: %s", resp.StatusCode, body)
	}
	if got := peerStatus(t, e, p.ID, "https://bob.example"); got != "revoked" {
		t.Errorf("status after re-revoke: got %q, want revoked", got)
	}
}

// TestRevokePeer_UnknownPeer asserts revoking a peer that is not joined → 404.
func TestRevokePeer_UnknownPeer(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers", p.ID),
		dto.RevokePeerRequest{InstanceURL: "https://nobody.example"}))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoke unknown peer: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

// TestRevokePeer_ProjectNotFound asserts revoke on an unknown project → 404.
func TestRevokePeer_ProjectNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		"/api/v1/projects/99999/federation/peers",
		dto.RevokePeerRequest{InstanceURL: "https://bob.example"}))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoke unknown project: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

// TestRevokePeer_MissingInstanceURL asserts a revoke body with no instanceUrl → 400.
func TestRevokePeer_MissingInstanceURL(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers", p.ID),
		dto.RevokePeerRequest{InstanceURL: ""}))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("revoke empty instanceUrl: got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

// TestResumePeer_RevokedRejected asserts resuming a REVOKED peer is rejected with
// 403 federation_revoked and does NOT clear the revoked state — revoke is
// irreversible (Federation v1 F5.4, US-6.2 AC5).
func TestResumePeer_RevokedRejected(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)
	recent := time.Now().Add(-1 * time.Hour)
	seedPeerVia(t, e, p.ID, "https://bob.example", "Bob", &recent, true /*paused*/, true /*revoked*/)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers/resume", p.ID),
		dto.PausePeerRequest{InstanceURL: "https://bob.example"}))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("resume revoked peer: got %d, want 403; body: %s", resp.StatusCode, body)
	}
	if got := peerStatus(t, e, p.ID, "https://bob.example"); got != "revoked" {
		t.Errorf("status after rejected resume: got %q, want revoked (US-6.2 AC5)", got)
	}
}

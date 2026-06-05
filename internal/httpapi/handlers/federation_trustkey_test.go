package handlers_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

func trustKeyB64(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(pub)
}

// TestTrustKey_FetchesNewKeyClearsIncident is the F5.6b US-6.4 AC3 handler path:
// POST .../peers/trust-key fetches the peer's new .well-known key, clears the
// sticky key_mismatch marker, and resolves the incident → 204. A follow-up GET
// .../peers shows the peer's keyMismatchAt back to empty.
func TestTrustKey_FetchesNewKeyClearsIncident(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	recent := time.Now().Add(-1 * time.Hour)
	seedPeerVia(t, e, p.ID, "https://bob.example", "Bob", &recent, false, false)
	// A key change was detected: the marker + an open incident are recorded.
	if _, err := e.fedProjects.MarkKeyMismatch(context.Background(), p.ID, "https://bob.example", "2026-06-03T10:00:00.000Z"); err != nil {
		t.Fatalf("mark mismatch: %v", err)
	}
	if _, err := e.fedIncidents.RecordKeyChange(context.Background(), p.ID, "https://bob.example", "pk", time.Now()); err != nil {
		t.Fatalf("record incident: %v", err)
	}

	// The peer's "new" key is served by the stubbed .well-known fetcher.
	newKey := trustKeyB64(t)
	*e.fedFetch = func(context.Context, string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: "https://bob.example", PublicKey: newKey, DisplayName: "Bob"}, nil
	}

	resp, rbody := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers/trust-key", p.ID),
		dto.TrustPeerKeyRequest{InstanceURL: "https://bob.example"}))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("trust-key: got %d, want 204; body: %s", resp.StatusCode, rbody)
	}

	// The durable pinned key was overwritten + the marker cleared (visible via GET).
	inst, err := e.fedInstances.Get(context.Background(), "https://bob.example")
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if inst.PublicKey != newKey {
		t.Errorf("durable key after trust: got %q, want the new key", inst.PublicKey)
	}

	resp, lbody := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers", p.ID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list peers: got %d, want 200; body: %s", resp.StatusCode, lbody)
	}
	var peers []dto.PeerDTO
	if err := json.Unmarshal(lbody, &peers); err != nil {
		t.Fatalf("parse peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peer count: got %d, want 1", len(peers))
	}
	if peers[0].KeyMismatchAt != "" {
		t.Errorf("keyMismatchAt after trust: got %q, want empty (incident cleared)", peers[0].KeyMismatchAt)
	}
}

// TestListPeers_SurfacesKeyMismatchAt asserts a peer with a detected key change
// surfaces keyMismatchAt on the peers list so the UI can render the incident alert
// (Federation v1 F5.6b, US-6.4 AC2). A healthy peer's keyMismatchAt is empty.
func TestListPeers_SurfacesKeyMismatchAt(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	recent := time.Now().Add(-1 * time.Hour)
	seedPeerVia(t, e, p.ID, "https://healthy.example", "Healthy", &recent, false, false)
	seedPeerVia(t, e, p.ID, "https://rotated.example", "Rotated", &recent, false, false)
	if _, err := e.fedProjects.MarkKeyMismatch(context.Background(), p.ID, "https://rotated.example", "2026-06-03T10:00:00.000Z"); err != nil {
		t.Fatalf("mark mismatch: %v", err)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers", p.ID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list peers: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var peers []dto.PeerDTO
	if err := json.Unmarshal(body, &peers); err != nil {
		t.Fatalf("parse peers: %v", err)
	}
	byURL := map[string]dto.PeerDTO{}
	for _, pr := range peers {
		byURL[pr.InstanceURL] = pr
	}
	if got := byURL["https://rotated.example"].KeyMismatchAt; got != "2026-06-03T10:00:00.000Z" {
		t.Errorf("rotated peer keyMismatchAt: got %q, want the stamped timestamp", got)
	}
	if got := byURL["https://healthy.example"].KeyMismatchAt; got != "" {
		t.Errorf("healthy peer keyMismatchAt: got %q, want empty", got)
	}
}

// TestTrustKey_UnknownPeer404 asserts trusting a peer not joined to the project is
// a 404 and never reaches the fetcher.
func TestTrustKey_UnknownPeer404(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	var fetched int
	*e.fedFetch = func(context.Context, string) (*peerkeys.Instance, error) {
		fetched++
		return &peerkeys.Instance{InstanceURL: "x", PublicKey: trustKeyB64(t)}, nil
	}

	resp, rbody := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers/trust-key", p.ID),
		dto.TrustPeerKeyRequest{InstanceURL: "https://stranger.example"}))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("trust-key unknown peer: got %d, want 404; body: %s", resp.StatusCode, rbody)
	}
	if fetched != 0 {
		t.Errorf("fetches for unknown peer: got %d, want 0", fetched)
	}
}

// TestTrustKey_MissingInstanceURL400 asserts an empty instanceUrl body is a 400.
func TestTrustKey_MissingInstanceURL400(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Shared")
	enableFederationOn(t, e, p.ID)

	resp, rbody := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/federation/peers/trust-key", p.ID),
		dto.TrustPeerKeyRequest{InstanceURL: ""}))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("trust-key empty url: got %d, want 400; body: %s", resp.StatusCode, rbody)
	}
}

package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/handshake"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// errorsIsKeyMismatch reports whether err is the owner key-pinning rejection.
func errorsIsKeyMismatch(err error) bool {
	return errors.Is(err, fedsvc.ErrHandshakeKeyMismatch)
}

const (
	ownerURL  = "https://alice.example"
	joinerURL = "https://bob.example"
)

// TestHandshake_ValidJoinCreatesRowsAndConsumesInvite asserts a signed handshake
// (joiner Ed25519, verified by the owner — US-2.2 AC1) creates the peer rows on
// both sides and consumes the single-use invite (US-2.2 AC3 / US-1.2 AC3). The
// owner key the joiner returns is corroborated against the owner .well-known
// (US-2.2 AC2) and the cache is warmed (US-2.2 AC6).
func TestHandshake_ValidJoinCreatesRowsAndConsumesInvite(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)

	inv, projectID := owner.enableAndInvite(t, model.FederationPermissionWrite)

	resp, body := joiner.join(t, ownerURL, inv)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("join: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var got dto.JoinResultResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode join result: %v (%s)", err, body)
	}
	if got.ProjectID != projectID {
		t.Errorf("join projectId: got %d, want %d", got.ProjectID, projectID)
	}
	if got.ProjectName != "Roadmap" {
		t.Errorf("join projectName: got %q, want Roadmap", got.ProjectName)
	}
	if got.Permissions != string(model.FederationPermissionWrite) {
		t.Errorf("join permissions: got %q, want write", got.Permissions)
	}

	// Owner side: the peer row + instance directory row exist, display_name and
	// key persisted (US-2.2 display_name round-trip).
	ctx := context.Background()
	peer, err := owner.fedProjects.Get(ctx, projectID, joinerURL)
	if err != nil {
		t.Fatalf("owner peer row missing: %v", err)
	}
	if peer.IsOwner {
		t.Error("owner peer row should be is_owner=0")
	}
	if peer.Permissions != model.FederationPermissionWrite {
		t.Errorf("owner peer permissions: got %q, want write", peer.Permissions)
	}
	inst, err := owner.fedInstances.Get(ctx, joinerURL)
	if err != nil {
		t.Fatalf("owner instance directory row missing: %v", err)
	}
	if inst.DisplayName != "bob.example" {
		t.Errorf("joiner display_name persisted on owner: got %q, want bob.example", inst.DisplayName)
	}
	if inst.PublicKey == "" {
		t.Error("joiner public key not persisted on owner")
	}

	// The single-use invite is now consumed: a second join → 409 (consumed).
	resp2, body2 := joiner.join(t, ownerURL, inv)
	if resp2.StatusCode == http.StatusOK {
		t.Fatalf("second use of single-use invite unexpectedly succeeded: %s", body2)
	}

	// Joiner side: the owner identity persisted (key + display_name) — US-2.2 AC2/AC6.
	ownerInst, err := joiner.fedInstances.Get(ctx, ownerURL)
	if err != nil {
		t.Fatalf("joiner did not persist owner instance: %v", err)
	}
	if ownerInst.DisplayName != "alice.example" {
		t.Errorf("owner display_name on joiner: got %q, want alice.example", ownerInst.DisplayName)
	}
	ownerKeys, _ := owner.keys.Get(ctx)
	if ownerInst.PublicKey != ownerKeys.PublicKey {
		t.Errorf("joiner stored owner key %q, want %q", ownerInst.PublicKey, ownerKeys.PublicKey)
	}
}

// TestHandshake_SecondUseRejected asserts a single-use invite cannot be consumed
// twice: the second handshake is rejected (US-1.2 AC3 / US-2.2 AC3) and no second
// peer relationship is recorded.
func TestHandshake_SecondUseRejected(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)

	inv, projectID := owner.enableAndInvite(t, model.FederationPermissionRead)
	if resp, body := joiner.join(t, ownerURL, inv); resp.StatusCode != http.StatusOK {
		t.Fatalf("first join: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	resp, body := joiner.join(t, ownerURL, inv)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("second join unexpectedly succeeded: %s", body)
	}

	// The invite is now derived-status "consumed".
	views, err := owner.svc.ListInvites(context.Background(), projectID)
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(views) != 1 || views[0].Status != model.InviteStatusConsumed {
		t.Errorf("invite status after consume: got %+v, want consumed", views)
	}
}

// TestHandshake_ConcurrentSingleUseConsumesOnce asserts the single-use invariant
// holds under a concurrent TOCTOU race (US-1.2 AC3 / US-2.2 AC3): two handshakes
// presenting the SAME leaked single-use secret from DISTINCT joiner instance_urls
// (so the federated_projects ON CONFLICT key (local_project_id, peer_instance_url)
// does NOT collide and cannot mask over-consumption) race the same invite. Exactly
// one must win — exactly one used_count increment and exactly one peer row — even
// though both can read used_count=0 before either consumes. The loser fails (it
// does not silently create a second peer row). This is the gap the SEQUENTIAL
// TestHandshake_SecondUseRejected does not cover.
func TestHandshake_ConcurrentSingleUseConsumesOnce(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)

	inv, projectID := owner.enableAndInvite(t, model.FederationPermissionWrite)

	// Two DISTINCT joiner identities (different URL + key) presenting the same
	// single-use secret. Driving the owner Handshake service directly is the
	// deterministic way to overlap the non-transactional consumability check.
	const (
		urlA = "https://joiner-a.example"
		urlB = "https://joiner-b.example"
		keyA = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
		keyB = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="
	)
	now := timeNowUTC()
	doHandshake := func(url, key string) error {
		_, err := owner.svc.Handshake(context.Background(), fedsvc.HandshakeInput{
			Body: handshake.Request{
				InviteID:          inv.InviteID,
				Secret:            inv.Secret,
				JoinerInstanceURL: url,
				JoinerPublicKey:   key,
				JoinerDisplayName: url,
				ProtocolVersions:  []int{1},
			},
			VerifiedPeerURL: url,
			VerifiedPeerKey: key,
		}, now)
		return err
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, jc := range []struct{ url, key string }{{urlA, keyA}, {urlB, keyB}} {
		wg.Add(1)
		go func(url, key string) {
			defer wg.Done()
			<-start // release both as close together as possible
			results <- doHandshake(url, key)
		}(jc.url, jc.key)
	}
	close(start)
	wg.Wait()
	close(results)

	var okCount, failCount int
	for err := range results {
		if err == nil {
			okCount++
			continue
		}
		failCount++
		// The loser must be the generic invalid handshake (no disclosure, AC4),
		// not some unexpected internal error.
		if !errors.Is(err, fedsvc.ErrHandshakeInvalid) {
			t.Errorf("concurrent loser: got %v, want ErrHandshakeInvalid", err)
		}
	}
	if okCount != 1 || failCount != 1 {
		t.Fatalf("concurrent single-use: got ok=%d fail=%d, want ok=1 fail=1", okCount, failCount)
	}

	ctx := context.Background()

	// Exactly one used_count increment — never bumped to 2.
	views, err := owner.svc.ListInvites(ctx, projectID)
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("invite count: got %d, want 1", len(views))
	}
	if views[0].UsedCount != 1 {
		t.Errorf("used_count after concurrent race: got %d, want 1", views[0].UsedCount)
	}
	if views[0].Status != model.InviteStatusConsumed {
		t.Errorf("invite status after concurrent race: got %q, want consumed", views[0].Status)
	}

	// Exactly one peer row — the loser created no second federated_projects row.
	peers, err := owner.fedProjects.ListPeersByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peer rows after concurrent race: got %d, want 1", len(peers))
	}
}

// TestHandshake_WrongSecretGeneric401 asserts a wrong secret is a GENERIC 401 at
// the owner handshake endpoint — no disclosure of whether the id or the secret
// was wrong (US-2.2 AC4) — and consumes nothing.
func TestHandshake_WrongSecretGeneric401(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)

	inv, projectID := owner.enableAndInvite(t, model.FederationPermissionWrite)
	bad := fedsvc.ParsedInvite{InviteID: inv.InviteID, Secret: "not-the-secret"}

	resp, body := joiner.join(t, ownerURL, bad)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong secret: got %d, want 401; body: %s", resp.StatusCode, body)
	}
	if code := errCodeOf(t, body); code != httpapi.CodeFederationSignatureInvalid {
		t.Errorf("wrong secret code: got %q, want %q", code, httpapi.CodeFederationSignatureInvalid)
	}

	// Nothing consumed: the invite is still active and usable.
	views, _ := owner.svc.ListInvites(context.Background(), projectID)
	if len(views) != 1 || views[0].Status != model.InviteStatusActive {
		t.Errorf("invite must be untouched after wrong secret: got %+v", views)
	}
	if _, err := owner.fedProjects.Get(context.Background(), projectID, joinerURL); err == nil {
		t.Error("no peer row should exist after a wrong-secret handshake")
	}
}

// TestHandshake_UnknownInviteGeneric401 asserts an unknown invite id is the SAME
// generic 401 as a wrong secret (US-2.2 AC4 — no id-vs-secret distinction).
func TestHandshake_UnknownInviteGeneric401(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)
	owner.enableAndInvite(t, model.FederationPermissionWrite)

	resp, body := joiner.join(t, ownerURL, fedsvc.ParsedInvite{InviteID: "does-not-exist", Secret: "x"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown invite: got %d, want 401; body: %s", resp.StatusCode, body)
	}
	if code := errCodeOf(t, body); code != httpapi.CodeFederationSignatureInvalid {
		t.Errorf("unknown invite code: got %q, want %q", code, httpapi.CodeFederationSignatureInvalid)
	}
}

// TestHandshake_KeyMismatch409 asserts that a handshake from an instance_url
// already known with a DIFFERENT Ed25519 key is rejected with 409 (US-2.2 AC5) —
// the owner refuses to silently rotate a pinned peer key. The transport-layer
// fetch-once cache (F0.3) would catch an impersonation at the signature stage, so
// the handler-level key-pinning check is exercised directly against the service:
// a real join pins bob's key, then a fresh-key handshake for the same URL maps to
// ErrHandshakeKeyMismatch → 409.
func TestHandshake_KeyMismatch409(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)

	inv, projectID := owner.enableAndInvite(t, model.FederationPermissionWrite)
	if resp, body := joiner.join(t, ownerURL, inv); resp.StatusCode != http.StatusOK {
		t.Fatalf("first join: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	// A second handshake for bob's URL presenting a DIFFERENT (new) key — the
	// owner already pinned bob's key on the first join, so this is a 409.
	inv2, _ := owner.enableAndInviteOn(t, projectID, model.FederationPermissionWrite)
	const newKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	_, err := owner.svc.Handshake(context.Background(), fedsvc.HandshakeInput{
		Body: handshake.Request{
			InviteID:          inv2.InviteID,
			Secret:            inv2.Secret,
			JoinerInstanceURL: joinerURL,
			JoinerPublicKey:   newKey,
			JoinerDisplayName: "bob.example",
			ProtocolVersions:  []int{1},
		},
		VerifiedPeerURL: joinerURL,
		VerifiedPeerKey: newKey,
	}, timeNowUTC())
	if !errorsIsKeyMismatch(err) {
		t.Fatalf("key mismatch: got err %v, want ErrHandshakeKeyMismatch", err)
	}
}

// TestHandshake_KeyMismatch409OverHTTP asserts the owner endpoint returns a 409
// (not a generic 401) when a peer URL is already pinned to a different key and a
// genuinely-signed handshake arrives (US-2.2 AC5, end-to-end through the signed
// route). The peer's signature verifies (its real key is fetched fresh), so the
// request reaches the handler, which rejects on the stored-vs-presented key
// pinning.
func TestHandshake_KeyMismatch409OverHTTP(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)
	inv, _ := owner.enableAndInvite(t, model.FederationPermissionWrite)

	// Pin a DIFFERENT key for the joiner URL on the owner before the join, without
	// going through a prior handshake (so the transport key cache stays empty and
	// the joiner's real signature still verifies).
	now := timeNowUTC()
	if err := owner.fedInstances.Upsert(context.Background(), model.FederatedInstance{
		InstanceURL: joinerURL,
		PublicKey:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		DisplayName: "bob.example",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed pinned key: %v", err)
	}

	resp, body := joiner.join(t, ownerURL, inv)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("key mismatch over http: got %d, want 409; body: %s", resp.StatusCode, body)
	}
	if code := errCodeOf(t, body); code != httpapi.CodeFederationKeyMismatch {
		t.Errorf("key mismatch code: got %q, want %q", code, httpapi.CodeFederationKeyMismatch)
	}
}

// TestHandshake_NoVersionOverlap400NothingConsumed asserts that when the joiner
// advertises a version set with no overlap, the owner rejects with 400 BEFORE
// consuming the invite (US-9.1 AC2 / R23 atomicity).
func TestHandshake_NoVersionOverlap400NothingConsumed(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)

	inv, projectID := owner.enableAndInvite(t, model.FederationPermissionWrite)

	// Drive the owner handshake service directly with a non-overlapping version
	// set (the joiner endpoint always advertises the supported set, so this is the
	// deterministic way to exercise the no-overlap branch).
	ownerKeys, _ := owner.keys.Get(context.Background())
	_ = ownerKeys
	out, err := owner.handshakeDirect(t, inv, []int{99})
	if err == nil {
		t.Fatalf("expected version-unsupported error, got result %+v", out)
	}
	if views, _ := owner.svc.ListInvites(context.Background(), projectID); len(views) != 1 || views[0].Status != model.InviteStatusActive {
		t.Errorf("invite must stay active after no-version-overlap: got %+v", views)
	}
}

// TestHandshake_OwnerKeyMismatchUntrusted asserts the joiner refuses an owner
// whose handshake-returned key does not match its published .well-known key
// (US-2.2 AC2). Exercised at the service level by corrupting the corroboration
// fetch.
func TestHandshake_OwnerKeyMismatchUntrusted(t *testing.T) {
	reg := newFedRegistry()
	owner := newFedInstance(t, reg, ownerURL)
	joiner := newFedInstance(t, reg, joinerURL)
	inv, _ := owner.enableAndInvite(t, model.FederationPermissionWrite)

	// Point the joiner's well-known fetcher at a DIFFERENT key than the owner
	// actually signs with, so corroboration fails.
	joiner.svc.WithJoinDeps(reg, lyingFetcher(), nil, nil)
	_, err := joiner.svc.Join(context.Background(), ownerURL, inv, "")
	if err == nil {
		t.Fatal("expected owner-untrusted error when .well-known key disagrees")
	}
}

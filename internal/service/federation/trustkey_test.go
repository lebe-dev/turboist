package federation_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// newTrustKeySvc builds a federation service wired with the join deps (peer-key
// cache + .well-known fetcher) and the security-incident repo so TrustPeerKey can
// fetch the new key, overwrite the pinned key, clear the marker, and resolve the
// incident (Federation v1 F5.6b, US-6.4 AC3). The fetcher is a stub the test
// controls. It returns the service + the repos/cache needed to seed + inspect.
func newTrustKeySvc(t *testing.T, instanceURL string, fetch peerkeys.Fetcher) (
	*fedsvc.Service, *repo.ProjectRepo, *repo.FederatedProjectRepo, *repo.FederatedInstanceRepo,
	*repo.FederationSecurityIncidentRepo, *peerkeys.Cache,
) {
	t.Helper()
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	instances := repo.NewFederatedInstanceRepo(d)
	incidents := repo.NewFederationSecurityIncidentRepo(d)
	cache := peerkeys.NewCache(fetch)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), instances, crypto.NewTokenCipher(fedSvcKey), instanceURL).
		WithJoinDeps(nil, fetch, cache, nil).
		WithTrustKeyDeps(incidents)
	return svc, projects, fedProjects, instances, incidents, cache
}

func newKeyB64(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(pub)
}

// TestTrustPeerKey_FetchesOverwritesClearsAndResolves is the F5.6b US-6.4 AC3
// happy path: the operator clicks "Trust new key"; the service fetches the peer's
// CURRENT .well-known key, overwrites the durable + in-memory pinned key, clears
// the sticky key_mismatch marker, and resolves the open incident with the new key.
func TestTrustPeerKey_FetchesOverwritesClearsAndResolves(t *testing.T) {
	newKey := newKeyB64(t)
	var fetched int
	fetch := func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
		fetched++
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: newKey, DisplayName: "Bob"}, nil
	}
	svc, projects, fp, instances, incidents, cache := newTrustKeySvc(t, "https://me.example", fetch)
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)
	// The peer rotated its key: a mismatch was observed, stamping the marker + an
	// open incident (what the F4.3 / F5.6b inbox path records).
	if _, err := fp.MarkKeyMismatch(ctx, pid, "https://bob.example", "2026-06-03T10:00:00.000Z"); err != nil {
		t.Fatalf("mark mismatch: %v", err)
	}
	if _, err := incidents.RecordKeyChange(ctx, pid, "https://bob.example", "pk", time.Now()); err != nil {
		t.Fatalf("record incident: %v", err)
	}

	if err := svc.TrustPeerKey(ctx, pid, "https://bob.example"); err != nil {
		t.Fatalf("TrustPeerKey: %v", err)
	}

	// AC3: a .well-known fetch happened.
	if fetched != 1 {
		t.Errorf("well-known fetches: got %d, want 1 (AC3 fetches the new key)", fetched)
	}
	// AC3: the durable pinned key was overwritten with the fetched key.
	inst, err := instances.Get(ctx, "https://bob.example")
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if inst.PublicKey != newKey {
		t.Errorf("durable public_key: got %q, want the fetched new key", inst.PublicKey)
	}
	// AC3: the in-memory cache now serves the new key too.
	rk, err := cache.Resolve(ctx, "https://bob.example")
	if err != nil {
		t.Fatalf("resolve after trust: %v", err)
	}
	if base64.StdEncoding.EncodeToString(rk.Key) != newKey {
		t.Errorf("cache key after trust: not the new key")
	}
	// The sticky marker is cleared (the badge goes back to healthy).
	health, err := fp.ListPeerHealthByProject(ctx, pid)
	if err != nil {
		t.Fatalf("list health: %v", err)
	}
	if len(health) != 1 || health[0].KeyMismatchAt != "" {
		t.Errorf("key_mismatch_at after trust: not cleared: %+v", health)
	}
	// The incident is resolved with the new key (the audit trail).
	open, err := incidents.OpenIncident(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("open incident: %v", err)
	}
	if open != nil {
		t.Errorf("open incident after trust: got %+v, want nil (resolved)", open)
	}
}

// TestMarkKeyMismatchByRemote_OpensIncident asserts the inbox signature-check
// writer (Federation v1 F5.6b, US-6.4 AC2) opens a security incident — not just
// the sticky marker — when a peer's event signature fails against the pinned key,
// and is idempotent under a flood (one open incident, not one per rejected event).
func TestMarkKeyMismatchByRemote_OpensIncident(t *testing.T) {
	fetch := func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
		t.Fatal("fetch must not be called by the mismatch recorder")
		return nil, nil
	}
	svc, projects, fp, instances, incidents, _ := newTrustKeySvc(t, "https://me.example", fetch)
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	proj, err := projects.Get(ctx, pid)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}

	// First mismatch opens the incident + stamps the marker.
	if err := svc.MarkKeyMismatchByRemote(ctx, "https://bob.example", proj.ClientID); err != nil {
		t.Fatalf("mark mismatch 1: %v", err)
	}
	// A flood of further mismatches records NOTHING new.
	if err := svc.MarkKeyMismatchByRemote(ctx, "https://bob.example", proj.ClientID); err != nil {
		t.Fatalf("mark mismatch 2: %v", err)
	}

	open, err := incidents.OpenIncident(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("open incident: %v", err)
	}
	if open == nil {
		t.Fatalf("expected an open incident after a key mismatch, got nil (US-6.4 AC2)")
	}
	// Idempotent under a flood: a second RecordKeyChange while the incident is open
	// opens NOTHING new (the recorder already ran twice above; one more must no-op).
	opened, err := incidents.RecordKeyChange(ctx, pid, "https://bob.example", "pk", time.Now())
	if err != nil {
		t.Fatalf("record while open: %v", err)
	}
	if opened {
		t.Errorf("RecordKeyChange while incident open: got opened=true, want false (idempotent)")
	}

	// The sticky marker is also set (the badge goes red).
	health, err := fp.ListPeerHealthByProject(ctx, pid)
	if err != nil {
		t.Fatalf("list health: %v", err)
	}
	if len(health) != 1 || health[0].KeyMismatchAt == "" {
		t.Errorf("key_mismatch_at after mismatch: not set: %+v", health)
	}
}

// TestTrustPeerKey_MalformedFetchedKeyDoesNotCorruptDurable asserts a non-empty
// but UN-DECODABLE .well-known key aborts the trust action fetch-first, leaving the
// durable pinned key UNTOUCHED (Federation v1 F5.6b review fix — validate-before-
// write so a malformed key can never corrupt federated_instances.public_key and
// diverge durable/cache state).
func TestTrustPeerKey_MalformedFetchedKeyDoesNotCorruptDurable(t *testing.T) {
	fetch := func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: "!!!not-base64-ed25519!!!", DisplayName: "Bob"}, nil
	}
	svc, projects, fp, instances, _, _ := newTrustKeySvc(t, "https://me.example", fetch)
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	if err := svc.TrustPeerKey(ctx, pid, "https://bob.example"); err == nil {
		t.Fatalf("expected TrustPeerKey to error on a malformed fetched key")
	}
	// The durable pinned key must be UNCHANGED — the malformed garbage must NOT have
	// been written before the (later) decode in Cache.Trust would have failed.
	inst, err := instances.Get(ctx, "https://bob.example")
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if inst.PublicKey != "pk" {
		t.Errorf("durable public_key after malformed trust: got %q, want pk (unchanged — malformed key must not corrupt durable state)", inst.PublicKey)
	}
}

// TestTrustPeerKey_UnknownPeer asserts trusting a peer not joined to the project
// is a 404-mapping ErrPeerNotFound and performs NO fetch (no side effects).
func TestTrustPeerKey_UnknownPeer(t *testing.T) {
	var fetched int
	fetch := func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
		fetched++
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: newKeyB64(t)}, nil
	}
	svc, projects, _, _, _, _ := newTrustKeySvc(t, "https://me.example", fetch)
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}

	err := svc.TrustPeerKey(ctx, pid, "https://stranger.example")
	if !errors.Is(err, fedsvc.ErrPeerNotFound) {
		t.Errorf("TrustPeerKey unknown peer: got %v, want ErrPeerNotFound", err)
	}
	if fetched != 0 {
		t.Errorf("fetches for unknown peer: got %d, want 0 (no side effects)", fetched)
	}
}

// TestTrustPeerKey_UnknownProject asserts trusting a peer on a non-existent
// project maps to ErrProjectNotFound (404) and performs no fetch.
func TestTrustPeerKey_UnknownProject(t *testing.T) {
	fetch := func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
		t.Fatal("fetch must not be called for an unknown project")
		return nil, nil
	}
	svc, _, _, _, _, _ := newTrustKeySvc(t, "https://me.example", fetch)
	err := svc.TrustPeerKey(context.Background(), 9999, "https://bob.example")
	if !errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Errorf("TrustPeerKey unknown project: got %v, want ErrProjectNotFound", err)
	}
}

// TestTrustPeerKey_FetchFailureDoesNotMutate asserts that when the peer's
// .well-known fetch fails, the trust action errors and changes NOTHING — the old
// key, the sticky marker, and the open incident all survive so the operator can
// retry (US-6.4 AC3 is fetch-first; a failed fetch must not clear the incident).
func TestTrustPeerKey_FetchFailureDoesNotMutate(t *testing.T) {
	fetch := func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return nil, errors.New("network down")
	}
	svc, projects, fp, instances, incidents, _ := newTrustKeySvc(t, "https://me.example", fetch)
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)
	if _, err := fp.MarkKeyMismatch(ctx, pid, "https://bob.example", "2026-06-03T10:00:00.000Z"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if _, err := incidents.RecordKeyChange(ctx, pid, "https://bob.example", "pk", time.Now()); err != nil {
		t.Fatalf("record: %v", err)
	}

	if err := svc.TrustPeerKey(ctx, pid, "https://bob.example"); err == nil {
		t.Fatalf("expected TrustPeerKey to error on fetch failure")
	}

	// The old key, marker, and open incident all survive (retryable).
	inst, err := instances.Get(ctx, "https://bob.example")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if inst.PublicKey != "pk" {
		t.Errorf("public_key after failed trust: got %q, want pk (unchanged)", inst.PublicKey)
	}
	health, err := fp.ListPeerHealthByProject(ctx, pid)
	if err != nil {
		t.Fatalf("list health: %v", err)
	}
	if len(health) != 1 || health[0].KeyMismatchAt == "" {
		t.Errorf("marker after failed trust: got cleared, want still set")
	}
	open, err := incidents.OpenIncident(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("open incident: %v", err)
	}
	if open == nil {
		t.Errorf("incident after failed trust: got resolved, want still open")
	}
}

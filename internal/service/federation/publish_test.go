package federation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/transport"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// captureSender records the last SignedRequest and returns a canned response.
type captureSender struct {
	last        fedsvc.SignedRequest
	respCode    int
	respBody    []byte
	respHeaders map[string]string
}

func (s *captureSender) Send(_ context.Context, req fedsvc.SignedRequest) (*fedsvc.SignedResponse, error) {
	s.last = req
	code := s.respCode
	if code == 0 {
		code = 200
	}
	return &fedsvc.SignedResponse{StatusCode: code, Body: s.respBody, Headers: s.respHeaders}, nil
}

func newPublisher(t *testing.T, sender fedsvc.HandshakeSender) (*fedsvc.Publisher, *repo.FederatedProjectRepo, func(context.Context) int64) {
	t.Helper()
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	ctx := context.Background()
	if _, err := keys.Ensure(ctx, crypto.NewTokenCipher(fedSvcKey), "me"); err != nil {
		t.Fatalf("ensure keys: %v", err)
	}
	pub := fedsvc.NewPublisher(fedProjects, keys, crypto.NewTokenCipher(fedSvcKey), sender, "https://me.example", nil)
	mkProject := func(ctx context.Context) int64 {
		p, err := projects.Create(ctx, repo.CreateProject{ContextID: 1, Title: "Shared", Color: "blue"})
		if err != nil {
			t.Fatalf("create project: %v", err)
		}
		return p.ID
	}
	return pub, fedProjects, mkProject
}

// TestPublisher_PeersForProject_ExcludesSelfAndRevoked asserts the delivery
// target set excludes the owner self-row and any revoked peer (US-5.2 AC1
// fan-out filtering precursor; the worker never delivers to itself).
func TestPublisher_PeersForProject_ExcludesSelfAndRevoked(t *testing.T) {
	pub, fedProjects, mkProject := newPublisher(t, &captureSender{})
	ctx := context.Background()
	pid := mkProject(ctx)

	// self-row
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: pid, PeerInstanceURL: "https://me.example", IsOwner: true,
		OriginInstanceURL: "https://me.example", Permissions: model.FederationPermissionAdmin,
	}); err != nil {
		t.Fatalf("self row: %v", err)
	}
	// active write peer
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: pid, PeerInstanceURL: "https://a.example",
		OriginInstanceURL: "https://me.example", Permissions: model.FederationPermissionWrite,
	}); err != nil {
		t.Fatalf("active peer: %v", err)
	}
	// revoked peer
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: pid, PeerInstanceURL: "https://gone.example", Revoked: true,
		OriginInstanceURL: "https://me.example", Permissions: model.FederationPermissionWrite,
	}); err != nil {
		t.Fatalf("revoked peer: %v", err)
	}

	peers, err := pub.PeersForProject(ctx, pid)
	if err != nil {
		t.Fatalf("peers: %v", err)
	}
	if len(peers) != 1 || peers[0].InstanceURL != "https://a.example" {
		t.Fatalf("peers: got %+v, want only https://a.example", peers)
	}
}

// TestPublisher_PeersForProject_ExcludesPaused asserts a temporarily-paused peer
// is dropped from the outbox fan-out targets (Federation v1 F5.3, US-6.1 AC1):
// the owner-worker does not POST to a paused peer, so its events accumulate in
// federation_outbox (delivered_to stays unstamped) and flush on resume. Unlike a
// revoked peer it is NOT removed permanently — the row stays, paused=1.
func TestPublisher_PeersForProject_ExcludesPaused(t *testing.T) {
	pub, fedProjects, mkProject := newPublisher(t, &captureSender{})
	ctx := context.Background()
	pid := mkProject(ctx)

	// active write peer — a fan-out target.
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: pid, PeerInstanceURL: "https://a.example",
		OriginInstanceURL: "https://me.example", Permissions: model.FederationPermissionWrite,
	}); err != nil {
		t.Fatalf("active peer: %v", err)
	}
	// paused write peer — must be skipped (events accumulate, not lost).
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: pid, PeerInstanceURL: "https://paused.example", Paused: true,
		OriginInstanceURL: "https://me.example", Permissions: model.FederationPermissionWrite,
	}); err != nil {
		t.Fatalf("paused peer: %v", err)
	}

	peers, err := pub.PeersForProject(ctx, pid)
	if err != nil {
		t.Fatalf("peers: %v", err)
	}
	if len(peers) != 1 || peers[0].InstanceURL != "https://a.example" {
		t.Fatalf("peers: got %+v, want only https://a.example (paused excluded, US-6.1 AC1)", peers)
	}
}

// TestPublisher_PeersForProject_ExcludesLeft asserts a peer that voluntarily LEFT
// (lost=1, reason=left) is dropped from the outbox fan-out targets (Federation v1
// F5.5, US-6.3 AC2): the owner stops sending to it, and any already-queued events
// for it never go out — like a revoked peer it is removed permanently.
func TestPublisher_PeersForProject_ExcludesLeft(t *testing.T) {
	pub, fedProjects, mkProject := newPublisher(t, &captureSender{})
	ctx := context.Background()
	pid := mkProject(ctx)

	// active write peer — a fan-out target.
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: pid, PeerInstanceURL: "https://a.example",
		OriginInstanceURL: "https://me.example", Permissions: model.FederationPermissionWrite,
	}); err != nil {
		t.Fatalf("active peer: %v", err)
	}
	// a peer that left — must be skipped (the owner stops sending to it). UpsertPeerRow
	// does not persist lost/lost_reason, so the left state is stamped via MarkLeftByPeer
	// (the owner-side leave-apply leg).
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: pid, PeerInstanceURL: "https://left.example",
		OriginInstanceURL: "https://me.example", Permissions: model.FederationPermissionWrite,
	}); err != nil {
		t.Fatalf("left peer: %v", err)
	}
	if _, err := fedProjects.MarkLeftByPeer(ctx, pid, "https://left.example"); err != nil {
		t.Fatalf("mark left: %v", err)
	}

	peers, err := pub.PeersForProject(ctx, pid)
	if err != nil {
		t.Fatalf("peers: %v", err)
	}
	if len(peers) != 1 || peers[0].InstanceURL != "https://a.example" {
		t.Fatalf("peers: got %+v, want only https://a.example (left peer excluded, US-6.3 AC2)", peers)
	}
}

// TestPublisher_Push_SignsAndWrapsBatch asserts Push POSTs a transport-signed
// batch envelope to the peer's /federation/events, the body verifies against the
// digest header, and the verbatim payloads round-trip as the events array
// (US-3.1 push path; F3.2a sign-over-received-bytes).
func TestPublisher_Push_SignsAndWrapsBatch(t *testing.T) {
	sender := &captureSender{respCode: 202}
	pub, _, _ := newPublisher(t, sender)
	ctx := context.Background()

	payloads := []string{
		`{"event_id":"e1","op":"update","entity_type":"task"}`,
		`{"event_id":"e2","op":"create","entity_type":"task"}`,
	}
	if err := pub.Push(ctx, "https://a.example/", payloads); err != nil {
		t.Fatalf("push: %v", err)
	}

	req := sender.last
	if req.Method != "POST" {
		t.Errorf("method: got %q, want POST", req.Method)
	}
	if req.URL != "https://a.example"+events.PushPath {
		t.Errorf("url: got %q, want %q", req.URL, "https://a.example"+events.PushPath)
	}
	if req.Path != events.PushPath {
		t.Errorf("signed path: got %q, want %q", req.Path, events.PushPath)
	}
	// Digest header binds the exact body bytes (the signature covers the digest).
	if got, want := req.Headers[transport.HeaderDigest], transport.BodyDigest(req.Body); got != want {
		t.Errorf("digest header: got %q, want %q (body digest)", got, want)
	}
	if req.Headers[transport.HeaderSignature] == "" {
		t.Errorf("missing signature header")
	}

	var batch events.Batch
	if err := json.Unmarshal(req.Body, &batch); err != nil {
		t.Fatalf("decode batch body: %v", err)
	}
	if len(batch.Events) != 2 {
		t.Fatalf("batch events: got %d, want 2", len(batch.Events))
	}
	if batch.Events[0].EventID != "e1" || batch.Events[1].EventID != "e2" {
		t.Errorf("batch order: got %q,%q want e1,e2", batch.Events[0].EventID, batch.Events[1].EventID)
	}
}

// TestPublisher_Push_Non2xxIsError asserts a non-2xx peer response surfaces as an
// error so the outbox worker leaves the batch pending for retry (US-3.2 AC3).
func TestPublisher_Push_Non2xxIsError(t *testing.T) {
	sender := &captureSender{respCode: 503, respBody: []byte(`{"error":{"code":"x"}}`)}
	pub, _, _ := newPublisher(t, sender)
	if err := pub.Push(context.Background(), "https://a.example", []string{`{"event_id":"e1"}`}); err == nil {
		t.Fatal("expected error on 503 push, got nil")
	}
}

// TestPublisher_Push_429CarriesRetryAfter asserts a 429 push response is
// classified as a transient backpressure error carrying the peer's Retry-After
// window verbatim (Federation v1 F4.4, US-4.4 AC1) — NOT a permanent reject.
func TestPublisher_Push_429CarriesRetryAfter(t *testing.T) {
	sender := &captureSender{
		respCode:    429,
		respBody:    []byte(`{"error":{"code":"federation_rate_limited"}}`),
		respHeaders: map[string]string{"Retry-After": "42"},
	}
	pub, _, _ := newPublisher(t, sender)
	err := pub.Push(context.Background(), "https://a.example", []string{`{"event_id":"e1"}`})
	if err == nil {
		t.Fatal("expected error on 429 push, got nil")
	}
	var re *fedsvc.RemoteHandshakeError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RemoteHandshakeError, got %T", err)
	}
	if re.FederationPermanent() {
		t.Errorf("429 must be transient, not permanent")
	}
	d, ok := re.FederationRetryAfter()
	if !ok || d != 42*time.Second {
		t.Errorf("Retry-After: got (%v, %v), want (42s, true)", d, ok)
	}
}

// TestPublisher_Push_403IsPermanentWithReason asserts a 403 push response is a
// PERMANENT, PEER-SCOPED reject carrying the federation error code so the worker
// dead-letters it with that reason AND gates the whole link (Federation v1 F4.4,
// US-4.4 AC3 + the §9.2/§9.3 link-reject case).
func TestPublisher_Push_403IsPermanentWithReason(t *testing.T) {
	sender := &captureSender{respCode: 403, respBody: []byte(`{"error":{"code":"federation_read_only"}}`)}
	pub, _, _ := newPublisher(t, sender)
	err := pub.Push(context.Background(), "https://a.example", []string{`{"event_id":"e1"}`})
	var re *fedsvc.RemoteHandshakeError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RemoteHandshakeError, got %T", err)
	}
	if !re.FederationPermanent() {
		t.Errorf("403 must be permanent")
	}
	if !re.FederationPeerScoped() {
		t.Errorf("403 must be peer-scoped (a whole-link reject)")
	}
	if re.FederationStatusCode() != 403 || re.FederationReason() != "federation_read_only" {
		t.Errorf("dead-letter classification: got (%d, %q), want (403, federation_read_only)", re.FederationStatusCode(), re.FederationReason())
	}
}

// TestPublisher_Push_EventScoped4xxIsPermanentNotPeerScoped asserts that a
// permanent 4xx that is SPECIFIC to the offending event — a 400 author/origin-
// mismatch, a 401 signature-rejected, a 410 stale-tombstone (a re-edit of a
// tombstoned entity per the offline contract) — is classified permanent (so the
// event is dead-lettered) but NOT peer-scoped, so the worker does NOT gate the
// whole link and the peer's other healthy events keep flowing (F4.4 hardening).
func TestPublisher_Push_EventScoped4xxIsPermanentNotPeerScoped(t *testing.T) {
	cases := []struct {
		name string
		code int
		body string
	}{
		{"author_mismatch_400", 400, `{"error":{"code":"federation_author_mismatch"}}`},
		{"signature_invalid_401", 401, `{"error":{"code":"federation_signature_invalid"}}`},
		{"stale_tombstone_410", 410, `{"error":{"code":"federation_stale_pull"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender := &captureSender{respCode: tc.code, respBody: []byte(tc.body)}
			pub, _, _ := newPublisher(t, sender)
			err := pub.Push(context.Background(), "https://a.example", []string{`{"event_id":"e1"}`})
			var re *fedsvc.RemoteHandshakeError
			if !errors.As(err, &re) {
				t.Fatalf("expected *RemoteHandshakeError, got %T", err)
			}
			if !re.FederationPermanent() {
				t.Errorf("%d must be permanent (dead-letter the event)", tc.code)
			}
			if re.FederationPeerScoped() {
				t.Errorf("%d is event-scoped — it must NOT gate the whole peer link", tc.code)
			}
		})
	}
}

// TestPublisher_Pull_SignsGetAndDecodes asserts Pull issues a transport-signed
// GET to the peer's pull endpoint (since_hlc + limit as query, signed path is the
// query-less concrete path, R4) and decodes the PullResponse (US-4.1 recovery).
func TestPublisher_Pull_SignsGetAndDecodes(t *testing.T) {
	respBody, _ := json.Marshal(events.PullResponse{
		Events:  []events.Event{{EventID: "e9"}},
		NextHLC: "00000000000900-0000-nodeA",
	})
	sender := &captureSender{respCode: 200, respBody: respBody}
	pub, _, _ := newPublisher(t, sender)

	out, err := pub.Pull(context.Background(), "https://a.example", "remote-proj-1", "00000000000100-0000-nodeA", 500)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(out.Events) != 1 || out.Events[0].EventID != "e9" {
		t.Errorf("pull events: got %+v", out.Events)
	}
	if out.NextHLC != "00000000000900-0000-nodeA" {
		t.Errorf("next hlc: got %q", out.NextHLC)
	}

	req := sender.last
	if req.Method != "GET" {
		t.Errorf("method: got %q, want GET", req.Method)
	}
	if req.Path != "/federation/projects/remote-proj-1/events" {
		t.Errorf("signed path must be query-less concrete path: got %q", req.Path)
	}
	// Empty GET body → SHA256("") digest.
	if got, want := req.Headers[transport.HeaderDigest], transport.BodyDigest(nil); got != want {
		t.Errorf("get digest: got %q, want SHA256(\"\") %q", got, want)
	}
}

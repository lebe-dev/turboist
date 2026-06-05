package handlers_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/federation/nonce"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/federation/transport"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// Federation v1 F6.2 — security hardening tie-off, consolidated regression suite.
//
// The core Must-grade security checks already landed in earlier phases: the
// TRANSPORT request signature (digest / ±5min window / nonce anti-replay /
// Ed25519 over the pinned canonical string) in F0.3, and the PER-EVENT payload
// validation (per-event Ed25519 / author==origin / HLC clock-skew / membership)
// in F3.2a. F6.2 is the regression/consolidation pass: it wires BOTH planes onto
// the SAME signed endpoint and asserts, end-to-end, that:
//
//   - the two signature planes stay DISTINCT — a transport-valid request can still
//     carry a forged or stale event payload, and that payload is still rejected
//     with zero inbox/domain rows (the §15.5 "tampered payload not applied" leg);
//   - the documented order is preserved — the stale timestamp is rejected BEFORE
//     the nonce is recorded (US-7.3 AC2), and a replayed nonce is rejected (AC1);
//   - the events endpoint is UNREACHABLE without the transport middleware active
//     (R22 — no multi-phase exposure window);
//   - the in-memory nonce cache resets on restart (US-7.3 AC3 documented gap, R18).
//
// Unlike federation_events_test.go (which injects the verified peer directly to
// unit-test the handler logic), this suite drives the REAL HTTPSignatureMiddleware
// in front of the handler, so the transport plane is genuinely exercised.

// fedSecEnv wires the signed events endpoint behind the REAL transport signature
// middleware, over a migrated DB with one federated project and one joined write
// peer. mw is rebuildable with a fresh nonce cache to model a process restart.
type fedSecEnv struct {
	app          *fiber.App
	db           *sql.DB
	store        *store.Store
	fedProjects  *repo.FederatedProjectRepo
	localProject int64
	projClientID string
	peerURL      string
	peerPriv     ed25519.PrivateKey
	peerPub      ed25519.PublicKey
	peerPubB64   string
	now          time.Time
	nonces       *nonce.Cache
}

const fedSecPeerURL = "https://sec-peer.example"

func newFedSecEnv(t *testing.T) *fedSecEnv {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "fedsec.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, client_id, created_at, updated_at)
		 VALUES (1, 'c', 'blue', 'sec-ctx', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("ctx: %v", err)
	}
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	p, err := projects.Create(ctx, repo.CreateProject{ContextID: 1, Title: "Shared", Color: "blue"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err := d.Exec(`UPDATE projects SET is_federated = 1 WHERE id = ?`, p.ID); err != nil {
		t.Fatalf("federate: %v", err)
	}

	peerPub, peerPriv, _ := ed25519.GenerateKey(rand.Reader)
	peerPubB64 := base64.StdEncoding.EncodeToString(peerPub)

	fedProjects := repo.NewFederatedProjectRepo(d)
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: p.ID, PeerInstanceURL: fedSecPeerURL,
		OriginInstanceURL: fedSecPeerURL, Permissions: model.FederationPermissionWrite,
	}); err != nil {
		t.Fatalf("peer row: %v", err)
	}

	st := store.New(d)
	env := &fedSecEnv{
		db: d, store: st, fedProjects: fedProjects, localProject: p.ID, projClientID: p.ClientID,
		peerURL: fedSecPeerURL, peerPriv: peerPriv, peerPub: peerPub, peerPubB64: peerPubB64,
		now: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	}

	// The per-event validator resolves the peer's published key (a static fetcher
	// here) and checks membership against the seeded DB row, with a pinned clock so
	// the asymmetric skew bounds are deterministic.
	validator := inbox.NewValidator(
		func(_ context.Context, _ string) (ed25519.PublicKey, error) { return peerPub, nil },
		inbox.DBMembershipLookup(d, fedProjects),
		func() time.Time { return env.now },
	)
	queue := &captureQueue{}
	cipher := crypto.NewTokenCipher("federation-security-cipher-key-32-bytes!")
	fedSvc := fedsvc.NewService(
		d, projects, fedProjects, repo.NewFederationKeysRepo(d), repo.NewFederationInviteRepo(d),
		repo.NewFederatedInstanceRepo(d), cipher, fedSecPeerURL,
	)
	h := handlers.NewFederationHandler(repo.NewFederationKeysRepo(d), cipher, fedSecPeerURL).
		WithEventsDeps(handlers.FederationEventsDeps{
			Store:       st,
			Validator:   validator,
			Queue:       queue,
			Projects:    fedProjects,
			KeyMismatch: fedSvc,
		})

	env.app = env.buildApp(h)
	return env
}

// buildApp mounts the signed events endpoint behind the REAL transport signature
// middleware with a FRESH nonce cache (peer key resolved via a static fetcher).
// Calling it again models a process restart: the new app shares the same DB but a
// brand-new in-memory nonce cache (US-7.3 AC3).
func (e *fedSecEnv) buildApp(h *handlers.FederationHandler) *fiber.App {
	e.nonces = nonce.NewCache()
	keys := peerkeys.NewCache(func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: e.peerPubB64, DisplayName: "Sec Peer"}, nil
	})
	mw := httpapi.HTTPSignatureMiddleware(httpapi.FederationSignatureDeps{
		Nonces:   e.nonces,
		PeerKeys: keys,
		Now:      func() time.Time { return e.now },
	})
	app := httpapi.NewApp(httpapi.Deps{})
	grp := app.Group("/federation", mw)
	h.RegisterSigned(grp)
	return app
}

// restart rebuilds the Fiber app + signature middleware with a fresh in-memory
// nonce cache while keeping the same DB (the persistent state). It models the
// process restarting: the durable federation rows survive, the in-memory
// anti-replay state does not (R18 / US-7.3 AC3).
func (e *fedSecEnv) restart(t *testing.T) {
	t.Helper()
	cipher := crypto.NewTokenCipher("federation-security-cipher-key-32-bytes!")
	projects := repo.NewProjectRepo(e.db, repo.NewProjectLabelsRepo(e.db))
	validator := inbox.NewValidator(
		func(_ context.Context, _ string) (ed25519.PublicKey, error) { return e.peerPub, nil },
		inbox.DBMembershipLookup(e.db, e.fedProjects),
		func() time.Time { return e.now },
	)
	queue := &captureQueue{}
	fedSvc := fedsvc.NewService(
		e.db, projects, e.fedProjects, repo.NewFederationKeysRepo(e.db), repo.NewFederationInviteRepo(e.db),
		repo.NewFederatedInstanceRepo(e.db), cipher, e.peerURL,
	)
	h := handlers.NewFederationHandler(repo.NewFederationKeysRepo(e.db), cipher, e.peerURL).
		WithEventsDeps(handlers.FederationEventsDeps{
			Store: e.store, Validator: validator, Queue: queue, Projects: e.fedProjects, KeyMismatch: fedSvc,
		})
	e.app = e.buildApp(h)
}

// signedEvent builds a valid per-event-signed batch event from the peer.
func (e *fedSecEnv) signedEvent(t *testing.T, entityID string) events.Event {
	t.Helper()
	evt := events.Event{
		EventID:         model.NewClientID(),
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        entityID,
		ProjectClientID: e.projClientID,
		Author:          e.peerURL,
		OriginInstance:  e.peerURL,
		CreatedAt:       model.FormatUTC(e.now),
		Fields:          map[string]events.Field{"title": {Value: "remote", HLC: e.hlcAt(e.now)}},
	}
	signed, err := events.Sign(evt, e.peerPriv)
	if err != nil {
		t.Fatalf("sign event: %v", err)
	}
	return signed
}

// hlcAt renders a canonical HLC string at wall time t (logical 0).
func (e *fedSecEnv) hlcAt(t time.Time) string {
	return hlc.HLC{PhysicalMS: t.UnixMilli(), Logical: 0, NodeID: "nodeSec"}.String()
}

// transportNonce is a per-request nonce so a normal POST is never itself a replay.
func transportNonce(seed string) string {
	return "sec-nonce-" + seed
}

// postSigned signs a batch over the PINNED transport canonical string and POSTs
// it. mutate may tamper the transmitted body, the signature params, or the
// headers after signing — modelling an attacker between the peer and this
// instance. nonceSeed lets a caller force a fixed nonce to drive the replay case.
func (e *fedSecEnv) postSigned(t *testing.T, batch events.Batch, nonceSeed string, mutate func(body *[]byte, p *transport.SignatureParams, h http.Header)) *http.Response {
	t.Helper()
	body, _ := json.Marshal(batch)
	ts := model.FormatUTC(e.now)
	params := transport.SignatureParams{
		Method:          http.MethodPost,
		Path:            events.PushPath,
		InstanceURL:     e.peerURL,
		Timestamp:       ts,
		Nonce:           transportNonce(nonceSeed),
		ProtocolVersion: "1",
		BodyDigest:      transport.BodyDigest(body),
	}
	extra := http.Header{}
	if mutate != nil {
		mutate(&body, &params, extra)
	}
	sig := transport.SignB64(e.peerPriv, params)

	req := httptest.NewRequest(http.MethodPost, events.PushPath, bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(transport.HeaderInstance, params.InstanceURL)
	req.Header.Set(transport.HeaderTimestamp, params.Timestamp)
	req.Header.Set(transport.HeaderNonce, params.Nonce)
	req.Header.Set(transport.HeaderProtocolVer, params.ProtocolVersion)
	req.Header.Set(transport.HeaderDigest, params.BodyDigest)
	req.Header.Set(transport.HeaderSignature, sig)
	for k := range extra {
		req.Header.Set(k, extra.Get(k))
	}
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("test request: %v", err)
	}
	return resp
}

func errCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &out)
	return out.Error.Code
}

func fedSecInboxCount(t *testing.T, d *sql.DB) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federation_inbox`).Scan(&n); err != nil {
		t.Fatalf("count inbox: %v", err)
	}
	return n
}

// --- Happy path through BOTH planes (baseline the regressions deviate from). ---

// TestSecuritySuite_ValidRequestThroughBothPlanes asserts a well-formed,
// transport-signed AND per-event-signed batch is accepted end-to-end through the
// real signature middleware and the per-event validator, recording the event in
// the inbox. This is the baseline: every rejection test below changes exactly one
// thing from this request.
func TestSecuritySuite_ValidRequestThroughBothPlanes(t *testing.T) {
	env := newFedSecEnv(t)
	resp := env.postSigned(t, events.Batch{Events: []events.Event{env.signedEvent(t, "task-ok")}}, "ok", nil)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("valid request through both planes: got %d (%s), want 2xx", resp.StatusCode, errCode(t, resp))
	}
	if n := fedSecInboxCount(t, env.db); n != 1 {
		t.Errorf("valid event must record one inbox row: got %d", n)
	}
}

// --- Transport plane (F0.3) regressions. ---

// TestSecuritySuite_ReplayedNonceRejected asserts a replayed transport nonce is
// rejected 401 federation_replay (US-7.3 AC1). The first request is accepted; the
// second with the identical nonce/timestamp/signature is the replay.
func TestSecuritySuite_ReplayedNonceRejected(t *testing.T) {
	env := newFedSecEnv(t)
	first := env.postSigned(t, events.Batch{Events: []events.Event{env.signedEvent(t, "task-r1")}}, "fixed", nil)
	if first.StatusCode < 200 || first.StatusCode >= 300 {
		t.Fatalf("first request: got %d (%s), want 2xx", first.StatusCode, errCode(t, first))
	}
	// Replay the SAME nonce (same body content shape, fresh event id is irrelevant —
	// the transport nonce is what is replayed and it is rejected before the body is
	// even validated).
	second := env.postSigned(t, events.Batch{Events: []events.Event{env.signedEvent(t, "task-r2")}}, "fixed", nil)
	if second.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replayed nonce: got %d, want 401", second.StatusCode)
	}
	if code := errCode(t, second); code != httpapi.CodeFederationReplay {
		t.Errorf("replay code: got %q, want %q (US-7.3 AC1)", code, httpapi.CodeFederationReplay)
	}
}

// TestSecuritySuite_StaleTimestampRejectedBeforeNonce asserts a timestamp outside
// the ±5min window is rejected 401 federation_timestamp_stale AND that the check
// runs BEFORE the nonce is recorded (US-7.3 AC2). The proof of ordering: after a
// stale request with nonce N, a FRESH (in-window) request reusing the SAME nonce N
// must still be accepted — if the stale path had recorded N, the fresh request
// would be a false replay. The stale request signs over its own stale timestamp so
// the only reason to reject is staleness, not a bad signature.
func TestSecuritySuite_StaleTimestampRejectedBeforeNonce(t *testing.T) {
	env := newFedSecEnv(t)
	staleTs := model.FormatUTC(env.now.Add(-10 * time.Minute))
	const sharedNonce = "shared-with-stale"

	stale := env.postSigned(t, events.Batch{Events: []events.Event{env.signedEvent(t, "task-s1")}}, sharedNonce,
		func(_ *[]byte, p *transport.SignatureParams, h http.Header) {
			p.Timestamp = staleTs
			h.Set(transport.HeaderTimestamp, staleTs)
		})
	if stale.StatusCode != http.StatusUnauthorized {
		t.Fatalf("stale timestamp: got %d, want 401", stale.StatusCode)
	}
	if code := errCode(t, stale); code != httpapi.CodeFederationTimestampStale {
		t.Fatalf("stale code: got %q, want %q (US-7.3 AC2)", code, httpapi.CodeFederationTimestampStale)
	}

	// Reuse the SAME nonce on an in-window request. If the stale path had recorded
	// the nonce (i.e. checked nonce before timestamp), this would be rejected as a
	// replay. It must be accepted, proving the timestamp window is checked first.
	fresh := env.postSigned(t, events.Batch{Events: []events.Event{env.signedEvent(t, "task-s2")}}, sharedNonce, nil)
	if fresh.StatusCode < 200 || fresh.StatusCode >= 300 {
		t.Fatalf("nonce reused after a STALE reject must be fresh (timestamp checked before nonce): got %d (%s)", fresh.StatusCode, errCode(t, fresh))
	}
}

// TestSecuritySuite_DigestMismatchRejected asserts a transmitted body that differs
// from the one whose digest was signed is rejected 400 federation_digest_mismatch
// (US-7.2 AC2 transport leg) — before any per-event validation. The signature is
// over the original body's digest; only the transmitted bytes are swapped.
func TestSecuritySuite_DigestMismatchRejected(t *testing.T) {
	env := newFedSecEnv(t)
	resp := env.postSigned(t, events.Batch{Events: []events.Event{env.signedEvent(t, "task-d1")}}, "digest",
		func(body *[]byte, _ *transport.SignatureParams, _ http.Header) {
			// Swap the transmitted body AFTER the digest was computed over the original.
			*body = []byte(`{"events":[{"event_id":"swapped"}]}`)
		})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("digest mismatch: got %d, want 400", resp.StatusCode)
	}
	if code := errCode(t, resp); code != httpapi.CodeFederationDigestMismatch {
		t.Errorf("digest code: got %q, want %q (US-7.2 AC2)", code, httpapi.CodeFederationDigestMismatch)
	}
	if n := fedSecInboxCount(t, env.db); n != 0 {
		t.Errorf("digest-mismatch request must write no inbox rows: got %d", n)
	}
}

// TestSecuritySuite_TransportSignatureRequired asserts a request with NO transport
// signature headers is rejected 401 by the middleware before the handler runs
// (R22 — the endpoint cannot be reached without the transport plane active). A
// bare POST to /federation/events leaves zero inbox rows.
func TestSecuritySuite_TransportSignatureRequired(t *testing.T) {
	env := newFedSecEnv(t)
	body, _ := json.Marshal(events.Batch{Events: []events.Event{env.signedEvent(t, "task-bare")}})
	req := httptest.NewRequest(http.MethodPost, events.PushPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned request: got %d, want 401", resp.StatusCode)
	}
	if code := errCode(t, resp); code != httpapi.CodeFederationSignatureInvalid {
		t.Errorf("unsigned code: got %q, want %q", code, httpapi.CodeFederationSignatureInvalid)
	}
	if n := fedSecInboxCount(t, env.db); n != 0 {
		t.Errorf("unsigned request must never reach the handler: got %d inbox rows", n)
	}
}

// --- Per-event plane (F3.2a) regressions, all behind a VALID transport signature. ---
//
// These prove the two planes are DISTINCT: the transport request is correctly
// signed (it passes F0.3), yet the per-event payload is still rejected by F3.2a.

// TestSecuritySuite_TamperedPayloadNotApplied asserts that a transport-valid
// request carrying a per-event-tampered payload is rejected 401 with ZERO inbox/
// domain rows (§15.5 "tampered payload not applied" / US-7.2 AC1). The event is
// signed, then a field is mutated, then the WHOLE (tampered) batch is correctly
// transport-signed — so the transport plane passes and only the per-event plane
// catches the tamper. This is the canonical two-distinct-planes assertion.
func TestSecuritySuite_TamperedPayloadNotApplied(t *testing.T) {
	env := newFedSecEnv(t)
	evt := env.signedEvent(t, "task-tamper")
	// Tamper a signed field AFTER per-event signing: the per-event canonical bytes
	// no longer match the per-event signature. The transport digest/signature below
	// are computed over THIS tampered body, so the transport plane is satisfied.
	evt.Fields["title"] = events.Field{Value: "Hijacked", HLC: env.hlcAt(env.now)}

	resp := env.postSigned(t, events.Batch{Events: []events.Event{evt}}, "tamper", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered payload behind a valid transport sig: got %d, want 401 (US-7.2 AC1)", resp.StatusCode)
	}
	if code := errCode(t, resp); code != httpapi.CodeFederationSignatureInvalid {
		t.Errorf("tampered payload code: got %q, want %q (per-event signature)", code, httpapi.CodeFederationSignatureInvalid)
	}
	if n := fedSecInboxCount(t, env.db); n != 0 {
		t.Errorf("tampered payload must leave zero inbox rows: got %d (§15.5)", n)
	}
}

// TestSecuritySuite_EventSignatureFailLeavesZeroRows asserts a per-event signature
// failure (signed with a foreign key) behind a valid transport signature is
// rejected 401 and records ZERO inbox rows (US-7.2 AC1, the inbox-count-0 leg).
func TestSecuritySuite_EventSignatureFailLeavesZeroRows(t *testing.T) {
	env := newFedSecEnv(t)
	_, foreignPriv, _ := ed25519.GenerateKey(rand.Reader)
	evt := env.signedEvent(t, "task-forged")
	// Re-sign the per-event payload with a foreign key (the resolver returns the
	// peer's real key, so per-event verification fails). The transport sig over the
	// resulting body is still valid.
	forged, err := events.Sign(evt, foreignPriv)
	if err != nil {
		t.Fatalf("forge sign: %v", err)
	}
	resp := env.postSigned(t, events.Batch{Events: []events.Event{forged}}, "forged", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("forged per-event signature: got %d, want 401 (US-7.2 AC1)", resp.StatusCode)
	}
	if n := fedSecInboxCount(t, env.db); n != 0 {
		t.Errorf("event-signature failure must leave zero inbox rows: got %d (US-7.2 AC1)", n)
	}
}

// TestSecuritySuite_AuthorOriginMismatchRejected asserts a per-event author !=
// origin_instance, behind a valid transport signature, is rejected 400
// federation_author_mismatch with zero rows (US-7.2 AC3). The event is re-signed
// after the origin is changed so the per-event signature is itself valid and ONLY
// the author/origin equality check fails.
func TestSecuritySuite_AuthorOriginMismatchRejected(t *testing.T) {
	env := newFedSecEnv(t)
	evt := env.signedEvent(t, "task-spoof")
	evt.OriginInstance = "https://eve.example"
	signed, err := events.Sign(evt, env.peerPriv)
	if err != nil {
		t.Fatalf("re-sign: %v", err)
	}
	resp := env.postSigned(t, events.Batch{Events: []events.Event{signed}}, "spoof", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("author/origin mismatch: got %d, want 400 (US-7.2 AC3)", resp.StatusCode)
	}
	if code := errCode(t, resp); code != httpapi.CodeFederationAuthorMismatch {
		t.Errorf("author-mismatch code: got %q, want %q", code, httpapi.CodeFederationAuthorMismatch)
	}
	if n := fedSecInboxCount(t, env.db); n != 0 {
		t.Errorf("author-mismatch must leave zero inbox rows: got %d", n)
	}
}

// TestSecuritySuite_ClockSkewBoundaries asserts the asymmetric per-event HLC skew
// bounds (US-7.2 AC4): an HLC at exactly +10min is accepted (the edge of the
// future window), just past +10min is rejected, a 30min-past HLC is accepted (the
// past window is wider, 1h), and a >1h-past HLC is rejected. All behind a valid
// transport signature so only the per-event skew check can fail.
func TestSecuritySuite_ClockSkewBoundaries(t *testing.T) {
	env := newFedSecEnv(t)
	cases := []struct {
		name       string
		offset     time.Duration
		wantAccept bool
	}{
		{"future-at-boundary", 10 * time.Minute, true},
		{"future-past-boundary", 11 * time.Minute, false},
		{"past-within-window", -30 * time.Minute, true},
		{"past-beyond-window", -61 * time.Minute, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evt := env.signedEvent(t, "task-skew-"+tc.name)
			evt.Fields["title"] = events.Field{Value: "Skewed", HLC: env.hlcAt(env.now.Add(tc.offset))}
			signed, err := events.Sign(evt, env.peerPriv)
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			resp := env.postSigned(t, events.Batch{Events: []events.Event{signed}}, "skew-"+tc.name, nil)
			if tc.wantAccept {
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					t.Fatalf("offset %s must be accepted: got %d (%s)", tc.offset, resp.StatusCode, errCode(t, resp))
				}
				return
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("offset %s must be rejected: got %d, want 400", tc.offset, resp.StatusCode)
			}
			if code := errCode(t, resp); code != httpapi.CodeFederationClockSkew {
				t.Errorf("offset %s code: got %q, want %q (US-7.2 AC4)", tc.offset, code, httpapi.CodeFederationClockSkew)
			}
		})
	}
}

// --- US-7.3 AC3 documented gap (R18): in-memory nonce state resets on restart. ---

// TestSecuritySuite_NonceCacheResetsOnRestart pins the documented v1 gap (R18 /
// US-7.3 AC3): the anti-replay nonce cache is in-memory, so it is wiped on a
// process restart. A nonce that was rejected as a replay BEFORE a restart is
// accepted ONCE AGAIN after the restart, opening a single in-window replay window.
// This is a deliberate, documented limitation (see docs/architecture/
// federation-threat-model.md) — the test exists so the behaviour cannot silently
// change without updating the threat model.
func TestSecuritySuite_NonceCacheResetsOnRestart(t *testing.T) {
	env := newFedSecEnv(t)
	const replayedNonce = "survives-restart"

	first := env.postSigned(t, events.Batch{Events: []events.Event{env.signedEvent(t, "task-pre")}}, replayedNonce, nil)
	if first.StatusCode < 200 || first.StatusCode >= 300 {
		t.Fatalf("first request: got %d (%s), want 2xx", first.StatusCode, errCode(t, first))
	}
	// Same nonce again, same process: rejected as a replay (the pre-restart guard).
	replayBefore := env.postSigned(t, events.Batch{Events: []events.Event{env.signedEvent(t, "task-pre2")}}, replayedNonce, nil)
	if code := errCode(t, replayBefore); replayBefore.StatusCode != http.StatusUnauthorized || code != httpapi.CodeFederationReplay {
		t.Fatalf("pre-restart replay: got %d (%s), want 401 %s", replayBefore.StatusCode, code, httpapi.CodeFederationReplay)
	}

	// Restart the process: the DB survives, the in-memory nonce cache is wiped.
	env.restart(t)

	// The SAME nonce is now accepted again — the documented single-replay window
	// (R18). This is the gap the threat model records; if a future change makes the
	// nonce store durable, this assertion flips and the threat model must be updated.
	afterRestart := env.postSigned(t, events.Batch{Events: []events.Event{env.signedEvent(t, "task-post")}}, replayedNonce, nil)
	if afterRestart.StatusCode < 200 || afterRestart.StatusCode >= 300 {
		t.Fatalf("post-restart, a previously-seen nonce is accepted again (R18 documented gap): got %d (%s)", afterRestart.StatusCode, errCode(t, afterRestart))
	}
}

// TestSecuritySuite_TransportAndPerEventSignaturesAreDistinct is the explicit
// "two distinct signature planes" regression (F3.2a / F6.2): a request whose
// TRANSPORT signature is valid but whose PER-EVENT signature is invalid is
// rejected, AND a request whose per-event signature is valid but whose TRANSPORT
// signature is invalid is rejected — proving neither plane can be satisfied by the
// other. The first leg reuses the forged-per-event path; the second tampers the
// transport signature only.
func TestSecuritySuite_TransportAndPerEventSignaturesAreDistinct(t *testing.T) {
	// Leg 1: valid transport, invalid per-event → 401 (already proven distinct in
	// TestSecuritySuite_EventSignatureFailLeavesZeroRows). Here we re-assert it
	// alongside leg 2 so the symmetry is visible in one place.
	env := newFedSecEnv(t)
	_, foreignPriv, _ := ed25519.GenerateKey(rand.Reader)
	forged, err := events.Sign(env.signedEvent(t, "task-leg1"), foreignPriv)
	if err != nil {
		t.Fatalf("forge: %v", err)
	}
	leg1 := env.postSigned(t, events.Batch{Events: []events.Event{forged}}, "leg1", nil)
	if leg1.StatusCode != http.StatusUnauthorized {
		t.Fatalf("valid transport + forged per-event: got %d, want 401", leg1.StatusCode)
	}

	// Leg 2: valid per-event, invalid TRANSPORT signature → 401, never reaching the
	// per-event plane. We tamper the transmitted transport signature header only;
	// the body (with its valid per-event signature) is untouched.
	env2 := newFedSecEnv(t)
	leg2 := env2.postSigned(t, events.Batch{Events: []events.Event{env2.signedEvent(t, "task-leg2")}}, "leg2",
		func(_ *[]byte, _ *transport.SignatureParams, h http.Header) {
			h.Set(transport.HeaderSignature, base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)))
		})
	if leg2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("valid per-event + forged transport: got %d, want 401", leg2.StatusCode)
	}
	if code := errCode(t, leg2); code != httpapi.CodeFederationSignatureInvalid {
		t.Errorf("transport-signature failure code: got %q, want %q", code, httpapi.CodeFederationSignatureInvalid)
	}
	if n := fedSecInboxCount(t, env2.db); n != 0 {
		t.Errorf("transport-signature failure must never reach the inbox: got %d rows", n)
	}
}

// sanity uses sha256 so the import is exercised even if a future edit removes the
// only digest reference; it documents the empty-body digest construction the
// transport layer pins (NFR-4.3). It is not an AC test, just a guard against an
// accidental import drift in this consolidation file.
func TestSecuritySuite_EmptyBodyDigestConstruction(t *testing.T) {
	want := sha256.Sum256(nil)
	got := transport.BodyDigest(nil)
	if base64.StdEncoding.EncodeToString(want[:]) != got {
		t.Errorf("empty-body digest: got %q, want SHA256(\"\") (NFR-4.3)", got)
	}
}

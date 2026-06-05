package handlers_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// fedEventsEnv wires the events handler over a migrated DB with one federated
// project and one joined write peer.
type fedEventsEnv struct {
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
	enqueued     []inbox.Job
	// resolveKey is the validator's author-key resolver. By default it returns the
	// seeded peer key; tests override it to simulate a key rotation (returns a
	// DIFFERENT key) or a transient .well-known fetch error (returns an error) so
	// the F4.3 sticky-marker false-positive boundary can be driven end-to-end.
	resolveKey func(ctx context.Context, instanceURL string) (ed25519.PublicKey, error)
	// notifyCalls counts ScopeFederation SSE publishes the key-mismatch transition
	// fires, so a test can assert the once-only transition semantics (US-4.3 AC4).
	notifyCalls int
	// rateLimiter + maxBatchEvents are the F4.4 inbound backpressure knobs. They
	// are set on the env BEFORE newFedEventsEnv wires the handler so a backpressure
	// test injects a deterministic limiter / small batch cap; nil / 0 keep the
	// defaults (no limiting / 500-event cap) so existing tests are unaffected.
	rateLimiter    handlers.FederationRateLimiter
	maxBatchEvents int
	// fedInstances is the trust-directory repo wired as the push freshness toucher
	// (Federation v1 F5.6a, US-6.5 AC1/AC3) so a test can assert a successful push
	// stamps the sending peer's last_contact_at.
	fedInstances *repo.FederatedInstanceRepo
	// auditor captures the per-event audit rows the handler emits on a rejection
	// (Federation v1 F6.3, US-7.4 AC1). nil keeps existing tests unaffected.
	auditor *captureEventAuditor
}

// captureEventAuditor records the per-event audit entries the events handler
// emits (Federation v1 F6.3, US-7.4 AC1).
type captureEventAuditor struct {
	entries []repo.AuditEntry
}

func (c *captureEventAuditor) Record(e repo.AuditEntry) { c.entries = append(c.entries, e) }

func (c *captureEventAuditor) only(kind repo.AuditKind) *repo.AuditEntry {
	for i := range c.entries {
		if c.entries[i].Kind == kind {
			return &c.entries[i]
		}
	}
	return nil
}

// stubRateLimiter is a deterministic inbound limiter for the F4.4 429 test: it
// allows the first allowCount AllowN calls, then throttles with retryAfterSecs.
type stubRateLimiter struct {
	allowCount     int
	retryAfterSecs int
}

func (s *stubRateLimiter) AllowN(_ string, _ int) (bool, time.Duration) {
	if s.allowCount > 0 {
		s.allowCount--
		return true, 0
	}
	return false, time.Duration(s.retryAfterSecs) * time.Second
}

func newFedEventsEnv(t *testing.T, opts ...func(*fedEventsEnv)) *fedEventsEnv {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "fedev.db"))
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
		 VALUES (1, 'c', 'blue', 'ev-ctx', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
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
	const peer = "https://peer.example"

	fedProjects := repo.NewFederatedProjectRepo(d)
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: p.ID, PeerInstanceURL: peer,
		OriginInstanceURL: peer, Permissions: model.FederationPermissionWrite,
	}); err != nil {
		t.Fatalf("peer row: %v", err)
	}

	st := store.New(d)
	fedInstances := repo.NewFederatedInstanceRepo(d)
	env := &fedEventsEnv{
		db: d, store: st, fedProjects: fedProjects, localProject: p.ID, projClientID: p.ClientID,
		peerURL: peer, peerPriv: peerPriv, peerPub: peerPub, peerPubB64: peerPubB64, fedInstances: fedInstances,
	}
	for _, opt := range opts {
		opt(env)
	}
	// Default resolver returns the seeded peer key; tests may swap env.resolveKey
	// before posting to drive a key rotation or a transient fetch failure.
	env.resolveKey = func(_ context.Context, _ string) (ed25519.PublicKey, error) {
		return peerPub, nil
	}

	// A validator that resolves the peer's key (via the swappable env.resolveKey) +
	// membership against the seeded DB.
	validator := inbox.NewValidator(
		func(ctx context.Context, instanceURL string) (ed25519.PublicKey, error) {
			return env.resolveKey(ctx, instanceURL)
		},
		inbox.DBMembershipLookup(d, fedProjects),
		func() time.Time { return time.Now() },
	)
	queue := &captureQueue{}
	// Wire the REAL service as the KeyMismatch marker so the production
	// resolve->classify->stamp chain (MarkKeyMismatchByRemote) runs end-to-end, and
	// a spy notifier so the once-only transition (SSE) can be asserted (US-4.3 AC4).
	fedSvc := fedsvc.NewService(
		d, projects, fedProjects, repo.NewFederationKeysRepo(d), repo.NewFederationInviteRepo(d),
		repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher("federation-handler-cipher-key-32-bytes!!"), peer,
	).WithStatusNotifier(notifyFunc(func(context.Context) { env.notifyCalls++ }))
	// Wire the auditor as a nil interface (not a typed-nil pointer) when unset so the
	// handler's nil guard behaves: a typed-nil *captureEventAuditor in an interface
	// is NOT == nil.
	var auditorDep httpapi.FederationAuditor
	if env.auditor != nil {
		auditorDep = env.auditor
	}
	h := handlers.NewFederationHandler(repo.NewFederationKeysRepo(d), crypto.NewTokenCipher("federation-handler-cipher-key-32-bytes!!"), peer).
		WithEventsDeps(handlers.FederationEventsDeps{
			Store:          st,
			Validator:      validator,
			Queue:          queue,
			Projects:       fedProjects,
			KeyMismatch:    fedSvc,
			RateLimiter:    env.rateLimiter,
			MaxBatchEvents: env.maxBatchEvents,
			Contact:        fedInstances,
			Auditor:        auditorDep,
		})
	env.enqueued = nil
	queue.onEnqueue = func(j inbox.Job) { env.enqueued = append(env.enqueued, j) }

	app := httpapi.NewApp(httpapi.Deps{})
	// Register the signed endpoints WITHOUT the signature middleware: the
	// middleware is exercised by federation_signature_middleware_test; here we
	// inject the verified peer directly so the handler logic is the unit under
	// test. We stash the peer in Locals the way the middleware would.
	grp := app.Group("/federation", func(c fiber.Ctx) error {
		c.Locals("federation_peer", httpapi.FederationPeer{
			InstanceURL: peer, PublicKey: peerPubB64, DisplayName: "Peer",
		})
		return c.Next()
	})
	h.RegisterSigned(grp)
	env.app = app
	return env
}

// notifyFunc adapts a func to the fedsvc.StatusNotifier interface so a test can
// count ScopeFederation SSE publishes the key-mismatch transition fires.
type notifyFunc func(context.Context)

func (f notifyFunc) NotifyFederation(ctx context.Context) { f(ctx) }

// captureQueue records enqueued jobs.
type captureQueue struct {
	onEnqueue func(inbox.Job)
}

func (q *captureQueue) Enqueue(job inbox.Job) {
	if q.onEnqueue != nil {
		q.onEnqueue(job)
	}
}

// hlcNow renders a current-wall-clock HLC string so the F3.2a clock-skew check
// (>1h past / >10min future) passes. logical disambiguates two events minted in
// the same test.
func hlcNow(logical int64) string {
	return hlc.HLC{PhysicalMS: time.Now().UnixMilli(), Logical: logical, NodeID: "nodeA"}.String()
}

func (e *fedEventsEnv) signedEvent(t *testing.T, op events.Op, entityID, fieldHLC string) events.Event {
	t.Helper()
	evt := events.Event{
		EventID:         model.NewClientID(),
		Op:              op,
		EntityType:      events.EntityTask,
		EntityID:        entityID,
		ProjectClientID: e.projClientID,
		Author:          e.peerURL,
		OriginInstance:  e.peerURL,
		CreatedAt:       model.FormatUTC(time.Now()),
		Fields:          map[string]events.Field{"title": {Value: "remote", HLC: fieldHLC}},
	}
	signed, err := events.Sign(evt, e.peerPriv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func (e *fedEventsEnv) postEvents(t *testing.T, batch events.Batch) *http.Response {
	t.Helper()
	body, _ := json.Marshal(batch)
	req := httptest.NewRequest(http.MethodPost, events.PushPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("test request: %v", err)
	}
	return resp
}

func inboxCount(t *testing.T, d *sql.DB) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federation_inbox`).Scan(&n); err != nil {
		t.Fatalf("count inbox: %v", err)
	}
	return n
}

// keyMismatchAt reads the sticky key_mismatch_at marker for the (project, peer),
// returning "" when NULL/unset. It is the durable signal GET /federation/status
// rolls up to key_mismatch (US-4.3 AC4).
func keyMismatchAt(t *testing.T, d *sql.DB, projectID int64, peerURL string) string {
	t.Helper()
	var at sql.NullString
	err := d.QueryRow(
		`SELECT key_mismatch_at FROM federated_projects WHERE local_project_id = ? AND peer_instance_url = ?`,
		projectID, peerURL).Scan(&at)
	if err != nil {
		t.Fatalf("read key_mismatch_at: %v", err)
	}
	if !at.Valid {
		return ""
	}
	return at.String
}

// TestEvents_ValidEventDedupedAndEnqueued asserts a well-formed event is recorded
// in federation_inbox once and enqueued for apply, returning fast 2xx (US-3.1/
// US-3.2). A duplicate event_id is a no-op insert (NFR-2 dedup).
func TestEvents_ValidEventDedupedAndEnqueued(t *testing.T) {
	env := newFedEventsEnv(t)
	evt := env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0))

	resp := env.postEvents(t, events.Batch{Events: []events.Event{evt}})
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, body %s", resp.StatusCode, b)
	}
	if n := inboxCount(t, env.db); n != 1 {
		t.Errorf("inbox rows: got %d, want 1", n)
	}
	if len(env.enqueued) != 1 {
		t.Errorf("enqueued: got %d, want 1", len(env.enqueued))
	}

	// Resend the SAME event_id: dedup → still one inbox row, NOT re-enqueued.
	env.enqueued = nil
	resp2 := env.postEvents(t, events.Batch{Events: []events.Event{evt}})
	if resp2.StatusCode < 200 || resp2.StatusCode >= 300 {
		t.Fatalf("dup status: %d", resp2.StatusCode)
	}
	if n := inboxCount(t, env.db); n != 1 {
		t.Errorf("inbox rows after dup: got %d, want 1 (dedup)", n)
	}
	if len(env.enqueued) != 0 {
		t.Errorf("duplicate must not re-enqueue: got %d", len(env.enqueued))
	}
}

// TestEvents_ValidPushTouchesLastContact asserts a successful inbound push
// refreshes the sending peer's last_contact_at (Federation v1 F5.6a, US-6.5
// AC1/AC3 — the push touchpoint that clears a joiner's owner-offline flag when the
// owner reaches it again). The touch advances a stale directory timestamp.
func TestEvents_ValidPushTouchesLastContact(t *testing.T) {
	env := newFedEventsEnv(t)
	// Seed the peer directory row with a STALE last_contact_at so the touch is
	// observable as an advance (a never-contacted peer would also be touched, but
	// asserting an advance is the stronger signal).
	stale := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := env.fedInstances.Upsert(context.Background(), model.FederatedInstance{
		InstanceURL: env.peerURL, PublicKey: env.peerPubB64, DisplayName: "Peer",
		LastContactAt: &stale, CreatedAt: stale, UpdatedAt: stale,
	}); err != nil {
		t.Fatalf("seed peer directory: %v", err)
	}

	evt := env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0))
	resp := env.postEvents(t, events.Batch{Events: []events.Event{evt}})
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, body %s", resp.StatusCode, b)
	}

	inst, err := env.fedInstances.Get(context.Background(), env.peerURL)
	if err != nil {
		t.Fatalf("get peer directory: %v", err)
	}
	if inst.LastContactAt == nil {
		t.Fatalf("last_contact_at: got nil, want advanced after push")
	}
	if !inst.LastContactAt.After(stale) {
		t.Errorf("last_contact_at: got %s, want advanced past the stale %s", *inst.LastContactAt, stale)
	}
}

// TestEvents_BadSignatureRejectedNoRows asserts a per-event signature failure is
// rejected with 401 and writes ZERO inbox/enqueue (US-7.2 AC1 end-to-end via the
// handler — the validator runs before any inbox write).
func TestEvents_BadSignatureRejectedNoRows(t *testing.T) {
	env := newFedEventsEnv(t)
	h := hlcNow(0)
	evt := env.signedEvent(t, events.OpUpdate, "task-1", h)
	evt.Fields["title"] = events.Field{Value: "tampered", HLC: h} // breaks the signature

	resp := env.postEvents(t, events.Batch{Events: []events.Event{evt}})
	if resp.StatusCode != 401 {
		t.Errorf("tampered event status: got %d, want 401", resp.StatusCode)
	}
	if n := inboxCount(t, env.db); n != 0 {
		t.Errorf("rejected event must write no inbox rows: got %d", n)
	}
	if len(env.enqueued) != 0 {
		t.Errorf("rejected event must not enqueue: got %d", len(env.enqueued))
	}
}

// TestEvents_KeyRotationStampsStickyMarkerOnce drives the REAL production writer
// (MarkKeyMismatchByRemote via the wired service) end-to-end: an inbound event
// whose per-event signature fails against a RESOLVED-but-DIFFERENT key (a genuine
// key rotation) is rejected 401 with ZERO inbox/domain rows AND stamps the sticky
// (project, peer) key_mismatch_at marker exactly once. A second such event does
// NOT re-transition (no duplicate ScopeFederation SSE), pinning the once-only
// semantics (US-4.3 AC4 — the "events rejected" + sticky-marker legs).
func TestEvents_KeyRotationStampsStickyMarkerOnce(t *testing.T) {
	env := newFedEventsEnv(t)
	// Resolve a DIFFERENT key than the one that signs the event: the signature was
	// produced by env.peerPriv but verifies against an unrelated key → a genuine
	// verified-and-rejected mismatch (the peer rotated its key). This reaches
	// ErrEventSignatureInvalid (NOT ErrEventKeyUnresolved), the only signal the
	// sticky marker acts on.
	rotatedPub, _, _ := ed25519.GenerateKey(rand.Reader)
	env.resolveKey = func(_ context.Context, _ string) (ed25519.PublicKey, error) {
		return rotatedPub, nil
	}

	evt := env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0))
	resp := env.postEvents(t, events.Batch{Events: []events.Event{evt}})
	if resp.StatusCode != 401 {
		t.Errorf("rotated-key event status: got %d, want 401 (US-7.2 AC1)", resp.StatusCode)
	}
	if n := inboxCount(t, env.db); n != 0 {
		t.Errorf("rejected event must write no inbox rows: got %d", n)
	}
	if len(env.enqueued) != 0 {
		t.Errorf("rejected event must not enqueue: got %d", len(env.enqueued))
	}

	// The sticky marker is stamped (the resolve->classify->mark chain ran) and the
	// transition fired exactly one SSE.
	first := keyMismatchAt(t, env.db, env.localProject, env.peerURL)
	if first == "" {
		t.Fatalf("first rotated-key event must stamp key_mismatch_at (US-4.3 AC4), got empty")
	}
	if env.notifyCalls != 1 {
		t.Errorf("transition must fire exactly one SSE: got %d", env.notifyCalls)
	}

	// A SECOND rotated-key event: still rejected, still stamped, but the marker is
	// sticky → it must NOT move and must NOT re-fire the SSE (once-only transition).
	evt2 := env.signedEvent(t, events.OpUpdate, "task-2", hlcNow(1))
	resp2 := env.postEvents(t, events.Batch{Events: []events.Event{evt2}})
	if resp2.StatusCode != 401 {
		t.Errorf("second rotated-key event status: got %d, want 401", resp2.StatusCode)
	}
	if again := keyMismatchAt(t, env.db, env.localProject, env.peerURL); again != first {
		t.Errorf("sticky marker must not move on a second mismatch: got %q, want %q", again, first)
	}
	if env.notifyCalls != 1 {
		t.Errorf("second mismatch must not re-fire SSE (sticky): got %d notify calls, want 1", env.notifyCalls)
	}
}

// TestEvents_TransientKeyResolveDoesNotStampMarker is the negative companion that
// pins the F4.3 false-positive boundary: an inbound event whose author key cannot
// be RESOLVED (a transient .well-known fetch error) is rejected as retryable BUT
// must NOT flip the project to a sticky red — key_mismatch_at stays NULL and no
// SSE fires. A brief network blip must never turn the badge permanently red
// (Federation v1 F4.3 review fix).
func TestEvents_TransientKeyResolveDoesNotStampMarker(t *testing.T) {
	env := newFedEventsEnv(t)
	env.resolveKey = func(_ context.Context, _ string) (ed25519.PublicKey, error) {
		return nil, errors.New("well-known fetch timed out")
	}

	evt := env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0))
	resp := env.postEvents(t, events.Batch{Events: []events.Event{evt}})
	if resp.StatusCode != 503 {
		t.Errorf("unresolvable-key event status: got %d, want 503 (retryable, not a rotation)", resp.StatusCode)
	}
	if n := inboxCount(t, env.db); n != 0 {
		t.Errorf("rejected event must write no inbox rows: got %d", n)
	}
	if at := keyMismatchAt(t, env.db, env.localProject, env.peerURL); at != "" {
		t.Errorf("a TRANSIENT resolve failure must NOT stamp the sticky marker: got key_mismatch_at=%q, want empty", at)
	}
	if env.notifyCalls != 0 {
		t.Errorf("a transient resolve failure must not fire an SSE: got %d", env.notifyCalls)
	}
}

// TestEvents_AuthorOriginMismatch400 asserts an event whose author != origin is
// rejected 400 with no rows (US-7.2 AC3 end-to-end).
func TestEvents_AuthorOriginMismatch400(t *testing.T) {
	env := newFedEventsEnv(t)
	evt := events.Event{
		EventID: model.NewClientID(), Op: events.OpUpdate, EntityType: events.EntityTask,
		EntityID: "task-1", ProjectClientID: env.projClientID,
		Author: env.peerURL, OriginInstance: "https://someone-else.example",
		Fields: map[string]events.Field{"title": {Value: "x", HLC: hlcNow(0)}},
	}
	signed, _ := events.Sign(evt, env.peerPriv)
	resp := env.postEvents(t, events.Batch{Events: []events.Event{signed}})
	if resp.StatusCode != 400 {
		t.Errorf("author/origin mismatch status: got %d, want 400", resp.StatusCode)
	}
	if n := inboxCount(t, env.db); n != 0 {
		t.Errorf("author-mismatch event must write no inbox rows: got %d", n)
	}
}

// TestEvents_PerEventRejectionsRecordAudit asserts the events handler records one
// audit row per per-event rejection with the correct kind and outcome=rejected
// (Federation v1 F6.3, US-7.4 AC1): a verified-but-wrong signature → signature_invalid,
// an author/origin mismatch → author_mismatch. The detail never carries a secret.
func TestEvents_PerEventRejectionsRecordAudit(t *testing.T) {
	t.Run("signature_invalid", func(t *testing.T) {
		auditor := &captureEventAuditor{}
		env := newFedEventsEnv(t, func(e *fedEventsEnv) { e.auditor = auditor })
		h := hlcNow(0)
		evt := env.signedEvent(t, events.OpUpdate, "task-1", h)
		evt.Fields["title"] = events.Field{Value: "tampered", HLC: h} // breaks the signature
		resp := env.postEvents(t, events.Batch{Events: []events.Event{evt}})
		if resp.StatusCode != 401 {
			t.Fatalf("tampered event status: got %d, want 401", resp.StatusCode)
		}
		got := auditor.only(repo.AuditKindSignatureInvalid)
		if got == nil {
			t.Fatalf("expected a signature_invalid audit row")
		}
		if got.Outcome != repo.AuditOutcomeRejected {
			t.Errorf("outcome: got %q, want rejected", got.Outcome)
		}
		if got.PeerInstanceURL != env.peerURL {
			t.Errorf("peer: got %q, want %q", got.PeerInstanceURL, env.peerURL)
		}
	})

	t.Run("author_mismatch", func(t *testing.T) {
		auditor := &captureEventAuditor{}
		env := newFedEventsEnv(t, func(e *fedEventsEnv) { e.auditor = auditor })
		evt := events.Event{
			EventID: model.NewClientID(), Op: events.OpUpdate, EntityType: events.EntityTask,
			EntityID: "task-1", ProjectClientID: env.projClientID,
			Author: env.peerURL, OriginInstance: "https://someone-else.example",
			Fields: map[string]events.Field{"title": {Value: "x", HLC: hlcNow(0)}},
		}
		signed, _ := events.Sign(evt, env.peerPriv)
		resp := env.postEvents(t, events.Batch{Events: []events.Event{signed}})
		if resp.StatusCode != 400 {
			t.Fatalf("author/origin mismatch status: got %d, want 400", resp.StatusCode)
		}
		if auditor.only(repo.AuditKindAuthorMismatch) == nil {
			t.Errorf("expected an author_mismatch audit row")
		}
	})
}

// TestEvents_PausedPeerRejected403NoRows asserts an inbound event from a peer the
// owner has paused is rejected with 403 federation_paused and writes ZERO inbox
// rows (Federation v1 F5.3, US-6.1 AC1). The pause is non-destructive — the link
// stays trusted and the same event would be accepted after a resume — so the
// reject code is the distinct federation_paused, not the generic untrusted 403.
func TestEvents_PausedPeerRejected403NoRows(t *testing.T) {
	env := newFedEventsEnv(t)
	// Pause the peer on the owner side. The peer row's OriginInstanceURL == peer in
	// this env (owner-relay shape), so the paused check must fire BEFORE the owner-
	// relay accept leg — this asserts that ordering end-to-end.
	if n, err := env.fedProjects.SetPaused(context.Background(), env.localProject, env.peerURL, true); err != nil || n != 1 {
		t.Fatalf("pause peer: n=%d err=%v", n, err)
	}

	signed := env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0))
	resp := env.postEvents(t, events.Batch{Events: []events.Event{signed}})
	if resp.StatusCode != 403 {
		t.Fatalf("paused peer status: got %d, want 403 (US-6.1 AC1)", resp.StatusCode)
	}

	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if out.Error.Code != httpapi.CodeFederationPaused {
		t.Errorf("paused reject code: got %q, want %q (distinct, resumable)", out.Error.Code, httpapi.CodeFederationPaused)
	}
	if n := inboxCount(t, env.db); n != 0 {
		t.Errorf("paused event must write no inbox rows: got %d (US-6.1 AC1)", n)
	}
}

// TestEvents_RevokedPeerRejected403NoRows asserts an inbound event from a REVOKED
// peer is rejected with 403 federation_revoked and writes ZERO inbox rows
// (Federation v1 F5.4, US-6.2 AC2). The reject code is distinct from the
// reversible paused 403 and the generic untrusted 403 so the peer can tell it has
// been permanently revoked and self-mark federation_lost.
func TestEvents_RevokedPeerRejected403NoRows(t *testing.T) {
	env := newFedEventsEnv(t)
	if n, err := env.fedProjects.Revoke(context.Background(), env.localProject, env.peerURL); err != nil || n != 1 {
		t.Fatalf("revoke peer: n=%d err=%v", n, err)
	}

	signed := env.signedEvent(t, events.OpUpdate, "task-1", hlcNow(0))
	resp := env.postEvents(t, events.Batch{Events: []events.Event{signed}})
	if resp.StatusCode != 403 {
		t.Fatalf("revoked peer status: got %d, want 403 (US-6.2 AC2)", resp.StatusCode)
	}

	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if out.Error.Code != httpapi.CodeFederationRevoked {
		t.Errorf("revoked reject code: got %q, want %q (terminal, irreversible)", out.Error.Code, httpapi.CodeFederationRevoked)
	}
	if n := inboxCount(t, env.db); n != 0 {
		t.Errorf("revoked event must write no inbox rows: got %d (US-6.2 AC2)", n)
	}
}

// TestPull_RevokedPeerRejected403 asserts a REVOKED peer's catch-up pull is
// rejected with 403 federation_revoked — the offline-return path of US-6.2 AC4: a
// peer that missed the in-band revoke event learns it is revoked on its next sync.
func TestPull_RevokedPeerRejected403(t *testing.T) {
	env := newFedEventsEnv(t)
	if n, err := env.fedProjects.Revoke(context.Background(), env.localProject, env.peerURL); err != nil || n != 1 {
		t.Fatalf("revoke peer: n=%d err=%v", n, err)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/federation/projects/"+itoa(env.localProject)+"/events?since_hlc=00000000010000-0000-nodeA", nil)
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("pull request: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("revoked pull status: got %d, want 403 (US-6.2 AC4)", resp.StatusCode)
	}
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if out.Error.Code != httpapi.CodeFederationRevoked {
		t.Errorf("revoked pull code: got %q, want %q (US-6.2 AC4)", out.Error.Code, httpapi.CodeFederationRevoked)
	}
}

// TestPull_ReturnsEventsSinceHLC asserts the pull endpoint returns the project's
// outbox events with a max field HLC strictly greater than since_hlc, ascending
// (US-3.2 AC3 pull replay).
func TestPull_ReturnsEventsSinceHLC(t *testing.T) {
	env := newFedEventsEnv(t)
	ctx := context.Background()
	// Seed two outbox events with ascending HLCs (the owner's own emitted events).
	for _, e := range []struct{ id, hlc string }{
		{"o1", "00000000010000-0000-nodeA"},
		{"o2", "00000000020000-0000-nodeA"},
	} {
		payload := `{"event_id":"` + e.id + `","fields":{"title":{"value":"x","hlc":"` + e.hlc + `"}}}`
		tx, _ := env.db.BeginTx(ctx, nil)
		if err := env.store.InsertOutboxTx(ctx, tx, e.id, env.localProject, payload, 1, "2024-01-01T00:00:00.000Z"); err != nil {
			t.Fatalf("outbox: %v", err)
		}
		_ = tx.Commit()
	}

	req := httptest.NewRequest(http.MethodGet,
		"/federation/projects/"+itoa(env.localProject)+"/events?since_hlc=00000000010000-0000-nodeA", nil)
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("pull request: %v", err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("pull status: got %d, body %s", resp.StatusCode, b)
	}
	var out events.PullResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Events) != 1 || out.Events[0].EventID != "o2" {
		t.Errorf("pull events: got %+v, want only o2", out.Events)
	}
}

// TestPull_LimitCapsBatchInHLCOrder asserts the pull handler honours the limit
// query param, returning the limit oldest-by-HLC events and a next_hlc cursor
// that advances only as far as the served batch — so the caller resumes from
// there on the next pass (Federation v1 F4.1, US-4.1 — handler ordering+limit).
func TestPull_LimitCapsBatchInHLCOrder(t *testing.T) {
	env := newFedEventsEnv(t)
	ctx := context.Background()
	// Seed three events with ascending HLCs.
	for _, e := range []struct{ id, hlc string }{
		{"a", "00000000010000-0000-nodeA"},
		{"b", "00000000020000-0000-nodeA"},
		{"c", "00000000030000-0000-nodeA"},
	} {
		payload := `{"event_id":"` + e.id + `","fields":{"title":{"value":"x","hlc":"` + e.hlc + `"}}}`
		tx, _ := env.db.BeginTx(ctx, nil)
		if err := env.store.InsertOutboxTx(ctx, tx, e.id, env.localProject, payload, 1, "2024-01-01T00:00:00.000Z"); err != nil {
			t.Fatalf("outbox: %v", err)
		}
		_ = tx.Commit()
	}

	req := httptest.NewRequest(http.MethodGet,
		"/federation/projects/"+itoa(env.localProject)+"/events?limit=2", nil)
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("pull request: %v", err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("pull status: got %d, body %s", resp.StatusCode, b)
	}
	var out events.PullResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Events) != 2 {
		t.Fatalf("limited batch size: got %d, want 2", len(out.Events))
	}
	if out.Events[0].EventID != "a" || out.Events[1].EventID != "b" {
		t.Errorf("limit batch order: got %q,%q want a,b (oldest by HLC)", out.Events[0].EventID, out.Events[1].EventID)
	}
	if out.NextHLC != "00000000020000-0000-nodeA" {
		t.Errorf("next_hlc cursor: got %q, want the served batch's max (b)", out.NextHLC)
	}
}

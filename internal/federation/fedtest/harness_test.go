// Package fedtest is the Federation v1 F7.1 two/three-instance in-process
// integration harness. It builds FULL federation nodes from httpapi.NewApp +
// app.Test() and routes every cross-instance call A↔B (handshake, snapshot,
// events push, catch-up pull) through the REAL HTTP-signature middleware and the
// REAL federation handlers — nothing is stubbed but the network transport (it is
// in-process) and the .well-known fetch (resolved from a shared key map seeded as
// instances are added). It is the foundation the §15.5 correctness scenarios
// (F7.2 HLC, F7.3 3-way convergence, F7.4 offline/snapshot, F7.5 crash-safety)
// build on.
//
// The harness deliberately uses EXPORTED, reusable types (Harness, Instance,
// PeerClient, AssertConverged, PumpOutbox) distinct from the per-file inline
// helpers the earlier F3.2/F4.1/F4.2/F5.1 slices grew, so it can be adopted
// incrementally without rewriting those tests.
//
// Key F7.1 properties (from the milestone + R1):
//   - app.Test() is synchronous, so cross-instance calls are serialized — the
//     harness never races two apps against each other.
//   - The outbox is drained SYNCHRONOUSLY via PumpOutbox (worker.DrainOnce), which
//     reads the batch, RELEASES the connection, then does the network POST — so a
//     drain never holds the lone SetMaxOpenConns(1) connection across a peer call.
//   - The snapshot bootstrap is buffer-first on the owner, so the join in the
//     smoke test does not deadlock the lone connection during bootstrap.
//   - Clock injection (WithFixedClock / WithClock) reaches the HLC store, the
//     transport timestamp, AND therefore BOTH receiver-side skew checks: the
//     transport ±5min timestamp window (FederationSignatureDeps.Now) and the
//     per-event HLC skew validator (inbox.NewDBValidator now). This is asserted by
//     TestHarness_FixedClockReachesReceiverSkewCheck, not just claimed here.
package fedtest

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/federation/nonce"
	"github.com/lebe-dev/turboist/internal/federation/outbox"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/recovery"
	fedsnapshot "github.com/lebe-dev/turboist/internal/federation/snapshot"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// convergeBudget is the NFR-1.1 push-latency budget AssertConverged polls within.
const convergeBudget = 5 * time.Second

// Harness owns a set of in-process federation instances plus the shared peer-key
// directory + router every instance signs/verifies against. One Harness models a
// whole federation (two or three instances) for one test.
type Harness struct {
	// pubKeys is the shared .well-known directory: instance_url → public key b64.
	// It is the source the resolver (and thus every signature verification + owner
	// key corroboration) reads, populated as AddInstance registers each node.
	pubKeys map[string]string
	// displayNames mirrors pubKeys for the handshake display-name round-trip.
	displayNames map[string]string
	// apps routes an instance_url to its in-process Fiber app (the PeerClient target).
	apps map[string]*fiber.App
	// cache is the shared peer-key cache backed by the resolver below; all
	// instances share it so a warmed key on join is visible to every node.
	cache *peerkeys.Cache
	// tamperNextBody, when true, flips one byte of the NEXT routed request body so a
	// test can prove the real signature middleware rejects a tampered request (it is
	// reset after one use). It models a malicious relay / corruption on the wire.
	tamperNextBody bool
}

// NewHarness constructs an empty federation harness. Instances are added with
// AddInstance; the shared resolver reads the harness's growing key directory so a
// node added later is still resolvable by one added earlier.
func NewHarness(t *testing.T) *Harness {
	t.Helper()
	h := &Harness{
		pubKeys:      map[string]string{},
		displayNames: map[string]string{},
		apps:         map[string]*fiber.App{},
	}
	resolver := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return h.wellKnown(instanceURL), nil
	}
	h.cache = peerkeys.NewCache(resolver)
	return h
}

// wellKnown returns the published discovery document for url from the harness
// directory — the in-process source the shared key resolver and the joiner's
// independent owner-key corroboration both read.
func (h *Harness) wellKnown(url string) *peerkeys.Instance {
	url = trimSlashURL(url)
	return &peerkeys.Instance{
		InstanceURL: url,
		PublicKey:   h.pubKeys[url],
		DisplayName: h.displayName(url),
	}
}

// displayName returns the registered display name for url (host fallback).
func (h *Harness) displayName(url string) string {
	if dn := h.displayNames[url]; dn != "" {
		return dn
	}
	return url
}

// instanceOption tweaks an Instance at construction.
type instanceOption func(*instanceConfig)

type instanceConfig struct {
	clock            func() time.Time
	hlcClock         func() time.Time
	reBroadcast      bool
	permissiveNonces bool
}

// WithFixedClock pins the instance's wall clock to a fixed instant so its HLC and
// transport timestamps are deterministic (F7.1: clock injection must reach HLC +
// nonce + skew; F7.2/F7.4 drive ordering/skew through it).
func WithFixedClock(at time.Time) instanceOption {
	return func(c *instanceConfig) { c.clock = func() time.Time { return at } }
}

// WithClock injects an arbitrary clock function (e.g. an advancing fake) onto the
// instance — the general form of WithFixedClock.
func WithClock(now func() time.Time) instanceOption {
	return func(c *instanceConfig) { c.clock = now }
}

// WithHLCClock injects a SEPARATE clock for ONLY the HLC store (and therefore the
// emitter that stamps an event's per-field HLC physical_ms), leaving the
// transport/auth clock — the outbound signature timestamp, the receiver's ±5min
// window, and the per-event validator's skew reference — on the instance's base
// clock (WithFixedClock / WithClock, default time.Now).
//
// This is the seam F7.4 ("isolate federation clock from auth clocks") needs: it
// lets a test skew an instance's FEDERATION (HLC) clock relative to the federation
// it talks to WITHOUT moving its transport/auth clock out of the other side's
// ±5min window. So a +6min federation skew can be a SOFT pass (handshake/push stay
// inside the transport window; the event HLC is +6min in the future, below the
// 10min hard ceiling, so the per-event validator accepts it; the HLC Recv-merge
// still converges) while a +6min TRANSPORT skew (a skewed base clock, not this
// one) remains a hard 401 and a +11min HLC skew (this clock, above the ceiling)
// remains a hard 400. When unset, the HLC clock IS the base clock (the production
// wiring, where one time.Now feeds both).
func WithHLCClock(now func() time.Time) instanceOption {
	return func(c *instanceConfig) { c.hlcClock = now }
}

// WithReBroadcast enables owner-hub re-broadcast on this instance's applier
// (Federation v1 F5.1, US-5.2 AC2): an apply that changes an entity of a project
// this instance owns is re-enqueued for fan-out to the OTHER peers.
func WithReBroadcast() instanceOption {
	return func(c *instanceConfig) { c.reBroadcast = true }
}

// WithPermissiveNonces swaps this instance's transport anti-replay nonce cache for
// a no-op (always-fresh) one. It exists ONLY to neutralise an in-process TEST
// artifact, never to weaken a real federation assertion:
//
// Fiber v3's app.Test() serves the raw request over a reusable testConn whose
// ServeConn loop can intermittently re-parse and DOUBLE-PROCESS the SAME signed
// request — even for a single, sequential Send under no concurrency. When that
// happens the receiver applies the first copy and rejects the re-served IDENTICAL
// copy as a transport replay (federation_replay, 401). For the multi-hop owner-hub
// convergence tests (F7.3) that 401 makes the outbox worker dead-letter the event,
// permanently stranding it and breaking convergence — a pure harness flake, not a
// domain bug.
//
// F7.3 verifies DOMAIN convergence (per-field LWW, tombstone no-resurrection, and
// dedup BY EVENT_ID — the real NFR-2.2 idempotency mechanism, which is unaffected
// and still asserted). The TRANSPORT anti-replay (nonce) is a SEPARATE concern
// owned by F0.3, exercised by the dedicated HTTPSignatureMiddleware tests and by
// the smoke body-tamper no-bypass guard (both single-request, deterministic, and
// stable). Disabling only the transport nonce cache here removes the app.Test
// re-serve flake without weakening any F7.3 assertion or the real anti-replay
// coverage elsewhere.
func WithPermissiveNonces() instanceOption {
	return func(c *instanceConfig) { c.permissiveNonces = true }
}

// Instance is one full in-process federation node: its own migrated SQLite DB,
// repos, federation store, the federation Service (handshake/invite/snapshot),
// the inbox apply queue + per-event validator, and a Fiber app exposing the REAL
// signed federation group. It also carries the joiner-side collaborators (sender,
// publisher) wired to route through the harness PeerClient.
type Instance struct {
	h   *Harness
	url string
	now func() time.Time

	db          *sql.DB
	dbPath      string
	store       *store.Store
	keys        *repo.FederationKeysRepo
	fedProjects *repo.FederatedProjectRepo
	projects    *repo.ProjectRepo
	tasks       *repo.TaskRepo
	cipher      *crypto.TokenCipher

	app       *fiber.App
	queue     *inbox.Queue
	validator *inbox.Validator
	svc       *fedsvc.Service
	hlc       *hlc.Store
	publisher *fedsvc.Publisher
	worker    *outbox.Worker
	pubKeyB64 string

	// pingEmitter, when set by StartCommitPingWorker, is the commit-ping-wired
	// emitter every subsequent Mutator() uses, so a federated mutation pushes
	// immediately on commit (the NFR-1.1 mechanism) rather than waiting on a
	// manual PumpOutbox or a polling tick. nil → emitter() mints a fresh
	// un-pinged emitter (the default for the synchronous-drain F7.1/F7.3 tests).
	pingEmitter *fedsvc.Emitter
}

// AddInstance builds a full federation node at url and registers it in the
// harness directory (key + display name + app), so every other instance can
// resolve and route to it. It wires the REAL signature middleware in front of the
// REAL signed federation handlers (handshake/snapshot/events/pull), the snapshot
// service, the inbox apply queue + validator, and the joiner-side publisher
// routing through the harness PeerClient.
func (h *Harness) AddInstance(t *testing.T, url string, opts ...instanceOption) *Instance {
	t.Helper()
	url = trimSlashURL(url)

	var cfg instanceConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	clock := cfg.clock
	if clock == nil {
		clock = time.Now
	}
	// The HLC (federation) clock defaults to the transport/auth clock — in production
	// one time.Now feeds both. WithHLCClock decouples them so F7.4 can skew the
	// federation clock without moving the transport/auth clock out of the ±5min window.
	hlcClock := cfg.hlcClock
	if hlcClock == nil {
		hlcClock = clock
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "node.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db (%s): %v", url, err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := db.RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate (%s): %v", url, err)
	}

	cipher := crypto.NewTokenCipher(cipherKey)
	keys := repo.NewFederationKeysRepo(d)
	fk, err := keys.Ensure(ctx, cipher, url)
	if err != nil {
		t.Fatalf("ensure keys (%s): %v", url, err)
	}

	// Seed the single context (id=1) every project/task is placed under.
	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, client_id, created_at, updated_at)
		 VALUES (1, 'c', 'blue', ?, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
		model.NewClientID()); err != nil {
		t.Fatalf("seed context (%s): %v", url, err)
	}

	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	tasks := repo.NewTaskRepo(d, repo.NewTaskLabelsRepo(d))
	fedProjects := repo.NewFederatedProjectRepo(d)
	sections := repo.NewProjectSectionRepo(d)
	st := store.New(d)

	in := &Instance{
		h: h, url: url, now: clock,
		db: d, dbPath: dbPath, store: st, keys: keys, fedProjects: fedProjects,
		projects: projects, tasks: tasks, cipher: cipher,
		hlc:       hlc.NewStore(d, fk.NodeID).WithClock(hlcClock),
		pubKeyB64: fk.PublicKey,
	}

	// Inbox apply: per-field LWW through the production applier, optionally with the
	// owner-hub re-broadcast relay enabled.
	applier := inbox.NewApplier(d, tasks, projects, sections, fedProjects, st)
	if cfg.reBroadcast {
		applier = applier.WithReBroadcast(st, url, func() {})
	}
	in.queue = inbox.NewQueue(applier, nil, inbox.NewStoreRecoverer(st), nil)
	// The per-event HLC skew validator (F3.2a, US-7.2 AC4) is the OTHER receiver-side
	// skew check the doc/spec contract covers ("clock injection must reach HLC +
	// nonce + skew"). Thread the injected clock here too, otherwise an event whose
	// HLC is stamped on a fixed clock would be rejected federation_clock_skew against
	// the real wall clock. nil defaults to time.Now (the production wiring).
	in.validator = inbox.NewDBValidator(d, fedProjects, h.cache, clock)

	// Federation service (handshake/invite/snapshot build) + joiner-side join deps
	// routed through the harness PeerClient, on the injected clock.
	svc := fedsvc.NewService(
		d, projects, fedProjects, keys,
		repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), cipher, url,
	)
	svc.WithSnapshotDeps(tasks, sections, repo.NewContextRepo(d), repo.NewFederationSnapshotRepo(d))
	// The joiner's independent .well-known fetch (US-2.2 AC2 owner-key
	// corroboration) reads the harness directory directly — the in-process stand-in
	// for a real GET of the owner's published discovery document.
	fetch := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return h.wellKnown(instanceURL), nil
	}
	svc.WithJoinDeps(h.PeerClient(), fetch, h.cache, clock)
	in.svc = svc

	// REAL signature middleware in front of the REAL signed handlers, including the
	// snapshot route backed by the service and the events deps (membership for the
	// token-less re-bootstrap + the push/pull endpoints).
	fedHandler := handlers.NewFederationHandler(keys, cipher, url).
		WithService(svc).
		WithEventsDeps(handlers.FederationEventsDeps{
			Store: st, Validator: in.validator, Queue: in.queue, Projects: fedProjects, BaseURL: url,
		})
	app := httpapi.NewApp(httpapi.Deps{})
	// Thread the instance's injected clock into the RECEIVER's transport
	// ±5min window check too (Now), not just the HLC store / outbound timestamp:
	// otherwise a request stamped on the injected clock would be compared against
	// the real wall clock and rejected 401 federation_timestamp_stale. In
	// production this field is deliberately left at time.Now (cmd/turboist/main.go).
	// This is what makes the package-doc contract — "clock injection reaches the
	// HLC store, the transport timestamp, AND therefore the skew checks" — true,
	// and what lets F7.4 drive a +6min handshake as a SOFT warn rather than a
	// spurious HARD 401 from an un-injected receiver window.
	// The real anti-replay nonce cache by default; a no-op cache when the test opts
	// in (WithPermissiveNonces) to neutralise the app.Test() re-serve flake without
	// weakening any F7.3 domain assertion (see WithPermissiveNonces).
	nonces := nonce.NewCache()
	if cfg.permissiveNonces {
		nonces = nonce.NewDisabledCache()
	}
	signed := app.Group("/federation", httpapi.HTTPSignatureMiddleware(httpapi.FederationSignatureDeps{
		Nonces:   nonces,
		PeerKeys: h.cache,
		Now:      clock,
	}))
	fedHandler.RegisterSigned(signed)
	in.app = app

	// Joiner/owner-side publisher (push + pull) routed through the PeerClient.
	in.publisher = fedsvc.NewPublisher(fedProjects, keys, cipher, h.PeerClient(), url, clock)
	in.worker = outbox.NewWorker(st, in.publisher, in.publisher, nil)

	// Register the node in the shared directory so peers can resolve + route to it.
	h.pubKeys[url] = fk.PublicKey
	h.displayNames[url] = fk.DisplayName
	h.apps[url] = app
	return in
}

// URL returns this instance's federation instance_url.
func (in *Instance) URL() string { return in.url }

// DB exposes the instance's *sql.DB for assertions.
func (in *Instance) DB() *sql.DB { return in.db }

// Service exposes the instance's federation service (handshake/invite/snapshot).
func (in *Instance) Service() *fedsvc.Service { return in.svc }

// Mutator builds a production TaskMutator over this instance's emitter so a test
// drives federated task creates/updates through the real emit path (HLC bump +
// signed outbox event), on the instance's injected clock.
func (in *Instance) Mutator() *fedsvc.TaskMutator {
	return fedsvc.NewTaskMutator(in.emitter(), in.tasks)
}

// emitter builds a transactional emitter on the instance's injected clock so an
// emit's HLC physical_ms tracks the same wall clock the harness pinned. When
// StartCommitPingWorker has wired a commit-ping emitter (the NFR-1.1 path), that
// one is returned so the emit pings the publisher worker on commit.
func (in *Instance) emitter() *fedsvc.Emitter {
	if in.pingEmitter != nil {
		return in.pingEmitter
	}
	return fedsvc.NewEmitter(in.db, in.keys, in.cipher, in.hlc, in.url)
}

// CreateFederatedProject creates a local project and enables federation on it
// through the PRODUCTION enable path (flips is_federated, writes the is_owner=1
// self-row, ensures the keypair). It returns the local int64 project id.
func (in *Instance) CreateFederatedProject(t *testing.T, ctx context.Context, title string) int64 {
	t.Helper()
	p, err := in.projects.Create(ctx, repo.CreateProject{ContextID: 1, Title: title, Color: "blue"})
	if err != nil {
		t.Fatalf("create project (%s): %v", in.url, err)
	}
	if _, err := in.svc.EnableForProject(ctx, p.ID); err != nil {
		t.Fatalf("enable federation (%s): %v", in.url, err)
	}
	return p.ID
}

// CreateInvite mints a real share invite for projectID at the given permission
// grade and returns the parsed (id, secret) a joiner holds.
func (in *Instance) CreateInvite(t *testing.T, ctx context.Context, projectID int64, perm model.FederationPermission) fedsvc.ParsedInvite {
	t.Helper()
	res, err := in.svc.CreateInvite(ctx, projectID, fedsvc.CreateInviteParams{Permissions: perm})
	if err != nil {
		t.Fatalf("create invite (%s): %v", in.url, err)
	}
	return fedsvc.ParsedInvite{InviteID: res.InviteID, Secret: res.Secret}
}

// Join performs the joiner side of the handshake + snapshot bootstrap against the
// owner, routed through the harness PeerClient and the owner's REAL signed
// handshake/snapshot endpoints. It fails the test on any error.
func (in *Instance) Join(t *testing.T, ctx context.Context, ownerURL string, invite fedsvc.ParsedInvite) *fedsvc.JoinResult {
	t.Helper()
	res, err := in.TryJoin(ctx, ownerURL, invite)
	if err != nil {
		t.Fatalf("join %s → %s: %v", in.url, ownerURL, err)
	}
	return res
}

// TryJoin is Join without the test-fatal wrapper, for negative tests (e.g. a
// tampered handshake must return an error and create no rows).
func (in *Instance) TryJoin(ctx context.Context, ownerURL string, invite fedsvc.ParsedInvite) (*fedsvc.JoinResult, error) {
	return in.svc.Join(ctx, trimSlashURL(ownerURL), invite, in.h.displayName(in.url))
}

// StartApply starts the instance's single inbox-apply goroutine for the duration
// of the test (cancelled on cleanup), so a pushed event is applied off the HTTP
// path the way production runs it.
func (in *Instance) StartApply(t *testing.T, ctx context.Context) {
	t.Helper()
	applyCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	in.queue.Start(applyCtx)
}

// PumpOutbox drains this instance's outbox SYNCHRONOUSLY through the real
// publisher fan-out (worker.DrainOnce): for every project with pending events it
// resolves the peers, reads each peer's undelivered batch, RELEASES the
// connection, POSTs to the peer's REAL signed /federation/events through the
// PeerClient, and stamps delivered rows in a short tx. It never holds the lone
// connection across the network call (R1). This is the F7.1 synchronous drain.
func (in *Instance) PumpOutbox(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := in.worker.DrainOnce(ctx); err != nil {
		t.Fatalf("pump outbox (%s): %v", in.url, err)
	}
}

// StartWorker launches the instance's PRODUCTION publisher worker goroutine under
// ctx with the given safety-net tick interval (outbox.Worker.Start), exactly as
// cmd/turboist wires it under cleanupCtx. F7.5 uses it with a LONG interval and a
// ctx the test cancels so the only thing that can deliver a just-committed event is
// the run loop's best-effort FINAL drain on ctx cancel (the graceful-shutdown leg
// of NFR-2.1). Pair with StopWorker, which blocks until that final drain returns.
func (in *Instance) StartWorker(t *testing.T, ctx context.Context, interval time.Duration) {
	t.Helper()
	in.worker.Start(ctx, interval)
}

// StopWorker blocks until the instance's publisher worker goroutine has returned
// (outbox.Worker.Stop). It is called AFTER the worker's context is cancelled, so it
// observes the best-effort final drain the run loop performs on the way out — the
// cmd/turboist graceful-shutdown sequence (cleanupCancel → Worker.Stop). The worker
// must have been started with StartWorker.
func (in *Instance) StopWorker(t *testing.T) {
	t.Helper()
	in.worker.Stop()
}

// EmitTaskCreate drives the instance's PRODUCTION transactional Emitter for an
// op=create task on a federated project with a CALLER-SUPPLIED write closure, so a
// crash-safety test can inject a domain-write FAILURE inside the emit tx and assert
// the whole tx (domain + entity_field_hlc + federation_outbox) rolls back together
// (F7.5 NFR-2 one-tx atomicity). The closure receives the SAME *sql.Tx the
// federation sidecar writes under; returning an error from it rolls everything back.
// It is the deliberately-failing analogue of TaskMutator.Create (which uses the same
// EmitMutation path with the real CreateTx closure).
func (in *Instance) EmitTaskCreate(ctx context.Context, localProjectID int64, clientID string, create repo.CreateTask, write func(tx *sql.Tx) error) error {
	return in.emitter().EmitMutation(ctx, fedsvc.MutationSpec{
		LocalProjectID: localProjectID,
		EntityType:     events.EntityTask,
		EntityID:       clientID,
		Op:             events.OpCreate,
		Fields:         map[string]any{"title": create.Title},
	}, write)
}

// ReopenDB approximates a kill -9 + process restart on this instance: it CLOSES the
// instance's *sql.DB and reopens the SAME on-disk DB file via the production
// db.Open (WAL + synchronous=NORMAL), then rebuilds every collaborator bound to the
// connection — the federation store, the repos, the publisher, and the publisher
// WORKER — over the reopened DB. Anything durably committed before the close (an
// undelivered federation_outbox row) is still present after; a fresh PumpOutbox over
// the rebuilt worker re-sends it (NFR-2.1 at-least-once across a crash).
//
// Only the connection-bound delivery collaborators are rebuilt; the in-process app
// (the signed receive side) and the harness directory are unchanged — a restart
// re-establishes the same federation identity from the durable federation_keys row.
func (in *Instance) ReopenDB(t *testing.T) {
	t.Helper()
	if err := in.db.Close(); err != nil {
		t.Fatalf("close db for reopen (%s): %v", in.url, err)
	}
	d, err := db.Open(in.dbPath)
	if err != nil {
		t.Fatalf("reopen db (%s): %v", in.url, err)
	}
	t.Cleanup(func() { _ = d.Close() })

	in.db = d
	in.store = store.New(d)
	in.keys = repo.NewFederationKeysRepo(d)
	in.fedProjects = repo.NewFederatedProjectRepo(d)
	in.projects = repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	in.tasks = repo.NewTaskRepo(d, repo.NewTaskLabelsRepo(d))

	// Rebuild the publisher + worker over the reopened DB exactly as AddInstance
	// wires them (the same PeerClient transport, the same injected clock). The
	// undelivered outbox row read by ListUndeliveredForPeer is the durable source of
	// truth the re-sent delivery flows from.
	in.publisher = fedsvc.NewPublisher(in.fedProjects, in.keys, in.cipher, in.h.PeerClient(), in.url, in.now)
	in.worker = outbox.NewWorker(in.store, in.publisher, in.publisher, nil)
}

// RedeliverToOwner re-POSTs a verbatim signed event payload to ownerURL's REAL
// signed /federation/events a second time through this instance's production
// publisher (the same transport-signing path PumpOutbox uses). It models
// at-least-once redelivery (NFR-2): the receiver must dedup on the event's id so a
// redelivered event is a no-op. It fails the test on a transport error (a non-2xx
// is surfaced as an error by Publisher.Push).
func (in *Instance) RedeliverToOwner(t *testing.T, ctx context.Context, ownerURL, payload string) {
	t.Helper()
	if err := in.publisher.Push(ctx, ownerURL, []string{payload}); err != nil {
		t.Fatalf("redeliver to %s (%s): %v", ownerURL, in.url, err)
	}
}

// TaskByClientID loads a live task by its cross-instance client_id, failing the
// test if it is absent.
func (in *Instance) TaskByClientID(t *testing.T, ctx context.Context, clientID string) *model.Task {
	t.Helper()
	var id int64
	if err := in.db.QueryRow(`SELECT id FROM tasks WHERE client_id = ? AND deleted_at IS NULL`, clientID).Scan(&id); err != nil {
		t.Fatalf("resolve task by client_id (%s): %v", in.url, err)
	}
	task, err := in.tasks.Get(ctx, id)
	if err != nil {
		t.Fatalf("get task (%s): %v", in.url, err)
	}
	return task
}

// TaskTitle returns the live title of a task by cross-instance client_id, or ""
// when the task is absent or tombstoned — the convergence predicate primitive.
func (in *Instance) TaskTitle(clientID string) string {
	var title string
	if err := in.db.QueryRow(`SELECT title FROM tasks WHERE client_id = ? AND deleted_at IS NULL`, clientID).Scan(&title); err != nil {
		return ""
	}
	return title
}

// FirstOutboxPayload returns the payload of the earliest outbox row for a project.
func (in *Instance) FirstOutboxPayload(t *testing.T, projectID int64) string {
	t.Helper()
	var payload string
	if err := in.db.QueryRow(
		`SELECT payload FROM federation_outbox WHERE local_project_id = ? ORDER BY id ASC LIMIT 1`,
		projectID).Scan(&payload); err != nil {
		t.Fatalf("first outbox payload (%s): %v", in.url, err)
	}
	return payload
}

// PeerClient returns the cross-instance router that delivers a transport-signed
// federation request to the in-process app for the request's host (the
// fedsvc.HandshakeSender every join/push/pull is wired to). It exercises the REAL
// signature middleware + handlers — there is no bypass.
func (h *Harness) PeerClient() *PeerClient {
	return &PeerClient{h: h}
}

// TamperNextBody arms a one-shot body tamper on the NEXT routed request, so a
// test can prove the real signature middleware rejects a request whose signed
// body was altered after signing (the §15.5 / F7.1 no-bypass guard).
func (h *Harness) TamperNextBody() { h.tamperNextBody = true }

// PeerClient routes an outbound SignedRequest to the in-process Fiber app for the
// request's host, exercising the REAL signature middleware + handlers. It is the
// in-process stand-in for production's HTTP client; app.Test() is synchronous, so
// cross-instance calls are serialized (no app races).
type PeerClient struct {
	h *Harness
}

// Send delivers req to the target instance's app via app.Test() and returns its
// status + body. A request to an unknown host returns 502 (no panic), matching a
// real client hitting a down peer.
func (pc *PeerClient) Send(_ context.Context, req fedsvc.SignedRequest) (*fedsvc.SignedResponse, error) {
	app := pc.h.apps[hostOf(req.URL)]
	if app == nil {
		return &fedsvc.SignedResponse{StatusCode: http.StatusBadGateway}, nil
	}

	body := req.Body
	if pc.h.tamperNextBody {
		pc.h.tamperNextBody = false
		body = tamperBytes(body)
	}

	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	httpReq := httptest.NewRequest(req.Method, req.URL, reader)
	if body != nil {
		httpReq.ContentLength = int64(len(body))
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := app.Test(httpReq, fiber.TestConfig{Timeout: convergeBudget})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return &fedsvc.SignedResponse{StatusCode: resp.StatusCode, Body: readAllBody(resp)}, nil
}

// AssertConverged polls cond until it is true or the NFR-1.1 5s budget elapses,
// failing the test with msg if it never converges. It is the harness's single
// convergence gate — every two/three-instance test asserts the converged state
// through it rather than open-coding a poll loop.
func AssertConverged(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(convergeBudget)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("did not converge within %s: %s", convergeBudget, msg)
	}
}

// SeedRemoteProjectID points a joined mapping row at the owner's project id so the
// pull URL targets the owner's project segment (the F4.1 pull leg needs this when
// the mapping is seeded rather than established by a real Join).
func (in *Instance) SeedRemoteProjectID(t *testing.T, localProjectID, remoteProjectID int64) {
	t.Helper()
	if _, err := in.db.Exec(
		`UPDATE federated_projects SET remote_project_id = ? WHERE local_project_id = ? AND is_owner = 0`,
		strconv.FormatInt(remoteProjectID, 10), localProjectID); err != nil {
		t.Fatalf("set remote_project_id (%s): %v", in.url, err)
	}
}

// RecoveryLoop builds this instance's PRODUCTION recovery pull loop, wired exactly
// as cmd/turboist wires it: the instance's federation store enumerates the joined
// pull targets, the instance's Publisher issues the signed catch-up GET (Puller),
// the StoreSink durably records + enqueues to the running apply queue, the REAL
// per-event validator authenticates every pulled event (F3.2a), and the
// ReBootstrapConsumer (over the instance's snapshot-capable Service) CONSUMES a 410
// stale_pull into a re-bootstrap (F4.2). It is the single seam F7.4 drives a stale
// pull → 410 → snapshot re-bootstrap through end-to-end. StartApply must be called
// before RunOnce so a re-bootstrapped / pulled event is applied off the HTTP path.
func (in *Instance) RecoveryLoop() *recovery.Loop {
	consumer := fedsvc.NewReBootstrapConsumer(in.svc, nil, nil)
	return recovery.NewLoop(in.store, in.publisher, recovery.NewStoreSink(in.store, in.queue), nil).
		WithValidator(in.validator).
		WithStaleConsumer(consumer)
}

// SetStaleCursor pins the JOINED peer mapping's last_received_hlc to an old HLC so
// the next signed pull is answered 410 once the owner's outbox is pruned above it —
// the F7.4 "joiner fell behind retention" precondition.
func (in *Instance) SetStaleCursor(t *testing.T, localProjectID int64, sinceHLC string) {
	t.Helper()
	if _, err := in.db.Exec(
		`UPDATE federated_projects SET last_received_hlc = ? WHERE local_project_id = ? AND is_owner = 0`,
		sinceHLC, localProjectID); err != nil {
		t.Fatalf("set stale cursor (%s): %v", in.url, err)
	}
}

// AdvancePrunedFloor advances the OWNER's per-project durable pruned floor to
// floorHLC (the F3.3 §15.5 "outbox GC'd above the joiner's cursor" precondition):
// once the floor sits above a peer's since_hlc with no retained outbox, the owner's
// pull endpoint answers 410 stale_pull regardless of whether any outbox rows remain.
func (in *Instance) AdvancePrunedFloor(t *testing.T, ctx context.Context, localProjectID int64, floorHLC, now string) {
	t.Helper()
	if _, err := in.store.AdvancePrunedFloor(ctx, localProjectID, floorHLC, now); err != nil {
		t.Fatalf("advance pruned floor (%s): %v", in.url, err)
	}
}

// SeedUnsentOutbox seeds an UNSENT local outbox row (delivered_to empty) under a
// project so a test can assert the re-bootstrap does NOT clear the joiner's
// pending work (R3, the highest-impact F4.2 bug). The payload is opaque here — the
// assertion is only that the row survives the overwrite tx.
func (in *Instance) SeedUnsentOutbox(t *testing.T, localProjectID int64, eventID string) {
	t.Helper()
	if _, err := in.db.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
		 VALUES (?, ?, '{}', '', '2026-06-02T12:00:00.000Z')`,
		eventID, localProjectID); err != nil {
		t.Fatalf("seed unsent outbox (%s): %v", in.url, err)
	}
}

// OutboxRowCount counts outbox rows carrying eventID — used to assert an unsent
// event survives a re-bootstrap (count stays 1) without inspecting its payload.
func (in *Instance) OutboxRowCount(eventID string) int {
	var n int
	if err := in.db.QueryRow(
		`SELECT COUNT(*) FROM federation_outbox WHERE event_id = ?`, eventID).Scan(&n); err != nil {
		return -1
	}
	return n
}

// TaskTombstoned reports whether the task with clientID EXISTS locally AND carries
// a deleted_at tombstone (a soft-delete, never a hard DELETE — §8). An absent row
// returns false, distinguishing "deleted" from "never seen" — the no-resurrection
// predicate for the offline re-bootstrap scenario.
func (in *Instance) TaskTombstoned(clientID string) bool {
	var deletedAt sql.NullString
	if err := in.db.QueryRow(`SELECT deleted_at FROM tasks WHERE client_id = ?`, clientID).Scan(&deletedAt); err != nil {
		return false
	}
	return deletedAt.Valid && deletedAt.String != ""
}

// ReBootstrapMarker reads the re-bootstrap cutoff X (cutoff HLC + wall-clock)
// stamped on the JOINED peer mapping after a 410 consume (US-4.2 AC4). Both must be
// real persisted values for the F4.2 ResyncBanner to render X.
func (in *Instance) ReBootstrapMarker(t *testing.T, localProjectID int64) (cutoffHLC, rebootstrappedAt string) {
	t.Helper()
	var hlcPtr, atPtr *string
	if err := in.db.QueryRow(
		`SELECT rebootstrap_cutoff_hlc, rebootstrapped_at FROM federated_projects WHERE local_project_id = ? AND is_owner = 0`,
		localProjectID).Scan(&hlcPtr, &atPtr); err != nil {
		t.Fatalf("read re-bootstrap marker (%s): %v", in.url, err)
	}
	if hlcPtr != nil {
		cutoffHLC = *hlcPtr
	}
	if atPtr != nil {
		rebootstrappedAt = *atPtr
	}
	return cutoffHLC, rebootstrappedAt
}

// PeerRowCount returns how many NON-owner peer mapping rows exist under a project —
// used to assert a hard-rejected (transport-skewed / tampered) handshake created no
// owner-side relationship.
func (in *Instance) PeerRowCount(localProjectID int64) int {
	var n int
	if err := in.db.QueryRow(
		`SELECT COUNT(*) FROM federated_projects WHERE local_project_id = ? AND is_owner = 0`,
		localProjectID).Scan(&n); err != nil {
		return -1
	}
	return n
}

// PushFirstOutboxToOwner POSTs the earliest outbox payload for localProjectID to
// ownerURL's REAL signed /federation/events through this instance's production
// Publisher and returns the receiver's HTTP status. Unlike PumpOutbox (which fails
// the test on a non-2xx), this surfaces the status so a test can assert a HARD
// per-event reject — e.g. a +11min-HLC event must return 400 federation_clock_skew
// (F3.2a), distinct from the +6min soft accept (200). A 2xx push returns 200; a
// classified non-2xx surfaces the receiver's real status from the publisher's
// RemoteHandshakeError; a transport failure fails the test.
func (in *Instance) PushFirstOutboxToOwner(t *testing.T, ctx context.Context, ownerURL string, localProjectID int64) int {
	t.Helper()
	payload := in.FirstOutboxPayload(t, localProjectID)
	err := in.publisher.Push(ctx, trimSlashURL(ownerURL), []string{payload})
	if err == nil {
		return http.StatusOK
	}
	var remote *fedsvc.RemoteHandshakeError
	if errors.As(err, &remote) {
		return remote.StatusCode
	}
	t.Fatalf("push first outbox to %s (%s): %v", ownerURL, in.url, err)
	return 0
}

// ProjectClientID returns the cross-instance client_id of a local project — the
// project_client_id every event targeting that project carries (F7.6 needs it to
// address apply events at the seeded corpus).
func (in *Instance) ProjectClientID(t *testing.T, localProjectID int64) string {
	t.Helper()
	var clientID string
	if err := in.db.QueryRow(
		`SELECT client_id FROM projects WHERE id = ? AND deleted_at IS NULL`, localProjectID).Scan(&clientID); err != nil {
		t.Fatalf("project client_id (%s): %v", in.url, err)
	}
	return clientID
}

// SeedFederatedTasks bulk-inserts n federated tasks under localProjectID, each
// carrying a fresh cross-instance client_id AND a baseline `title` per-field HLC
// row, and returns the client_ids. It is the F7.6 corpus builder: it lets the
// inbox-apply p95 benchmark run against a project already holding 100k entities
// and the snapshot/bootstrap benchmarks against 10k/1k. The inserts share ONE
// transaction so seeding 100k rows on SetMaxOpenConns(1) is a single commit, not
// 100k. The baseline field_hlc row sits BELOW any apply HLC so an apply event
// exercises the steady-state CAS (read prior → advance), the realistic apply
// cost rather than the create-on-missing path.
func (in *Instance) SeedFederatedTasks(t *testing.T, ctx context.Context, localProjectID int64, n int) []string {
	t.Helper()
	clientIDs := make([]string, n)
	const baselineHLC = "00000000000001-0000-seed" // below every apply HLC
	now := model.FormatUTC(in.now())
	tx, err := in.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("seed tasks begin (%s): %v", in.url, err)
	}
	defer func() { _ = tx.Rollback() }()

	taskStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO tasks (title, context_id, project_id, client_id, created_at, updated_at)
		 VALUES (?, 1, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatalf("seed tasks prepare task (%s): %v", in.url, err)
	}
	defer func() { _ = taskStmt.Close() }()

	hlcStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO entity_field_hlc (entity_type, entity_id, field_name, hlc) VALUES ('task', ?, 'title', ?)`)
	if err != nil {
		t.Fatalf("seed tasks prepare hlc (%s): %v", in.url, err)
	}
	defer func() { _ = hlcStmt.Close() }()

	for i := 0; i < n; i++ {
		cid := model.NewClientID()
		clientIDs[i] = cid
		if _, err := taskStmt.ExecContext(ctx, "seed", localProjectID, cid, now, now); err != nil {
			t.Fatalf("seed task %d (%s): %v", i, in.url, err)
		}
		if _, err := hlcStmt.ExecContext(ctx, cid, baselineHLC); err != nil {
			t.Fatalf("seed task hlc %d (%s): %v", i, in.url, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("seed tasks commit (%s): %v", in.url, err)
	}
	return clientIDs
}

// ApplyEvent applies a single decoded event through a PRODUCTION inbox Applier
// over this instance's DB — the exact per-field-LWW merge the single inbox-apply
// goroutine runs (resolve project → ghost-row if missing → per-field CAS over
// entity_field_hlc → domain UPDATE). It is the seam F7.6 samples inbox-apply
// latency through. The Applier is stateless over the connection, so a fresh one
// here behaves identically to the queue's (AddInstance wires the queue's the same
// way). Re-broadcast is intentionally OFF so the measured cost is the bare merge.
func (in *Instance) ApplyEvent(ctx context.Context, e events.Event, peerURL string) (*inbox.ApplyResult, error) {
	applier := inbox.NewApplier(
		in.db, in.tasks, in.projects,
		repo.NewProjectSectionRepo(in.db), in.fedProjects, in.store,
	)
	return applier.Apply(ctx, e, peerURL)
}

// BuildAndSerializeMemberSnapshot builds a member snapshot through the PRODUCTION
// buffer-first path (Service.BuildSnapshotForMember: consistent read → release
// the lone writer connection → return the buffer) and serialises it to NDJSON
// exactly as the snapshot handler streams it (snapshot.WriteNDJSON), returning
// the serialised byte count. It is the F7.6 NFR-1.2 measurement leg AND the
// build the buffer-first availability test runs concurrently with writes.
func (in *Instance) BuildAndSerializeMemberSnapshot(t *testing.T, ctx context.Context, localProjectID int64) int {
	t.Helper()
	snap, err := in.svc.BuildSnapshotForMember(ctx, localProjectID)
	if err != nil {
		t.Fatalf("build member snapshot (%s): %v", in.url, err)
	}
	w := bufio.NewWriter(io.Discard)
	if err := fedsnapshot.WriteNDJSON(w, snap); err != nil {
		t.Fatalf("serialise snapshot (%s): %v", in.url, err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush snapshot (%s): %v", in.url, err)
	}
	// Re-serialise into a counting writer for the reported byte count (the discard
	// run above is the timed one when called under a timer).
	cw := &countingWriter{}
	bw := bufio.NewWriter(cw)
	if err := fedsnapshot.WriteNDJSON(bw, snap); err != nil {
		t.Fatalf("count snapshot (%s): %v", in.url, err)
	}
	if err := bw.Flush(); err != nil {
		t.Fatalf("flush count snapshot (%s): %v", in.url, err)
	}
	return cw.n
}

// BuildAndSerializeMemberSnapshotBackground builds + serialises a member snapshot
// returning any error, safe to call OFF the main test goroutine. It is the
// background-loop form the buffer-first availability test uses; the timed,
// byte-counted, test-fatal form (BuildAndSerializeMemberSnapshot) stays the
// measurement leg run on the test goroutine.
func (in *Instance) BuildAndSerializeMemberSnapshotBackground(ctx context.Context, localProjectID int64) error {
	snap, err := in.svc.BuildSnapshotForMember(ctx, localProjectID)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(io.Discard)
	if err := fedsnapshot.WriteNDJSON(w, snap); err != nil {
		return err
	}
	return w.Flush()
}

// TaskCount returns how many LIVE (non-tombstoned) tasks exist under a local
// project — the bootstrap convergence count predicate (every owner task must
// have landed on the joiner).
func (in *Instance) TaskCount(localProjectID int64) int {
	var n int
	if err := in.db.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE project_id = ? AND deleted_at IS NULL`, localProjectID).Scan(&n); err != nil {
		return -1
	}
	return n
}

// StartCommitPingWorker starts this instance's PRODUCTION publisher worker under
// ctx with a deliberately LONG polling interval and wires a commit-ping emitter
// as the instance's default emitter, so a subsequent Mutator() emit pushes the
// event IMMEDIATELY on commit (worker.Ping → DrainOnce → signed POST) rather than
// waiting on the polling tick. This is the NFR-1.1 "push <5s WITH commit-ping"
// mechanism — the long interval ensures the only thing fast enough to make the 5s
// budget is the commit-ping, not the tick. The worker goroutine is cancelled on
// cleanup.
func (in *Instance) StartCommitPingWorker(t *testing.T, ctx context.Context) {
	t.Helper()
	workerCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	in.worker.Start(workerCtx, time.Hour)
	in.pingEmitter = fedsvc.NewEmitter(in.db, in.keys, in.cipher, in.hlc, in.url).WithCommitPing(in.worker.Ping)
}

// countingWriter counts bytes written without retaining them (the snapshot
// byte-count sink for the perf log).
type countingWriter struct{ n int }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += len(p)
	return len(p), nil
}

// tamperBytes flips the last byte of b (or returns a single-byte body when b is
// empty) so a signed request body no longer matches its digest.
func tamperBytes(b []byte) []byte {
	if len(b) == 0 {
		return []byte("x")
	}
	out := make([]byte, len(b))
	copy(out, b)
	out[len(out)-1] ^= 0xFF
	return out
}

// hostOf returns the scheme://host prefix of a URL (the PeerClient routing key).
func hostOf(raw string) string {
	idx := strings.Index(raw, "://")
	if idx < 0 {
		return raw
	}
	rest := raw[idx+3:]
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		return raw[:idx+3+slash]
	}
	return raw
}

// trimSlashURL strips a trailing slash so a registered URL and a looked-up URL
// always match (the federation service trims, the directory must too).
func trimSlashURL(u string) string { return strings.TrimRight(u, "/") }

// readAllBody drains an http.Response body into a byte slice.
func readAllBody(resp *http.Response) []byte {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return buf.Bytes()
}

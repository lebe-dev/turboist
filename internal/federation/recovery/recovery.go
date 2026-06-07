// Package recovery implements the federation pull/catch-up loop (Federation v1
// F4.1, US-4.1). Where the outbox publisher PUSHES a peer's new events the moment
// they commit, the recovery loop is the symmetric PULL backstop: a single
// ctx-cancellable goroutine that, on startup and on a ticker, enumerates every
// JOINED peer (ListPullTargets), issues a SIGNED GET from that peer's
// last_received_hlc cursor, runs the SAME F3.2a per-event payload validator the
// push handler uses over each returned event, feeds the survivors into the same
// inbox-apply path push uses (durable record → dedup → enqueue), and advances the
// cursor — so an instance that was briefly offline auto-catches-up without losing
// or duplicating events (US-4.1 AC1/AC2/AC3).
//
// Per-event authentication (the F3.2a seam — Federation v1 R22/§404): the
// transport HTTP signature on the pull RESPONSE authenticates only the RELAYING
// peer (the owner in hub-and-spoke), not the original author of each event. A
// pulled batch routinely carries events authored by OTHER instances the owner
// re-broadcasts (F5.1/US-5.2 AC2), so each event must be re-authenticated
// end-to-end exactly as a pushed event is. Before any federation_inbox or domain
// write the loop runs inbox.Validator.Validate over the event: per-event Ed25519
// signature, author == origin_instance, HLC clock-skew (>10min future / >1h
// past), and (peer, project) membership + write/admin permission. An event that
// fails validation is NOT recorded, NOT enqueued, and the cursor is NOT advanced
// past it (the same "zero rows on a rejected event" guarantee push gives) — the
// rejection is logged with the event_id + sentinel and the batch stops, so the
// next pass re-pulls (and re-rejects) it without admitting later events behind it.
// A verified-and-rejected per-event signature (a relaying author's key rotation)
// additionally stamps the SAME sticky key-mismatch marker + durable security
// incident the push handler does (Federation v1 F4.3/F5.6b, US-4.3 AC4 / US-6.4
// AC2/AC3), so a rotation first observed via pull is not invisible to the operator
// — detection is symmetric across both transports, best-effort and never changing
// the rejection itself.
//
// Pull-error classification (F4.1): a failed pull is NOT all-or-nothing-retried.
// A 410 stale_pull is CONSUMED (re-bootstrap, see below). A PERMANENT reject — a
// 4xx the peer would return identically forever (a revoked/untrusted 403, a
// signature-rejected 401), surfaced by the service-layer *RemoteHandshakeError's
// FederationPermanent seam — GATES the (peer, project) target for
// permanentPullBackoff so the loop stops re-issuing the same failing pull every
// 60s tick; it is re-probed hourly and the gate clears the moment a pull succeeds
// (trust restored). A TRANSIENT error (peer unreachable, 5xx, 429, a reversible
// paused 403, DB busy) is NOT gated — it is retried on the very next tick so
// catch-up resumes the instant the peer recovers. The gate is per (peer, project)
// (a 403 can be project-scoped) and in-memory (R18 — a restart re-probes once).
//
// Connection discipline (R1 — SetMaxOpenConns(1)): a pass reads its targets on
// the store's connection, then RELEASES it before the per-peer network GET. The
// signed pull, the durable inbox record, and the cursor advance never hold the
// lone connection across the peer HTTP call.
//
// Cursor safety (the F4.1 risk): the cursor is advanced ONLY after the whole
// pulled batch is durably recorded in federation_inbox. A pull error (peer
// unreachable), a record error (DB busy), or an empty/caught-up response leaves
// the cursor where it was, so the same range is re-pulled next pass — partial
// progress never advances the cursor past un-recorded events. The advance itself
// is monotonic in the store, so a concurrent push that moved the peer forward is
// never rewound.
package recovery

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/federation/store"
)

// defaultInterval is the recovery loop's catch-up cadence (the spec's 60s pull
// interval). Push is the fast path (NFR-1.1); this tick is the backstop that
// recovers a peer after a short outage / a missed push.
const defaultInterval = time.Minute

// defaultBatchLimit caps how many events one pull pass requests from a peer (the
// spec's 500 limit). A peer with a larger backlog is drained over successive
// passes as the cursor advances.
const defaultBatchLimit = 500

// permanentPullBackoff is how long a peer is gated after a PERMANENT pull rejection
// (a revoked/untrusted 403, a signature-rejected 401) before the loop re-probes it
// (F4.1 pull-error classification). A permanent reject would fail identically every
// 60s tick, so the loop must not re-issue it each tick; an hourly re-probe lets a
// peer whose trust was restored (re-invite / trust-key) resume catch-up without a
// restart, while a still-dead peer is hit at most hourly instead of every minute.
const permanentPullBackoff = time.Hour

// permanentPullError is the classification seam a pull rejection implements to
// signal a PERMANENT (do-not-retry) reject — a 4xx (≠429, ≠paused) the loop cannot
// fix by re-issuing the identical request each tick (a revoked/untrusted 403, a
// signature-rejected 401). The service-layer *RemoteHandshakeError satisfies it. A
// transient error (network drop, 5xx, 429, a reversible paused 403, DB busy) does
// NOT implement it (or returns false) and is retried on the next tick. Defined here
// (not imported) so the dependency direction stays service→recovery.
type permanentPullError interface {
	FederationPermanent() bool
}

// isPermanentPull reports whether a pull error is a permanent (do-not-retry) reject.
// A transient error (no seam, or a 429 / paused 403 the seam reports false for) is
// retried on the next tick instead of gating the peer.
func isPermanentPull(err error) bool {
	var pe permanentPullError
	if errors.As(err, &pe) {
		return pe.FederationPermanent()
	}
	return false
}

// pullKey identifies a (peer, project) pull target for the permanent-reject gate. A
// 403 on pull can be project-specific (revoked from project X but still a member of
// project Y), so the gate is keyed per (project, peer) — never the whole peer — so
// one project's permanent reject never silently stops catch-up for another.
type pullKey struct {
	localProjectID int64
	peerURL        string
}

// TargetLister enumerates the joined peers the loop pulls from (satisfied by
// *store.Store). It runs on the store's own connection, before any network I/O.
type TargetLister interface {
	ListPullTargets(ctx context.Context) ([]store.PullTarget, error)
}

// Puller issues a signed catch-up GET to a peer's pull endpoint from sinceHLC
// (satisfied by *service/federation.Publisher). It performs NO DB access — it is
// called while no connection is held (R1).
type Puller interface {
	Pull(ctx context.Context, peerURL, remoteProjectID, sinceHLC string, limit int) (*events.PullResponse, error)
}

// EventSink durably records a pulled event (dedup), enqueues a newly-recorded
// event to the single inbox-apply goroutine, and advances the (peer,project)
// cursor (satisfied by *StoreSink). Record returns whether the event was newly
// inserted (true → enqueue) or a duplicate already recorded (false → skip
// enqueue, NFR-2 dedup).
type EventSink interface {
	Record(ctx context.Context, e events.Event, peerURL string, localProjectID int64) (inserted bool, err error)
	Enqueue(job inbox.Job)
	AdvanceCursor(ctx context.Context, localProjectID int64, peerURL, toHLC string) error
	// TouchContact stamps the peer's last_contact_at on a successful pull so the
	// joiner's owner-offline derivation (Federation v1 F5.6a, US-6.5 AC1/AC3) and the
	// per-peer stale status reflect real recency — the pull leg of "updated on every
	// push/pull/handshake touchpoint". It is best-effort; a touch failure never
	// blocks catch-up.
	TouchContact(ctx context.Context, peerURL string) error
}

// EventValidator runs the F3.2a per-event payload checks (per-event Ed25519
// signature, author == origin_instance, HLC clock-skew, and (peer, project)
// membership + write/admin permission) BEFORE any inbox/domain write (satisfied
// by *inbox.Validator). It is the SAME validator the push handler runs, so the
// pull and push transports share one per-event authentication seam (R22/§404):
// the transport response signature authenticates only the relaying peer, never
// the original author of each event in a hub-and-spoke fan-out. The returned
// ValidationResult carries the resolved local project id so the membership-checked
// project — not whatever the event claims — is the one a recorded event maps to.
type EventValidator interface {
	Validate(ctx context.Context, e events.Event, peerURL string) (*inbox.ValidationResult, error)
}

// KeyMismatchMarker records the sticky per-peer key-mismatch health marker when a
// PULLED event's per-event signature stops validating against the pinned key
// (Federation v1 F4.3 / F5.6b, US-4.3 AC4 / US-6.4 AC2). It is the SAME
// collaborator the push handler holds (federation_events.go): a verified-and-
// rejected per-event signature is genuine proof the relaying author rotated its
// key, and detection must be SYMMETRIC across transports — a rotation first
// observed via pull must raise the same incident + sticky badge as one observed
// via push (else a pull-first rotation silently fails to apply with only a buried
// WARN). It resolves the event's project_client_id to the local project the peer
// maps to, stamps the sticky marker (SSE-on-transition), and opens the durable
// security incident the "Trust new key" affordance reads (US-6.4 AC2/AC3).
// Satisfied by *service/federation.Service. nil → a build without the status
// surface simply does not record the marker (the event is still rejected with
// zero rows by the validator, and the cursor still does not advance, either way).
type KeyMismatchMarker interface {
	MarkKeyMismatchByRemote(ctx context.Context, peerURL, projectClientID string) error
}

// StaleConsumer consumes a 410 stale_pull (Federation v1 F4.2, US-4.2): when a
// peer's pull is answered 410 because the cursor predates the owner's retained
// history (the F3.3 emit half of US-3.7 AC4), the loop hands the
// {snapshot_url, as_of_hlc} the 410 advertised to ConsumeStalePull, which
// re-fetches the owner snapshot and overwrites the local project in one
// transaction WITHOUT touching federation_outbox (the joiner's unsent edits
// survive — R3). It is satisfied by *service/federation.Service via a thin
// adapter. nil → a F4.1-only build treats a 410 like any other pull error
// (isolated, logged, no cursor advance).
type StaleConsumer interface {
	ConsumeStalePull(ctx context.Context, localProjectID int64, peerURL, snapshotURL, asOfHLC string) error
}

// Loop is the recovery pull goroutine driver. Construct with NewLoop; tune with
// WithInterval / WithBatchLimit before Start.
type Loop struct {
	targets     TargetLister
	puller      Puller
	sink        EventSink
	validator   EventValidator
	stale       StaleConsumer
	keyMismatch KeyMismatchMarker
	log         *slog.Logger

	interval   time.Duration
	batchLimit int

	now func() time.Time

	// gateMu guards gated; the loop is single-goroutine but RunOnce is exported for
	// synchronous test/harness use, so the gate is mutex-protected (matches the
	// outbox worker's peer-gate discipline).
	gateMu sync.Mutex
	// gated maps a (project, peer) target to the earliest wall-clock it may be
	// re-pulled after a PERMANENT reject. In-memory by design (R18): a restart loses
	// the gate and re-probes the peer once, then re-gates if it still rejects.
	gated map[pullKey]time.Time

	doneCh chan struct{}
}

// NewLoop constructs the recovery loop. A nil log uses slog.Default. The
// per-event validator is REQUIRED in production (it is the only per-event
// authentication on the pull path) and is wired via WithValidator; production
// must always supply it.
func NewLoop(targets TargetLister, puller Puller, sink EventSink, log *slog.Logger) *Loop {
	if log == nil {
		log = slog.Default()
	}
	return &Loop{
		targets:    targets,
		puller:     puller,
		sink:       sink,
		log:        log,
		interval:   defaultInterval,
		batchLimit: defaultBatchLimit,
		now:        time.Now,
		gated:      map[pullKey]time.Time{},
		doneCh:     make(chan struct{}),
	}
}

// WithClock overrides the loop's wall-clock (default time.Now), for deterministic
// permanent-reject backoff-gating tests. Must be set before Start.
func (l *Loop) WithClock(now func() time.Time) *Loop {
	if now != nil {
		l.now = now
	}
	return l
}

// WithValidator wires the F3.2a per-event payload validator the loop runs over
// every pulled event before recording it (the SAME validator the push handler
// uses). Production MUST set this; without it the pull path would record events
// with zero per-event authentication. Must be set before Start.
func (l *Loop) WithValidator(v EventValidator) *Loop {
	l.validator = v
	return l
}

// WithKeyMismatch wires the sticky per-peer key-mismatch marker the loop stamps
// when a PULLED event's per-event signature stops validating against the pinned
// key (Federation v1 F4.3 / F5.6b, US-4.3 AC4 / US-6.4 AC2). It is the SAME
// collaborator the push handler holds, so a key rotation detected via pull raises
// the same durable security incident + sticky red badge as one detected via push
// — closing the asymmetric-detection gap (detection must not work for one
// transport only). Without it the pull path still rejects the event with zero
// rows; it simply does not surface the rotation. Must be set before Start.
func (l *Loop) WithKeyMismatch(m KeyMismatchMarker) *Loop {
	l.keyMismatch = m
	return l
}

// WithStaleConsumer wires the F4.2 410-stale-pull consumer the loop drives when a
// peer's pull is answered 410 because the cursor fell behind retention (US-4.2 /
// the consume half of US-3.7 AC4). Without it a 410 is treated as a plain pull
// error (isolated, logged, no cursor advance) — the F4.1 behaviour. Must be set
// before Start.
func (l *Loop) WithStaleConsumer(c StaleConsumer) *Loop {
	l.stale = c
	return l
}

// WithInterval overrides the catch-up tick cadence (default defaultInterval). A
// non-positive value is ignored. Must be set before Start.
func (l *Loop) WithInterval(d time.Duration) *Loop {
	if d > 0 {
		l.interval = d
	}
	return l
}

// WithBatchLimit overrides the per-peer pull batch cap (default
// defaultBatchLimit). A non-positive value is ignored. Must be set before Start.
func (l *Loop) WithBatchLimit(n int) *Loop {
	if n > 0 {
		l.batchLimit = n
	}
	return l
}

// Start launches the recovery goroutine. It runs one pass immediately (catch up
// anything missed while the process was down), then on every tick until ctx is
// cancelled.
func (l *Loop) Start(ctx context.Context) {
	go l.run(ctx)
}

// Stop blocks until the recovery goroutine has returned (post-cancel teardown).
func (l *Loop) Stop() {
	<-l.doneCh
}

func (l *Loop) run(ctx context.Context) {
	defer close(l.doneCh)
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	l.runLogged(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.runLogged(ctx)
		}
	}
}

func (l *Loop) runLogged(ctx context.Context) {
	if err := l.RunOnce(ctx); err != nil {
		l.log.ErrorContext(ctx, "federation: recovery pass failed",
			slog.String("op", "federation.recovery.Run"),
			slog.String("err", err.Error()),
		)
	}
}

// RunOnce performs one full recovery pass over every joined peer. It is exported
// so tests / a synchronous harness can drive a pull deterministically. The pass
// is best-effort and resilient: a failure for one (peer, project) is logged and
// skipped, never aborting the whole pass (US-4.1 — one unreachable peer never
// blocks catch-up from a healthy one). Only an error LISTING the targets (the
// initial store read) is returned.
func (l *Loop) RunOnce(ctx context.Context) error {
	targets, err := l.targets.ListPullTargets(ctx)
	if err != nil {
		return err
	}
	for _, tgt := range targets {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		l.pullTarget(ctx, tgt)
	}
	return nil
}

// pullTarget issues one signed pull to a peer and feeds the result through the
// sink. The connection is NOT held across the network GET (R1): the target was
// resolved by RunOnce's store read, the pull runs with nothing held, then each
// record + the cursor advance are short store writes.
func (l *Loop) pullTarget(ctx context.Context, tgt store.PullTarget) {
	if !l.pullReady(tgt) {
		// The peer returned a PERMANENT reject (a revoked/untrusted 403, a signature-
		// rejected 401) recently and is gated, so the loop does not re-issue the
		// identical failing pull every tick. It is re-probed once the backoff window
		// elapses (F4.1 pull-error classification).
		l.log.DebugContext(ctx, "federation: recovery skipping peer inside permanent-reject backoff",
			slog.String("op", "federation.recovery.Pull"),
			slog.Int64("project_id", tgt.LocalProjectID),
			slog.String("peer", tgt.PeerInstanceURL),
		)
		return
	}

	resp, err := l.puller.Pull(ctx, tgt.PeerInstanceURL, tgt.RemoteProjectID, tgt.LastReceivedHLC, l.batchLimit)
	if err != nil {
		// A 410 stale_pull means the cursor predates the owner's retained history
		// (the F3.3 emit half of US-3.7 AC4): the in-between events were GC'd, so
		// re-pulling the same range forever would never converge. Consume it — drive
		// the F4.2 re-bootstrap, which overwrites local state from a fresh snapshot
		// WITHOUT touching the outbox and advances last_received_hlc itself (so the
		// failed pull never advances the cursor here). The peer DID respond (410), so
		// it counts as a successful contact for owner-offline freshness (US-6.5 AC3)
		// and clears any permanent-reject gate.
		var staleErr *events.StalePullError
		if errors.As(err, &staleErr) {
			l.clearGate(tgt)
			l.touchContact(ctx, tgt.PeerInstanceURL)
			l.consumeStalePull(ctx, tgt, staleErr)
			return
		}
		// A PERMANENT reject (a revoked/untrusted 403, a signature-rejected 401) would
		// fail identically on every tick. Gate the peer so the loop stops re-issuing
		// the same failing pull each minute; it is re-probed after the backoff window
		// (F4.1). The cursor is NOT advanced.
		if isPermanentPull(err) {
			wait := l.gatePeer(tgt)
			l.log.WarnContext(ctx, "federation: recovery pull permanently rejected — gating peer to stop re-issuing the identical failing pull each tick",
				slog.String("op", "federation.recovery.Pull"),
				slog.Int64("project_id", tgt.LocalProjectID),
				slog.String("peer", tgt.PeerInstanceURL),
				slog.Duration("backoff", wait),
				slog.String("remediation", "a 403 likely means the peer revoked/forbade this instance, a 401 a transport-key mismatch — restore the trust relationship (re-invite / trust-key) to resume catch-up"),
				slog.String("err", err.Error()),
			)
			return
		}
		// A TRANSIENT error (peer unreachable, 5xx, 429, a reversible paused 403, DB
		// busy) is isolated and retried next tick: the cursor is NOT advanced and the
		// peer is NOT gated, so catch-up resumes the moment the peer recovers.
		l.log.WarnContext(ctx, "federation: recovery pull failed",
			slog.String("op", "federation.recovery.Pull"),
			slog.Int64("project_id", tgt.LocalProjectID),
			slog.String("peer", tgt.PeerInstanceURL),
			slog.String("err", err.Error()),
		)
		return
	}

	// The peer answered 2xx — a successful contact. Clear any permanent-reject gate
	// (trust restored), then refresh its last_contact_at so a joiner stops flagging
	// the OWNER "offline" the moment a pull reaches it again (Federation v1 F5.6a,
	// US-6.5 AC1/AC3). This runs even on an empty (caught-up) response — reachability,
	// not new events, is what clears the owner-offline flag.
	l.clearGate(tgt)
	l.touchContact(ctx, tgt.PeerInstanceURL)

	if resp == nil || len(resp.Events) == 0 {
		// Already caught up — nothing to record, do not touch the cursor.
		return
	}

	// Authenticate + durably record EVERY returned event before advancing the
	// cursor. Each event runs the SAME F3.2a per-event validator the push handler
	// uses BEFORE any inbox/domain write (per-event signature, author == origin,
	// clock-skew, and (peer, project) membership + write/admin permission): the
	// transport response signature only authenticates the relaying peer, not the
	// author of each relayed event (R22/§404). A validation failure (forged /
	// wrong-author / far-future-HLC / read-only-or-revoked peer) is NOT recorded,
	// NOT enqueued, and STOPS the batch without advancing the cursor — the same
	// "zero rows on a rejected event" guarantee push gives. A record failure (DB
	// busy) likewise stops the batch, so the un-recorded tail is re-pulled next
	// pass (partial-apply must not advance the cursor).
	for i := range resp.Events {
		evt := resp.Events[i]

		// F3.2a per-event authentication. The validator resolves the membership-checked
		// LOCAL project id; the event is recorded against THAT project, never against
		// whatever the target row or the event payload claims (membership gates which
		// project an applied event may target on the pull path too).
		localProjectID, ok := l.validate(ctx, evt, tgt)
		if !ok {
			return // rejected event — do NOT advance the cursor (zero rows for it).
		}

		inserted, err := l.sink.Record(ctx, evt, tgt.PeerInstanceURL, localProjectID)
		if err != nil {
			l.log.ErrorContext(ctx, "federation: recovery record failed",
				slog.String("op", "federation.recovery.Record"),
				slog.Int64("project_id", localProjectID),
				slog.String("peer", tgt.PeerInstanceURL),
				slog.String("event_id", evt.EventID),
				slog.String("err", err.Error()),
			)
			return // do NOT advance the cursor on a partial record.
		}
		if !inserted {
			// Duplicate (already delivered by push or an earlier pull) — recorded,
			// not re-enqueued (NFR-2 dedup, US-4.1 push+pull no-op).
			continue
		}
		l.sink.Enqueue(inbox.Job{Event: evt, PeerURL: tgt.PeerInstanceURL})
	}

	// The whole batch is durably recorded — advance the cursor (monotonic in the
	// store) so the next pass resumes from here.
	if err := l.sink.AdvanceCursor(ctx, tgt.LocalProjectID, tgt.PeerInstanceURL, resp.NextHLC); err != nil {
		l.log.ErrorContext(ctx, "federation: recovery cursor advance failed",
			slog.String("op", "federation.recovery.Advance"),
			slog.Int64("project_id", tgt.LocalProjectID),
			slog.String("peer", tgt.PeerInstanceURL),
			slog.String("err", err.Error()),
		)
	}
}

// consumeStalePull drives the F4.2 re-bootstrap when a pull returned 410 stale
// (US-4.2 / US-3.7 AC4 consume half). It hands the {snapshot_url, as_of_hlc} the
// 410 advertised to the StaleConsumer, which re-fetches the owner snapshot and
// overwrites the local project in one transaction WITHOUT touching the outbox.
// When no consumer is wired (a F4.1-only build) the 410 is logged and ignored —
// the cursor is NOT advanced, so the peer remains stuck until a consumer is
// configured (it does NOT silently advance past un-recovered events).
func (l *Loop) consumeStalePull(ctx context.Context, tgt store.PullTarget, staleErr *events.StalePullError) {
	if l.stale == nil {
		l.log.WarnContext(ctx, "federation: recovery pull stale but no re-bootstrap consumer wired",
			slog.String("op", "federation.recovery.StalePull"),
			slog.Int64("project_id", tgt.LocalProjectID),
			slog.String("peer", tgt.PeerInstanceURL),
		)
		return
	}
	l.log.InfoContext(ctx, "federation: recovery pull stale, re-bootstrapping",
		slog.String("op", "federation.recovery.StalePull"),
		slog.Int64("project_id", tgt.LocalProjectID),
		slog.String("peer", tgt.PeerInstanceURL),
		slog.String("as_of_hlc", staleErr.AsOfHLC),
	)
	if err := l.stale.ConsumeStalePull(ctx, tgt.LocalProjectID, tgt.PeerInstanceURL, staleErr.SnapshotURL, staleErr.AsOfHLC); err != nil {
		// A re-bootstrap failure (owner unreachable, mid-stream error → full
		// rollback) leaves the cursor unchanged so the next pass retries; the unsent
		// outbox is untouched either way (the overwrite tx rolled back).
		l.log.ErrorContext(ctx, "federation: recovery re-bootstrap failed",
			slog.String("op", "federation.recovery.StalePull"),
			slog.Int64("project_id", tgt.LocalProjectID),
			slog.String("peer", tgt.PeerInstanceURL),
			slog.String("err", err.Error()),
		)
	}
}

// pullReady reports whether a (peer, project) target may be pulled now: it has no
// permanent-reject gate, or its backoff window has elapsed (F4.1).
func (l *Loop) pullReady(tgt store.PullTarget) bool {
	l.gateMu.Lock()
	defer l.gateMu.Unlock()
	until, gated := l.gated[pullKey{tgt.LocalProjectID, tgt.PeerInstanceURL}]
	if !gated {
		return true
	}
	return !l.now().Before(until)
}

// gatePeer marks a (peer, project) target gated for permanentPullBackoff after a
// PERMANENT pull rejection, so the identical failing pull is not re-issued every
// tick (F4.1). Returns the window applied. The gate is in-memory (R18) and cleared
// by clearGate on the next successful (or 410) pull.
func (l *Loop) gatePeer(tgt store.PullTarget) time.Duration {
	l.gateMu.Lock()
	defer l.gateMu.Unlock()
	l.gated[pullKey{tgt.LocalProjectID, tgt.PeerInstanceURL}] = l.now().Add(permanentPullBackoff)
	return permanentPullBackoff
}

// clearGate removes a (peer, project) permanent-reject gate after the peer responds
// again (a 2xx pull or a 410 stale-pull), so a peer whose trust was restored resumes
// the normal tick cadence without a restart. A no-op for an un-gated target.
func (l *Loop) clearGate(tgt store.PullTarget) {
	l.gateMu.Lock()
	defer l.gateMu.Unlock()
	delete(l.gated, pullKey{tgt.LocalProjectID, tgt.PeerInstanceURL})
}

// touchContact refreshes a peer's last_contact_at after a successful pull
// exchange (Federation v1 F5.6a, US-6.5 AC1/AC3 — the pull touchpoint). It is
// best-effort: a touch failure is logged and ignored, never blocking catch-up
// (the events were already recorded/enqueued; only the freshness signal lags).
func (l *Loop) touchContact(ctx context.Context, peerURL string) {
	if err := l.sink.TouchContact(ctx, peerURL); err != nil {
		l.log.WarnContext(ctx, "federation: recovery touch last_contact_at failed",
			slog.String("op", "federation.recovery.TouchContact"),
			slog.String("peer", peerURL),
			slog.String("err", err.Error()),
		)
	}
}

// validate runs the F3.2a per-event validator over one pulled event and returns
// the membership-checked local project id on success. On a validation failure it
// logs the event_id + the matched sentinel (the SAME mapping the push handler
// uses) and returns ok=false so the caller stops the batch without recording the
// event or advancing the cursor (zero rows for a rejected event). When no
// validator is wired (a federation-disabled / test harness that never reaches
// here in production) the event is treated as rejected: the pull path must never
// record an unauthenticated event.
func (l *Loop) validate(ctx context.Context, evt events.Event, tgt store.PullTarget) (int64, bool) {
	if l.validator == nil {
		l.log.ErrorContext(ctx, "federation: recovery has no per-event validator; refusing to record",
			slog.String("op", "federation.recovery.Validate"),
			slog.Int64("project_id", tgt.LocalProjectID),
			slog.String("peer", tgt.PeerInstanceURL),
			slog.String("event_id", evt.EventID),
		)
		return 0, false
	}
	vr, err := l.validator.Validate(ctx, evt, tgt.PeerInstanceURL)
	if err != nil {
		l.logRejected(ctx, evt, tgt, err)
		l.markKeyMismatch(ctx, evt, tgt, err)
		return 0, false
	}
	return vr.LocalProjectID, true
}

// markKeyMismatch stamps the sticky per-peer key-mismatch marker + opens the
// durable security incident when a PULLED event was verified against the pinned
// key and REJECTED (inbox.ErrEventSignatureInvalid) — genuine proof the relaying
// author rotated its key. It mirrors the push handler (federation_events.go:181)
// so detection is SYMMETRIC: a rotation first observed via pull raises the same
// F5.6b incident banner (US-6.4 AC2) + sticky red badge (US-4.3 AC4) + "Trust new
// key" affordance (US-6.4 AC3) as one observed via push. A transient key-
// RESOLUTION failure (ErrEventKeyUnresolved) is deliberately NOT a rotation and is
// never marked (the batch just retries next pass), matching the push handler and
// mapEventValidationError. Recording is best-effort and never changes the
// rejection: the event was already NOT recorded, NOT enqueued, and the cursor was
// NOT advanced by the caller — a failure to stamp the marker only logs a WARN.
func (l *Loop) markKeyMismatch(ctx context.Context, evt events.Event, tgt store.PullTarget, err error) {
	if l.keyMismatch == nil || !errors.Is(err, inbox.ErrEventSignatureInvalid) {
		return
	}
	// ERROR-grade audit: the sticky marker is irreversible without an operator
	// action (F5.6b), so every stamp is recorded with peer + reason — matching the
	// push handler's audit line.
	l.log.ErrorContext(ctx, "federation: peer key rotation detected on pull, stamping sticky key_mismatch marker",
		slog.String("op", "federation.recovery.Validate"),
		slog.Int64("project_id", tgt.LocalProjectID),
		slog.String("peer", tgt.PeerInstanceURL),
		slog.String("project_client_id", evt.ProjectClientID),
		slog.String("reason", err.Error()),
	)
	if markErr := l.keyMismatch.MarkKeyMismatchByRemote(ctx, tgt.PeerInstanceURL, evt.ProjectClientID); markErr != nil {
		l.log.WarnContext(ctx, "federation: record key-mismatch marker on pull failed",
			slog.String("op", "federation.recovery.Validate"),
			slog.String("peer", tgt.PeerInstanceURL),
			slog.String("err", markErr.Error()),
		)
	}
}

// logRejected logs a per-event validation failure at WARN/ERROR with the event_id
// and a stable reason mapped from the validator sentinel — the same sentinel
// classification the push handler's mapEventValidationError uses, so a forged,
// wrong-author, clock-skewed, or not-permitted event is identifiable in logs on
// both transports. A signature failure is logged at WARN (a probe / a relay of a
// tampered payload); an unexpected (non-sentinel) error is ERROR (an
// infrastructure fault, not a rejected event).
func (l *Loop) logRejected(ctx context.Context, evt events.Event, tgt store.PullTarget, err error) {
	reason := "validation failed"
	level := slog.LevelError
	switch {
	case errors.Is(err, inbox.ErrEventKeyUnresolved):
		// Transient author-key resolution failure (.well-known fetch error): the
		// batch stops and is retried next pass — NOT a key rotation, so markKeyMismatch
		// deliberately does NOT stamp the sticky marker for it (matching the push
		// handler's mapEventValidationError; only ErrEventSignatureInvalid is a rotation).
		reason, level = "event author key unresolved", slog.LevelWarn
	case errors.Is(err, inbox.ErrEventSignatureInvalid):
		reason, level = "event signature invalid", slog.LevelWarn
	case errors.Is(err, inbox.ErrAuthorOriginMismatch):
		reason, level = "author/origin mismatch", slog.LevelWarn
	case errors.Is(err, inbox.ErrEventClockSkew):
		reason, level = "event clock skew", slog.LevelWarn
	case errors.Is(err, inbox.ErrNotMember), errors.Is(err, inbox.ErrPeerNotPermitted):
		reason, level = "peer not permitted for this project", slog.LevelWarn
	}
	l.log.Log(ctx, level, "federation: recovery rejected pulled event",
		slog.String("op", "federation.recovery.Validate"),
		slog.Int64("project_id", tgt.LocalProjectID),
		slog.String("peer", tgt.PeerInstanceURL),
		slog.String("event_id", evt.EventID),
		slog.String("reason", reason),
		slog.String("err", err.Error()),
	)
}

// Package outbox implements the federation publisher worker (Federation v1
// F3.2, US-3.1/US-3.2). A single ctx-cancellable goroutine drains the
// transactional outbox: it batch-reads undelivered events per (project, peer),
// POSTs them to the peer's /federation/events, and stamps delivered_to.
//
// Two delivery triggers keep push under the NFR-1.1 5s budget without busy-
// looping: a coarse ticker (catch-up safety net) and a commit-ping the emit path
// fires the moment a federated mutation is written, so a new event is pushed
// immediately rather than on the next tick.
//
// Connection discipline (R1 — SetMaxOpenConns(1)): the worker reads a batch and
// RELEASES the lone DB connection BEFORE any peer network I/O, then takes a short
// transaction to mark delivery. It NEVER holds the connection across a peer POST,
// which would stall every app write. A failed peer is isolated: its delivered_to
// is left unstamped so the event is retried on the next drain, and one peer's
// 5xx never blocks delivery to a healthy peer (US-3.2 AC3).
//
// Per-peer backoff + error classification (US-3.2 / F3.2 "per-peer backoff
// 1s..1h"). A push failure is classified transient vs permanent:
//
//   - TRANSIENT (5xx, 429, network drop): the peer enters exponential not-before
//     gating (1s doubling to a 1h cap) keyed by its URL, reset to zero on the next
//     success. A drain pass that fires while a peer is still gated (the safety-net
//     tick or a fresh commit-ping) SKIPS that peer without a POST, so a down peer
//     is not hammered every 60s — only re-probed when its backoff window elapses.
//   - PERMANENT (4xx ≠ 429): the failed events are parked in the dead-letter table
//     (federation_dead_letter, F4.4), skipped by future reads and excluded from the
//     pending-delivery count. A permanent reject's BLAST RADIUS depends on whether
//     it is PEER-SCOPED or EVENT-SCOPED:
//   - PEER-SCOPED — a revoked/read-only/untrusted 403 (the peer rejects the
//     whole link, §9.2/§9.3): the peer is marked permanently gated so its
//     remaining/future events are not re-POSTed (every one would be rejected
//     identically). The not-yet-attempted chunks of the SAME read batch are
//     dead-lettered too (rather than stranded pending behind the gate, invisible
//     and never re-delivered). An explicit operator re-enable path
//     (Worker.RetryPeer) clears the gate; a restart alone does not (RestoreBackoff
//     re-loads it).
//   - EVENT-SCOPED — a 400 author/origin-mismatch or clock-skew, a 401
//     signature-rejected, a 410 stale-tombstone (a re-edit of a tombstoned
//     entity returns 410 per the offline contract): ONLY the offending event is
//     dead-lettered. The peer is NOT gated, so its other healthy events keep
//     flowing — a single event-specific 4xx never silently strands the link.
//
// F4.4 backpressure additions (US-4.4 / US-8.3): a 429 honors the peer's
// Retry-After window verbatim; a 5xx/network drop backs off exponentially
// (1s..1h); a 4xx (≠429) dead-letters the offending event(s), gating the WHOLE
// peer only when the 4xx is a peer-scoped link reject (403) and not when it is
// event-scoped (400/401/410); and a delivery batch is CHUNKED by count (500) +
// bytes (5 MB) so a single POST never overruns the receiver's inbound 413 limit.
// The per-peer retry gate (not-before / attempt / permanent) is PERSISTED to
// federation_peer_retry and restored on Start, so a restart does not re-hammer a
// down/rejecting peer; a permanently-gated peer is re-enabled by an explicit
// operator call (Worker.RetryPeer), never silently.
package outbox

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
)

// defaultBatchLimit caps how many undelivered events one peer is READ per drain
// pass. The batch is then CHUNKED for delivery by count + bytes (F4.4).
const defaultBatchLimit = 500

// Per-peer transient backoff bounds (F3.2 "per-peer backoff 1s..1h"): the first
// transient failure gates the peer for backoffMin, each subsequent consecutive
// failure doubles the window up to backoffMax, and a success resets it.
const (
	backoffMin = time.Second
	backoffMax = time.Hour
)

// Batch-chunk defaults (Federation v1 F4.4, US-4.4 AC4): a delivery batch is
// split when it exceeds maxChunkEvents events OR maxChunkBytes serialized bytes,
// so a single POST never overruns the receiver's inbound 413 limit. The byte cap
// matches the §16 5 MB ceiling; the count cap matches the receiver's 500-event
// inbound limit.
const (
	defaultMaxChunkEvents = 500
	defaultMaxChunkBytes  = 5 * 1024 * 1024
)

// permanentError is the classification seam a Pusher error implements to signal a
// PERMANENT (do-not-retry) rejection — a 4xx that retrying cannot fix (a revoked
// 403, an author-mismatch 400, a signature-rejected 401, a stale 410). The
// service-layer *RemoteHandshakeError satisfies it. A nil/absent implementation
// (a plain network error) is treated as transient and backed off. Defined here
// (not imported) so the dependency direction stays service→outbox.
type permanentError interface {
	FederationPermanent() bool
}

// peerScopedError is the classification seam that splits a PERMANENT reject into
// two very different blast radii (Federation v1 F4.4 hardening):
//
//   - PEER-SCOPED (FederationPeerScoped() == true): the peer rejects the whole
//     federation LINK, not just this one event — a revoked / read-only / not-a-
//     member / untrusted 403 (§9.2/§9.3 ACL enforcement). Every future event to
//     this peer would be rejected identically, so gating the WHOLE peer permanent
//     (peerReady → false) is correct and stops a pointless re-POST storm.
//   - EVENT-SCOPED (FederationPeerScoped() == false): the reject is specific to
//     the offending event — a 400 author/origin-mismatch or clock-skew, a 401
//     signature-rejected, a 410 stale-tombstone (a re-edit of a tombstoned entity
//     returns 410 per the offline contract). Dead-lettering THAT event is correct,
//     but the link is healthy: other events must keep flowing. Marking the whole
//     peer permanent on one such event would silently strand every other healthy
//     event (the bug this seam fixes).
//
// An absent implementation is treated CONSERVATIVELY as event-scoped: a permanent
// error of unknown shape dead-letters its event but does NOT kill the link, so an
// unrelated healthy event is never silently stranded.
type peerScopedError interface {
	FederationPeerScoped() bool
}

// retryAfterError is the classification seam a 429 push error implements to carry
// the peer's Retry-After window (Federation v1 F4.4, US-4.4 AC1). When present
// (ok=true) the worker gates the peer for EXACTLY that duration instead of the
// exponential default, honoring the peer's backpressure signal.
type retryAfterError interface {
	FederationRetryAfter() (time.Duration, bool)
}

// statusReasonError is the classification seam a permanent push error implements
// to carry the HTTP status + federation error code recorded on a dead-letter row
// (Federation v1 F4.4, US-4.4 AC3). Optional: an absent implementation parks the
// event with status 0 / empty reason.
type statusReasonError interface {
	FederationStatusCode() int
	FederationReason() string
}

// isPermanent reports whether a push error is a permanent (do-not-retry) reject.
// A transient error (5xx, 429, network drop, or any error not implementing the
// classification seam) returns false and is backed off for retry instead.
func isPermanent(err error) bool {
	var pe permanentError
	if errors.As(err, &pe) {
		return pe.FederationPermanent()
	}
	return false
}

// isPeerScoped reports whether a PERMANENT reject is peer-scoped (kills the whole
// link, e.g. a revoked/read-only 403) versus event-scoped (only the offending
// event is dead, the link stays healthy, e.g. a 400/401/410). It is only
// consulted for an error already classified permanent. An error that does not
// implement the seam is treated as event-scoped (the safe default: dead-letter
// the event, keep the link alive) so a permanent reject of unknown shape can
// never silently strand a peer's other healthy events.
func isPeerScoped(err error) bool {
	var ps peerScopedError
	if errors.As(err, &ps) {
		return ps.FederationPeerScoped()
	}
	return false
}

// retryAfterOf extracts a 429 Retry-After window from a push error, or (0, false)
// when the error carries none (a plain 5xx / network drop backs off exponentially).
func retryAfterOf(err error) (time.Duration, bool) {
	var re retryAfterError
	if errors.As(err, &re) {
		return re.FederationRetryAfter()
	}
	return 0, false
}

// statusReasonOf extracts the HTTP status + federation error code from a permanent
// push error for the dead-letter row (or 0/"" when the error carries none).
func statusReasonOf(err error) (int, string) {
	var se statusReasonError
	if errors.As(err, &se) {
		return se.FederationStatusCode(), se.FederationReason()
	}
	return 0, ""
}

// Peer is one delivery target for a project (a non-revoked, non-self
// federated_projects row). InstanceURL is the peer's federation identity.
type Peer struct {
	InstanceURL string
}

// PeerLister resolves the delivery targets for a project. The production
// implementation reads non-revoked, non-self federated_projects rows; it must
// run on its own connection (the worker calls it during the read phase, before
// releasing the connection for network I/O).
type PeerLister interface {
	PeersForProject(ctx context.Context, localProjectID int64) ([]Peer, error)
}

// Pusher delivers a batch of canonical signed event payloads to a peer's
// /federation/events endpoint. It performs the transport-signed POST and returns
// an error on a non-2xx / network failure so the worker leaves the batch pending
// for retry. It MUST NOT touch the DB (R1): it runs while no connection is held.
type Pusher interface {
	Push(ctx context.Context, peerURL string, payloads []string) error
}

// peerState is the per-peer retry gate. notBefore is the earliest wall-clock the
// peer may be re-POSTed (a transient backoff window, or the far future once the
// peer is marked permanently failed); attempt counts consecutive transient
// failures to drive the exponential window. A success clears both.
type peerState struct {
	notBefore time.Time
	attempt   int
	permanent bool
}

// Worker is the publisher goroutine driver. It is safe to construct with a nil
// logger (a no-op logger is substituted).
type Worker struct {
	store  *store.Store
	peers  PeerLister
	pusher Pusher
	log    *slog.Logger
	now    func() time.Time

	maxChunkEvents int
	maxChunkBytes  int

	// sent records the outbound Prometheus counter (Federation v1 F6.5, US-8.2):
	// federation_events_sent_total{peer,result}. nil → no metric recorded (the
	// delivery behaviour is unaffected).
	sent SentObserver

	// peerMu guards peerStates; the worker is single-goroutine but DrainOnce is
	// exported for synchronous test/harness use, so the gate is mutex-protected.
	peerMu     sync.Mutex
	peerStates map[string]*peerState

	pingCh chan struct{}
	doneCh chan struct{}
}

// NewWorker constructs the publisher worker. A nil log uses slog.Default.
func NewWorker(st *store.Store, peers PeerLister, pusher Pusher, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{
		store:          st,
		peers:          peers,
		pusher:         pusher,
		log:            log,
		now:            time.Now,
		maxChunkEvents: defaultMaxChunkEvents,
		maxChunkBytes:  defaultMaxChunkBytes,
		peerStates:     map[string]*peerState{},
		// Buffered so a commit-ping never blocks the emit path even if a drain is
		// in flight (a single pending ping coalesces multiple commits).
		pingCh: make(chan struct{}, 1),
		doneCh: make(chan struct{}),
	}
}

// WithClock overrides the worker's wall-clock (default time.Now), for
// deterministic backoff-gating tests. It must be set before Start.
func (w *Worker) WithClock(now func() time.Time) *Worker {
	if now != nil {
		w.now = now
	}
	return w
}

// SentObserver records the outbound sent-events Prometheus counter (Federation v1
// F6.5, US-8.2). result is "success" for a delivered chunk and "error" for a
// failed push. Satisfied by a thin adapter over *federation/metrics.Collectors.
type SentObserver interface {
	RecordSent(peerURL, result string, n int)
}

// WithSentObserver wires the outbound sent-events metric observer (Federation v1
// F6.5, US-8.2). It must be set before Start. A nil observer records nothing.
func (w *Worker) WithSentObserver(o SentObserver) *Worker {
	w.sent = o
	return w
}

// WithChunkLimits overrides the batch-chunk caps (Federation v1 F4.4, US-4.4
// AC4). A non-positive value keeps the default (500 events / 5 MB). It must be
// set before Start. Tests use small caps to assert the split shape deterministically.
func (w *Worker) WithChunkLimits(maxEvents, maxBytes int) *Worker {
	if maxEvents > 0 {
		w.maxChunkEvents = maxEvents
	}
	if maxBytes > 0 {
		w.maxChunkBytes = maxBytes
	}
	return w
}

// RestoreBackoff loads the persisted per-peer retry gate into memory so a restart
// does NOT re-hammer a down/rejecting peer (Federation v1 F4.4, §7 risk: "persist
// retry-not-before across restart"). It is called once before Start. A peer whose
// persisted not_before is in the past (or whose record is stale) is simply re-
// probed on the next drain — restoring is best-effort and never blocks startup.
func (w *Worker) RestoreBackoff(ctx context.Context) error {
	rows, err := w.store.LoadPeerRetry(ctx)
	if err != nil {
		return err
	}
	w.peerMu.Lock()
	defer w.peerMu.Unlock()
	for _, r := range rows {
		st := &peerState{attempt: r.Attempt, permanent: r.Permanent}
		if r.NotBefore != "" {
			if nb, perr := model.ParseUTC(r.NotBefore); perr == nil {
				st.notBefore = nb
			}
		}
		w.peerStates[r.PeerURL] = st
	}
	return nil
}

// Start launches the worker goroutine. It drains once immediately (catch up any
// events left undelivered by a previous run), then drains on every tick and on
// every Ping until ctx is cancelled. interval is the safety-net ticker; the
// commit-ping is what makes push immediate.
func (w *Worker) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	// Restore the persisted per-peer backoff gate so a restart honors an in-flight
	// backoff / permanent failure rather than immediately re-hammering the peer
	// (Federation v1 F4.4). Best-effort: a failure here only loses cross-restart
	// gating, which the next failure re-establishes — it must not block startup.
	if err := w.RestoreBackoff(ctx); err != nil {
		w.log.WarnContext(ctx, "federation: restore outbox backoff failed",
			slog.String("op", "federation.outbox.RestoreBackoff"),
			slog.String("err", err.Error()),
		)
	}
	go w.run(ctx, interval)
}

// Ping signals the worker that a new event was committed so it drains now rather
// than on the next tick (the immediate-push trigger, NFR-1.1). It never blocks:
// a full channel already has a pending wake-up.
func (w *Worker) Ping() {
	select {
	case w.pingCh <- struct{}{}:
	default:
	}
}

// Stop blocks until the worker goroutine has returned. It is called after the
// worker's context is cancelled (the main.go shutdown drain).
func (w *Worker) Stop() {
	<-w.doneCh
}

func (w *Worker) run(ctx context.Context, interval time.Duration) {
	defer close(w.doneCh)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.drainLogged(ctx)
	for {
		select {
		case <-ctx.Done():
			// Best-effort final drain on shutdown so events committed just before
			// teardown are not stranded (still bounded by the parent's deadline).
			w.drainLogged(context.WithoutCancel(ctx))
			return
		case <-ticker.C:
			w.drainLogged(ctx)
		case <-w.pingCh:
			w.drainLogged(ctx)
		}
	}
}

func (w *Worker) drainLogged(ctx context.Context) {
	if err := w.DrainOnce(ctx); err != nil {
		w.log.ErrorContext(ctx, "federation: outbox drain failed",
			slog.String("op", "federation.outbox.Drain"),
			slog.String("err", err.Error()),
		)
	}
}

// DrainOnce performs one full drain pass over every federated project's pending
// events. It is exported so tests (and a synchronous PumpOutbox harness) can
// drive delivery deterministically. The pass is best-effort and resilient: a
// failure for one (project, peer) is logged and skipped, never aborting the
// whole pass.
func (w *Worker) DrainOnce(ctx context.Context) error {
	projectIDs, err := w.store.ListProjectsWithOutbox(ctx)
	if err != nil {
		return err
	}
	for _, pid := range projectIDs {
		w.drainProject(ctx, pid)
	}
	return nil
}

// drainProject delivers a project's pending events to each of its peers. A peer
// failure is isolated (US-3.2 AC3): the event stays pending for that peer only.
func (w *Worker) drainProject(ctx context.Context, projectID int64) {
	peers, err := w.peers.PeersForProject(ctx, projectID)
	if err != nil {
		w.log.ErrorContext(ctx, "federation: resolve peers failed",
			slog.String("op", "federation.outbox.Drain"),
			slog.Int64("project_id", projectID),
			slog.String("err", err.Error()),
		)
		return
	}
	for _, peer := range peers {
		w.drainPeer(ctx, projectID, peer)
	}
}

// drainPeer reads the peer's undelivered batch, RELEASES the connection by
// completing the read before the push, POSTs, and only on success stamps each
// delivered row in its own short transaction (R1). A peer still inside its
// not-before backoff window (or permanently failed) is SKIPPED before any DB read
// or POST, so a down / permanently-rejecting peer is not hammered (US-3.2).
func (w *Worker) drainPeer(ctx context.Context, projectID int64, peer Peer) {
	if !w.peerReady(peer.InstanceURL) {
		// Gated: still inside the backoff window or marked permanently failed. The
		// batch stays pending (delivered_to unstamped) and is re-probed only once the
		// window elapses — never re-POSTed on every tick/ping.
		return
	}

	batch, err := w.store.ListUndeliveredForPeer(ctx, projectID, peer.InstanceURL, defaultBatchLimit)
	if err != nil {
		w.log.ErrorContext(ctx, "federation: read outbox batch failed",
			slog.String("op", "federation.outbox.Drain"),
			slog.Int64("project_id", projectID),
			slog.String("peer", peer.InstanceURL),
			slog.String("err", err.Error()),
		)
		return
	}
	if len(batch) == 0 {
		return
	}

	// Chunk the batch by count + bytes (Federation v1 F4.4, US-4.4 AC4) so a single
	// POST never overruns the receiver's inbound 413 limit. Each chunk is delivered
	// and stamped before the next is attempted. A chunk failure is classified
	// (handlePushFailure): a TRANSIENT or PEER-SCOPED-permanent failure HALTS the
	// peer (halt=true). A TRANSIENT halt leaves the remaining chunks pending so they
	// retry next drain; a PEER-SCOPED-permanent halt (the link is dead) DEAD-LETTERS
	// the remaining un-attempted chunks too (deadLetterRemaining=true) so they are
	// never left stranded pending-but-undelivered behind the permanent gate. An
	// EVENT-SCOPED-permanent failure (a 400/401/410 tied to the offending event)
	// dead-letters only that chunk's events and KEEPS GOING (halt=false), so the
	// peer's other healthy events are never stranded.
	chunks := w.chunkBatch(batch)
	for i, chunk := range chunks {
		payloads := make([]string, len(chunk))
		for j, ev := range chunk {
			payloads[j] = ev.Payload
		}

		// Network I/O happens here with NO DB connection held (R1).
		if err := w.pusher.Push(ctx, peer.InstanceURL, payloads); err != nil {
			if w.sent != nil {
				w.sent.RecordSent(peer.InstanceURL, "error", len(chunk))
			}
			halt, deadLetterRemaining := w.handlePushFailure(ctx, projectID, peer.InstanceURL, chunk, err)
			if !halt {
				// Event-scoped permanent reject: this chunk is parked, but the link is
				// healthy — continue draining the peer's remaining chunks.
				continue
			}
			if deadLetterRemaining {
				// The whole link is permanently gated: its remaining un-attempted chunks
				// would never drain (peerReady blocks every future POST) and would not be
				// re-POSTed even after an operator RetryPeer (already dead-lettered are
				// excluded from the undelivered read). Park them now so they are visible
				// in the dead-letter diagnostics and excluded from the pending count,
				// instead of stranded pending-but-undelivered.
				w.deadLetterRemainingChunks(ctx, projectID, peer.InstanceURL, chunks[i+1:], err)
			}
			return
		}
		if w.sent != nil {
			w.sent.RecordSent(peer.InstanceURL, "success", len(chunk))
		}

		// Chunk succeeded: clear any backoff/permanent state so the peer drains freely.
		w.recordSuccess(ctx, peer.InstanceURL)

		for _, ev := range chunk {
			if err := w.store.MarkDelivered(ctx, ev.ID, peer.InstanceURL); err != nil {
				w.log.ErrorContext(ctx, "federation: mark delivered failed",
					slog.String("op", "federation.outbox.Drain"),
					slog.Int64("project_id", projectID),
					slog.String("peer", peer.InstanceURL),
					slog.String("event_id", ev.EventID),
					slog.String("err", err.Error()),
				)
				// The push succeeded but the stamp failed; the event will be re-pushed
				// next drain and the receiver dedups on event_id (NFR-2 at-least-once).
				return
			}
		}
		w.log.DebugContext(ctx, "federation: delivered chunk",
			slog.String("op", "federation.outbox.Drain"),
			slog.Int64("project_id", projectID),
			slog.String("peer", peer.InstanceURL),
			slog.Int("count", len(chunk)),
		)
	}
}

// handlePushFailure classifies a chunk push error, applies the F4.4 backpressure
// policy, and reports whether the caller should HALT draining this peer and, when
// halting, whether the peer's REMAINING un-attempted chunks should be dead-lettered
// (deadLetterRemaining) because the whole link is now permanently dead.
//
// A PERMANENT 4xx (≠429) always parks the failed chunk's events in the dead-letter
// table (US-4.4 AC3 — not retried). Its BLAST RADIUS then forks on whether the
// reject is peer-scoped or event-scoped:
//
//   - PEER-SCOPED (a revoked/read-only/untrusted 403 — the peer rejects the whole
//     LINK, §9.2/§9.3): mark the peer permanent so its remaining/future events are
//     not re-POSTed, log at ERROR with a remediation hint, and HALT with
//     deadLetterRemaining=true. Every other event to this peer would be rejected
//     identically, and the gate blocks any future drain, so the remaining chunks
//     must be dead-lettered too rather than stranded pending behind the gate.
//   - EVENT-SCOPED (a 400 author/origin-mismatch or clock-skew, a 401 signature-
//     rejected, a 410 stale-tombstone — specific to the offending event): dead-
//     letter ONLY that chunk, do NOT touch the peer gate, log at WARN, and KEEP
//     GOING (halt=false). The link is healthy; stranding the peer's other healthy
//     events on one event-specific 4xx is the bug this split fixes.
//
// A 429 honors the peer's Retry-After window (US-4.4 AC1) and HALTS; any other
// TRANSIENT error backs off exponentially (US-4.4 AC2) and HALTS. A TRANSIENT halt
// returns deadLetterRemaining=false: the remaining chunks stay pending and retry on
// the next drain once the backoff window elapses. The failed batch stays unstamped
// so a healthy peer is never blocked by a failing one (US-3.2 AC3).
func (w *Worker) handlePushFailure(ctx context.Context, projectID int64, peerURL string, chunk []store.OutboxEvent, err error) (halt, deadLetterRemaining bool) {
	permanent := isPermanent(err)
	// peerScoped only gates the whole peer when the reject is a peer-level link
	// reject (403). An event-scoped permanent reject (400/401/410) parks its event
	// but leaves the link alive.
	peerScoped := permanent && isPeerScoped(err)
	if permanent {
		statusCode, reason := statusReasonOf(err)
		w.deadLetterChunk(ctx, projectID, peerURL, chunk, statusCode, reason)
	}

	if permanent && !peerScoped {
		// Event-scoped permanent reject: dead-letter the event, keep the link.
		w.log.WarnContext(ctx, "federation: event-scoped permanent reject, dead-lettered (link stays healthy)",
			slog.String("op", "federation.outbox.Drain"),
			slog.Int64("project_id", projectID),
			slog.String("peer", peerURL),
			slog.Int("batch", len(chunk)),
			slog.String("err", err.Error()),
		)
		return false, false
	}

	retryAfter, hasRetry := retryAfterOf(err)
	wait := w.recordFailure(ctx, peerURL, peerScoped, retryAfter, hasRetry)
	if peerScoped {
		// The whole federation link is now permanently gated: no further events flow
		// to this peer until an operator re-enables it (RetryPeer) or the process
		// restarts AFTER the durable gate is cleared. Logged at ERROR with a
		// remediation hint so it is not silently lost in the WARN stream. The caller
		// dead-letters the remaining un-attempted chunks so they are not stranded.
		w.log.ErrorContext(ctx, "federation: peer permanently gated — link halted until operator re-enable",
			slog.String("op", "federation.outbox.Drain"),
			slog.Int64("project_id", projectID),
			slog.String("peer", peerURL),
			slog.Int("batch", len(chunk)),
			slog.String("remediation", "verify the peer revoked/read-only status, resolve the cause, then re-enable delivery via RetryPeer"),
			slog.String("err", err.Error()),
		)
		return true, true
	}

	w.log.WarnContext(ctx, "federation: push to peer failed",
		slog.String("op", "federation.outbox.Drain"),
		slog.Int64("project_id", projectID),
		slog.String("peer", peerURL),
		slog.Int("batch", len(chunk)),
		slog.Bool("retry_after", hasRetry),
		slog.Duration("backoff", wait),
		slog.String("err", err.Error()),
	)
	return true, false
}

// deadLetterRemainingChunks parks every event in the not-yet-attempted chunks of a
// batch whose peer was just permanently gated by a peer-scoped reject (Federation
// v1 F4.4 multi-chunk-tail fix). Without this the remaining chunks would sit in
// federation_outbox forever — unstamped (so counted as pending) yet never re-POSTed
// (peerReady blocks the gated peer) and never re-delivered after an operator
// RetryPeer (the undelivered read excludes dead-lettered rows). Each park reuses the
// offending error's status/reason. Best-effort per row (deadLetterChunk logs its own
// failures); the peer is gated regardless.
func (w *Worker) deadLetterRemainingChunks(ctx context.Context, projectID int64, peerURL string, remaining [][]store.OutboxEvent, err error) {
	total := 0
	for _, chunk := range remaining {
		total += len(chunk)
	}
	if total == 0 {
		return
	}
	statusCode, reason := statusReasonOf(err)
	for _, chunk := range remaining {
		w.deadLetterChunk(ctx, projectID, peerURL, chunk, statusCode, reason)
	}
	w.log.WarnContext(ctx, "federation: dead-lettered remaining undelivered events after peer-scoped link reject",
		slog.String("op", "federation.outbox.Drain"),
		slog.Int64("project_id", projectID),
		slog.String("peer", peerURL),
		slog.Int("remaining", total),
	)
}

// deadLetterChunk parks every event in a permanently-failed chunk in the dead-
// letter table (Federation v1 F4.4, US-4.4 AC3). Each park is best-effort: a
// store error is logged but does not abort the others (the peer is still gated
// permanent so the events are not re-POSTed regardless).
func (w *Worker) deadLetterChunk(ctx context.Context, projectID int64, peerURL string, chunk []store.OutboxEvent, statusCode int, reason string) {
	failedAt := model.FormatUTC(w.now())
	for _, ev := range chunk {
		if err := w.store.InsertDeadLetter(ctx, store.DeadLetterRow{
			EventID:        ev.EventID,
			PeerURL:        peerURL,
			LocalProjectID: projectID,
			Payload:        ev.Payload,
			StatusCode:     statusCode,
			Reason:         reason,
			FailedAt:       failedAt,
		}); err != nil {
			w.log.ErrorContext(ctx, "federation: dead-letter park failed",
				slog.String("op", "federation.outbox.Drain"),
				slog.Int64("project_id", projectID),
				slog.String("peer", peerURL),
				slog.String("event_id", ev.EventID),
				slog.String("err", err.Error()),
			)
		}
	}
}

// chunkBatch splits an undelivered batch into delivery chunks bounded by both the
// max-events count and the max serialized-payload bytes (Federation v1 F4.4,
// US-4.4 AC4). A single event larger than the byte cap is sent alone (never
// dropped): the cap is a batching hint, not a hard per-event limit. The byte
// measure is the sum of payload lengths, the dominant term of the serialized
// batch body.
func (w *Worker) chunkBatch(batch []store.OutboxEvent) [][]store.OutboxEvent {
	maxEvents := w.maxChunkEvents
	if maxEvents <= 0 {
		maxEvents = defaultMaxChunkEvents
	}
	maxBytes := w.maxChunkBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxChunkBytes
	}

	chunks := make([][]store.OutboxEvent, 0, 1)
	cur := make([]store.OutboxEvent, 0, maxEvents)
	curBytes := 0
	for _, ev := range batch {
		n := len(ev.Payload)
		// Flush the current chunk before adding this event if it would overflow
		// either cap — but never flush an empty chunk (a single oversized event is
		// sent alone rather than dropped).
		overEvents := len(cur) >= maxEvents
		overBytes := len(cur) > 0 && curBytes+n > maxBytes
		if len(cur) > 0 && (overEvents || overBytes) {
			chunks = append(chunks, cur)
			cur = make([]store.OutboxEvent, 0, maxEvents)
			curBytes = 0
		}
		cur = append(cur, ev)
		curBytes += n
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

// peerReady reports whether a peer may be POSTed now: it has no failure state, or
// its transient backoff window has elapsed. A permanently-gated peer (a
// peer-scoped 403 link reject) is never ready until an operator clears the gate
// via Worker.RetryPeer — a restart alone does NOT clear it, because RestoreBackoff
// re-loads the durable permanent=true row on startup.
func (w *Worker) peerReady(peerURL string) bool {
	w.peerMu.Lock()
	defer w.peerMu.Unlock()
	st, ok := w.peerStates[peerURL]
	if !ok {
		return true
	}
	if st.permanent {
		return false
	}
	return !w.now().Before(st.notBefore)
}

// recordFailure updates a peer's gate after a push error and returns the backoff
// window applied (0 for a peer-scoped permanent failure, which gates
// indefinitely). The permanent argument is true ONLY for a PEER-SCOPED reject (a
// revoked/read-only 403 — the whole link is dead); an event-scoped permanent
// reject (400/401/410) never reaches here, so it never gates the peer. A
// peer-scoped permanent failure marks the peer so it is not re-probed; a 429
// honors the peer's Retry-After window verbatim (US-4.4 AC1); any other transient
// failure doubles the exponential window (1s..1h cap, US-4.4 AC2). The resulting
// gate is PERSISTED so it survives a restart (§7 F4.4 risk) — the persist is
// best-effort and never changes the in-memory decision.
func (w *Worker) recordFailure(ctx context.Context, peerURL string, permanent bool, retryAfter time.Duration, hasRetry bool) time.Duration {
	w.peerMu.Lock()
	st := w.peerStates[peerURL]
	if st == nil {
		st = &peerState{}
		w.peerStates[peerURL] = st
	}
	var wait time.Duration
	switch {
	case permanent:
		st.permanent = true
		st.notBefore = w.now().Add(backoffMax) // belt-and-braces; peerReady gates on permanent.
	case hasRetry && retryAfter > 0:
		// 429: gate for exactly the peer's Retry-After. It does not advance the
		// exponential attempt counter (it is an explicit backpressure window, not a
		// fault), so a recovered peer resumes at the base rate.
		wait = retryAfter
		st.notBefore = w.now().Add(wait)
	default:
		st.attempt++
		wait = backoffFor(st.attempt)
		st.notBefore = w.now().Add(wait)
	}
	snapshot := store.PeerRetryRow{
		PeerURL:   peerURL,
		NotBefore: model.FormatUTC(st.notBefore),
		Attempt:   st.attempt,
		Permanent: st.permanent,
		UpdatedAt: model.FormatUTC(w.now()),
	}
	w.peerMu.Unlock()

	if err := w.store.SavePeerRetry(ctx, snapshot); err != nil {
		w.log.WarnContext(ctx, "federation: persist outbox backoff failed",
			slog.String("op", "federation.outbox.Drain"),
			slog.String("peer", peerURL),
			slog.String("err", err.Error()),
		)
	}
	return wait
}

// recordSuccess clears a peer's failure state so it drains without gating, and
// removes the persisted gate so a restart does not re-apply a now-stale backoff.
func (w *Worker) recordSuccess(ctx context.Context, peerURL string) {
	w.peerMu.Lock()
	_, gated := w.peerStates[peerURL]
	delete(w.peerStates, peerURL)
	w.peerMu.Unlock()
	if !gated {
		return // nothing persisted to clear (the common steady-state path).
	}
	if err := w.store.DeletePeerRetry(ctx, peerURL); err != nil {
		w.log.WarnContext(ctx, "federation: clear outbox backoff failed",
			slog.String("op", "federation.outbox.Drain"),
			slog.String("peer", peerURL),
			slog.String("err", err.Error()),
		)
	}
}

// RetryPeer is the explicit OPERATOR re-enable path for a peer whose link was
// permanently gated by a peer-scoped reject (a revoked/read-only 403). It clears
// BOTH the in-memory gate and the durable federation_peer_retry row so the next
// drain re-probes the peer — without it a peer-scoped permanent gate is dead
// forever (peerReady blocks the POST that would let recordSuccess run, and
// RestoreBackoff re-loads permanent=true on every restart). A peer with no gate is
// a no-op. The next drain naturally re-establishes a gate if the underlying cause
// (e.g. the peer is still revoked) has not actually been resolved. It is safe to
// call from a handler/service goroutine: peerStates is mutex-guarded.
func (w *Worker) RetryPeer(ctx context.Context, peerURL string) error {
	w.peerMu.Lock()
	_, gated := w.peerStates[peerURL]
	delete(w.peerStates, peerURL)
	w.peerMu.Unlock()
	w.log.InfoContext(ctx, "federation: operator re-enabled peer delivery",
		slog.String("op", "federation.outbox.RetryPeer"),
		slog.String("peer", peerURL),
		slog.Bool("was_gated", gated),
	)
	if err := w.store.DeletePeerRetry(ctx, peerURL); err != nil {
		return err
	}
	// Wake the worker so the re-enabled peer is re-probed promptly rather than on
	// the next safety-net tick (best-effort; Ping never blocks).
	w.Ping()
	return nil
}

// backoffFor returns the exponential backoff for the n-th consecutive transient
// failure: backoffMin * 2^(n-1), capped at backoffMax (1s, 2s, 4s, ... 1h).
func backoffFor(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	wait := backoffMin
	for i := 1; i < attempt; i++ {
		wait *= 2
		if wait >= backoffMax {
			return backoffMax
		}
	}
	return wait
}

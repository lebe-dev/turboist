package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	fedmetrics "github.com/lebe-dev/turboist/internal/federation/metrics"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// EventEnqueuer hands a validated, deduped event to the single inbox-apply
// goroutine (satisfied by *inbox.Queue). Keeping it an interface lets the handler
// test inject a capturing stub.
type EventEnqueuer interface {
	Enqueue(job inbox.Job)
}

// EventValidator runs the per-event payload checks (F3.2a) before any inbox or
// domain write (satisfied by *inbox.Validator).
type EventValidator interface {
	Validate(ctx context.Context, e events.Event, peerURL string) (*inbox.ValidationResult, error)
}

// KeyMismatchMarker records the sticky per-peer key-mismatch health marker when
// an inbound event's per-event signature stops validating (Federation v1 F4.3,
// US-4.3 AC4 — the inbox-signature-check writer). It resolves the event's
// project_client_id to the local project the peer maps to and stamps the marker
// (sticky, SSE-on-transition). Satisfied by *service/federation.Service. nil → a
// build without the status surface simply does not record the marker (the event
// is still rejected with zero rows by the validator either way).
type KeyMismatchMarker interface {
	MarkKeyMismatchByRemote(ctx context.Context, peerURL, projectClientID string) error
}

// FederationRateLimiter is the inbound per-peer token bucket guarding the event
// endpoint (Federation v1 F4.4, US-8.3 AC1). AllowN reports whether the peer may
// send n more events now; when throttled it returns the Retry-After window. It is
// satisfied by *federation/ratelimit.PeerLimiter. nil → no inbound rate limiting
// (the limiter is in-memory and optional; an unset one accepts everything).
type FederationRateLimiter interface {
	AllowN(peerURL string, n int) (bool, time.Duration)
}

// ContactToucher stamps a peer's last_contact_at on a successful inbound push so
// the owner-offline derivation (Federation v1 F5.6a, US-6.5 AC1/AC3) and the
// per-peer stale status reflect real recency — the push leg of "updated on every
// push/pull/handshake touchpoint". Satisfied by *repo.FederatedInstanceRepo
// (TouchLastContact, a no-op for an unknown peer). nil → the touch is skipped
// (the endpoint still accepts events; only the freshness signal is not refreshed).
type ContactToucher interface {
	TouchLastContact(ctx context.Context, instanceURL string, at time.Time) error
}

// EventMetrics records the inbound federation Prometheus counters (Federation v1
// F6.5, US-8.2): received events by peer + result and per-peer signature failures.
// Satisfied by *federation/metrics.Collectors. nil → metrics are not recorded
// (the endpoint behaviour is unaffected). Kept a local interface so the handler
// holds no hard dependency on the metrics package.
type EventMetrics interface {
	RecordEventReceived(peer string, result fedmetrics.Result, n int)
	RecordSignatureFailure(peer string)
}

// defaultMaxBatchEvents is the inbound events-per-batch ceiling when unset
// (Federation v1 F4.4, US-8.3 AC3) — matching the publisher's outbound chunk cap.
const defaultMaxBatchEvents = 500

// FederationEventsDeps wires the F3.2 push/pull collaborators onto the federation
// handler: the store (inbox dedup + pull reads), the per-event validator (F3.2a),
// the inbox-apply queue, and the federated-project repo (pull membership check).
type FederationEventsDeps struct {
	Store     *store.Store
	Validator EventValidator
	Queue     EventEnqueuer
	Projects  *repo.FederatedProjectRepo
	// KeyMismatch records the sticky per-peer key-mismatch health marker when an
	// inbound event's signature fails (Federation v1 F4.3, US-4.3 AC4). nil → the
	// marker is not recorded (the event is still rejected with zero rows).
	KeyMismatch KeyMismatchMarker
	// BaseURL is this instance's federation URL, used to compose the snapshot_url
	// returned in a stale-pull 410 body (US-3.7 AC4 emit half). Empty → the 410
	// body carries a relative snapshot path (still actionable behind a proxy).
	BaseURL string
	// RateLimiter is the inbound per-peer token bucket (Federation v1 F4.4,
	// US-8.3 AC1). nil → inbound rate limiting is off.
	RateLimiter FederationRateLimiter
	// MaxBatchEvents caps how many events one inbound batch may carry before it is
	// rejected 413 WHOLE (Federation v1 F4.4, US-8.3 AC3). 0 → the default (500).
	MaxBatchEvents int
	// Contact stamps the sending peer's last_contact_at on a successful inbound push
	// (Federation v1 F5.6a, US-6.5 AC1/AC3 — the push touchpoint). nil → the touch is
	// skipped.
	Contact ContactToucher
	// Auditor records one audit row per per-event rejection (Federation v1 F6.3,
	// US-7.4 AC1). It is satisfied by *audit.Writer (non-blocking). nil → per-event
	// rejections are not audited (the rejection itself is unaffected).
	Auditor httpapi.FederationAuditor
	// Metrics records the inbound Prometheus counters (Federation v1 F6.5, US-8.2):
	// received events by peer+result and per-peer signature failures. nil → no
	// metrics recorded.
	Metrics EventMetrics
}

// WithEventsDeps wires the push/pull endpoints onto the handler (Federation v1
// F3.2). Returns the handler for chaining. Until called, the events endpoints
// short-circuit to a clear error so a misconfigured build never silently accepts
// events.
func (h *FederationHandler) WithEventsDeps(deps FederationEventsDeps) *FederationHandler {
	h.events = &deps
	return h
}

// receiveEvents is the signed inbound endpoint POST /federation/events (US-3.1/
// US-3.2). For EACH event in the batch it: (1) runs the F3.2a per-event validator
// (signature/author-origin/clock-skew/membership) BEFORE any write, (2) records
// it in federation_inbox ON CONFLICT(event_id) DO NOTHING (dedup, NFR-2), and
// (3) enqueues it to the single inbox-apply goroutine, then returns FAST (202)
// without waiting for apply. A rejected event leaves ZERO inbox/domain rows
// (US-7.2 AC1). The whole batch is rejected on the first invalid event so a
// caller cannot smuggle a forged event in alongside valid ones.
func (h *FederationHandler) receiveEvents(c fiber.Ctx) error {
	ctx := c.Context()
	logEntry(c, "handler.Federation.ReceiveEvents")

	if h.events == nil || h.events.Store == nil || h.events.Validator == nil || h.events.Queue == nil {
		return httpapi.ErrFederationKeyMissing()
	}

	var batch events.Batch
	if err := json.Unmarshal(c.Body(), &batch); err != nil {
		logValidation(c, "handler.Federation.ReceiveEvents", "invalid batch body")
		return httpapi.ErrValidation("invalid event batch")
	}

	peer := httpapi.GetFederationPeer(c)

	// Oversized-batch 413 (Federation v1 F4.4, US-8.3 AC3): reject a batch larger
	// than the per-request cap WHOLE — before any validation / inbox write — so an
	// oversized payload is cheap to reject and never partially applied.
	maxBatch := h.events.MaxBatchEvents
	if maxBatch <= 0 {
		maxBatch = defaultMaxBatchEvents
	}
	if len(batch.Events) > maxBatch {
		logValidation(c, "handler.Federation.ReceiveEvents", "inbound batch too large")
		return httpapi.ErrFederationPayloadTooLarge(maxBatch)
	}

	// Inbound per-peer rate limit 429 (Federation v1 F4.4, US-8.3 AC1): a peer
	// over its event rate is throttled with a Retry-After so it backs off. The
	// whole batch is rejected (no partial accept); nothing is written. A nil
	// limiter disables limiting. An empty batch is not metered.
	if h.events.RateLimiter != nil && len(batch.Events) > 0 {
		if ok, retryAfter := h.events.RateLimiter.AllowN(peer.InstanceURL, len(batch.Events)); !ok {
			secs := retryAfterSeconds(retryAfter)
			c.Set("Retry-After", strconv.Itoa(secs))
			logValidation(c, "handler.Federation.ReceiveEvents", "peer rate-limited")
			return httpapi.ErrFederationRateLimited(secs)
		}
	}

	// Validate every event FIRST (before any inbox write) so a forged event in the
	// batch rejects the whole request with zero side effects (US-7.2 AC1).
	results := make([]*inbox.ValidationResult, len(batch.Events))
	for i := range batch.Events {
		vr, err := h.events.Validator.Validate(ctx, batch.Events[i], peer.InstanceURL)
		if err != nil {
			// A per-event SIGNATURE failure (ErrEventSignatureInvalid) is now ONLY
			// returned when the event was verified against a key that WAS resolved and
			// did not match — genuine proof this peer rotated its key (the F4.3
			// inbox-signature-check signal). Record the sticky per-peer key-mismatch
			// health marker (US-4.3 AC4) so the owner UI flips the badge red, then
			// reject. A transient key-RESOLUTION failure (ErrEventKeyUnresolved) is
			// deliberately NOT treated as a rotation: it maps to a retryable 503 below
			// and never stamps the sticky marker (Federation v1 F4.3 review fix). The
			// event is still dropped with zero rows by returning before any inbox/domain
			// write. Recording the marker is best-effort: a failure to stamp it must NOT
			// change the rejection.
			if errors.Is(err, inbox.ErrEventSignatureInvalid) && h.events.KeyMismatch != nil {
				// logError-grade audit: the sticky marker is irreversible without an
				// operator action (F5.6b), so every stamp is recorded with peer + reason.
				logging.FromContext(ctx).ErrorContext(ctx, "federation: peer key rotation detected, stamping sticky key_mismatch marker",
					slog.String("op", "handler.Federation.ReceiveEvents"),
					slog.String("peer", peer.InstanceURL),
					slog.String("project_client_id", batch.Events[i].ProjectClientID),
					slog.String("reason", err.Error()),
				)
				if markErr := h.events.KeyMismatch.MarkKeyMismatchByRemote(ctx, peer.InstanceURL, batch.Events[i].ProjectClientID); markErr != nil {
					logging.FromContext(ctx).WarnContext(ctx, "federation: record key-mismatch marker failed",
						slog.String("op", "handler.Federation.ReceiveEvents"),
						slog.String("peer", peer.InstanceURL),
						slog.String("err", markErr.Error()),
					)
				}
			}
			// Audit the per-event rejection (Federation v1 F6.3, US-7.4 AC1). The detail
			// is a short coded reason (never the event's signature/fields), and the
			// recording is non-blocking so it cannot stall the rejection.
			h.recordEventRejection(peer.InstanceURL, err)
			// Metrics (Federation v1 F6.5, US-8.2): a signature failure bumps the
			// per-peer signature-failures counter; every rejection counts as a received
			// error toward federation_events_received_total{result="error"}.
			if h.events.Metrics != nil {
				if errors.Is(err, inbox.ErrEventSignatureInvalid) {
					h.events.Metrics.RecordSignatureFailure(peer.InstanceURL)
				}
				h.events.Metrics.RecordEventReceived(peer.InstanceURL, fedmetrics.ResultError, 1)
			}
			return mapEventValidationError(c, batch.Events[i], err)
		}
		results[i] = vr
	}

	now := model.FormatUTC(time.Now())
	for i := range batch.Events {
		evt := batch.Events[i]
		inserted, err := h.events.Store.InsertInbox(ctx, evt.EventID, peer.InstanceURL, results[i].LocalProjectID, payloadOf(ctx, evt), now)
		if err != nil {
			return httpapi.ErrInternal("record event").WithCause(err)
		}
		if !inserted {
			// Duplicate delivery (push+pull, or a retried POST) — already recorded,
			// skip re-enqueue (NFR-2 dedup).
			continue
		}
		h.events.Queue.Enqueue(inbox.Job{Event: evt, PeerURL: peer.InstanceURL})
	}

	// Metrics (Federation v1 F6.5, US-8.2): the whole batch validated and was
	// recorded/enqueued, so count it as received-success for this peer. Duplicate
	// (already-recorded) events are still genuinely received, so the batch length is
	// the received count.
	if h.events.Metrics != nil && len(batch.Events) > 0 {
		h.events.Metrics.RecordEventReceived(peer.InstanceURL, fedmetrics.ResultSuccess, len(batch.Events))
	}

	// Freshness touchpoint (Federation v1 F5.6a, US-6.5 AC1/AC3): a successful push
	// from a peer refreshes its last_contact_at so a joiner stops flagging the OWNER
	// "offline" the moment the owner reaches it again (the owner-returns signal), and
	// the owner's per-peer stale status clears too. Best-effort + non-blocking: the
	// events are already recorded/enqueued, so a touch failure (DB busy, unknown
	// peer) must NOT change the 202 — it only delays the freshness signal one cycle.
	if h.events.Contact != nil {
		if err := h.events.Contact.TouchLastContact(ctx, peer.InstanceURL, time.Now()); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "federation: touch last_contact_at on push failed",
				slog.String("op", "handler.Federation.ReceiveEvents"),
				slog.String("peer", peer.InstanceURL),
				slog.String("err", err.Error()),
			)
		}
	}

	logMutation(c, "handler.Federation.ReceiveEvents",
		slog.String("peer", peer.InstanceURL), slog.Int("events", len(batch.Events)))
	return c.SendStatus(fiber.StatusAccepted)
}

// pullEvents is the signed catch-up read GET /federation/projects/:id/events
// (US-3.2 AC3 / US-4.1). :id is THIS instance's local project id; the calling
// peer must be a non-revoked member of it. It returns the project's events whose
// max field HLC is strictly greater than since_hlc, ascending, plus next_hlc.
func (h *FederationHandler) pullEvents(c fiber.Ctx) error {
	ctx := c.Context()
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Federation.PullEvents", slog.Int64("project_id", id))

	if h.events == nil || h.events.Store == nil || h.events.Projects == nil {
		return httpapi.ErrFederationKeyMissing()
	}

	peer := httpapi.GetFederationPeer(c)
	fp, err := h.events.Projects.Get(ctx, id, peer.InstanceURL)
	if errors.Is(err, repo.ErrNotFound) {
		logValidation(c, "handler.Federation.PullEvents", "peer not a member")
		return httpapi.ErrFederationUntrusted("not a member of this project")
	}
	if err != nil {
		return httpapi.ErrInternal("pull membership").WithCause(err)
	}
	// Revoked (Federation v1 F5.4, US-6.2 AC2/AC4): a revoked peer's catch-up pull
	// is rejected with the distinct, terminal 403 federation_revoked — the symmetric
	// pull leg of the receiveEvents reject — so a peer returning from offline that
	// missed the in-band federation_revoke event self-marks federation_lost (AC4).
	if fp.Revoked {
		return httpapi.ErrFederationRevoked()
	}
	// Paused (Federation v1 F5.3, US-6.1 AC1): a paused peer's catch-up pull is
	// rejected with the distinct, resumable 403 federation_paused so a paused peer
	// neither pushes nor pulls while the link stays trusted (symmetric with the
	// receiveEvents reject). The events it missed flush on resume.
	if fp.Paused {
		logValidation(c, "handler.Federation.PullEvents", "peer paused")
		return httpapi.ErrFederationPaused()
	}

	sinceHLC := c.Query("since_hlc")
	limit := parseLimit(c.Query("limit"))

	// Stale-pull 410 emit (US-3.7 AC4 emit half): a NON-empty since_hlc whose
	// position predates the events still recoverable from this owner means the
	// peer has missed GC'd changes and must re-snapshot rather than be falsely told
	// it is caught up. An empty cursor (a fresh peer / initial bootstrap) is never
	// stale: it is served the full retained log. The consume half (fetch snapshot,
	// preserve outbox) lands in F4.2; here we emit only.
	//
	// The gate must NOT be anchored only to the transient PRESENCE of outbox rows:
	// outbox retention (default 30d) is shorter than tombstone retention (default
	// 90d), so a quiet project's outbox can be GC'd to empty while tombstones still
	// guard against resurrection. We therefore key the decision off THREE durable
	// signals (US-3.7 AC4 review fix):
	//   1. the persisted pruned-floor HLC the GC advances as it purges outbox rows
	//      (since_hlc < floor → stale, EVEN when the outbox is now empty),
	//   2. the oldest retained outbox event (since_hlc < oldest → stale), and
	//   3. a federated project with a non-empty cursor but NO retained outbox at all
	//      (a long-quiet project that cannot prove the cursor is caught up → stale).
	if sinceHLC != "" {
		oldest, err := h.events.Store.OldestRetainedHLC(ctx, id)
		if err != nil {
			return httpapi.ErrInternal("pull retention bound").WithCause(err)
		}
		floor, err := h.events.Store.PrunedFloorHLC(ctx, id)
		if err != nil {
			return httpapi.ErrInternal("pull pruned floor").WithCause(err)
		}

		stale := false
		switch {
		case floor != "" && hlc.CompareString(sinceHLC, floor) < 0:
			// The cursor predates events the GC has durably recorded as pruned.
			stale = true
		case oldest != "" && hlc.CompareString(sinceHLC, oldest) < 0:
			// The cursor predates the oldest event still in the outbox.
			stale = true
		case oldest == "":
			// Federated project (membership already verified above) with a non-empty
			// cursor and no retained outbox: nothing proves the cursor is caught up, so
			// it cannot be safely served 200. Re-snapshot.
			stale = true
		}

		if stale {
			head, err := h.events.Store.HeadRetainedHLC(ctx, id)
			if err != nil {
				return httpapi.ErrInternal("pull retention head").WithCause(err)
			}
			// as_of_hlc is the best cutoff the peer can catch up to: the head retained
			// event if any survives, else the durable pruned floor.
			asOf := head
			if asOf == "" {
				asOf = floor
			}
			logValidation(c, "handler.Federation.PullEvents", "since_hlc older than recoverable history")
			return httpapi.ErrFederationStalePull(h.snapshotURL(id), asOf)
		}
	}

	rows, err := h.events.Store.ListEventsSinceHLC(ctx, id, sinceHLC, limit)
	if err != nil {
		return httpapi.ErrInternal("pull events").WithCause(err)
	}

	out := events.PullResponse{Events: make([]events.Event, 0, len(rows)), NextHLC: sinceHLC}
	for _, r := range rows {
		var e events.Event
		if err := events.Unmarshal([]byte(r.Payload), &e); err != nil {
			// A stored event that no longer decodes is skipped rather than failing
			// the whole pull (forward-compat / corruption resilience).
			logging.FromContext(ctx).WarnContext(ctx, "federation: skip undecodable pull event",
				slog.String("op", "handler.Federation.PullEvents"),
				slog.String("event_id", r.EventID),
			)
			continue
		}
		out.Events = append(out.Events, e)
		if r.MaxHLC != "" {
			out.NextHLC = r.MaxHLC
		}
	}
	return c.JSON(out)
}

// snapshotURL composes the owner-side snapshot endpoint URL for a project,
// matching service/federation's snapshotURL so the stale-pull 410 body points the
// peer at the same bootstrap endpoint the handshake advertised. A missing BaseURL
// yields a relative path (still actionable behind a reverse proxy).
func (h *FederationHandler) snapshotURL(projectID int64) string {
	base := ""
	if h.events != nil {
		base = strings.TrimRight(h.events.BaseURL, "/")
	}
	return fmt.Sprintf("%s/federation/projects/%d/snapshot", base, projectID)
}

// recordEventRejection emits one audit row for a per-event rejection (Federation
// v1 F6.3, US-7.4 AC1). It maps the validator sentinel to an audit kind and
// records via the non-blocking auditor; a transient key-resolution failure
// (ErrEventKeyUnresolved) and a membership/permission reject are NOT audited as
// signature-class anomalies — only the security-relevant signature/author/skew
// failures are. A nil auditor is a no-op.
func (h *FederationHandler) recordEventRejection(peerURL string, err error) {
	if h.events == nil || h.events.Auditor == nil {
		return
	}
	var kind repo.AuditKind
	var detail string
	switch {
	case errors.Is(err, inbox.ErrEventSignatureInvalid):
		kind, detail = repo.AuditKindSignatureInvalid, "event signature invalid"
	case errors.Is(err, inbox.ErrAuthorOriginMismatch):
		kind, detail = repo.AuditKindAuthorMismatch, "event author/origin mismatch"
	case errors.Is(err, inbox.ErrEventClockSkew):
		kind, detail = repo.AuditKindClockSkew, "event clock skew"
	default:
		// ErrEventKeyUnresolved (transient), revoked/paused/not-member — not a
		// signature-class security anomaly; left to the existing WARN logs.
		return
	}
	h.events.Auditor.Record(repo.AuditEntry{
		Kind:            kind,
		Outcome:         repo.AuditOutcomeRejected,
		PeerInstanceURL: peerURL,
		Detail:          detail,
		CreatedAt:       time.Now(),
	})
}

// mapEventValidationError translates a per-event validator sentinel to the wire
// status. The signature plane is split three ways: a transient author-key
// resolution failure is a retryable 503 (Federation v1 F4.3 review fix — NOT a
// key rotation), a verified-and-rejected per-event signature is 401 (US-7.2 AC1),
// author/origin mismatch + clock skew are 400 (US-7.2 AC3/AC4), and a membership/
// permission failure is 403.
func mapEventValidationError(c fiber.Ctx, e events.Event, err error) error {
	switch {
	case errors.Is(err, inbox.ErrEventKeyUnresolved):
		// Transient: the author key could not be fetched, so the event was neither
		// verified nor disproven. Retryable 503, and NOT a key-rotation signal — the
		// sticky marker is deliberately not stamped (Federation v1 F4.3 review fix).
		logValidation(c, "handler.Federation.ReceiveEvents", "event author key unresolved")
		return httpapi.ErrFederationKeyUnresolved()
	case errors.Is(err, inbox.ErrEventSignatureInvalid):
		logValidation(c, "handler.Federation.ReceiveEvents", "event signature invalid")
		return httpapi.ErrFederationSignatureInvalid("event signature invalid")
	case errors.Is(err, inbox.ErrAuthorOriginMismatch):
		logValidation(c, "handler.Federation.ReceiveEvents", "author/origin mismatch")
		return httpapi.ErrFederationAuthorMismatch()
	case errors.Is(err, inbox.ErrEventClockSkew):
		logValidation(c, "handler.Federation.ReceiveEvents", "event clock skew")
		return httpapi.ErrFederationClockSkew()
	case errors.Is(err, inbox.ErrPeerRevoked):
		// Permanent revoke (Federation v1 F5.4, US-6.2 AC2/AC4): reject with the
		// distinct, terminal 403 federation_revoked (NOT the generic untrusted 403 or
		// the reversible paused 403) so a revoked peer — including one returning from
		// offline that missed the in-band revoke event — can tell it has been revoked
		// and self-mark federation_lost (US-6.2 AC4).
		logValidation(c, "handler.Federation.ReceiveEvents", "peer revoked")
		return httpapi.ErrFederationRevoked()
	case errors.Is(err, inbox.ErrPeerPaused):
		// Non-destructive pause (Federation v1 F5.3, US-6.1 AC1): the link is still
		// trusted — reject the event with the distinct, resumable 403 federation_paused
		// (NOT the generic untrusted 403) so the peer can tell a pause from a revoke.
		logValidation(c, "handler.Federation.ReceiveEvents", "peer paused")
		return httpapi.ErrFederationPaused()
	case errors.Is(err, inbox.ErrNotMember), errors.Is(err, inbox.ErrPeerNotPermitted):
		logValidation(c, "handler.Federation.ReceiveEvents", "peer not permitted")
		return httpapi.ErrFederationUntrusted("peer not permitted for this project")
	default:
		return httpapi.ErrInternal("validate event").WithCause(err)
	}
}

// payloadOf re-marshals an event to the bytes stored in federation_inbox. The
// inbox payload is the loser-record history retained until GC; it carries the
// full event including its signature.
func payloadOf(ctx context.Context, e events.Event) string {
	b, err := events.Marshal(e)
	if err != nil {
		// Marshalling an already-validated event should never fail; if it does we
		// keep the graceful "{}" fallback so the event is still recorded/enqueued,
		// but surface the loss of the retained payload at WARN so it is diagnosable.
		logging.FromContext(ctx).WarnContext(ctx, "federation: re-marshal inbox payload failed, storing empty payload",
			slog.String("op", "handler.Federation.ReceiveEvents"),
			slog.String("event_id", e.EventID),
			slog.String("err", err.Error()),
		)
		return "{}"
	}
	return string(b)
}

// retryAfterSeconds rounds a Retry-After window UP to whole seconds, clamped to a
// minimum of 1 (the HTTP Retry-After header is an integer second count). Shared by
// the inbound-event 429 and the handshake 429 (Federation v1 F4.4 / F7.7).
func retryAfterSeconds(d time.Duration) int {
	secs := int(d / time.Second)
	if d%time.Second != 0 {
		secs++
	}
	if secs < 1 {
		secs = 1
	}
	return secs
}

// parseLimit parses the pull batch-size cap, clamping to a sane default/maximum.
func parseLimit(raw string) int {
	const def, max = 500, 1000
	if raw == "" {
		return def
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
		if n > max {
			return max
		}
	}
	if n == 0 {
		return def
	}
	return n
}

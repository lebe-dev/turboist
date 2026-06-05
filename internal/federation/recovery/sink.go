package recovery

import (
	"context"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
)

// Enqueuer hands a recorded event to the single inbox-apply goroutine (satisfied
// by *inbox.Queue). The recovery loop and the push handler share the SAME queue
// so a pulled event is applied through the exact per-field LWW merge a pushed
// event is — one applier, one writer.
type Enqueuer interface {
	Enqueue(job inbox.Job)
}

// ContactToucher stamps a peer's last_contact_at on a successful pull (Federation
// v1 F5.6a, US-6.5 AC1/AC3 — the pull touchpoint). Satisfied by
// *repo.FederatedInstanceRepo (TouchLastContact, a no-op for an unknown peer).
type ContactToucher interface {
	TouchLastContact(ctx context.Context, instanceURL string, at time.Time) error
}

// StoreSink is the production EventSink: it records pulled events in
// federation_inbox (dedup via ON CONFLICT(event_id)), enqueues newly-recorded
// events to the inbox-apply queue, and advances the (project, peer) cursor
// monotonically (Federation v1 F4.1). Recording BEFORE enqueue is the same
// at-least-once contract the push handler uses (NFR-2): a redelivery of an
// already-recorded event is deduped and not re-applied.
//
// The sink performs NO per-event authentication: the F3.2a validator (per-event
// signature / author-origin / clock-skew / membership) runs in the Loop BEFORE
// Record is ever called, and Record is handed the membership-checked local
// project id the validator resolved — never a project the event payload merely
// claims. Keeping validation in the Loop puts push and pull on one validation
// seam (the push handler validates before InsertInbox too).
type StoreSink struct {
	store     *store.Store
	queue     Enqueuer
	instances ContactToucher
	now       func() time.Time
}

// NewStoreSink wraps the federation store + inbox queue as the recovery sink.
func NewStoreSink(st *store.Store, queue Enqueuer) *StoreSink {
	return &StoreSink{store: st, queue: queue, now: time.Now}
}

// WithInstances wires the peer trust-directory toucher so a successful pull
// refreshes the peer's last_contact_at (Federation v1 F5.6a, US-6.5 AC1/AC3).
// nil leaves TouchContact a no-op (a F4.1-only build without owner-offline simply
// does not refresh the signal on pull; correctness is unaffected). Returns the
// sink for chaining.
func (s *StoreSink) WithInstances(instances ContactToucher) *StoreSink {
	s.instances = instances
	return s
}

// WithClock overrides the received-at clock (default time.Now), for deterministic
// tests.
func (s *StoreSink) WithClock(now func() time.Time) *StoreSink {
	if now != nil {
		s.now = now
	}
	return s
}

// Record durably writes the pulled event to federation_inbox, deduped on
// event_id. It returns whether the row was NEWLY inserted (true → the loop
// enqueues it for apply) or already present (false → a push+pull / earlier-pull
// duplicate, skipped). The stored payload is the re-marshalled canonical event,
// matching the push handler's inbox payload (the loser-record history).
func (s *StoreSink) Record(ctx context.Context, e events.Event, peerURL string, localProjectID int64) (bool, error) {
	payload, err := events.Marshal(e)
	if err != nil {
		return false, err
	}
	return s.store.InsertInbox(ctx, e.EventID, peerURL, localProjectID, string(payload), model.FormatUTC(s.now()))
}

// Enqueue hands a newly-recorded event to the inbox-apply goroutine.
func (s *StoreSink) Enqueue(job inbox.Job) {
	s.queue.Enqueue(job)
}

// AdvanceCursor moves the (project, peer) last_received_hlc forward to toHLC
// monotonically (a no-op for a lower-or-equal HLC).
func (s *StoreSink) AdvanceCursor(ctx context.Context, localProjectID int64, peerURL, toHLC string) error {
	return s.store.AdvanceLastReceivedHLC(ctx, localProjectID, peerURL, toHLC)
}

// TouchContact refreshes the peer's last_contact_at after a successful pull
// (Federation v1 F5.6a, US-6.5 AC1/AC3). A nil instances toucher (a build without
// owner-offline) is a no-op.
func (s *StoreSink) TouchContact(ctx context.Context, peerURL string) error {
	if s.instances == nil {
		return nil
	}
	return s.instances.TouchLastContact(ctx, peerURL, s.now())
}

package inbox

import (
	"errors"
	"fmt"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
)

// PoisonError is a PERMANENT reject: the event payload is malformed in a way no
// amount of retrying can fix (an out-of-domain enum/color value, a non-integer
// position, etc.). It MUST be distinguished from a transient error (DB busy,
// connection drop) so the F3.2 inbox-apply worker can DROP/quarantine the event
// and stamp applied_at — instead of retrying it forever and head-of-line
// blocking every later event in the same per-project queue.
//
// §3/W-8: the federation field-set whitelist ignores unknown field NAMES; this
// type closes the matching gap on field VALUES — a peer (or a buggy/cross-app
// sender) shipping status:"garbage" / priority:"p1" / color:"chartreuse" is
// rejected as a per-event poison error, never blindly trusted to a raw UPDATE
// where the table CHECK constraint would roll the whole apply tx back with an
// opaque, retried-forever error.
//
// The carried ErrorID + peer/EventID/EntityType/Field/Value give the operator
// an actionable handle (logged once, surfaced in the F8.4 audit log later)
// without leaking the payload across the whole call stack.
type PoisonError struct {
	// ErrorID is a fresh correlation id minted per poison event so a single log
	// line / audit row can be tied back to the exact rejection.
	ErrorID string
	// EventID is the offending federation event's id.
	EventID string
	// PeerURL is the instance the event arrived from.
	PeerURL string
	// EntityType / EntityID locate the targeted entity.
	EntityType string
	EntityID   string
	// Field is the federated field name whose VALUE was out of domain.
	Field string
	// Value is the rejected (raw, decoded JSON) value, for the log/audit detail.
	Value any
	// Reason is a short machine-stable cause (e.g. "invalid status").
	Reason string
}

// Error renders the poison reject with its correlation id and full context.
func (e *PoisonError) Error() string {
	return fmt.Sprintf(
		"inbox: poison event rejected (errorId=%s peer=%s event=%s entity=%s/%s field=%s value=%v): %s",
		e.ErrorID, e.PeerURL, e.EventID, e.EntityType, e.EntityID, e.Field, e.Value, e.Reason)
}

// newPoison builds a PoisonError with a freshly-minted correlation id.
func newPoison(e eventCtx, field string, value any, reason string) *PoisonError {
	return &PoisonError{
		ErrorID:    model.NewClientID(),
		EventID:    e.eventID,
		PeerURL:    e.peerURL,
		EntityType: e.entityType,
		EntityID:   e.entityID,
		Field:      field,
		Value:      value,
		Reason:     reason,
	}
}

// casError classifies an error from the per-field HLC compare-and-set. A
// store.ErrMalformedHLC is a PERMANENT poison reject (an unparseable HLC reached
// the store — it must never be persisted, lest it permanently poison per-field
// LWW for the field), so it is mapped to a PoisonError the F3.2 worker drops
// rather than retries forever. The F3.2a skew validator normally rejects such an
// event before apply; this is the defense-in-depth classification for any bypass.
// Any OTHER error (DB busy, connection drop) is transient and passed through
// unchanged so the worker retries it.
func casError(e events.Event, peerURL, field, hlc string, err error) error {
	if errors.Is(err, store.ErrMalformedHLC) {
		ectx := eventCtx{
			eventID:    e.EventID,
			peerURL:    peerURL,
			entityType: string(e.EntityType),
			entityID:   e.EntityID,
		}
		return newPoison(ectx, field, hlc, "malformed HLC reached field-HLC CAS")
	}
	return err
}

// eventCtx carries the identifying fields a poison reject needs without dragging
// the whole events.Event (and its imports) into the per-field validation path.
type eventCtx struct {
	eventID    string
	peerURL    string
	entityType string
	entityID   string
}

// IsPoison reports whether err is a permanent (do-not-retry) poison reject. The
// F3.2 inbox-apply worker uses this to drop/quarantine the event and stamp
// applied_at rather than retrying it forever; any OTHER non-nil error is treated
// as transient (retryable). A *PoisonError is returned unwrapped via the second
// result for the caller to log its ErrorID/context.
func IsPoison(err error) (*PoisonError, bool) {
	var pe *PoisonError
	if errors.As(err, &pe) {
		return pe, true
	}
	return nil, false
}

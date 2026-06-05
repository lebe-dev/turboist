package events

import "fmt"

// Wire paths + batch envelope for the F3.2 peer-to-peer event endpoints.
//
// PushPath is the signed inbound endpoint a peer POSTs a batch of events to.
// PullPath (with a :id route param on the server) is the signed catch-up read a
// peer GETs from its last_received_hlc cursor. Both are mounted on the same
// HTTP-signature-verified /federation group as the handshake and snapshot.

// PushPath is the route the owner-hub / peer POSTs an event batch to.
const PushPath = "/federation/events"

// Batch is the wire envelope for a push: one or more canonical signed events.
// The receiver decodes each event from the SAME bytes it verifies the per-event
// signature over (no re-serialise before verify — F3.2a).
type Batch struct {
	Events []Event `json:"events"`
}

// PullResponse is the wire shape of GET /federation/projects/:id/events: the
// events with a max field HLC strictly greater than the requested since_hlc,
// in HLC-ascending order, plus the cursor a caller advances to.
type PullResponse struct {
	Events  []Event `json:"events"`
	NextHLC string  `json:"next_hlc"`
}

// StalePullError is the typed outcome of a pull that the owner answered with a
// 410 stale_pull (the F3.3 emit half of US-3.7 AC4): the caller's since_hlc
// predates the owner's retained history, so the in-between events were GC'd and
// the caller must RE-SNAPSHOT instead of being told it is caught up. It carries
// the {snapshot_url, as_of_hlc} the 410 body advertised, which the F4.2 consume
// half (recovery loop → ReBootstrap) uses to re-fetch and overwrite local state
// while preserving the unsent outbox. The outbound peer client (Publisher.Pull)
// returns this when it decodes a 410 federation_stale_pull response; the recovery
// loop detects it via errors.As and drives the re-bootstrap consumer.
type StalePullError struct {
	SnapshotURL string
	AsOfHLC     string
}

func (e *StalePullError) Error() string {
	return fmt.Sprintf("federation: pull cursor stale — re-snapshot from %s (as_of %s)", e.SnapshotURL, e.AsOfHLC)
}

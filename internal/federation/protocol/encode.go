package protocol

import (
	"fmt"

	"github.com/lebe-dev/turboist/internal/federation/events"
)

// Encoder seam (Federation v1 F6.1).
//
// Encode/Decode are the SINGLE place a federation event is turned into / read
// from its on-the-wire bytes for a given negotiated protocol version. In v1 they
// are the IDENTITY transform — Encode at version 1 is exactly events.Marshal and
// Decode is exactly events.Unmarshal — but they exist as the forward-compat seam
// where a FUTURE build would dual-write (emit a v1-shaped event alongside a
// v2-shaped one, or down-convert a v2 event for a v1 peer). Routing every
// serialise/deserialise through the seam keeps that future change confined here
// rather than scattered across the outbox/inbox/relay call sites.
//
// The seam preserves a forward-compatible peer's unknown fields (events.Event
// carries an Extras bag), so a v1 relay re-emits a newer peer's event verbatim
// and the per-event signature still verifies downstream (relay integrity).

// IsSupported reports whether this build can speak the given protocol version
// (it is present in SupportedProtocolVersions).
func IsSupported(version int) bool {
	for _, v := range SupportedProtocolVersions {
		if v == version {
			return true
		}
	}
	return false
}

// Encode serialises an event to its wire bytes for the negotiated protocol
// version. v1 is the identity transform (== events.Marshal); the unknown-field
// Extras bag is re-emitted verbatim for relay integrity. An unsupported version
// is rejected so a caller never silently emits bytes a peer cannot read.
func Encode(e events.Event, version int) ([]byte, error) {
	if !IsSupported(version) {
		return nil, fmt.Errorf("federation: cannot encode at unsupported protocol version %d", version)
	}
	// v1: identity. Future versions branch here to dual-write / down-convert.
	return events.Marshal(e)
}

// Decode reads an event from its wire bytes. It is the identity counterpart of
// Encode: it does NOT use DisallowUnknownFields, so a forward-compatible peer's
// extra top-level fields decode into the event's Extras bag (US-9.1 AC3) rather
// than breaking the decode. The known fields are populated; the apply path acts
// only on those.
func Decode(b []byte) (events.Event, error) {
	var e events.Event
	if err := events.Unmarshal(b, &e); err != nil {
		return events.Event{}, err
	}
	return e, nil
}

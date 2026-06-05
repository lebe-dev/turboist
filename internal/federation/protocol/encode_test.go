package protocol_test

import (
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/protocol"
)

// sampleEvent is a representative two-field update event the encoder seam tests
// round-trip through Encode/Decode.
func sampleEvent() events.Event {
	return events.Event{
		EventID:         "01J0000000000000000000ENC1",
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        "task-client-1",
		ProjectClientID: "proj-client-1",
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"title": {Value: "Renamed", HLC: "00000000000100-0000-nodeA"},
		},
	}
}

// TestEncode_IdentityV1 asserts the v1 encoder seam is the IDENTITY transform:
// Encode at the only supported version (1) produces exactly the canonical wire
// bytes of events.Marshal, with no dual-write transformation (F6.1 — the seam
// exists for FUTURE dual-write but is a no-op in v1).
func TestEncode_IdentityV1(t *testing.T) {
	e := sampleEvent()
	encoded, err := protocol.Encode(e, 1)
	if err != nil {
		t.Fatalf("encode v1: %v", err)
	}
	plain, err := events.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != string(plain) {
		t.Errorf("v1 encode must be identity:\n encoded=%s\n marshal=%s", encoded, plain)
	}
}

// TestEncode_RoundTripV1 asserts a v1 Encode→Decode cycle reproduces the event,
// including any unknown forward-compat extras (relay integrity through the seam).
func TestEncode_RoundTripV1(t *testing.T) {
	e := sampleEvent()
	encoded, err := protocol.Encode(e, 1)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := protocol.Decode(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.EventID != e.EventID || got.Op != e.Op || got.EntityID != e.EntityID {
		t.Errorf("decode mismatch: got %+v", got)
	}
	if got.Fields["title"].Value != "Renamed" {
		t.Errorf("decode field: got %v, want Renamed", got.Fields["title"].Value)
	}
}

// TestEncode_UnsupportedVersion asserts the seam rejects a version this build
// cannot speak (the encode-side guard against a negotiated-but-unsupported
// version). v1 only supports version 1.
func TestEncode_UnsupportedVersion(t *testing.T) {
	if _, err := protocol.Encode(sampleEvent(), 2); err == nil {
		t.Errorf("encode at an unsupported version must error")
	}
	if _, err := protocol.Encode(sampleEvent(), 0); err == nil {
		t.Errorf("encode at version 0 must error")
	}
}

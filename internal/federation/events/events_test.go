package events_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
)

// newEvent builds a representative two-field update event for the sign/verify
// tests (op=update on a task, two fields each carrying their own HLC).
func newEvent() events.Event {
	return events.Event{
		EventID:         "01J0000000000000000000EVNT",
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        "task-client-1",
		ProjectClientID: "proj-client-1",
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"title":    {Value: "Renamed", HLC: "00000000000100-0000-nodeA"},
			"priority": {Value: "p1", HLC: "00000000000100-0000-nodeA"},
		},
	}
}

// TestCanonicalBytes_Deterministic asserts the canonical encoding is independent
// of map iteration order: two events with the same logical content (fields built
// in different insertion order) canonicalize to identical bytes, which is what
// makes a cross-instance signature verifiable (canonical determinism).
func TestCanonicalBytes_Deterministic(t *testing.T) {
	e1 := newEvent()
	e2 := events.Event{
		EventID:         e1.EventID,
		Op:              e1.Op,
		EntityType:      e1.EntityType,
		EntityID:        e1.EntityID,
		ProjectClientID: e1.ProjectClientID,
		Author:          e1.Author,
		OriginInstance:  e1.OriginInstance,
		CreatedAt:       e1.CreatedAt,
		Fields: map[string]events.Field{
			"priority": {Value: "p1", HLC: "00000000000100-0000-nodeA"},
			"title":    {Value: "Renamed", HLC: "00000000000100-0000-nodeA"},
		},
	}

	b1, err := events.CanonicalBytes(e1)
	if err != nil {
		t.Fatalf("canonical e1: %v", err)
	}
	b2, err := events.CanonicalBytes(e2)
	if err != nil {
		t.Fatalf("canonical e2: %v", err)
	}
	if string(b1) != string(b2) {
		t.Errorf("canonical bytes differ for equal events:\n %s\n %s", b1, b2)
	}
}

// TestCanonicalBytes_ExcludesSignature asserts the signature field is excluded
// from the canonical bytes so the signature can be embedded into the event after
// signing without invalidating it (sign over event-minus-signature).
func TestCanonicalBytes_ExcludesSignature(t *testing.T) {
	e := newEvent()
	before, err := events.CanonicalBytes(e)
	if err != nil {
		t.Fatalf("canonical before: %v", err)
	}
	e.Signature = "ZmFrZS1zaWduYXR1cmU="
	after, err := events.CanonicalBytes(e)
	if err != nil {
		t.Fatalf("canonical after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("signature must not change canonical bytes:\n %s\n %s", before, after)
	}
}

// TestSignVerify_RoundTrip asserts an event signed with the private key verifies
// under the matching public key (US-7.2 AC1 foundation — per-event signature).
func TestSignVerify_RoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	e := newEvent()
	signed, err := events.Sign(e, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if signed.Signature == "" {
		t.Fatalf("signed event carries no signature")
	}
	if err := events.Verify(signed, pub); err != nil {
		t.Errorf("verify good signature: %v", err)
	}
}

// TestVerify_TamperReject asserts that mutating ANY signed field after signing
// makes verification fail (US-7.2 AC1 / §15.5 tampered payload not applied).
func TestVerify_TamperReject(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	signed, err := events.Sign(newEvent(), priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Tamper a field value after signing -> must fail.
	tampered := signed
	tampered.Fields = map[string]events.Field{
		"title":    {Value: "Hijacked", HLC: "00000000000100-0000-nodeA"},
		"priority": {Value: "p1", HLC: "00000000000100-0000-nodeA"},
	}
	if err := events.Verify(tampered, pub); err == nil {
		t.Errorf("tampered field value must fail verification")
	}

	// Tamper the origin instance -> must fail.
	tampered = signed
	tampered.OriginInstance = "https://attacker.example"
	if err := events.Verify(tampered, pub); err == nil {
		t.Errorf("tampered origin must fail verification")
	}

	// Verify under the wrong key -> must fail.
	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen2: %v", err)
	}
	if err := events.Verify(signed, otherPub); err == nil {
		t.Errorf("wrong public key must fail verification")
	}
}

// TestMaxFieldHLC returns the lexically-greatest per-field HLC carried by an
// event, the value the inbox apply advances last_received_hlc / hlc_state to.
func TestMaxFieldHLC(t *testing.T) {
	e := events.Event{
		Fields: map[string]events.Field{
			"a": {Value: "x", HLC: "00000000000100-0000-nodeA"},
			"b": {Value: "y", HLC: "00000000000300-0000-nodeA"},
			"c": {Value: "z", HLC: "00000000000200-0000-nodeA"},
		},
	}
	if got := e.MaxFieldHLC(); got != "00000000000300-0000-nodeA" {
		t.Errorf("max field HLC: got %q, want the highest", got)
	}
}

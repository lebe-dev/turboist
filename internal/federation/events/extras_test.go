package events_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
)

// newerPeerEventJSON is the wire bytes a FORWARD-COMPATIBLE peer (a future
// protocol version) emits: the v1-known fields PLUS two unknown top-level fields
// ("schema_version" and "extra_meta") that this v1 build does not model. The
// per-event signature the peer computed covers the canonical JSON of ALL of
// these fields (minus signature), so a v1 relay that drops the unknowns and
// re-serialises would produce different canonical bytes and break the signature.
const newerPeerEventJSON = `{
  "event_id": "01J0000000000000000000EVNT",
  "op": "update",
  "entity_type": "task",
  "entity_id": "task-client-1",
  "project_client_id": "proj-client-1",
  "author": "https://alice.example",
  "origin_instance": "https://alice.example",
  "created_at": "2026-06-01T10:00:00.000Z",
  "fields": {
    "title": {"value": "Renamed", "hlc": "00000000000100-0000-nodeA"}
  },
  "schema_version": 2,
  "extra_meta": {"future_flag": true, "label": "from-v2"}
}`

// TestUnmarshal_NoStrictDecode is the regression guard for the F6.1 contract:
// events MUST decode WITHOUT DisallowUnknownFields, so a forward-compatible peer's
// extra top-level fields never break decoding (US-9.1 AC3). The event decodes
// cleanly and the known fields are populated.
func TestUnmarshal_NoStrictDecode(t *testing.T) {
	var e events.Event
	if err := events.Unmarshal([]byte(newerPeerEventJSON), &e); err != nil {
		t.Fatalf("unknown top-level fields must not break decoding: %v", err)
	}
	if e.EventID != "01J0000000000000000000EVNT" {
		t.Errorf("event_id: got %q, want decoded", e.EventID)
	}
	if e.Op != events.OpUpdate {
		t.Errorf("op: got %q, want update", e.Op)
	}
	f, ok := e.Fields["title"]
	if !ok {
		t.Fatalf("known field 'title' missing after decode")
	}
	if f.Value != "Renamed" {
		t.Errorf("title value: got %v, want Renamed", f.Value)
	}
}

// TestUnmarshal_KeepsUnknownExtras asserts the unknown top-level fields are kept
// in the event's extras bag (NOT silently dropped) so they survive a relay
// re-marshal — the relay-integrity half of F6.1. The known fields are NOT
// duplicated into the bag.
func TestUnmarshal_KeepsUnknownExtras(t *testing.T) {
	var e events.Event
	if err := events.Unmarshal([]byte(newerPeerEventJSON), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(e.Extras) != 2 {
		t.Fatalf("extras bag: got %d keys, want 2 (schema_version, extra_meta); bag=%v", len(e.Extras), e.Extras)
	}
	if _, ok := e.Extras["schema_version"]; !ok {
		t.Errorf("extras bag missing 'schema_version'")
	}
	if _, ok := e.Extras["extra_meta"]; !ok {
		t.Errorf("extras bag missing 'extra_meta'")
	}
	// A KNOWN field must never leak into the extras bag (it would double-encode).
	for _, known := range []string{"event_id", "op", "entity_type", "entity_id", "fields", "signature"} {
		if _, ok := e.Extras[known]; ok {
			t.Errorf("known field %q must not appear in the extras bag", known)
		}
	}
}

// TestMarshal_RoundTripsUnknownExtras asserts a decode → re-marshal cycle
// re-emits the unknown top-level fields verbatim (relay integrity). Decoding the
// re-marshalled bytes back yields the same extras, so an owner relaying a newer
// peer's event does not strip the peer's forward-compat fields.
func TestMarshal_RoundTripsUnknownExtras(t *testing.T) {
	var e events.Event
	if err := events.Unmarshal([]byte(newerPeerEventJSON), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := events.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// The re-marshalled wire JSON carries the unknown fields at the top level.
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(out, &generic); err != nil {
		t.Fatalf("decode re-marshalled: %v", err)
	}
	if _, ok := generic["schema_version"]; !ok {
		t.Errorf("re-marshalled event dropped 'schema_version' (relay integrity broken)")
	}
	if _, ok := generic["extra_meta"]; !ok {
		t.Errorf("re-marshalled event dropped 'extra_meta' (relay integrity broken)")
	}
	// The known fields are still present and not duplicated.
	if _, ok := generic["event_id"]; !ok {
		t.Errorf("re-marshalled event lost 'event_id'")
	}

	// A second decode recovers the same extras (full round-trip).
	var e2 events.Event
	if err := events.Unmarshal(out, &e2); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if len(e2.Extras) != 2 {
		t.Errorf("extras after round-trip: got %d, want 2", len(e2.Extras))
	}
}

// TestVerify_RelayPreservesSignatureWithExtras is the end-to-end relay-integrity
// assertion: a newer peer signs an event whose canonical bytes INCLUDE unknown
// fields; a v1 relay decodes it (keeping the extras), re-marshals it (as a hub
// re-broadcast does), and the downstream verifier still validates the signature
// because the re-marshalled event reproduces the same canonical bytes. Without
// the extras bag the relay would drop the unknowns and verification would fail.
func TestVerify_RelayPreservesSignatureWithExtras(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	// A "newer peer" decodes its own richer event and signs over canonical bytes
	// that include the unknown fields.
	var peerEvent events.Event
	if err := events.Unmarshal([]byte(newerPeerEventJSON), &peerEvent); err != nil {
		t.Fatalf("peer unmarshal: %v", err)
	}
	signed, err := events.Sign(peerEvent, priv)
	if err != nil {
		t.Fatalf("peer sign: %v", err)
	}
	wire, err := events.Marshal(signed)
	if err != nil {
		t.Fatalf("peer marshal: %v", err)
	}

	// The v1 relay (owner hub) receives the wire bytes, decodes, and re-marshals
	// to re-enqueue for fan-out (the reBroadcastTx path does events.Marshal(e)).
	var relayed events.Event
	if err := events.Unmarshal(wire, &relayed); err != nil {
		t.Fatalf("relay unmarshal: %v", err)
	}
	relayedWire, err := events.Marshal(relayed)
	if err != nil {
		t.Fatalf("relay marshal: %v", err)
	}

	// The downstream peer decodes the relayed event and verifies the ORIGINAL
	// signature — it must still validate (relay integrity preserved).
	var downstream events.Event
	if err := events.Unmarshal(relayedWire, &downstream); err != nil {
		t.Fatalf("downstream unmarshal: %v", err)
	}
	if err := events.Verify(downstream, pub); err != nil {
		t.Errorf("relayed event signature must still verify (extras preserved): %v", err)
	}
}

// TestVerify_TamperedExtraReject asserts the extras bag is part of the signed
// payload: mutating an unknown field after signing breaks verification, so a
// relay cannot smuggle a forged forward-compat field past the signature.
func TestVerify_TamperedExtraReject(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	var e events.Event
	if err := events.Unmarshal([]byte(newerPeerEventJSON), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	signed, err := events.Sign(e, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// Tamper an extra value after signing -> verification must fail.
	signed.Extras["schema_version"] = json.RawMessage(`999`)
	if err := events.Verify(signed, pub); err == nil {
		t.Errorf("tampered extras field must fail verification")
	}
}

// TestCanonicalBytes_IncludesExtras asserts the canonical bytes include the
// unknown fields (sorted in among the known keys) so two instances signing the
// same forward-compat event agree, and the signature covers the extras.
func TestCanonicalBytes_IncludesExtras(t *testing.T) {
	var e events.Event
	if err := events.Unmarshal([]byte(newerPeerEventJSON), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	canon, err := events.CanonicalBytes(e)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	s := string(canon)
	for _, want := range []string{`"schema_version":2`, `"extra_meta":`, `"future_flag":true`} {
		if !contains(s, want) {
			t.Errorf("canonical bytes missing %q; got %s", want, s)
		}
	}
}

// TestTroikiOpaqueRoundTrip asserts turboist-specific fields carried as opaque
// per-field values (e.g. troiki_category) round-trip through decode/encode
// unchanged so a peer without troiki neither rejects nor mangles them (§3
// DEVIATE — opaque/local-only fields are excluded from the canonical field set
// but must survive transport when a peer DOES carry them).
func TestTroikiOpaqueRoundTrip(t *testing.T) {
	in := events.Event{
		EventID:         "01J0000000000000000000TROI",
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        "task-client-2",
		ProjectClientID: "proj-client-1",
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			// An opaque turboist field value carried verbatim (a peer that does not
			// model troiki must not corrupt it).
			"troiki_category": {Value: "morning", HLC: "00000000000100-0000-nodeA"},
			"title":           {Value: "Plan day", HLC: "00000000000100-0000-nodeA"},
		},
	}
	out, err := events.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got events.Event
	if err := events.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	f, ok := got.Fields["troiki_category"]
	if !ok {
		t.Fatalf("opaque troiki_category field dropped on round-trip")
	}
	if f.Value != "morning" {
		t.Errorf("troiki_category value: got %v, want morning", f.Value)
	}
	if f.HLC != "00000000000100-0000-nodeA" {
		t.Errorf("troiki_category hlc: got %q, want preserved", f.HLC)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

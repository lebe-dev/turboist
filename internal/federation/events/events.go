// Package events defines the federation event schema and the per-event Ed25519
// signature over the canonical event-minus-signature (Federation v1 F3.1).
//
// An Event is the unit of replication: one op (create/update/delete) on one
// federated entity, carrying ONLY the federated field set, with EACH field
// stamped by its own HLC so the receiver can resolve per-field Last-Writer-Wins
// (§5.4). The event is signed by the originating instance's trust-plane Ed25519
// key; the signature covers the canonical JSON of the event with the signature
// field itself excluded, so a peer can embed the signature into the event after
// signing and a verifier can re-derive the exact signed bytes.
//
// This per-event signature is DISTINCT from the transport request signature
// (internal/federation/transport): the transport signature authenticates the
// HTTP request, the event signature authenticates the payload end-to-end across
// an owner-hub relay. The two are intentionally kept separate (F3.2a).
package events

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
)

// Op is the kind of mutation an event carries.
type Op string

const (
	// OpCreate creates a new entity (a missing target becomes a ghost row, §10.4a).
	OpCreate Op = "create"
	// OpUpdate merges per-field changes via per-field LWW.
	OpUpdate Op = "update"
	// OpDelete soft-deletes the entity; it carries the synthetic _deleted field.
	OpDelete Op = "delete"
	// OpRevoke is the owner→joiner CONTROL event that permanently revokes the
	// joiner's access to a federated project (Federation v1 F5.4, US-6.2 AC1/AC3).
	// It is NOT a per-field LWW entity mutation: it carries no Fields, targets the
	// project (EntityType=project, EntityID = the project's client_id), and is
	// signed by the owner like any event. On receipt the joiner marks its local copy
	// federation_lost (read-only, reason=revoked), idempotently. The owner never
	// re-broadcasts it (it is point-to-point to the revoked peer).
	OpRevoke Op = "revoke"
	// OpLeave is the joiner→owner CONTROL event that announces the joiner has
	// VOLUNTARILY left a federated project (Federation v1 F5.5, US-6.3 AC1/AC2). It
	// is the symmetric counterpart of OpRevoke (owner→joiner): it carries no Fields,
	// targets the project (EntityType=project, EntityID = the OWNER's project
	// client_id so the owner can resolve it locally), and is signed by the LEAVING
	// joiner. On receipt the owner marks that peer's mapping federation_lost
	// (reason=left) and stops fanning out to it (US-6.3 AC2), idempotently. ANY
	// member may leave regardless of its write grant — a leave is not a write — so
	// the validator accepts it without the canWrite gate. The owner never
	// re-broadcasts it (it is point-to-point from the leaver to the owner).
	OpLeave Op = "leave"
)

// EntityType is the kind of federated entity an event targets. Only Project +
// Task (+ Section, and Comment/Checklist when the F0.2 schema is present)
// federate; labels/contexts are name-matched on import, not event-logged (§3).
type EntityType string

const (
	EntityProject       EntityType = "project"
	EntitySection       EntityType = "section"
	EntityTask          EntityType = "task"
	EntityComment       EntityType = "comment"
	EntityChecklistItem EntityType = "checklist_item"
)

// FieldDeleted is the synthetic field name an op=delete stamps so the tombstone
// participates in per-field LWW and a later stale update cannot resurrect the
// entity (US-3.7 AC2 / §8).
const FieldDeleted = "_deleted"

// Field is one federated field's value plus the HLC at which it was written. The
// HLC is per-field so two instances editing different fields of the same entity
// both win (US-3.3 AC1), and a stale write to one field is ignored without
// reverting the others (US-3.3 AC2).
type Field struct {
	Value any    `json:"value"`
	HLC   string `json:"hlc"`
}

// Event is one signed federation event. Author is the instance that signed/sent
// this event (in owner-hub relay the owner may forward a peer's event, but the
// SIGNATURE and OriginInstance stay the original author's — F3.2a verifies
// author == origin). OriginInstance is where the change originated. Fields maps
// federated field name → value+HLC. Signature is base64-std Ed25519 over the
// canonical event-minus-signature; it is excluded from the canonical bytes.
//
// Extras is the forward-compatibility bag (Federation v1 F6.1): any UNKNOWN
// top-level JSON field a newer-protocol peer sends is captured here on decode and
// re-emitted verbatim on encode. This is REQUIRED for relay integrity — the
// per-event signature covers the canonical JSON of the WHOLE event (including a
// future peer's extra fields), so an owner-hub relay that decoded and dropped the
// unknowns before re-broadcasting would produce different canonical bytes and
// break the downstream signature (US-9.1 AC3 / §3 "sign over received canonical
// bytes"). The v1 apply path ignores these fields (it only acts on known fields),
// but the transport MUST preserve them. Extras never holds a known key (those are
// captured by their typed fields) and never the signature.
type Event struct {
	EventID         string           `json:"event_id"`
	Op              Op               `json:"op"`
	EntityType      EntityType       `json:"entity_type"`
	EntityID        string           `json:"entity_id"`
	ProjectClientID string           `json:"project_client_id"`
	Author          string           `json:"author"`
	OriginInstance  string           `json:"origin_instance"`
	CreatedAt       string           `json:"created_at"`
	Fields          map[string]Field `json:"fields"`
	Signature       string           `json:"signature,omitempty"`

	// Extras carries unknown top-level fields a forward-compatible peer sent, so a
	// relay re-emits them verbatim (F6.1 relay integrity). It is NOT a JSON field
	// itself — MarshalJSON flattens it into the top-level object and UnmarshalJSON
	// fills it from the leftover keys; see the custom (Un)MarshalJSON below.
	Extras map[string]json.RawMessage `json:"-"`
}

// eventKnownKeys is the set of top-level JSON keys the typed Event fields own.
// Any decoded key NOT in this set is an unknown forward-compat field and is
// captured into Event.Extras (F6.1). It is the single source kept in lockstep
// with the struct tags above.
var eventKnownKeys = map[string]struct{}{
	"event_id":          {},
	"op":                {},
	"entity_type":       {},
	"entity_id":         {},
	"project_client_id": {},
	"author":            {},
	"origin_instance":   {},
	"created_at":        {},
	"fields":            {},
	"signature":         {},
}

// eventAlias mirrors Event WITHOUT the custom (Un)MarshalJSON so the standard
// encoder/decoder can handle the known fields; the custom methods on Event layer
// the Extras flattening on top. Keeping Extras off the alias avoids recursion.
type eventAlias struct {
	EventID         string           `json:"event_id"`
	Op              Op               `json:"op"`
	EntityType      EntityType       `json:"entity_type"`
	EntityID        string           `json:"entity_id"`
	ProjectClientID string           `json:"project_client_id"`
	Author          string           `json:"author"`
	OriginInstance  string           `json:"origin_instance"`
	CreatedAt       string           `json:"created_at"`
	Fields          map[string]Field `json:"fields"`
	Signature       string           `json:"signature,omitempty"`
}

// MarshalJSON encodes the event's known fields AND its forward-compat Extras into
// one flat top-level JSON object (F6.1 relay integrity). A known field always
// wins over a same-named extra (Extras should never hold a known key, but the
// guard keeps the encoding unambiguous). The result is deterministic only after
// CanonicalJSON re-sorts it; plain Marshal preserves the unknown keys so a relay
// re-emits them verbatim.
func (e Event) MarshalJSON() ([]byte, error) {
	base, err := json.Marshal(eventAlias(e.toAlias()))
	if err != nil {
		return nil, fmt.Errorf("marshal event base: %w", err)
	}
	if len(e.Extras) == 0 {
		return base, nil
	}

	// Decode the known-field object back into an ordered-agnostic map, layer the
	// extras under it (known keys win), and re-encode. SetEscapeHTML(false) keeps
	// &,<,> verbatim so the bytes round-trip consistently with the canonicaliser.
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, fmt.Errorf("merge event base: %w", err)
	}
	for k, v := range e.Extras {
		if _, known := eventKnownKeys[k]; known {
			continue // a known key is owned by its typed field — never overwrite.
		}
		merged[k] = v
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(merged); err != nil {
		return nil, fmt.Errorf("marshal event extras: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// UnmarshalJSON decodes the known fields into their typed slots and captures
// every UNKNOWN top-level key into Extras (F6.1). It does NOT use
// DisallowUnknownFields — that is the whole point: a forward-compatible peer's
// extra fields must decode cleanly and survive a relay re-marshal (US-9.1 AC3).
func (e *Event) UnmarshalJSON(b []byte) error {
	var alias eventAlias
	if err := json.Unmarshal(b, &alias); err != nil {
		return err
	}
	*e = alias.toEvent()

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	var extras map[string]json.RawMessage
	for k, v := range raw {
		if _, known := eventKnownKeys[k]; known {
			continue
		}
		if extras == nil {
			extras = make(map[string]json.RawMessage)
		}
		extras[k] = v
	}
	e.Extras = extras
	return nil
}

// toAlias projects the typed Event fields onto the recursion-free alias used by
// MarshalJSON (Extras is layered separately).
func (e Event) toAlias() eventAlias {
	return eventAlias{
		EventID:         e.EventID,
		Op:              e.Op,
		EntityType:      e.EntityType,
		EntityID:        e.EntityID,
		ProjectClientID: e.ProjectClientID,
		Author:          e.Author,
		OriginInstance:  e.OriginInstance,
		CreatedAt:       e.CreatedAt,
		Fields:          e.Fields,
		Signature:       e.Signature,
	}
}

// toEvent lifts the decoded alias back into an Event (Extras filled by the caller).
func (a eventAlias) toEvent() Event {
	return Event{
		EventID:         a.EventID,
		Op:              a.Op,
		EntityType:      a.EntityType,
		EntityID:        a.EntityID,
		ProjectClientID: a.ProjectClientID,
		Author:          a.Author,
		OriginInstance:  a.OriginInstance,
		CreatedAt:       a.CreatedAt,
		Fields:          a.Fields,
		Signature:       a.Signature,
	}
}

// CanonicalBytes returns the deterministic canonical JSON of the event with the
// signature field excluded — the exact bytes Sign signs and Verify verifies.
// Two instances computing this over the same logical event MUST agree (sorted
// keys, no whitespace, HTML escaping disabled — crypto.CanonicalJSON).
func CanonicalBytes(e Event) ([]byte, error) {
	unsigned := e
	unsigned.Signature = ""
	b, err := crypto.CanonicalJSON(unsigned)
	if err != nil {
		return nil, fmt.Errorf("canonical event: %w", err)
	}
	return b, nil
}

// Sign returns a copy of e with Signature set to the base64-std Ed25519
// signature over its canonical bytes (sign over event-minus-signature).
func Sign(e Event, priv ed25519.PrivateKey) (Event, error) {
	canon, err := CanonicalBytes(e)
	if err != nil {
		return Event{}, err
	}
	sig := ed25519.Sign(priv, canon)
	e.Signature = base64.StdEncoding.EncodeToString(sig)
	return e, nil
}

// Verify checks the per-event Ed25519 signature against pub over the event's
// canonical bytes. A missing/malformed signature or any tampered field fails.
func Verify(e Event, pub ed25519.PublicKey) error {
	if e.Signature == "" {
		return fmt.Errorf("event %s: missing signature", e.EventID)
	}
	sig, err := base64.StdEncoding.DecodeString(e.Signature)
	if err != nil {
		return fmt.Errorf("event %s: decode signature: %w", e.EventID, err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("event %s: signature size", e.EventID)
	}
	canon, err := CanonicalBytes(e)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, canon, sig) {
		return fmt.Errorf("event %s: signature does not verify", e.EventID)
	}
	return nil
}

// MaxFieldHLC returns the lexically-greatest per-field HLC carried by the event,
// or the empty string when it carries no fields. This is the HLC the receiver
// merges into its clock (hlc.Recv) and advances last_received_hlc to.
func (e Event) MaxFieldHLC() string {
	var max string
	for _, f := range e.Fields {
		if hlc.CompareString(f.HLC, max) > 0 {
			max = f.HLC
		}
	}
	return max
}

// Marshal encodes the event (including its signature) to wire JSON — used to
// store the canonical signed event in the outbox payload and to POST it.
func Marshal(e Event) ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}
	return b, nil
}

// Unmarshal decodes a wire event into out. It intentionally does NOT use
// DisallowUnknownFields so a forward-compatible peer's extra fields do not break
// decoding (the strict forward-compat extras bag is refined in F6.1).
func Unmarshal(b []byte, out *Event) error {
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}
	return nil
}

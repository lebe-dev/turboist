package inbox

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/model"
)

// Per-event payload validation (Federation v1 F3.2a, US-7.2/US-7.3).
//
// This is the SECOND, distinct signature plane. The transport request signature
// (internal/federation/transport, F0.3) authenticates the HTTP request itself —
// who is talking to this instance. The per-event payload validation here
// authenticates each EVENT end-to-end across an owner-hub relay: it proves the
// originating instance signed the event, that the claimed author is the origin,
// that the event's clock is sane, and that the sending peer is a write member of
// the targeted project. The two checks are intentionally kept separate (F3.2a):
// a transport-valid request can still carry a forged or stale event payload, and
// in owner-hub relay the request signer (the owner) is NOT the event author.
//
// EVERY check runs BEFORE any federation_inbox or domain write. The F3.2
// POST /federation/events handler calls Validate first and only enqueues the
// event to the inbox-apply path when it returns no error, so a rejected event
// leaves zero inbox/domain rows (US-7.2 AC1).

// Clock-skew bounds (US-7.2 AC4). The thresholds are intentionally ASYMMETRIC:
// a future event is suspicious (a peer's clock should not be ahead of ours by
// more than a few minutes), while a past event is routinely legitimate (a peer
// that was briefly offline replays older events), so the past window is far wider.
const (
	maxClockSkewFuture = 10 * time.Minute
	maxClockSkewPast   = 1 * time.Hour
)

// Per-event payload rejection reasons. They are sentinels so the F3.2 handler can
// map each to its HTTP status without string matching:
//
//	ErrEventSignatureInvalid → 401 (US-7.2 AC1)
//	ErrAuthorOriginMismatch  → 400 (US-7.2 AC3)
//	ErrEventClockSkew        → 400 (US-7.2 AC4)
//	ErrNotMember             → 403 (sender is not a peer of this project)
//	ErrPeerNotPermitted      → 403 (peer is revoked or read-only)
var (
	// ErrEventSignatureInvalid is a per-event Ed25519 verification failure against
	// a key that WAS resolved — a forged, tampered, or unsigned event, OR a genuine
	// author key rotation (the event no longer verifies against the published key).
	// DISTINCT from the transport request-signature failure. This is the ONLY
	// signature sentinel that proves the peer's key changed, so it is the only one
	// the F4.3 inbox-signature-check writer acts on to stamp the sticky key_mismatch
	// marker (US-4.3 AC4).
	ErrEventSignatureInvalid = errors.New("inbox: per-event signature invalid")
	// ErrEventKeyUnresolved is returned when the AUTHOR's public key could not be
	// resolved at all — a transient .well-known fetch error (5xx / timeout / DNS).
	// It is DISTINCT from ErrEventSignatureInvalid: the event was neither verified
	// nor disproven, so it is "could not verify" rather than "verified-and-rejected".
	// The handler maps it to a retryable 503 and MUST NOT stamp the sticky
	// key_mismatch marker — a brief network blip must never turn the badge
	// permanently red (Federation v1 F4.3 review fix).
	ErrEventKeyUnresolved = errors.New("inbox: per-event author key could not be resolved")
	// ErrAuthorOriginMismatch is returned when author != origin_instance (US-7.2
	// AC3): the signing author must be the origin of the change, never a relay.
	ErrAuthorOriginMismatch = errors.New("inbox: event author does not match origin instance")
	// ErrEventClockSkew is returned when the event's HLC physical clock is too far
	// in the future (>10min) or the past (>1h), or is unparseable (US-7.2 AC4).
	ErrEventClockSkew = errors.New("inbox: event clock skew out of bounds")
	// ErrNotMember is returned when the (project, peer) has no federation mapping —
	// the sender is not a peer of this project.
	ErrNotMember = errors.New("inbox: sender is not a member of the federated project")
	// ErrPeerNotPermitted is returned when the peer holds only read permission and
	// therefore may not push writes.
	ErrPeerNotPermitted = errors.New("inbox: peer is not permitted to write")
	// ErrPeerRevoked is returned when the owner has PERMANENTLY revoked this peer
	// (Federation v1 F5.4, US-6.2 AC2/AC4). It is DISTINCT from ErrPeerNotPermitted
	// (read-only) and ErrPeerPaused (reversible): a revoke is terminal and
	// irreversible. It is checked BEFORE membership-permission and pause so a
	// revoked peer is rejected on the highest-precedence terminal state, and the
	// handler maps it to a 403 federation_revoked (also covering offline-return,
	// US-6.2 AC4 — the peer self-marks federation_lost on the 403).
	ErrPeerRevoked = errors.New("inbox: peer access has been revoked")
	// ErrPeerPaused is returned when the owner has temporarily paused exchange with
	// this peer (Federation v1 F5.3, US-6.1 AC1). The event is rejected and not
	// applied, but — unlike ErrPeerNotPermitted (revoked/read-only) — the link is
	// still trusted: resuming the peer flushes its accumulated events. The handler
	// maps it to a 403 federation_paused so the pause is reported as a distinct,
	// reversible state.
	ErrPeerPaused = errors.New("inbox: exchange with peer is paused")
)

// KeyResolver resolves the Ed25519 public key the event AUTHOR (origin instance)
// signs with. In production this is backed by the peer-key cache
// (peerkeys.Cache.Resolve); injected here so the validator is unit-testable and
// holds no DB connection across the resolve (R1). A resolve failure (transient
// .well-known fetch error) is surfaced as ErrEventKeyUnresolved — distinct from a
// signature mismatch — so the caller can treat it as "could not verify" (retry)
// rather than "verified-and-rejected" (a real key rotation). See verifySignature.
type KeyResolver func(ctx context.Context, instanceURL string) (ed25519.PublicKey, error)

// MembershipLookup resolves the federation mapping for the (project, peer) the
// event arrived on. It returns ErrNotMember when no such row exists. projectClientID
// is the event's project_client_id; peerURL is the transport-verified caller.
type MembershipLookup func(ctx context.Context, projectClientID, peerURL string) (*model.FederatedProject, error)

// Validator runs the per-event payload checks. It is stateless beyond its
// injected collaborators and safe for concurrent use.
type Validator struct {
	resolveKey KeyResolver
	membership MembershipLookup
	now        func() time.Time
}

// NewValidator constructs the per-event payload validator. now may be nil
// (defaults to time.Now); resolveKey and membership are required.
func NewValidator(resolveKey KeyResolver, membership MembershipLookup, now func() time.Time) *Validator {
	if now == nil {
		now = time.Now
	}
	return &Validator{resolveKey: resolveKey, membership: membership, now: now}
}

// ValidationResult carries the resolved federation mapping so the caller can pass
// the local project id straight through to Apply without re-resolving it.
type ValidationResult struct {
	LocalProjectID int64
	Permissions    model.FederationPermission
}

// Validate runs the full per-event payload validation for an event received from
// peerURL (the transport-verified caller). It runs the checks in a deliberate
// order — signature, then author/origin, then clock-skew, then membership — so
// the cheapest authenticity check rejects a forged event before any trust lookup,
// and returns BEFORE the caller performs any inbox/domain write. On success it
// returns the resolved local project mapping.
func (v *Validator) Validate(ctx context.Context, e events.Event, peerURL string) (*ValidationResult, error) {
	// 1. Per-event Ed25519 signature (US-7.2 AC1). Resolve the AUTHOR's key (not
	// the transport caller's): in owner-hub relay the owner forwards a peer's
	// event keeping the original author + signature, so the event is verified
	// against the author's key end-to-end.
	if err := v.verifySignature(ctx, e); err != nil {
		return nil, err
	}

	// 2. author == origin_instance (US-7.2 AC3). A relay may carry the event but
	// must never rewrite its author/origin; both must be present and equal.
	if e.Author == "" || e.OriginInstance == "" || e.Author != e.OriginInstance {
		return nil, fmt.Errorf("%w: author=%q origin=%q", ErrAuthorOriginMismatch, e.Author, e.OriginInstance)
	}

	// 3. HLC clock-skew over the event's clock (US-7.2 AC4), asymmetric bounds.
	if err := v.checkClockSkew(e); err != nil {
		return nil, err
	}

	// 4. Membership + permission. The sending peer must be a non-revoked write/
	// admin member of the targeted project.
	return v.checkMembership(ctx, e, peerURL)
}

// verifySignature resolves the author's public key and verifies the per-event
// signature over the canonical event-minus-signature. It deliberately splits two
// failure planes (Federation v1 F4.3 review fix):
//
//   - A missing signature/author, or a signature that does NOT verify against a
//     key that WAS resolved, is ErrEventSignatureInvalid — a verified-and-rejected
//     event (forged/tampered/rotated key). The handler maps it to 401 AND acts on
//     it as proof the peer's key changed (stamps the sticky key_mismatch marker).
//   - A failure to RESOLVE the author key at all (transient .well-known fetch
//     error) is ErrEventKeyUnresolved — "could not verify". The handler maps it to
//     a retryable 503 and MUST NOT stamp the sticky marker, so a brief network blip
//     never turns the badge permanently red.
//
// Both return BEFORE any inbox/domain write, so a rejected event leaves zero rows
// either way; a probing peer still learns nothing beyond the status class.
func (v *Validator) verifySignature(ctx context.Context, e events.Event) error {
	if e.Signature == "" {
		return fmt.Errorf("%w: missing signature on event %s", ErrEventSignatureInvalid, e.EventID)
	}
	if e.Author == "" {
		return fmt.Errorf("%w: missing author on event %s", ErrEventSignatureInvalid, e.EventID)
	}
	pub, err := v.resolveKey(ctx, e.Author)
	if err != nil {
		// Transient: the key could not be fetched, so the event was neither verified
		// nor disproven. This is NOT evidence of a key rotation — do not let the
		// handler stamp the sticky marker on it.
		return fmt.Errorf("%w: resolve author key for %q: %v", ErrEventKeyUnresolved, e.Author, err)
	}
	if err := events.Verify(e, pub); err != nil {
		return fmt.Errorf("%w: %v", ErrEventSignatureInvalid, err)
	}
	return nil
}

// checkClockSkew rejects an event whose HLC physical clock is too far from this
// instance's wall clock. It skew-checks (Parse + asymmetric-window bounds) EVERY
// field's HLC, not just the lexically-greatest one. The max-HLC alone is NOT
// enough: a malformed HLC that sorts BELOW a valid field's HLC would never be the
// max and would slip past a max-only check, then be applied verbatim by the store
// CAS (casFieldHLC does a raw string compare and does not validate format) and
// persisted as that field's authoritative HLC — permanently poisoning per-field
// LWW for it. This validator is the single gate for HLC well-formedness, so it
// must look at all fields. A malformed (unparseable) field HLC is rejected as a
// skew failure rather than silently accepted; a field carrying no HLC at all
// (empty string) is skipped (the downstream apply ignores it).
func (v *Validator) checkClockSkew(e events.Event) error {
	now := v.now()
	for name, field := range e.Fields {
		if field.HLC == "" {
			// This field carries no HLC; nothing to skew-check. The downstream apply
			// ignores it (a lexical CAS against an empty string is a no-op), so there
			// is nothing to reject here.
			continue
		}
		parsed, err := hlc.Parse(field.HLC)
		if err != nil {
			return fmt.Errorf("%w: unparseable hlc %q on field %q of event %s", ErrEventClockSkew, field.HLC, name, e.EventID)
		}
		eventTime := time.UnixMilli(parsed.PhysicalMS)
		if eventTime.After(now.Add(maxClockSkewFuture)) {
			return fmt.Errorf("%w: field %q of event %s is %s in the future (>%s)", ErrEventClockSkew, name, e.EventID, eventTime.Sub(now), maxClockSkewFuture)
		}
		if eventTime.Before(now.Add(-maxClockSkewPast)) {
			return fmt.Errorf("%w: field %q of event %s is %s in the past (>%s)", ErrEventClockSkew, name, e.EventID, now.Sub(eventTime), maxClockSkewPast)
		}
	}
	return nil
}

// checkMembership resolves the (project, peer) federation mapping and enforces
// the DIRECTION-AWARE incoming permission (Federation v1 F5.1, US-5.1/US-5.2). A
// missing mapping is ErrNotMember; a revoked peer is always ErrPeerRevoked (the
// terminal, irreversible state, Federation v1 F5.4, US-6.2 AC2 — checked first);
// a paused peer is always ErrPeerPaused (Federation v1 F5.3, US-6.1 AC1 — a
// non-destructive, resumable reject distinct from revoke).
//
// The permission gate depends on WHO is sending to WHOM (owner-hub, W-7):
//
//   - OWNER relaying to a JOINED read peer (this receiver's row is is_owner=0 AND
//     the transport peer IS the project's origin owner): ACCEPTED regardless of
//     this instance's own granted permission. A READ peer still RECEIVES fan-out
//     (US-5.1 AC3) — its read grant only constrains its OWN local edits (the F5.2
//     local FederationGuard), never what the authoritative owner pushes to it.
//   - PEER pushing to the OWNER (any other shape — the transport peer is NOT the
//     origin owner, i.e. this instance owns the project and a joined peer is
//     pushing): the SENDING peer's permission is enforced. A read peer that tries
//     to originate a write to the owner is rejected (US-5.1 AC2, ErrPeerNotPermitted)
//     and the event is not applied — only write/admin peers may push.
func (v *Validator) checkMembership(ctx context.Context, e events.Event, peerURL string) (*ValidationResult, error) {
	fp, err := v.membership(ctx, e.ProjectClientID, peerURL)
	if errors.Is(err, ErrNotMember) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("inbox membership lookup for %q/%q: %w", e.ProjectClientID, peerURL, err)
	}
	if fp == nil {
		return nil, ErrNotMember
	}
	// Revoked (Federation v1 F5.4, US-6.2 AC2): the owner permanently revoked this
	// peer. It is the TERMINAL, highest-precedence state — checked before pause and
	// permission so a revoked peer is rejected with the distinct, irreversible 403
	// federation_revoked (also covers the offline-return self-detect, US-6.2 AC4).
	if fp.Revoked {
		return nil, fmt.Errorf("%w: peer %q is revoked", ErrPeerRevoked, peerURL)
	}
	// Paused (Federation v1 F5.3, US-6.1 AC1): the owner has temporarily paused
	// exchange with this peer. Reject the inbound event — but DISTINCT from revoked,
	// so the link stays trusted and a resume flushes the accumulated events. Checked
	// AFTER revoked (the terminal, higher-precedence state) so a revoked+paused peer
	// is reported as revoked, mirroring the peers-list status precedence.
	if fp.Paused {
		return nil, fmt.Errorf("%w: peer %q is paused", ErrPeerPaused, peerURL)
	}
	// Leave control event (Federation v1 F5.5, US-6.3 AC1/AC2): a federation_leave
	// announces the SENDING peer has voluntarily left. ANY member may leave
	// regardless of its write grant — a leave is not a data write — so it is
	// accepted past the canWrite gate below. It is checked AFTER revoked/paused so a
	// revoked peer's leave is still rejected as revoked (leave-after-revoke is a
	// no-op) and a paused peer's leave is rejected paused; both higher-precedence
	// terminal/reversible states win.
	if e.Op == events.OpLeave {
		return &ValidationResult{LocalProjectID: fp.LocalProjectID, Permissions: fp.Permissions}, nil
	}
	// Owner-relay leg: this receiver is a joiner (is_owner=0) and the transport peer
	// is the project's origin owner. The owner is the authoritative hub; accept its
	// relayed fan-out regardless of this instance's own read/write grant (US-5.1 AC3).
	if !fp.IsOwner && fp.OriginInstanceURL == peerURL {
		return &ValidationResult{LocalProjectID: fp.LocalProjectID, Permissions: fp.Permissions}, nil
	}
	// Peer-to-owner leg: enforce the sending peer's permission. A read peer may not
	// originate writes to the owner (US-5.1 AC2).
	if !canWrite(fp.Permissions) {
		return nil, fmt.Errorf("%w: peer %q has %q permission", ErrPeerNotPermitted, peerURL, fp.Permissions)
	}
	return &ValidationResult{LocalProjectID: fp.LocalProjectID, Permissions: fp.Permissions}, nil
}

// canWrite reports whether a permission grade may push writes. Read peers receive
// fan-out but may not originate writes (US-5.1); only write/admin may.
func canWrite(p model.FederationPermission) bool {
	switch p {
	case model.FederationPermissionWrite, model.FederationPermissionAdmin:
		return true
	default:
		return false
	}
}

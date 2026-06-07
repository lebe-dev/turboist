package inbox_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/model"
)

// validateFixture is a self-contained per-event validator harness: a keypair the
// author signs with, an injectable membership row, and an injectable clock. It
// exercises the F3.2a per-event payload checks WITHOUT a DB so each rejection
// reason can be asserted in isolation (the end-to-end "zero rows" assertion that
// proves the validator runs BEFORE any write lives in validate_apply_test.go).
type validateFixture struct {
	pub        ed25519.PublicKey
	priv       ed25519.PrivateKey
	authorURL  string
	now        time.Time
	membership *model.FederatedProject
	memberErr  error
	resolveErr error
}

func newValidateFixture(t *testing.T) *validateFixture {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	const author = "https://alice.example"
	return &validateFixture{
		pub:       pub,
		priv:      priv,
		authorURL: author,
		now:       time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		membership: &model.FederatedProject{
			LocalProjectID:    7,
			PeerInstanceURL:   author,
			OriginInstanceURL: author,
			Permissions:       model.FederationPermissionWrite,
			ProtocolVersion:   1,
		},
	}
}

// validator wires the fixture's injectable resolver/membership/clock into a real
// inbox.Validator.
func (f *validateFixture) validator() *inbox.Validator {
	resolve := func(_ context.Context, instanceURL string) (ed25519.PublicKey, error) {
		if f.resolveErr != nil {
			return nil, f.resolveErr
		}
		if instanceURL != f.authorURL {
			return nil, errors.New("unknown instance")
		}
		return f.pub, nil
	}
	member := func(_ context.Context, _ /*projectClientID*/, _ /*peerURL*/ string) (*model.FederatedProject, error) {
		if f.memberErr != nil {
			return nil, f.memberErr
		}
		return f.membership, nil
	}
	return inbox.NewValidator(resolve, member, func() time.Time { return f.now })
}

// hlcAt renders a canonical HLC string at the given wall time (logical 0).
func hlcAt(t time.Time) string {
	return hlc.HLC{PhysicalMS: t.UnixMilli(), Logical: 0, NodeID: "nodeA"}.String()
}

// signedEvent builds a valid event signed by the fixture's author key, with all
// field HLCs stamped at the fixture's current clock so it is in-window by default.
func (f *validateFixture) signedEvent(t *testing.T) events.Event {
	t.Helper()
	e := events.Event{
		EventID:         "01J0000000000000000000EVNT",
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        "task-client-1",
		ProjectClientID: "proj-client-1",
		Author:          f.authorURL,
		OriginInstance:  f.authorURL,
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"title": {Value: "Renamed", HLC: hlcAt(f.now)},
		},
	}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign event: %v", err)
	}
	return signed
}

// TestValidate_Accepts asserts a well-formed, in-window, correctly-signed event
// from a write peer passes every per-event payload check.
func TestValidate_Accepts(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	res, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if err != nil {
		t.Fatalf("valid event must pass: %v", err)
	}
	if res == nil || res.LocalProjectID != 7 {
		t.Errorf("validate result must carry the resolved local project id, got %+v", res)
	}
}

// TestValidate_FieldlessCreateRejected asserts a signed, in-membership op=create
// carrying NO field HLC is rejected with ErrEventNoFields BEFORE any write (it
// would otherwise pass the skew gate vacuously and materialise an empty ghost row).
func TestValidate_FieldlessCreateRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := events.Event{
		EventID:         "01J0000000000000000000EVNT",
		Op:              events.OpCreate,
		EntityType:      events.EntityTask,
		EntityID:        "task-client-1",
		ProjectClientID: "proj-client-1",
		Author:          f.authorURL,
		OriginInstance:  f.authorURL,
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          map[string]events.Field{},
	}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign event: %v", err)
	}
	if _, err := f.validator().Validate(context.Background(), signed, f.authorURL); !errors.Is(err, inbox.ErrEventNoFields) {
		t.Errorf("field-less create must be rejected with ErrEventNoFields, got %v", err)
	}
}

// TestValidate_FieldlessUpdateRejected asserts the same guard for op=update: a
// field whose HLC is empty carries no LWW information, so an update made of only
// such fields is field-less and rejected.
func TestValidate_FieldlessUpdateRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := events.Event{
		EventID:         "01J0000000000000000000EVNT",
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        "task-client-1",
		ProjectClientID: "proj-client-1",
		Author:          f.authorURL,
		OriginInstance:  f.authorURL,
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          map[string]events.Field{"title": {Value: "x", HLC: ""}},
	}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign event: %v", err)
	}
	if _, err := f.validator().Validate(context.Background(), signed, f.authorURL); !errors.Is(err, inbox.ErrEventNoFields) {
		t.Errorf("field-less update must be rejected with ErrEventNoFields, got %v", err)
	}
}

// TestValidate_DeleteWithoutFieldsAccepted asserts the field-less guard is scoped
// to create/update: an op=delete legitimately carries no editable field HLC (the
// tombstone HLC is derived in apply) and must still pass validation.
func TestValidate_DeleteWithoutFieldsAccepted(t *testing.T) {
	f := newValidateFixture(t)
	e := events.Event{
		EventID:         "01J0000000000000000000EVNT",
		Op:              events.OpDelete,
		EntityType:      events.EntityTask,
		EntityID:        "task-client-1",
		ProjectClientID: "proj-client-1",
		Author:          f.authorURL,
		OriginInstance:  f.authorURL,
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          map[string]events.Field{events.FieldDeleted: {HLC: hlcAt(f.now)}},
	}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign event: %v", err)
	}
	if _, err := f.validator().Validate(context.Background(), signed, f.authorURL); err != nil {
		t.Errorf("op=delete must pass the field-less guard, got %v", err)
	}
}

// TestValidate_EventSignatureFail_Rejected asserts a wrong-key signature is a
// per-event signature failure mapped to 401 (US-7.2 AC1). The per-event check is
// DISTINCT from the transport request signature.
func TestValidate_EventSignatureFail_Rejected(t *testing.T) {
	f := newValidateFixture(t)
	// Sign with a foreign key, but the resolver returns the fixture's (different)
	// public key — so verification fails.
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate other keypair: %v", err)
	}
	e := f.signedEvent(t)
	tampered, err := events.Sign(e, otherPriv)
	if err != nil {
		t.Fatalf("re-sign: %v", err)
	}
	_, err = f.validator().Validate(context.Background(), tampered, f.authorURL)
	if !errors.Is(err, inbox.ErrEventSignatureInvalid) {
		t.Fatalf("wrong-key signature must be ErrEventSignatureInvalid, got %v", err)
	}
}

// TestValidate_TamperedPayloadRejected asserts mutating any signed field after
// signing fails the per-event signature (§15.5 tampered payload not applied).
func TestValidate_TamperedPayloadRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	// Flip a field value WITHOUT re-signing: the canonical bytes no longer match.
	e.Fields["title"] = events.Field{Value: "Hijacked", HLC: hlcAt(f.now)}
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrEventSignatureInvalid) {
		t.Fatalf("tampered payload must fail signature, got %v", err)
	}
}

// TestValidate_MissingSignatureRejected asserts an event with no signature is a
// signature failure (a peer cannot skip the per-event signature).
func TestValidate_MissingSignatureRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	e.Signature = ""
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrEventSignatureInvalid) {
		t.Fatalf("missing signature must fail, got %v", err)
	}
}

// TestValidate_AuthorOriginMismatch asserts author.instance_url != origin_instance
// is rejected with ErrAuthorOriginMismatch (US-7.2 AC3 → 400).
func TestValidate_AuthorOriginMismatch(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	// Author claims to be the verified peer but the origin is a third party: the
	// event must be re-signed so the signature itself is valid and only the
	// author/origin equality check fails.
	e.OriginInstance = "https://eve.example"
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	_, err = f.validator().Validate(context.Background(), signed, f.authorURL)
	if !errors.Is(err, inbox.ErrAuthorOriginMismatch) {
		t.Fatalf("author/origin mismatch must be ErrAuthorOriginMismatch, got %v", err)
	}
}

// TestValidate_EmptyAuthorRejected asserts an event with an empty author/origin
// is rejected (it cannot equal a real origin and has no resolvable key).
func TestValidate_EmptyAuthorRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	e.Author = ""
	e.OriginInstance = ""
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	_, err = f.validator().Validate(context.Background(), signed, f.authorURL)
	if err == nil {
		t.Fatal("empty author/origin must be rejected")
	}
}

// TestValidate_ClockSkewFutureRejected asserts an HLC more than 10 minutes in the
// future is rejected (US-7.2 AC4 — asymmetric future bound).
func TestValidate_ClockSkewFutureRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	e.Fields["title"] = events.Field{Value: "Future", HLC: hlcAt(f.now.Add(11 * time.Minute))}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	_, err = f.validator().Validate(context.Background(), signed, f.authorURL)
	if !errors.Is(err, inbox.ErrEventClockSkew) {
		t.Fatalf(">10min future HLC must be ErrEventClockSkew, got %v", err)
	}
}

// TestValidate_ClockSkewFutureWithinBound asserts an HLC inside the +10min future
// window is accepted (boundary case just under the limit).
func TestValidate_ClockSkewFutureWithinBound(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	e.Fields["title"] = events.Field{Value: "NearFuture", HLC: hlcAt(f.now.Add(9 * time.Minute))}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := f.validator().Validate(context.Background(), signed, f.authorURL); err != nil {
		t.Fatalf("9min future HLC must be accepted: %v", err)
	}
}

// TestValidate_ClockSkewPastRejected asserts an HLC more than 1 hour in the past
// is rejected (US-7.2 AC4 — asymmetric past bound, larger than the future bound).
func TestValidate_ClockSkewPastRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	e.Fields["title"] = events.Field{Value: "Ancient", HLC: hlcAt(f.now.Add(-61 * time.Minute))}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	_, err = f.validator().Validate(context.Background(), signed, f.authorURL)
	if !errors.Is(err, inbox.ErrEventClockSkew) {
		t.Fatalf(">1h past HLC must be ErrEventClockSkew, got %v", err)
	}
}

// TestValidate_ClockSkewPastWithinBound asserts an HLC inside the -1h past window
// is accepted — proving the past bound is wider than the future bound (a 30min
// past event, which would be far outside the +10min future window, still passes).
func TestValidate_ClockSkewPastWithinBound(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	e.Fields["title"] = events.Field{Value: "RecentPast", HLC: hlcAt(f.now.Add(-30 * time.Minute))}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := f.validator().Validate(context.Background(), signed, f.authorURL); err != nil {
		t.Fatalf("30min past HLC must be accepted (past bound is 1h): %v", err)
	}
}

// TestValidate_MalformedHLCRejected asserts an unparseable field HLC is rejected
// as a skew/clock failure rather than silently accepted.
func TestValidate_MalformedHLCRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	e.Fields["title"] = events.Field{Value: "Bad", HLC: "not-an-hlc"}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	_, err = f.validator().Validate(context.Background(), signed, f.authorURL)
	if !errors.Is(err, inbox.ErrEventClockSkew) {
		t.Fatalf("malformed HLC must be rejected as clock skew, got %v", err)
	}
}

// TestValidate_MalformedHLCBelowMaxRejected is the regression for the F3.2a
// review finding: checkClockSkew must skew-check EVERY field's HLC, not just the
// lexically-greatest one. Here a garbage HLC ("!") on one field sorts BELOW a
// valid zero-padded HLC on another field, so a max-only check would never parse
// it and the malformed HLC would slip through to the store CAS (which does a raw
// string compare and would persist it verbatim, poisoning per-field LWW). The
// event must be rejected as a clock-skew failure.
func TestValidate_MalformedHLCBelowMaxRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	// "!" (0x21) sorts below the digit '0' (0x30) that begins every zero-padded
	// HLC, so the valid "title" HLC is MaxFieldHLC and the garbage "description"
	// HLC is hidden behind it under a max-only check.
	e.Fields["title"] = events.Field{Value: "Valid", HLC: hlcAt(f.now)}
	e.Fields["description"] = events.Field{Value: "Garbage", HLC: "!"}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if hlc.CompareString(e.Fields["title"].HLC, e.Fields["description"].HLC) <= 0 {
		t.Fatalf("test setup: the garbage HLC must sort below the valid one to exercise the gap")
	}
	_, err = f.validator().Validate(context.Background(), signed, f.authorURL)
	if !errors.Is(err, inbox.ErrEventClockSkew) {
		t.Fatalf("malformed HLC below the max field HLC must still be rejected as clock skew, got %v", err)
	}
}

// TestValidate_PastSkewBelowMaxRejected asserts a well-formed but >1h-past field
// HLC is rejected even when another field carries a recent (in-window) HLC that
// is the lexical max. The far-past HLC has a smaller physical_ms, so it sorts
// below the recent one and a max-only skew check would never inspect it.
func TestValidate_PastSkewBelowMaxRejected(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	e.Fields["title"] = events.Field{Value: "Recent", HLC: hlcAt(f.now)}
	e.Fields["description"] = events.Field{Value: "Ancient", HLC: hlcAt(f.now.Add(-61 * time.Minute))}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if hlc.CompareString(e.Fields["title"].HLC, e.Fields["description"].HLC) <= 0 {
		t.Fatalf("test setup: the ancient HLC must sort below the recent one to exercise the gap")
	}
	_, err = f.validator().Validate(context.Background(), signed, f.authorURL)
	if !errors.Is(err, inbox.ErrEventClockSkew) {
		t.Fatalf(">1h-past field HLC below the max must still be rejected as clock skew, got %v", err)
	}
}

// TestValidate_DeleteUsesSyntheticHLC asserts an op=delete carrying only the
// synthetic _deleted field is skew-checked against that field's HLC.
func TestValidate_DeleteUsesSyntheticHLC(t *testing.T) {
	f := newValidateFixture(t)
	e := events.Event{
		EventID:         "01J0000000000000000000DEL",
		Op:              events.OpDelete,
		EntityType:      events.EntityTask,
		EntityID:        "task-client-1",
		ProjectClientID: "proj-client-1",
		Author:          f.authorURL,
		OriginInstance:  f.authorURL,
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			events.FieldDeleted: {Value: true, HLC: hlcAt(f.now)},
		},
	}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := f.validator().Validate(context.Background(), signed, f.authorURL); err != nil {
		t.Fatalf("in-window delete must pass: %v", err)
	}
}

// TestValidate_MembershipMissing asserts an event for a (project, peer) with no
// federation mapping is rejected as untrusted (403).
func TestValidate_MembershipMissing(t *testing.T) {
	f := newValidateFixture(t)
	f.memberErr = inbox.ErrNotMember
	e := f.signedEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrNotMember) {
		t.Fatalf("missing membership must be ErrNotMember, got %v", err)
	}
}

// TestValidate_ReadOnlyPeerRejected asserts a read-only peer's write event PUSHED
// TO THE OWNER is rejected (US-5.1 AC2; a read peer cannot originate writes). This
// is the peer→owner leg: the receiver owns the project (OriginInstanceURL is the
// owner's own URL, distinct from the read peer's transport URL), so the sending
// peer's permission is enforced and a read grant is rejected (F5.1 direction-aware
// membership; F3.2a enforces it alongside membership).
func TestValidate_ReadOnlyPeerRejected(t *testing.T) {
	f := newValidateFixture(t)
	f.membership.Permissions = model.FederationPermissionRead
	// Owner-receiving-from-read-peer topology: this instance owns the project (the
	// origin is the owner's URL), the read peer is the transport sender — NOT the
	// origin owner — so the owner-relay accept-leg does not apply.
	f.membership.OriginInstanceURL = "https://owner.example"
	e := f.signedEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrPeerNotPermitted) {
		t.Fatalf("read-only peer write must be ErrPeerNotPermitted, got %v", err)
	}
}

// TestValidate_ReadOnlyJoinerAcceptsOwnerRelay asserts the SYMMETRIC F5.1 case
// (US-5.1 AC3): a JOINED read peer ACCEPTS an event relayed by its OWNER. The
// receiver's mapping is is_owner=0 and the transport peer IS the origin owner, so
// the owner-relay leg accepts the fan-out regardless of the joiner's own read
// grant — a read peer still RECEIVES changes; its read status only constrains its
// OWN local edits (the F5.2 local guard), not what the owner pushes to it.
func TestValidate_ReadOnlyJoinerAcceptsOwnerRelay(t *testing.T) {
	f := newValidateFixture(t)
	f.membership.Permissions = model.FederationPermissionRead
	// Joiner-receiving-from-owner topology: the origin owner == the transport peer
	// (f.authorURL), and this receiver is a joiner (is_owner=0). The default fixture
	// already sets OriginInstanceURL == author, so the owner-relay accept-leg fires.
	e := f.signedEvent(t)
	res, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if err != nil {
		t.Fatalf("read joiner must accept its owner's relayed fan-out (US-5.1 AC3): %v", err)
	}
	if res.LocalProjectID != f.membership.LocalProjectID {
		t.Errorf("local project id: got %d, want %d", res.LocalProjectID, f.membership.LocalProjectID)
	}
}

// TestValidate_EmptyPermissionPeerRejected asserts the F5.2 incoming
// write-enforcement edge: a peer pushing a write UP TO THE OWNER whose mapping
// carries an empty/unset permission grade is rejected as not-permitted (§9.3,
// US-5.1 AC2 — "unknown/empty permissions → 403"). An empty grade is never
// treated as an implicit write capability; only an explicit write/admin grant
// may originate writes. This is the peer→owner leg (the receiver owns the
// project, so the sending peer's permission is enforced).
func TestValidate_EmptyPermissionPeerRejected(t *testing.T) {
	f := newValidateFixture(t)
	f.membership.Permissions = "" // unset / unknown grade
	f.membership.OriginInstanceURL = "https://owner.example"
	e := f.signedEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrPeerNotPermitted) {
		t.Fatalf("empty-permission peer write must be ErrPeerNotPermitted, got %v", err)
	}
}

// TestValidate_UnknownPermissionPeerRejected asserts a peer pushing UP TO THE
// OWNER with a bogus (non-read/write/admin) permission grade is rejected as
// not-permitted (§9.3 — "unknown permissions → 403"). canWrite must default to
// false for any grade it does not explicitly recognise, so a forged or
// corrupted grade can never be coerced into a write capability.
func TestValidate_UnknownPermissionPeerRejected(t *testing.T) {
	f := newValidateFixture(t)
	f.membership.Permissions = model.FederationPermission("superuser")
	f.membership.OriginInstanceURL = "https://owner.example"
	e := f.signedEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrPeerNotPermitted) {
		t.Fatalf("unknown-permission peer write must be ErrPeerNotPermitted, got %v", err)
	}
}

// TestValidate_RevokedPeerRejected asserts a revoked peer's event is rejected
// with the DISTINCT ErrPeerRevoked sentinel (Federation v1 F5.4, US-6.2 AC2),
// even if the permission grade would otherwise allow it. It must NOT collapse
// into ErrPeerNotPermitted (read-only) so the handler can return the terminal,
// irreversible 403 federation_revoked.
func TestValidate_RevokedPeerRejected(t *testing.T) {
	f := newValidateFixture(t)
	f.membership.Revoked = true
	e := f.signedEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrPeerRevoked) {
		t.Fatalf("revoked peer must be ErrPeerRevoked, got %v", err)
	}
	if errors.Is(err, inbox.ErrPeerNotPermitted) {
		t.Fatalf("revoked must be DISTINCT from ErrPeerNotPermitted (read-only), got %v", err)
	}
}

// TestValidate_PausedPeerRejected asserts an inbound event from a peer the owner
// has temporarily paused is rejected with ErrPeerPaused (Federation v1 F5.3,
// US-6.1 AC1 → 403 paused), even though the link is still trusted and the
// permission grade would otherwise allow the write. The default fixture is the
// peer→owner leg (this receiver owns the project). The pause check is non-
// destructive: it is DISTINCT from the revoked rejection (ErrPeerNotPermitted),
// so the handler can map paused → 403 federation_paused and the link stays
// resumable.
func TestValidate_PausedPeerRejected(t *testing.T) {
	f := newValidateFixture(t)
	// Peer→owner leg: the receiver owns the project, so the owner-relay accept-leg
	// (OriginInstanceURL == peer) does NOT fire; the paused check is reached.
	f.membership.OriginInstanceURL = "https://owner.example"
	f.membership.Paused = true
	e := f.signedEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrPeerPaused) {
		t.Fatalf("paused peer must be ErrPeerPaused, got %v", err)
	}
	// A pause is non-destructive: it must NOT collapse into the revoked/permission
	// rejection so the handler can return the resumable 403 federation_paused.
	if errors.Is(err, inbox.ErrPeerNotPermitted) {
		t.Fatalf("paused must be DISTINCT from ErrPeerNotPermitted (a pause is non-destructive), got %v", err)
	}
}

// TestValidate_RevokedBeforePaused asserts a peer that is BOTH revoked and paused
// is rejected as revoked (ErrPeerRevoked), not paused: revoke is the terminal,
// higher-precedence state (US-6.1 vs US-6.2). This matches the peers-list status
// precedence (revoked > paused).
func TestValidate_RevokedBeforePaused(t *testing.T) {
	f := newValidateFixture(t)
	f.membership.OriginInstanceURL = "https://owner.example"
	f.membership.Revoked = true
	f.membership.Paused = true
	e := f.signedEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrPeerRevoked) {
		t.Fatalf("revoked+paused peer must be rejected as revoked (ErrPeerRevoked), got %v", err)
	}
	if errors.Is(err, inbox.ErrPeerPaused) {
		t.Fatalf("revoked+paused must take the revoked branch, not paused, got %v", err)
	}
}

// TestValidate_KeyResolveFailure asserts a peer key that cannot be resolved is a
// TRANSIENT "could not verify" failure (ErrEventKeyUnresolved) — NOT a
// verified-and-rejected signature failure (Federation v1 F4.3 review fix). The
// distinction matters because only ErrEventSignatureInvalid stamps the sticky,
// irreversible key_mismatch marker; a transient .well-known fetch error must
// never be misread as a key rotation. This pins the false-positive boundary at
// the validator level.
func TestValidate_KeyResolveFailure(t *testing.T) {
	f := newValidateFixture(t)
	f.resolveErr = errors.New("network down")
	e := f.signedEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrEventKeyUnresolved) {
		t.Fatalf("unresolvable author key must be ErrEventKeyUnresolved, got %v", err)
	}
	if errors.Is(err, inbox.ErrEventSignatureInvalid) {
		t.Fatalf("a transient resolve failure must NOT be ErrEventSignatureInvalid (it would stamp the sticky marker), got %v", err)
	}
}

// TestValidate_OrderSignatureBeforeMembership asserts the signature check runs
// before membership/permission: an unsigned event from a perfectly-trusted peer
// is still rejected as a signature failure, not silently accepted.
func TestValidate_OrderSignatureBeforeMembership(t *testing.T) {
	f := newValidateFixture(t)
	e := f.signedEvent(t)
	e.Signature = "" // remove the signature but keep a valid membership
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrEventSignatureInvalid) {
		t.Fatalf("signature must be checked before membership, got %v", err)
	}
}

// leaveEvent builds a signed joiner→owner federation_leave control event from the
// fixture's author (the leaving peer). It carries no fields and targets the
// project, mirroring the OpLeave wire shape (Federation v1 F5.5, US-6.3).
func (f *validateFixture) leaveEvent(t *testing.T) events.Event {
	t.Helper()
	e := events.Event{
		EventID:         "01J0000000000000000000LEAV",
		Op:              events.OpLeave,
		EntityType:      events.EntityProject,
		EntityID:        "proj-client-1",
		ProjectClientID: "proj-client-1",
		Author:          f.authorURL,
		OriginInstance:  f.authorURL,
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          map[string]events.Field{},
	}
	signed, err := events.Sign(e, f.priv)
	if err != nil {
		t.Fatalf("sign leave event: %v", err)
	}
	return signed
}

// TestValidate_LeaveAcceptedFromReadPeer asserts a federation_leave control event
// is ACCEPTED from a peer pushing UP to the owner regardless of its permission
// grade — even a READ peer may leave (a leave is not a write, US-6.3 AC1/AC2). The
// canWrite gate that rejects a read peer's data write must NOT apply to a leave.
// This is the peer→owner leg (the receiver owns the project).
func TestValidate_LeaveAcceptedFromReadPeer(t *testing.T) {
	f := newValidateFixture(t)
	f.membership.Permissions = model.FederationPermissionRead
	// Peer→owner leg: the receiver owns the project (origin is the owner's URL, not
	// the leaving peer), so the owner-relay accept-leg does not fire and the canWrite
	// gate would otherwise reject — but a leave must still be accepted.
	f.membership.OriginInstanceURL = "https://owner.example"
	e := f.leaveEvent(t)
	res, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if err != nil {
		t.Fatalf("read peer leave must be accepted (US-6.3 AC1): %v", err)
	}
	if res == nil || res.LocalProjectID != f.membership.LocalProjectID {
		t.Errorf("leave validation result must carry the local project id, got %+v", res)
	}
}

// TestValidate_LeaveFromRevokedPeerRejected asserts a leave from an already-revoked
// peer is still rejected as revoked (Federation v1 F5.5 — leave-after-revoke is a
// no-op; revoke is the terminal, higher-precedence state checked before the
// leave-accept leg).
func TestValidate_LeaveFromRevokedPeerRejected(t *testing.T) {
	f := newValidateFixture(t)
	f.membership.OriginInstanceURL = "https://owner.example"
	f.membership.Revoked = true
	e := f.leaveEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrPeerRevoked) {
		t.Fatalf("leave from revoked peer must be ErrPeerRevoked, got %v", err)
	}
}

// TestValidate_LeaveFromPausedPeerRejected pins the documented precedence
// revoked > paused > leave-accept (Federation v1 F5.3/F5.5): a federation_leave
// from a PAUSED (but not revoked) peer is rejected with ErrPeerPaused, because the
// paused check is reached BEFORE the leave-accept leg. The path is correct in
// validate.go but was previously unguarded by a regression test.
func TestValidate_LeaveFromPausedPeerRejected(t *testing.T) {
	f := newValidateFixture(t)
	f.membership.OriginInstanceURL = "https://owner.example"
	f.membership.Paused = true
	e := f.leaveEvent(t)
	_, err := f.validator().Validate(context.Background(), e, f.authorURL)
	if !errors.Is(err, inbox.ErrPeerPaused) {
		t.Fatalf("leave from paused peer must be ErrPeerPaused (paused > leave-accept), got %v", err)
	}
	// A pause is non-destructive and DISTINCT from revoke/permission: the leave must
	// not collapse into the revoked rejection.
	if errors.Is(err, inbox.ErrPeerRevoked) {
		t.Fatalf("leave from paused (not revoked) peer must NOT be ErrPeerRevoked, got %v", err)
	}
}

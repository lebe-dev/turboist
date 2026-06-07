package httpapi

import "fmt"

// Error code constants matching the API.md error table.
const (
	CodeValidationFailed   = "validation_failed"
	CodeAuthInvalid        = "auth_invalid"
	CodeAuthExpired        = "auth_expired"
	CodeAuthRateLimited    = "auth_rate_limited"
	CodeForbidden          = "forbidden"
	CodeNotFound           = "not_found"
	CodeConflict           = "conflict"
	CodeSetupAlreadyDone   = "setup_already_done"
	CodeLimitExceeded      = "limit_exceeded"
	CodeForbiddenPlacement = "forbidden_placement"
	CodeRecurrenceInvalid  = "recurrence_invalid"
	CodeTroikiSlotFull     = "troiki_slot_full"
	CodeTOTPInvalidCode    = "totp_invalid_code"
	CodeTOTPAlreadyEnabled = "totp_already_enabled"
	CodeTOTPNotEnabled     = "totp_not_enabled"
	CodeInternalError      = "internal_error"
	CodeSetupRequired      = "setup_required"
	// CodeGone is returned when a soft-deleted (tombstoned) entity is
	// re-edited. The tombstone is final (Federation v1 F0.1, US-3.7 AC2).
	CodeGone = "gone"

	// Federation trust-plane error codes (Federation v1 F0.3). These are
	// returned by the HTTP-signature middleware and federation handlers; the
	// transport-level checks (signature, replay, timestamp window, digest)
	// finalize in F0.3 so the signed endpoint is never exposed without them.
	//
	// CodeFederationSignatureInvalid — the Ed25519 transport signature failed
	// to verify, or a required X-Federation-* header was missing/malformed.
	CodeFederationSignatureInvalid = "federation_signature_invalid"
	// CodeFederationReplay — the request nonce was already seen (anti-replay).
	CodeFederationReplay = "federation_replay"
	// CodeFederationTimestampStale — the request timestamp is outside the
	// ±5min window (checked BEFORE the nonce, US-7.3 AC2).
	CodeFederationTimestampStale = "federation_timestamp_stale"
	// CodeFederationUntrusted — the calling instance is not a known/trusted peer.
	CodeFederationUntrusted = "federation_untrusted"
	// CodeFederationKeyMissing — this instance has no federation keypair yet
	// (federation not enabled / keys not generated).
	CodeFederationKeyMissing = "federation_key_missing"
	// CodeFederationDigestMismatch — the X-Federation-Digest does not match the
	// SHA-256 of the request body (US-7.2 AC2 transport leg, 400).
	CodeFederationDigestMismatch = "federation_digest_mismatch"
	// CodeFederationVersionUnsupported — protocol-version negotiation found no
	// version in common between this instance and the peer (Federation v1 F0.4,
	// US-9.1 AC2). Returned at handshake BEFORE the invite is consumed (400).
	CodeFederationVersionUnsupported = "federation_version_unsupported"
	// CodeFederationNotEnabled — an operation that requires federation to be
	// enabled on the target project (e.g. creating an invite) was attempted on a
	// project that has not been enabled (Federation v1 F1.2, US-1.1 AC3, 400).
	CodeFederationNotEnabled = "federation_not_enabled"
	// CodeFederationKeyMismatch — a handshake from an instance_url already known
	// with a DIFFERENT Ed25519 key. The owner refuses to silently rotate a pinned
	// peer key (Federation v1 F2.2, US-2.2 AC5, 409 + WARN).
	CodeFederationKeyMismatch = "federation_key_mismatch"
	// CodeFederationReadOnly — a local mutation was attempted on a federated
	// project this instance only has read access to (a joined read-only peer copy).
	// This is the authoritative backend enforcement seam — UI disabling alone is
	// insufficient (Federation v1 F2.4, US-2.4 AC4, §9.2, 403).
	CodeFederationReadOnly = "federation_read_only"
	// CodeFederationUpstream — the owner instance returned a 5xx during the
	// handshake (e.g. a mid-build DB failure). This is a transient owner-side fault,
	// NOT an auth/invite rejection, so the joiner surfaces it as a retryable 502
	// rather than telling the user to chase a fresh invite (F2.3 #8).
	CodeFederationUpstream = "federation_upstream"
	// CodeFederationPaused — the owner has temporarily paused exchange with this
	// peer (Federation v1 F5.3, US-6.1). Inbound events from a paused peer are
	// rejected with a 403 (not applied) while the link stays trusted (non-
	// destructive — distinct from a permanent revoke). The owner's outbox simply
	// stops fanning out to a paused peer; events accumulate and flush on resume.
	CodeFederationPaused = "federation_paused"

	// CodeFederationRevoked — the owner has PERMANENTLY revoked this peer
	// (Federation v1 F5.4, US-6.2). Any inbound traffic from a revoked peer on the
	// revoked project is rejected with a 403 and NOT applied (US-6.2 AC2); the check
	// runs BEFORE membership/permission so a revoked peer learns nothing about the
	// project. It also covers the offline-return case (US-6.2 AC4): a peer that
	// missed the in-band federation_revoke event gets a 403 on its next sync and
	// self-marks federation_lost. Revoke is IRREVERSIBLE (re-collaboration needs a
	// fresh invite, US-6.2 AC5) — distinct from the reversible CodeFederationPaused.
	CodeFederationRevoked = "federation_revoked"

	// CodeFederationNotJoined — a "leave" was attempted on a project that is NOT a
	// joined federated copy: the owner's OWN federated project, or a non-federated
	// project (Federation v1 F5.5, US-6.3, 409). Only a peer that joined another
	// instance's project may leave it; the owner revokes peers instead.
	CodeFederationNotJoined = "federation_not_joined"

	// Per-event payload validation error codes (Federation v1 F3.2a, US-7.2).
	// These are DISTINCT from the transport-signature codes above: the transport
	// codes authenticate the HTTP request, these authenticate the event payload
	// end-to-end across an owner-hub relay. The POST /federation/events handler
	// maps the inbox.Validator sentinels to these before any inbox/domain write.
	//
	// CodeFederationAuthorMismatch — the event's author.instance_url does not equal
	// its origin_instance (a relay tried to rewrite authorship, US-7.2 AC3, 400).
	CodeFederationAuthorMismatch = "federation_author_mismatch"
	// CodeFederationClockSkew — the event's HLC physical clock is too far in the
	// future (>10min) or the past (>1h), or is unparseable (US-7.2 AC4, 400).
	CodeFederationClockSkew = "federation_clock_skew"
	// CodeFederationInvalidEvent — the event payload is structurally invalid in a
	// way no retry can fix: an op=create/op=update carrying no field HLC (which would
	// otherwise materialise an empty ghost row). 400, so the sending peer's outbox
	// worker classifies it event-scoped permanent and dead-letters just this event.
	CodeFederationInvalidEvent = "federation_invalid_event"
	// CodeFederationKeyUnresolved — the event author's public key could not be
	// resolved (transient .well-known fetch error). Retryable, so it is NEVER
	// conflated with a key rotation: it does not stamp the sticky key_mismatch
	// marker (Federation v1 F4.3 review fix, 503).
	CodeFederationKeyUnresolved = "federation_key_unresolved"
	// CodeFederationStalePull — a pull whose since_hlc predates the oldest retained
	// event: the events between have been GC'd, so the peer must re-snapshot. The
	// 410 body carries {snapshot_url, as_of_hlc} (US-3.7 AC4 emit half, F3.3); the
	// consume half (fetch snapshot, preserve outbox) lands in F4.2.
	CodeFederationStalePull = "federation_stale_pull"

	// Inbound backpressure codes (Federation v1 F4.4, US-8.3). The signed event
	// endpoint protects itself from a peer sending too much, too fast, or too big.
	//
	// CodeFederationRateLimited — the calling peer exceeded its inbound event rate
	// (default 600/min). Returned with a Retry-After header so the peer backs off
	// (US-8.3 AC1, 429). The symmetric inbound half of the outbox worker's outbound
	// 429 honoring.
	CodeFederationRateLimited = "federation_rate_limited"
	// CodeFederationPayloadTooLarge — an inbound event batch exceeded the max
	// events-per-batch limit (default 500). The batch is rejected WHOLE, not
	// partially applied (US-8.3 AC3, 413).
	CodeFederationPayloadTooLarge = "federation_payload_too_large"
)

// AppError is a structured API error carrying an HTTP status, code, message, and optional details.
type AppError struct {
	HTTPStatus int
	Code       string
	Message    string
	Details    any
	// Internal is the underlying cause. Surfaced via Unwrap and logged by the
	// central error handler for 5xx responses; never sent to clients.
	Internal error
}

func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Internal }

// WithCause attaches the underlying error so the central handler can log it.
// The cause is intentionally not added to Details — it stays server-side only.
func (e *AppError) WithCause(err error) *AppError {
	e.Internal = err
	return e
}

func newErr(status int, code, msg string, details ...any) *AppError {
	var d any
	if len(details) > 0 {
		d = details[0]
	}
	return &AppError{HTTPStatus: status, Code: code, Message: msg, Details: d}
}

func ErrValidation(msg string, details ...any) *AppError {
	return newErr(400, CodeValidationFailed, msg, details...)
}

// ErrUnprocessable signals that the request parsed cleanly but violates a
// semantic rule (e.g. API-token scopes that fail auth.ValidateScopes). Maps
// to HTTP 422 with the validation_failed code so frontend error handling
// stays uniform with field-level ErrValidation.
func ErrUnprocessable(msg string, details ...any) *AppError {
	return newErr(422, CodeValidationFailed, msg, details...)
}

func ErrAuthInvalid(msg string) *AppError {
	return newErr(401, CodeAuthInvalid, msg)
}

func ErrAuthExpired() *AppError {
	return newErr(401, CodeAuthExpired, "access token expired")
}

func ErrAuthRateLimited() *AppError {
	return newErr(429, CodeAuthRateLimited, "too many requests")
}

func ErrForbidden(msg string) *AppError {
	return newErr(403, CodeForbidden, msg)
}

func ErrNotFound(msg string) *AppError {
	return newErr(404, CodeNotFound, msg)
}

func ErrConflict(msg string) *AppError {
	return newErr(409, CodeConflict, msg)
}

// ErrGone signals that the targeted entity has been soft-deleted. The tombstone
// is final, so the mutation is rejected with HTTP 410 (Federation v1 F0.1,
// US-3.7 AC2 foundation).
func ErrGone(msg string) *AppError {
	return newErr(410, CodeGone, msg)
}

func ErrSetupAlreadyDone() *AppError {
	return newErr(410, CodeSetupAlreadyDone, "setup already completed")
}

func ErrLimitExceeded(msg string, details ...any) *AppError {
	return newErr(422, CodeLimitExceeded, msg, details...)
}

func ErrForbiddenPlacement(msg string) *AppError {
	return newErr(422, CodeForbiddenPlacement, msg)
}

func ErrRecurrenceInvalid(msg string) *AppError {
	return newErr(422, CodeRecurrenceInvalid, msg)
}

func ErrTroikiSlotFull(msg string) *AppError {
	return newErr(409, CodeTroikiSlotFull, msg)
}

func ErrInternal(msg string) *AppError {
	return newErr(500, CodeInternalError, msg)
}

// ErrSetupRequired signals that the instance has no admin user yet — the
// frontend should redirect to /setup. Returned by SetupCheckMiddleware before
// auth runs, so callers do not need a token.
func ErrSetupRequired() *AppError {
	return newErr(503, CodeSetupRequired, "setup required")
}

func ErrTOTPInvalidCode() *AppError {
	return newErr(401, CodeTOTPInvalidCode, "invalid TOTP code")
}

func ErrTOTPAlreadyEnabled() *AppError {
	return newErr(409, CodeTOTPAlreadyEnabled, "TOTP already enabled")
}

func ErrTOTPNotEnabled() *AppError {
	return newErr(409, CodeTOTPNotEnabled, "TOTP not enabled")
}

func ErrTOTPTicketInvalid() *AppError {
	return newErr(401, CodeAuthInvalid, "invalid or expired OTP ticket")
}

// Federation trust-plane error constructors (Federation v1 F0.3). Messages are
// intentionally generic so a probing peer cannot distinguish "bad signature"
// from "unknown header"; the precise reason is logged server-side.

func ErrFederationSignatureInvalid(msg string) *AppError {
	if msg == "" {
		msg = "federation signature verification failed"
	}
	return newErr(401, CodeFederationSignatureInvalid, msg)
}

func ErrFederationReplay() *AppError {
	return newErr(401, CodeFederationReplay, "federation request replayed")
}

func ErrFederationTimestampStale() *AppError {
	return newErr(401, CodeFederationTimestampStale, "federation request timestamp out of window")
}

func ErrFederationUntrusted(msg string) *AppError {
	if msg == "" {
		msg = "federation peer not trusted"
	}
	return newErr(403, CodeFederationUntrusted, msg)
}

func ErrFederationKeyMissing() *AppError {
	return newErr(409, CodeFederationKeyMissing, "federation keypair not generated")
}

func ErrFederationDigestMismatch() *AppError {
	return newErr(400, CodeFederationDigestMismatch, "federation request body digest mismatch")
}

// ErrFederationKeyMismatch is returned when a handshake arrives from an
// instance_url already known with a different Ed25519 key (Federation v1 F2.2,
// US-2.2 AC5). The owner refuses to silently rotate a pinned key → 409 + WARN.
func ErrFederationKeyMismatch() *AppError {
	return newErr(409, CodeFederationKeyMismatch, "federation peer key mismatch")
}

// ErrFederationVersionUnsupported is returned when protocol-version negotiation
// finds no common version (Federation v1 F0.4, US-9.1 AC2). The handshake maps
// protocol.ErrNoVersionOverlap to this and rejects BEFORE consuming the invite.
func ErrFederationVersionUnsupported() *AppError {
	return newErr(400, CodeFederationVersionUnsupported, "no common federation protocol version")
}

// ErrFederationNotEnabled is returned when an invite (or other federation-only
// operation) is attempted on a project that has not been enabled for federation
// (Federation v1 F1.2, US-1.1 AC3, 400).
func ErrFederationNotEnabled() *AppError {
	return newErr(400, CodeFederationNotEnabled, "federation is not enabled for this project")
}

// ErrFederationReadOnly is returned when a local mutation targets a federated
// project this instance only holds read access to (a joined read-only peer copy).
// It is the authoritative server-side guard behind the UI edit lockout
// (Federation v1 F2.4, US-2.4 AC4, §9.2, 403).
func ErrFederationReadOnly() *AppError {
	return newErr(403, CodeFederationReadOnly, "this federated project is read-only")
}

// ErrFederationNotJoined is returned when a "leave" is attempted on a project that
// is NOT a joined federated copy — the owner's OWN project, or a non-federated
// project (Federation v1 F5.5, US-6.3, 409). Only a peer that joined another
// instance's project may leave it; the owner revokes peers instead.
func ErrFederationNotJoined() *AppError {
	return newErr(409, CodeFederationNotJoined, "this project is not a joined federated copy")
}

// ErrFederationPaused is returned when an inbound event arrives from a peer the
// owner has temporarily paused (Federation v1 F5.3, US-6.1 AC1). It is a 403:
// the event is rejected and not applied, but — unlike a revoke — the link stays
// trusted, so resuming the peer flushes its accumulated events.
func ErrFederationPaused() *AppError {
	return newErr(403, CodeFederationPaused, "exchange with this peer is paused")
}

// ErrFederationRevoked is returned when an inbound event arrives from a peer the
// owner has PERMANENTLY revoked (Federation v1 F5.4, US-6.2 AC2/AC4). It is a 403:
// the event is rejected and not applied, and — unlike a pause — the revoke is
// irreversible (re-collaboration needs a fresh invite). It also surfaces on the
// offline-return path so a peer that missed the in-band revoke self-marks lost.
func ErrFederationRevoked() *AppError {
	return newErr(403, CodeFederationRevoked, "access to this federated project has been revoked")
}

// ErrFederationUpstream is returned when the owner instance replied with a 5xx
// during a handshake (Federation v1 F2.3 #8). It is a transient upstream fault,
// so the joiner surfaces it as a retryable 502 Bad Gateway instead of collapsing
// it to the generic 401 invite-rejection (which would wrongly send the user to
// chase a fresh invite for a problem a retry would fix).
func ErrFederationUpstream() *AppError {
	return newErr(502, CodeFederationUpstream, "owner instance temporarily unavailable, retry shortly")
}

// ErrFederationAuthorMismatch is returned when a received event's author does not
// match its origin_instance (Federation v1 F3.2a, US-7.2 AC3). It is a per-event
// payload rejection, separate from the transport signature, mapped to 400.
func ErrFederationAuthorMismatch() *AppError {
	return newErr(400, CodeFederationAuthorMismatch, "federation event author does not match origin")
}

// ErrFederationClockSkew is returned when a received event's HLC physical clock
// is outside the accepted skew window — more than 10 minutes in the future or
// more than 1 hour in the past (Federation v1 F3.2a, US-7.2 AC4), mapped to 400.
func ErrFederationClockSkew() *AppError {
	return newErr(400, CodeFederationClockSkew, "federation event clock skew out of bounds")
}

// ErrFederationInvalidEvent is returned when a received event is structurally
// invalid in a way no retry can fix — an op=create/op=update carrying no field HLC
// (Federation v1 F3.2a). Mapped to 400 so the sending peer's outbox worker treats
// it as an event-scoped permanent reject and dead-letters just this event.
func ErrFederationInvalidEvent() *AppError {
	return newErr(400, CodeFederationInvalidEvent, "federation event payload is structurally invalid")
}

// ErrFederationKeyUnresolved is returned when an inbound event's author public
// key could not be resolved — a transient .well-known fetch error (5xx / timeout
// / DNS) (Federation v1 F4.3 review fix). It is mapped to a RETRYABLE 503 and,
// unlike ErrFederationSignatureInvalid, NEVER stamps the sticky key_mismatch
// marker: a brief network blip must not turn the sync badge permanently red.
func ErrFederationKeyUnresolved() *AppError {
	return newErr(503, CodeFederationKeyUnresolved, "federation event author key temporarily unresolvable, retry shortly")
}

// ErrFederationStalePull is returned by the pull endpoint when the caller's
// since_hlc predates the oldest retained event for the project — the intermediate
// events have aged out of retention and been GC'd (Federation v1 F3.3, US-3.7 AC4
// emit half). The 410 body carries the snapshot_url + as_of_hlc the peer must
// re-bootstrap from; the consume side (fetch + preserve outbox) lands in F4.2.
func ErrFederationStalePull(snapshotURL, asOfHLC string) *AppError {
	return newErr(410, CodeFederationStalePull, "federation pull cursor older than retained history", map[string]any{
		"snapshot_url": snapshotURL,
		"as_of_hlc":    asOfHLC,
	})
}

// ErrFederationRateLimited is returned by the inbound event endpoint when the
// calling peer exceeds its inbound event rate (Federation v1 F4.4, US-8.3 AC1).
// The handler sets a Retry-After header alongside it; the body carries the
// retry_after_seconds detail for clients that read the envelope rather than the
// header. Mapped to 429.
func ErrFederationRateLimited(retryAfterSeconds int) *AppError {
	return newErr(429, CodeFederationRateLimited, "federation peer rate limit exceeded", map[string]any{
		"retry_after_seconds": retryAfterSeconds,
	})
}

// ErrFederationPayloadTooLarge is returned by the inbound event endpoint when a
// batch exceeds the max events-per-batch limit (Federation v1 F4.4, US-8.3 AC3).
// The batch is rejected WHOLE — never partially applied. Mapped to 413.
func ErrFederationPayloadTooLarge(maxEvents int) *AppError {
	return newErr(413, CodeFederationPayloadTooLarge, "federation event batch too large", map[string]any{
		"max_events": maxEvents,
	})
}

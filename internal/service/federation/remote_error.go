package federation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// RemoteHandshakeError carries a non-2xx reply the owner returned to the joiner's
// handshake (Federation v1 F2.2) or to an outbound push/pull (F3.2). The joiner
// handler re-surfaces the owner's federation error code/status to its own UI so a
// wrong-secret 401, a key-mismatch 409, a no-version 400, or a stale-invite 410
// are mapped to the matching typed frontend error (US-2.2 AC4/AC5, US-9.1 AC2);
// the outbox worker reads its classification seams for F4.4 backpressure.
//
// retryAfter is the parsed Retry-After window carried on a 429 (Federation v1
// F4.4, US-4.4 AC1); hasRetryAfter reports whether one was present.
type RemoteHandshakeError struct {
	StatusCode    int
	Code          string
	retryAfter    time.Duration
	hasRetryAfter bool
}

func (e *RemoteHandshakeError) Error() string {
	return fmt.Sprintf("federation: owner rejected handshake (status %d, code %q)", e.StatusCode, e.Code)
}

// FederationPermanent reports whether the remote rejection is PERMANENT — a
// client error (4xx) the sender cannot fix by retrying (a revoked-peer 403, an
// author/origin-mismatch 400, a not-a-member 403, a stale-tombstone 410, a
// signature-rejected 401). It satisfies the outbox worker's permanent-error
// classification seam so a permanent 4xx is not retried forever.
//
// 429 Too Many Requests is the sole 4xx that IS transient (a Retry-After
// backpressure signal, not a permanent reject), so it is excluded. Any 5xx /
// network error (StatusCode 0 from a transport failure never reaches here — that
// path returns a plain error) is transient by omission.
func (e *RemoteHandshakeError) FederationPermanent() bool {
	if e.StatusCode == 429 {
		return false
	}
	// A PAUSED peer is a reversible, NON-destructive state: the owner will resume
	// it and the paused peer's accumulated outbound backlog must then flush. Treat
	// its 403 federation_paused like a 429 backoff — TRANSIENT, never a permanent
	// link death — otherwise the paused peer's outbound link is gated forever and
	// never recovers after resume (Federation v1 F5.3, US-6.1 — pause is
	// non-destructive on BOTH directions).
	if e.Code == codeFederationPaused {
		return false
	}
	return e.StatusCode >= 400 && e.StatusCode < 500
}

// codeFederationPaused mirrors httpapi.CodeFederationPaused (the wire code a paused
// peer's inbound returns). Duplicated as a literal to avoid importing httpapi into
// the service layer (import cycle); a drift guard in the external test asserts the
// two stay equal.
const codeFederationPaused = "federation_paused"

// FederationPeerScoped reports whether a PERMANENT reject kills the whole
// federation LINK (peer-scoped) rather than just the offending event
// (event-scoped). It satisfies the outbox worker's peer-scoped classification
// seam (Federation v1 F4.4 hardening) so the worker gates the WHOLE peer only on a
// genuine link reject — never on a single event-specific 4xx that would otherwise
// silently strand every other healthy event to that peer.
//
// PEER-SCOPED is a 403: the peer rejects this instance's membership outright — a
// revoked peer, a read-only peer attempting a write, an untrusted/forbidden or
// paused peer (§9.2/§9.3 ACL enforcement all return 403). Every future event to
// the peer would be rejected identically, so gating the whole link is correct.
//
// EVENT-SCOPED is everything else permanent (a 400 author/origin-mismatch or
// clock-skew, a 401 signature-rejected, a 410 stale-tombstone re-edit per the
// offline contract): the reject is specific to the offending event. The event is
// dead-lettered, but the link stays healthy and other events keep flowing.
func (e *RemoteHandshakeError) FederationPeerScoped() bool {
	return e.StatusCode == 403
}

// FederationStatusCode + FederationReason satisfy the outbox worker's dead-letter
// classification seam (Federation v1 F4.4, US-4.4 AC3): the HTTP status + the
// federation error code are recorded on the parked dead-letter row.
func (e *RemoteHandshakeError) FederationStatusCode() int { return e.StatusCode }
func (e *RemoteHandshakeError) FederationReason() string  { return e.Code }

// FederationRetryAfter satisfies the outbox worker's 429 seam (Federation v1
// F4.4, US-4.4 AC1): a 429 carrying a Retry-After header gates the peer for
// exactly that window. (0, false) when no Retry-After was present.
func (e *RemoteHandshakeError) FederationRetryAfter() (time.Duration, bool) {
	return e.retryAfter, e.hasRetryAfter
}

// parseRetryAfter parses an RFC-7231 Retry-After header value — either
// delta-seconds (the common form a peer's rate limiter emits) or an HTTP-date —
// into a duration relative to now. An empty / unparseable value yields (0, false)
// so the worker falls back to its exponential backoff.
func parseRetryAfter(raw string, now time.Time) (time.Duration, bool) {
	if raw == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

// jsonUnmarshal is a thin wrapper so the join decode path has one place to evolve
// (e.g. unknown-field tolerance in F6.1). v1 uses the standard decoder.
func jsonUnmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

// errorCodeOf extracts the {error:{code}} envelope code from an owner error body,
// returning "" when the body is not the standard error shape.
func errorCodeOf(body []byte) string {
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	return env.Error.Code
}

// stalePullDetailsOf extracts the {error:{details:{snapshot_url, as_of_hlc}}}
// payload from a 410 federation_stale_pull body (the F3.3 emit half). The recovery
// loop's F4.2 consumer needs both fields to re-fetch + overwrite local state; an
// empty snapshot_url means the body was not the expected stale-pull shape and the
// caller should treat the 410 as a plain (non-consumable) error.
func stalePullDetailsOf(body []byte) (snapshotURL, asOfHLC string) {
	var env struct {
		Error struct {
			Details struct {
				SnapshotURL string `json:"snapshot_url"`
				AsOfHLC     string `json:"as_of_hlc"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", ""
	}
	return env.Error.Details.SnapshotURL, env.Error.Details.AsOfHLC
}

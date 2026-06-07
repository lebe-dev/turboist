package httpapi

import (
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/federation/nonce"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/transport"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// federationTimestampWindow is the ±window a federation request timestamp must
// fall within (US-7.3 AC2). Checked BEFORE the nonce so a stale request is
// rejected without polluting the replay cache.
const federationTimestampWindow = 5 * time.Minute

const localsFederationPeerKey = "federation_peer"

// FederationPeer is the verified caller stashed in Locals by the signature
// middleware. Downstream handlers read it via GetFederationPeer. PublicKey is the
// base64-std Ed25519 key the signature was verified under (the peer's published
// .well-known key) — the handshake handler enforces the request body's
// joiner_public_key equals it (US-2.2 AC1 defense-in-depth).
type FederationPeer struct {
	InstanceURL string
	DisplayName string
	PublicKey   string
}

// FederationAuditor is the non-blocking audit sink the signature middleware
// records transport rejections to (Federation v1 F6.3, US-7.4 AC1). It is
// satisfied by *audit.Writer; kept as a local interface taking repo.AuditEntry so
// httpapi holds no hard dependency on the audit package. A nil Auditor disables
// audit recording (a federation-off / partial build) — Record is guarded by nil.
type FederationAuditor interface {
	Record(e repo.AuditEntry)
}

// FederationSignatureDeps are the collaborators the HTTP-signature middleware
// needs. Now is injectable for deterministic timestamp-window tests. Auditor is
// optional; when set, every transport rejection records one audit row (US-7.4).
type FederationSignatureDeps struct {
	Nonces   *nonce.Cache
	PeerKeys *peerkeys.Cache
	Now      func() time.Time
	Auditor  FederationAuditor
}

// HTTPSignatureMiddleware verifies the federation transport signature on a
// signed route (Federation v1 F0.3). It runs the full set of Must-grade
// transport checks, in this order:
//
//  1. parse the X-Federation-* headers (missing/malformed → 401 invalid);
//  2. recompute the body digest and constant-time compare it to
//     X-Federation-Digest (mismatch → 400 — US-7.2 AC2 transport leg);
//  3. enforce the ±5min timestamp window BEFORE touching the nonce
//     (stale → 401 — US-7.3 AC2);
//  4. anti-replay the nonce (replay → 401 — US-7.3 AC1);
//  5. resolve the peer's Ed25519 key via the peer-key cache (.well-known
//     fetch-once) and verify the signature over the pinned canonical string,
//     which binds the protocol version (anti-downgrade);
//  6. on success, stash the verified peer (incl. display_name) in Locals.
//
// Nonce-before-verify is a DELIBERATE ordering decision (F4.4 review / R18), not an
// oversight: the nonce is consumed at step 4, BEFORE the step-5 key resolution +
// signature verify. A valid-timestamp / valid-digest request with a GARBAGE
// signature therefore burns its nonce-cache slot before the signature is checked.
// This is accepted because the alternative — consuming the nonce only AFTER a
// successful verify — would let a replayed request reach the peer-key Resolve
// (a potential .well-known fetch) on EVERY replay instead of being rejected cheaply
// at the nonce gate, turning replay into a key-resolution amplification vector. The
// burn cost is bounded: a nonce-cache entry expires with the ±5min timestamp window
// and a fresh-nonce probe gains the attacker nothing (the request still fails
// verify). See docs/architecture/federation-threat-model.md (R18).
//
// The canonical string uses the CONCRETE request path (Request().URI().Path()),
// never the Fiber route template (R4). Generic 401/400 messages avoid leaking
// which check failed; the precise reason is logged server-side.
func HTTPSignatureMiddleware(deps FederationSignatureDeps) fiber.Handler {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	// recordRejection emits one audit row for a transport rejection (Federation v1
	// F6.3, US-7.4 AC1). It is a no-op when no Auditor is wired. detail is a short,
	// NON-SENSITIVE coded reason — never a secret/signature/token (§7 F6.3).
	recordRejection := func(c fiber.Ctx, kind repo.AuditKind, instanceURL, detail string) {
		if deps.Auditor == nil {
			return
		}
		deps.Auditor.Record(repo.AuditEntry{
			Kind:            kind,
			Outcome:         repo.AuditOutcomeRejected,
			PeerInstanceURL: instanceURL,
			Detail:          detail,
			CreatedAt:       now(),
		})
	}

	return func(c fiber.Ctx) error {
		// /federation/join (and its client-side subpaths) is the browser-facing
		// SvelteKit invite page, NOT a signed server-to-server route. The signed
		// group shares the /federation prefix, so this prefix-scoped middleware
		// would otherwise fire on the invite-link navigation and return
		// federation_signature_invalid instead of letting the request fall through
		// to the SPA shell. Skip it for the join carve-out (the same boundary
		// isFederationAPIPath draws for the SPA fallback — Federation v1 F2.1).
		if !isFederationAPIPath(c.Path()) {
			return c.Next()
		}

		ctx := c.Context()
		log := logging.FromContext(ctx)

		instanceURL := c.Get(transport.HeaderInstance)
		timestamp := c.Get(transport.HeaderTimestamp)
		nonceVal := c.Get(transport.HeaderNonce)
		protocolVer := c.Get(transport.HeaderProtocolVer)
		digestHeader := c.Get(transport.HeaderDigest)
		sigB64 := c.Get(transport.HeaderSignature)

		if instanceURL == "" || timestamp == "" || nonceVal == "" || protocolVer == "" || digestHeader == "" || sigB64 == "" {
			log.WarnContext(ctx, "federation: missing signature headers",
				slog.String("op", "httpapi.HTTPSignatureMiddleware"),
				slog.String("instance", instanceURL),
			)
			recordRejection(c, repo.AuditKindSignatureInvalid, instanceURL, "missing federation signature headers")
			return ErrFederationSignatureInvalid("missing federation signature headers")
		}

		// (2) Body digest — constant-time compare against the recomputed digest.
		computed := transport.BodyDigest(c.Body())
		if subtle.ConstantTimeCompare([]byte(computed), []byte(digestHeader)) != 1 {
			log.WarnContext(ctx, "federation: body digest mismatch",
				slog.String("op", "httpapi.HTTPSignatureMiddleware"),
				slog.String("instance", instanceURL),
			)
			recordRejection(c, repo.AuditKindDigestMismatch, instanceURL, "body digest mismatch")
			return ErrFederationDigestMismatch()
		}

		// (3) Timestamp window — checked BEFORE the nonce (US-7.3 AC2).
		ts, err := model.ParseUTC(timestamp)
		if err != nil {
			log.WarnContext(ctx, "federation: malformed timestamp",
				slog.String("op", "httpapi.HTTPSignatureMiddleware"),
				slog.String("timestamp", timestamp),
			)
			recordRejection(c, repo.AuditKindTimestampStale, instanceURL, "malformed timestamp")
			return ErrFederationTimestampStale()
		}
		skew := now().UTC().Sub(ts.UTC())
		if skew < -federationTimestampWindow || skew > federationTimestampWindow {
			log.WarnContext(ctx, "federation: timestamp out of window",
				slog.String("op", "httpapi.HTTPSignatureMiddleware"),
				slog.Duration("skew", skew),
			)
			recordRejection(c, repo.AuditKindTimestampStale, instanceURL, "timestamp outside window")
			return ErrFederationTimestampStale()
		}

		// (4) Nonce anti-replay — DELIBERATELY before the step-5 key-resolve + verify
		// (see the function doc): a replay is rejected here cheaply, before any
		// .well-known fetch, so replay cannot be turned into a key-resolution
		// amplification vector. The accepted cost is that a fresh-nonce garbage-
		// signature probe burns a cache slot (bounded by the ±5min window, R18).
		if !deps.Nonces.Check(nonceVal) {
			log.WarnContext(ctx, "federation: nonce replay",
				slog.String("op", "httpapi.HTTPSignatureMiddleware"),
				slog.String("instance", instanceURL),
			)
			recordRejection(c, repo.AuditKindReplay, instanceURL, "nonce replay")
			return ErrFederationReplay()
		}

		// (5) Resolve the peer key and verify the signature over the pinned
		// canonical string. The concrete request path is used, not the route
		// template (R4).
		resolved, err := deps.PeerKeys.Resolve(ctx, instanceURL)
		if err != nil {
			log.WarnContext(ctx, "federation: peer key unresolved",
				slog.String("op", "httpapi.HTTPSignatureMiddleware"),
				slog.String("instance", instanceURL),
				slog.String("err", err.Error()),
			)
			recordRejection(c, repo.AuditKindSignatureInvalid, instanceURL, "peer key unavailable")
			return ErrFederationSignatureInvalid("peer key unavailable")
		}
		params := transport.SignatureParams{
			Method:          c.Method(),
			Path:            string(c.Request().URI().Path()),
			InstanceURL:     instanceURL,
			Timestamp:       timestamp,
			Nonce:           nonceVal,
			ProtocolVersion: protocolVer,
			BodyDigest:      digestHeader,
		}
		sig, err := base64.StdEncoding.DecodeString(sigB64)
		if err != nil || !transport.Verify(resolved.Key, params, sig) {
			log.WarnContext(ctx, "federation: signature verification failed",
				slog.String("op", "httpapi.HTTPSignatureMiddleware"),
				slog.String("instance", instanceURL),
			)
			recordRejection(c, repo.AuditKindSignatureInvalid, instanceURL, "transport signature invalid")
			return ErrFederationSignatureInvalid("")
		}

		// (6) Stash the verified peer for downstream handlers, including the
		// base64-std key the signature verified under so the handshake can enforce
		// the request body's joiner_public_key equals it (US-2.2 AC1).
		c.Locals(localsFederationPeerKey, FederationPeer{
			InstanceURL: instanceURL,
			DisplayName: resolved.DisplayName,
			PublicKey:   base64.StdEncoding.EncodeToString(resolved.Key),
		})
		enrichLogger(c,
			slog.String("federation_peer", instanceURL),
			slog.String("auth_method", "federation_signature"),
		)
		return c.Next()
	}
}

// GetFederationPeer returns the verified federation caller stashed by
// HTTPSignatureMiddleware, or the zero value when the middleware did not run.
func GetFederationPeer(c fiber.Ctx) FederationPeer {
	v, _ := c.Locals(localsFederationPeerKey).(FederationPeer)
	return v
}

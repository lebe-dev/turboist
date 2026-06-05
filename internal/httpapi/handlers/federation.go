package handlers

import (
	"bufio"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/handshake"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/protocol"
	fedsnapshot "github.com/lebe-dev/turboist/internal/federation/snapshot"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// FederationHandler serves the public federation trust-plane endpoints
// (Federation v1 F0.3). For F0.3 that is the discovery document at
// GET /federation/.well-known/instance; the signed handshake/events/pull/
// snapshot endpoints are added in later phases on the same handler.
type FederationHandler struct {
	keys    *repo.FederationKeysRepo
	cipher  *crypto.TokenCipher
	baseURL string
	svc     *fedsvc.Service
	// events carries the F3.2 push/pull collaborators (inbox dedup + validator +
	// apply queue + pull membership). nil until WithEventsDeps wires them.
	events *FederationEventsDeps
	// handshakeLimiter rate-limits the signed handshake endpoint per calling peer
	// (Federation v1 F7.7, NFR-3): a peer hammering /federation/handshake (invite
	// brute-force / DoS) is throttled with 429 + Retry-After. nil → no handshake
	// rate limiting (the bucket is in-memory and optional). It is a DISTINCT bucket
	// from the events limiter: a handshake is rare and trust-establishing, so it
	// gets its own, tighter budget that a noisy event stream cannot exhaust.
	handshakeLimiter FederationRateLimiter
	// startedAt anchors the /federation/health uptime_s field (Federation v1 F6.5,
	// US-8.1). Stamped at construction (process start) so the public liveness probe
	// can report how long this instance has been up.
	startedAt time.Time
}

// NewFederationHandler constructs a FederationHandler. cipher is the
// FEDERATION_KEY-derived TokenCipher used to lazily generate the instance
// keypair; baseURL is this instance's public URL (the federation instance_url).
func NewFederationHandler(keys *repo.FederationKeysRepo, cipher *crypto.TokenCipher, baseURL string) *FederationHandler {
	return &FederationHandler{keys: keys, cipher: cipher, baseURL: baseURL, startedAt: time.Now()}
}

// WithService wires the federation service so the signed handshake endpoint can
// validate invites + record peers (Federation v1 F2.2). Returns the handler for
// chaining. The public .well-known endpoint does not need the service.
func (h *FederationHandler) WithService(svc *fedsvc.Service) *FederationHandler {
	h.svc = svc
	return h
}

// WithHandshakeRateLimiter wires the per-peer handshake rate limiter (Federation
// v1 F7.7, NFR-3): a peer exceeding its handshake rate is rejected 429 +
// Retry-After before the invite is even looked up, blunting invite brute-force /
// handshake-flood DoS. Returns the handler for chaining. A nil limiter (the
// default) leaves the handshake unthrottled.
func (h *FederationHandler) WithHandshakeRateLimiter(l FederationRateLimiter) *FederationHandler {
	h.handshakeLimiter = l
	return h
}

// wellKnownResponse is the wire shape of the discovery document.
type wellKnownResponse struct {
	InstanceURL      string `json:"instance_url"`
	PublicKey        string `json:"public_key"`
	DisplayName      string `json:"display_name"`
	ProtocolVersions []int  `json:"protocol_versions"`
}

// RegisterPublic mounts the unauthenticated federation discovery endpoint onto
// app. It must be registered before RegisterSPA (so the SPA fallback does not
// swallow it) and is intentionally outside the /api/v1 group, so it is reachable
// before setup and without a JWT (US-2.2 AC2).
func (h *FederationHandler) RegisterPublic(app *fiber.App) {
	app.Get(peerkeys.WellKnownPath, h.wellKnown)
	// Public liveness probe (Federation v1 F6.5, US-8.1): no auth, no peer detail.
	// The peer directory is only returned on the JWT admin /federation/health route.
	app.Get("/federation/health", h.health)
}

// health is the public federation liveness probe (Federation v1 F6.5, US-8.1). It
// reports instance_url, protocol_versions, uptime_s, the live outbox_depth, and
// the rolled-up status (ok|degraded|peers_stale) — but NEVER the per-peer detail
// (that is admin-only) so an unauthenticated probe leaks no peer directory. When
// the service is not wired (federation off) it still answers with a minimal,
// stable shape so a probe never 500s on a federation-off build.
func (h *FederationHandler) health(c fiber.Ctx) error {
	logEntry(c, "handler.Federation.Health")
	uptime := int64(time.Since(h.startedAt).Seconds())
	if h.svc == nil {
		return c.JSON(dto.HealthResponse{
			InstanceUrl:      h.baseURL,
			ProtocolVersions: protocol.SupportedProtocolVersions,
			UptimeS:          uptime,
			Status:           string(fedsvc.HealthOK),
		})
	}
	rep, err := h.svc.Health(c.Context())
	if err != nil {
		return httpapi.ErrInternal("federation health").WithCause(err)
	}
	return c.JSON(dto.HealthResponse{
		InstanceUrl:      rep.InstanceURL,
		ProtocolVersions: rep.ProtocolVersions,
		UptimeS:          uptime,
		OutboxDepth:      rep.OutboxDepth,
		Status:           string(rep.Status),
		// Public probe omits peers (admin-only detail).
	})
}

// wellKnown returns this instance's federation identity, lazily generating the
// keypair on first request. The keypair generation is a one-shot INSERT OR
// IGNORE (FederationKeysRepo.Ensure) so concurrent requests under
// SetMaxOpenConns(1) never regenerate.
func (h *FederationHandler) wellKnown(c fiber.Ctx) error {
	ctx := c.Context()
	keys, err := h.keys.Ensure(ctx, h.cipher, defaultDisplayName(h.baseURL))
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "federation: ensure keys",
			slog.String("op", "handler.Federation.wellKnown"),
			slog.String("err", err.Error()),
		)
		return httpapi.ErrInternal("ensure federation keys").WithCause(err)
	}
	return c.JSON(wellKnownResponse{
		InstanceURL:      h.baseURL,
		PublicKey:        keys.PublicKey,
		DisplayName:      keys.DisplayName,
		ProtocolVersions: protocol.SupportedProtocolVersions,
	})
}

// RegisterSigned mounts the peer-to-peer signed federation endpoints onto r
// (expected to be a group already wrapped by HTTPSignatureMiddleware, so the
// caller is a transport-verified peer). F2.2 lands the handshake; F2.3 adds the
// project snapshot; later phases add events/pull on the same signed group.
func (h *FederationHandler) RegisterSigned(r fiber.Router) {
	r.Post("/handshake", h.handshake)
	r.Get("/projects/:id/snapshot", h.snapshot)
	// F3.2 sync core: the inbound event push and the catch-up pull. Static
	// segments (snapshot/events) and the param route coexist; Fiber matches the
	// concrete /events suffix distinctly from /snapshot.
	r.Post("/events", h.receiveEvents)
	r.Get("/projects/:id/events", h.pullEvents)
}

// handshake is the owner side of the join handshake (Federation v1 F2.2,
// US-2.2). The HTTP-signature middleware has already proven the caller's
// instance_url + Ed25519 key (US-2.2 AC1); this handler validates the invite,
// negotiates the protocol version, and — on success — atomically consumes the
// invite and records the peer, returning the owner identity + a 15-min snapshot
// token. Errors are mapped so a wrong secret is a GENERIC 401 (US-2.2 AC4), a
// key mismatch is a 409 + WARN (US-2.2 AC5), and no version overlap is a 400
// with NOTHING consumed (US-9.1 AC2).
func (h *FederationHandler) handshake(c fiber.Ctx) error {
	ctx := c.Context()
	logEntry(c, "handler.Federation.Handshake")

	if h.svc == nil {
		return httpapi.ErrFederationKeyMissing()
	}

	// Per-peer handshake rate limit (Federation v1 F7.7, NFR-3): a peer hammering
	// the signed handshake endpoint (invite brute-force / DoS) is throttled with
	// 429 + Retry-After BEFORE any invite lookup, so a flood is cheap to reject and
	// never consumes invite-validation work. The middleware has already proven the
	// caller's instance_url, so the bucket is keyed on the verified peer. A nil
	// limiter disables throttling.
	peerHS := httpapi.GetFederationPeer(c)
	if h.handshakeLimiter != nil {
		if ok, retryAfter := h.handshakeLimiter.AllowN(peerHS.InstanceURL, 1); !ok {
			secs := retryAfterSeconds(retryAfter)
			c.Set("Retry-After", strconv.Itoa(secs))
			logValidation(c, "handler.Federation.Handshake", "peer handshake rate-limited")
			return httpapi.ErrFederationRateLimited(secs)
		}
	}

	var body handshake.Request
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		logValidation(c, "handler.Federation.Handshake", "invalid body")
		return httpapi.ErrFederationSignatureInvalid("invalid handshake body")
	}

	// The signature middleware verified the request under the peer's published
	// Ed25519 key (resolved from its .well-known) and stashed it in Locals. The
	// service enforces body.joiner_public_key == that verified key, so a peer
	// cannot present a body key different from the one it signed with (US-2.2 AC1).
	peer := peerHS
	in := fedsvc.HandshakeInput{
		Body:            body,
		VerifiedPeerURL: peer.InstanceURL,
		VerifiedPeerKey: peer.PublicKey,
	}

	resp, err := h.svc.Handshake(ctx, in, time.Now())
	if err != nil {
		return mapHandshakeError(c, err)
	}

	logMutation(c, "handler.Federation.Handshake",
		slog.Int64("project_id", resp.ProjectID),
		slog.String("peer", peer.InstanceURL),
	)
	return c.JSON(resp)
}

// mapHandshakeError translates a service handshake error to the wire response.
// The disclosure rules are deliberate: invalid invite/secret collapse to a
// generic 401 (US-2.2 AC4), a key mismatch is a 409 + WARN (US-2.2 AC5), and a
// version mismatch is a 400 (US-9.1 AC2).
func mapHandshakeError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, fedsvc.ErrHandshakeInvalid):
		logValidation(c, "handler.Federation.Handshake", "invalid invite/secret")
		return httpapi.ErrFederationSignatureInvalid("invalid invite")
	case errors.Is(err, fedsvc.ErrVersionUnsupported):
		logValidation(c, "handler.Federation.Handshake", "no protocol version overlap")
		return httpapi.ErrFederationVersionUnsupported()
	case errors.Is(err, fedsvc.ErrHandshakeKeyMismatch):
		logging.FromContext(c.Context()).WarnContext(c.Context(), "federation: handshake peer key mismatch",
			slog.String("op", "handler.Federation.Handshake"),
			slog.String("peer", httpapi.GetFederationPeer(c).InstanceURL),
		)
		return httpapi.ErrFederationKeyMismatch()
	case errors.Is(err, fedsvc.ErrFederationNotEnabled):
		return httpapi.ErrFederationNotEnabled()
	case errors.Is(err, fedsvc.ErrKeyMissing):
		return httpapi.ErrFederationKeyMissing()
	default:
		return httpapi.ErrInternal("handshake").WithCause(err)
	}
}

// snapshot is the owner side of the project bootstrap (Federation v1 F2.3,
// US-2.3). The HTTP-signature middleware already proved the calling peer; this
// handler additionally verifies the 15-min snapshot token (issued at handshake)
// and — on success — streams the project as NDJSON. The read is buffer-first:
// the consistent snapshot is taken into memory (releasing the lone writer
// connection) BEFORE streaming, so the bootstrap never stalls app writes (§3 /
// R1). An expired token → 401 (US-2.3 AC4); a token for a different project or a
// bad signature → 401.
func (h *FederationHandler) snapshot(c fiber.Ctx) error {
	ctx := c.Context()
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Federation.Snapshot", slog.Int64("project_id", id))

	if h.svc == nil {
		return httpapi.ErrFederationKeyMissing()
	}

	token := c.Query("token")

	// Buffer-first build: the consistent read is taken now (the writer connection
	// is released the moment the build returns) — streaming below holds no DB
	// connection.
	var snap *fedsnapshot.Snapshot
	if token == "" {
		// Token-less re-bootstrap (Federation v1 F4.2 consume half): a member whose
		// 15-min handshake token expired during a > retention offline re-snapshots
		// without a token. The HTTP-signature middleware already proved the caller's
		// instance_url + key; here we additionally require it to be a non-revoked
		// MEMBER of this project — the SAME trust the pull endpoint extends to serve a
		// signed member with no token. Without the events deps wired (no membership
		// source) we cannot make that check, so we fall back to requiring a token.
		if h.events == nil || h.events.Projects == nil {
			logValidation(c, "handler.Federation.Snapshot", "missing snapshot token")
			return httpapi.ErrFederationSignatureInvalid("snapshot token required")
		}
		peer := httpapi.GetFederationPeer(c)
		fp, gerr := h.events.Projects.Get(ctx, id, peer.InstanceURL)
		if errors.Is(gerr, repo.ErrNotFound) {
			logValidation(c, "handler.Federation.Snapshot", "re-bootstrap caller not a member")
			return httpapi.ErrFederationUntrusted("not a member of this project")
		}
		if gerr != nil {
			return httpapi.ErrInternal("snapshot membership").WithCause(gerr)
		}
		if fp.Revoked {
			logValidation(c, "handler.Federation.Snapshot", "re-bootstrap caller revoked")
			return httpapi.ErrFederationUntrusted("peer revoked")
		}
		snap, err = h.svc.BuildSnapshotForMember(ctx, id)
	} else {
		snap, err = h.svc.BuildSnapshot(ctx, id, token, time.Now())
	}
	if err != nil {
		return mapSnapshotError(c, err)
	}

	c.Set(fiber.HeaderContentType, "application/x-ndjson")
	return c.SendStreamWriter(func(w *bufio.Writer) {
		if err := fedsnapshot.WriteNDJSON(w, snap); err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "federation: stream snapshot",
				slog.String("op", "handler.Federation.Snapshot"),
				slog.String("err", err.Error()),
			)
		}
	})
}

// mapSnapshotError translates a snapshot build error to the wire response: an
// expired or invalid token is a 401 (US-2.3 AC4 — no disclosure beyond
// "unauthorized"); a missing project is a 404.
func mapSnapshotError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, fedsvc.ErrSnapshotTokenExpired):
		logValidation(c, "handler.Federation.Snapshot", "snapshot token expired")
		return httpapi.ErrFederationSignatureInvalid("snapshot token expired")
	case errors.Is(err, fedsvc.ErrSnapshotTokenInvalid):
		logValidation(c, "handler.Federation.Snapshot", "snapshot token invalid")
		return httpapi.ErrFederationSignatureInvalid("snapshot token invalid")
	case errors.Is(err, fedsvc.ErrProjectNotFound):
		return httpapi.ErrNotFound("project not found")
	case errors.Is(err, fedsvc.ErrKeyMissing):
		return httpapi.ErrFederationKeyMissing()
	default:
		return httpapi.ErrInternal("snapshot").WithCause(err)
	}
}

// defaultDisplayName derives the fallback instance display name from the host
// of baseURL (R24 — users has no display_name, so the host is the seed). Falls
// back to the raw baseURL if it cannot be parsed.
func defaultDisplayName(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return strings.TrimSpace(baseURL)
	}
	return u.Host
}

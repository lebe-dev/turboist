package handlers

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/service"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// FederationAdminHandler implements the owner-facing JWT federation control
// plane (Federation v1). F1.1 lands per-project enable; later phases add
// invites, peers, pause/resume/revoke, status, and overview on this handler.
//
// These routes are JWT-only (web owner UI) — they are NOT exposed to API tokens
// and they are NOT the signed peer-to-peer trust plane (that is FederationHandler).
type FederationAdminHandler struct {
	svc *fedsvc.Service
	// retention is the live retention-window holder backing the GET/PATCH retention
	// endpoints (Federation v1 F6.5, US-8.4). nil when federation is off (the route
	// short-circuits to CodeFederationKeyMissing). The config defaults are the
	// fallbacks surfaced for an unset window.
	retention     *fedsvc.RetentionService
	retentionDefs RetentionDefaults

	// backup is the VACUUM INTO federation-aware backup service (Federation v1 F6.5,
	// US-8.5). nil → the federation backup download route reports unavailable.
	// backupDir is where the temp VACUUM file is written before streaming.
	backup    *service.BackupService
	backupDir string
}

// RetentionDefaults are the compiled/config retention defaults (in days) surfaced
// in the GET retention response when an override is unset, so the UI can show what
// the GC will use without re-deriving the defaults client-side (Federation v1
// F6.5, US-8.4).
type RetentionDefaults struct {
	TombstoneDays int
	OutboxDays    int
	InboxDays     int
}

// NewFederationAdminHandler constructs the admin handler. svc may be nil when
// federation is not configured (no FEDERATION_KEY); in that case the routes
// short-circuit to CodeFederationKeyMissing so the UI can surface a clear
// "federation not set up" state (US-1.1 AC4 guard).
func NewFederationAdminHandler(svc *fedsvc.Service) *FederationAdminHandler {
	return &FederationAdminHandler{svc: svc}
}

// WithRetention wires the live retention-window service + its defaults so the
// admin GET/PATCH retention endpoints can read and runtime-update the GC windows
// (Federation v1 F6.5, US-8.4). Returns the handler for chaining. A nil service
// leaves those endpoints reporting CodeFederationKeyMissing.
func (h *FederationAdminHandler) WithRetention(r *fedsvc.RetentionService, defs RetentionDefaults) *FederationAdminHandler {
	h.retention = r
	h.retentionDefs = defs
	return h
}

// WithBackup wires the VACUUM INTO federation-aware backup service so the owner can
// download a physical backup that includes the federation tables + keypair
// (Federation v1 F6.5, US-8.5). backupDir is where the temp VACUUM file is written.
// Returns the handler for chaining. A nil service leaves the route unavailable.
func (h *FederationAdminHandler) WithBackup(svc *service.BackupService, backupDir string) *FederationAdminHandler {
	h.backup = svc
	h.backupDir = backupDir
	return h
}

// Register wires the federation admin routes onto r (expected to be a JWT-gated
// group, e.g. api.Group("", httpapi.RequireJWTAuth())).
func (h *FederationAdminHandler) Register(r fiber.Router) {
	r.Post("/projects/:id/federation/enable", h.enable)
	r.Post("/projects/:id/invites", h.createInvite)
	// Static segments before the parameterized :inviteId route so /revoke is not
	// swallowed by the path param.
	r.Get("/projects/:id/invites", h.listInvites)
	r.Post("/projects/:id/invites/:inviteId/revoke", h.revokeInvite)
	r.Delete("/projects/:id/invites/:inviteId", h.deleteInvite)
	r.Get("/projects/:id/federation/peers", h.listPeers)
	// Pause / resume a peer (Federation v1 F5.3, US-6.1). Static segments so they
	// are not swallowed by a path param; the peer URL rides in the body, never the
	// path, so a peer URL with a scheme/slashes need not be URL-encoded.
	r.Post("/projects/:id/federation/peers/pause", h.pausePeer)
	r.Post("/projects/:id/federation/peers/resume", h.resumePeer)
	// Permanently revoke a peer (Federation v1 F5.4, US-6.2). DELETE on the peers
	// collection with the peer URL in the body (never URL-encoded into the path).
	// Irreversible — the UI confirms before calling.
	r.Delete("/projects/:id/federation/peers", h.revokePeer)
	// Manually trust a peer's new key after a key-change incident (Federation v1
	// F5.6b, US-6.4 AC3). The operator confirms; the server fetches the peer's new
	// .well-known key, overwrites the pinned key, clears the incident, and audits.
	// Static segment + peer URL in the body so a peer URL need not be URL-encoded.
	r.Post("/projects/:id/federation/peers/trust-key", h.trustKey)
	// Voluntarily leave a JOINED federated project (Federation v1 F5.5, US-6.3): the
	// joiner sends the owner a federation_leave and marks its local copy a plain
	// editable local project. No body — the project id is in the path. Static segment
	// so it is not swallowed by a path param.
	r.Post("/projects/:id/federation/leave", h.leave)
	// Federation sync-status indicator (Federation v1 F4.3, US-4.3): one server-
	// read status per shared project. Static segment, no :id param.
	r.Get("/federation/status", h.status)
	// Privacy / federation overview (Federation v1 F6.4, US-7.1 AC1): every
	// federated project with this instance's role + the named peer list it is
	// visible to. Static segment, no :id param.
	r.Get("/federation/overview", h.overview)
	// Dead-letter diagnostics (Federation v1 F4.4, US-4.4 AC3): the parked,
	// permanently-failed outbound events the owner can inspect.
	r.Get("/federation/dead-letter", h.deadLetter)
	// Federation liveness detail (Federation v1 F6.5, US-8.1): the same liveness
	// report as the public /federation/health probe, but WITH the per-peer detail
	// (the public probe strips it). JWT-only owner read. Static segment, no :id.
	r.Get("/federation/health", h.health)
	// Retention settings (Federation v1 F6.5, US-8.4): read + runtime-update the GC
	// retention windows. The PATCH takes effect on the next GC pass without a
	// restart (atomic.Pointer holder). Static segment, no :id param.
	r.Get("/federation/retention", h.getRetention)
	r.Patch("/federation/retention", h.patchRetention)
	// Federation-aware physical backup (Federation v1 F6.5, US-8.5): a VACUUM INTO
	// snapshot of the ENTIRE database including the federation tables + keypair,
	// downloaded as a .db file. Static segment, no :id param.
	r.Get("/federation/backup", h.backupDownload)
	// Federation audit log (Federation v1 F6.3, US-7.4): the security-relevant
	// federation events the owner can browse to investigate anomalies. JWT + the
	// settings:read scope (an API token must hold it; JWT sessions always pass).
	// Static segment, no :id param.
	r.Get("/federation/audit", httpapi.RequireScope("settings:read"), h.audit)
	// Joiner-side flow (Federation v1 F2.2): the owner UI of THIS instance posts
	// the invite to our own JWT endpoints, which sign + send the handshake to the
	// owner server-to-server (the secret never travels browser→owner).
	r.Post("/federation/preview", h.preview)
	r.Post("/federation/join", h.join)
}

// enable turns on federation for a project (US-1.1). It returns the updated
// ProjectDTO (now with isFederated=true) so the caller can refresh its store in
// place without a second round-trip.
func (h *FederationAdminHandler) enable(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.FederationAdmin.Enable", slog.Int64("project_id", id))

	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.Enable", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	p, err := h.svc.EnableForProject(c.Context(), id)
	if err != nil {
		if errors.Is(err, fedsvc.ErrProjectNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		if errors.Is(err, fedsvc.ErrKeyMissing) {
			return httpapi.ErrFederationKeyMissing()
		}
		return httpapi.ErrInternal("enable federation").WithCause(err)
	}

	logMutation(c, "handler.FederationAdmin.Enable", slog.Int64("project_id", id))
	return c.JSON(dto.ProjectFromModel(*p))
}

// createInvite mints a one-time share invite for a federated project (US-1.2).
// The response carries the plaintext secret and the shareable join link exactly
// once; the secret is stored only as its SHA-256 hash (US-1.2 AC2) and rides in
// the URL fragment of the link so it never reaches the server / access logs
// (US-1.2 AC6). Defaults: max_uses=1, expires_at=now+7d (US-1.2 AC1, AC4).
func (h *FederationAdminHandler) createInvite(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.FederationAdmin.CreateInvite", slog.Int64("project_id", id))

	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.CreateInvite", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	var req dto.CreateInviteRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.FederationAdmin.CreateInvite", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}

	params := fedsvc.CreateInviteParams{
		Permissions: model.FederationPermission(req.Permissions),
		MaxUses:     req.MaxUses,
	}
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		exp, err := model.ParseUTC(*req.ExpiresAt)
		if err != nil {
			logValidation(c, "handler.FederationAdmin.CreateInvite", "invalid expiresAt")
			return httpapi.ErrValidation("invalid expiresAt")
		}
		params.ExpiresAt = &exp
	}

	res, err := h.svc.CreateInvite(c.Context(), id, params)
	if err != nil {
		if errors.Is(err, fedsvc.ErrProjectNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		if errors.Is(err, fedsvc.ErrFederationNotEnabled) {
			return httpapi.ErrFederationNotEnabled()
		}
		if errors.Is(err, fedsvc.ErrInvalidPermissions) {
			return httpapi.ErrValidation("invalid permissions")
		}
		if errors.Is(err, fedsvc.ErrInviteExpiryInPast) {
			logValidation(c, "handler.FederationAdmin.CreateInvite", "expiresAt in the past")
			return httpapi.ErrValidation("expiresAt must be in the future")
		}
		return httpapi.ErrInternal("create invite").WithCause(err)
	}

	// Never log the secret or the link — only the id and id-only metadata.
	logMutation(c, "handler.FederationAdmin.CreateInvite",
		slog.Int64("project_id", id), slog.String("invite_id", res.InviteID))

	resp := dto.CreateInviteResponse{
		InviteID:    res.InviteID,
		Secret:      res.Secret,
		Link:        res.Link,
		Permissions: string(res.Permissions),
		MaxUses:     res.MaxUses,
	}
	if res.ExpiresAt != nil {
		resp.ExpiresAt = model.FormatUTC(*res.ExpiresAt)
	}
	return c.JSON(resp)
}

// listInvites returns every invite for a project with its derived lifecycle
// status (US-1.3 AC1). The response carries NO secret — only id + metadata +
// status — so the secret is shown to the owner once at creation and never
// re-served (US-1.3 AC5).
func (h *FederationAdminHandler) listInvites(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.FederationAdmin.ListInvites", slog.Int64("project_id", id))

	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.ListInvites", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	views, err := h.svc.ListInvites(c.Context(), id)
	if err != nil {
		if errors.Is(err, fedsvc.ErrProjectNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("list invites").WithCause(err)
	}

	out := make([]dto.InviteDTO, 0, len(views))
	for _, v := range views {
		out = append(out, inviteViewToDTO(v))
	}
	return c.JSON(out)
}

// inviteViewToDTO maps a service InviteView to the wire DTO. It never copies a
// secret (the view carries none); optional timestamps render as empty strings
// when nil so the JSON shape is stable.
func inviteViewToDTO(v fedsvc.InviteView) dto.InviteDTO {
	out := dto.InviteDTO{
		InviteID:    v.InviteID,
		Permissions: string(v.Permissions),
		MaxUses:     v.MaxUses,
		UsedCount:   v.UsedCount,
		Status:      string(v.Status),
		CreatedAt:   model.FormatUTC(v.CreatedAt),
	}
	if v.ExpiresAt != nil {
		out.ExpiresAt = model.FormatUTC(*v.ExpiresAt)
	}
	if v.RevokedAt != nil {
		out.RevokedAt = model.FormatUTC(*v.RevokedAt)
	}
	if v.ConsumedAt != nil {
		out.ConsumedAt = model.FormatUTC(*v.ConsumedAt)
	}
	return out
}

// revokeInvite stamps revoked_at on an invite, flipping its derived status to
// revoked (US-1.3 AC2). It is idempotent: re-revoking returns 204 without moving
// the timestamp. An unknown invite (or one belonging to another project) → 404.
func (h *FederationAdminHandler) revokeInvite(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	inviteID := c.Params("inviteId")
	logEntry(c, "handler.FederationAdmin.RevokeInvite",
		slog.Int64("project_id", id), slog.String("invite_id", inviteID))

	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.RevokeInvite", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	if err := h.svc.RevokeInvite(c.Context(), id, inviteID); err != nil {
		if errors.Is(err, fedsvc.ErrInviteNotFound) {
			return httpapi.ErrNotFound("invite not found")
		}
		return httpapi.ErrInternal("revoke invite").WithCause(err)
	}

	logMutation(c, "handler.FederationAdmin.RevokeInvite",
		slog.Int64("project_id", id), slog.String("invite_id", inviteID))
	return c.SendStatus(fiber.StatusNoContent)
}

// deleteInvite hard-removes an invite row (US-1.3 AC3). It does NOT touch
// federated_projects — a peer that already consumed the invite stays joined. An
// unknown invite (or one belonging to another project) → 404.
func (h *FederationAdminHandler) deleteInvite(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	inviteID := c.Params("inviteId")
	logEntry(c, "handler.FederationAdmin.DeleteInvite",
		slog.Int64("project_id", id), slog.String("invite_id", inviteID))

	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.DeleteInvite", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	if err := h.svc.DeleteInvite(c.Context(), id, inviteID); err != nil {
		if errors.Is(err, fedsvc.ErrInviteNotFound) {
			return httpapi.ErrNotFound("invite not found")
		}
		return httpapi.ErrInternal("delete invite").WithCause(err)
	}

	logMutation(c, "handler.FederationAdmin.DeleteInvite",
		slog.Int64("project_id", id), slog.String("invite_id", inviteID))
	return c.SendStatus(fiber.StatusNoContent)
}

// listPeers returns every remote instance joined to a federated project, each
// with its handshake-supplied display_name and derived collaboration status
// (US-1.4 AC1/AC2/AC3). The owner self-row is excluded server-side.
// pendingDelivery is present and 0 until the Phase-3 outbox lands (US-1.4 AC4
// partial). An unknown project → 404.
func (h *FederationAdminHandler) listPeers(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.FederationAdmin.ListPeers", slog.Int64("project_id", id))

	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.ListPeers", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	views, err := h.svc.ListPeers(c.Context(), id)
	if err != nil {
		if errors.Is(err, fedsvc.ErrProjectNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("list peers").WithCause(err)
	}

	out := make([]dto.PeerDTO, 0, len(views))
	for _, v := range views {
		out = append(out, peerViewToDTO(v))
	}
	return c.JSON(out)
}

// pausePeer temporarily pauses exchange with one peer of a project without
// breaking the trust link (Federation v1 F5.3, US-6.1 AC1). The outbox stops
// fanning out to the peer (events accumulate) and the peer's inbound traffic is
// rejected 403 federation_paused. The peer URL rides in the body. Non-destructive
// and idempotent → 204. Unknown project/peer → 404.
func (h *FederationAdminHandler) pausePeer(c fiber.Ctx) error {
	return h.setPeerPaused(c, true)
}

// resumePeer un-pauses a previously paused peer (Federation v1 F5.3, US-6.1 AC2):
// the outbox resumes and flushes the events that accumulated while paused. → 204.
func (h *FederationAdminHandler) resumePeer(c fiber.Ctx) error {
	return h.setPeerPaused(c, false)
}

// setPeerPaused is the shared pause/resume handler body: parse the project id,
// require federation to be configured, bind the peer URL from the body (NOT the
// path, so a peer URL need not be URL-encoded), and delegate to the service. A
// missing project or unknown peer maps to 404; an empty instanceUrl is a 400.
func (h *FederationAdminHandler) setPeerPaused(c fiber.Ctx, paused bool) error {
	op := "handler.FederationAdmin.ResumePeer"
	if paused {
		op = "handler.FederationAdmin.PausePeer"
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, op, slog.Int64("project_id", id))

	if h.svc == nil {
		logValidation(c, op, "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	var req dto.PausePeerRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, op, "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.InstanceURL == "" {
		logValidation(c, op, "missing instanceUrl")
		return httpapi.ErrValidation("instanceUrl is required")
	}

	if paused {
		err = h.svc.PausePeer(c.Context(), id, req.InstanceURL)
	} else {
		err = h.svc.ResumePeer(c.Context(), id, req.InstanceURL)
	}
	if err != nil {
		if errors.Is(err, fedsvc.ErrProjectNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		if errors.Is(err, fedsvc.ErrPeerNotFound) {
			return httpapi.ErrNotFound("peer not found")
		}
		// A resume on a revoked peer is rejected — revoke is irreversible (US-6.2 AC5).
		if errors.Is(err, fedsvc.ErrPeerRevoked) {
			logValidation(c, op, "resume on revoked peer rejected")
			return httpapi.ErrFederationRevoked()
		}
		return httpapi.ErrInternal("set peer paused").WithCause(err)
	}

	logMutation(c, op, slog.Int64("project_id", id), slog.String("peer", req.InstanceURL), slog.Bool("paused", paused))
	return c.SendStatus(fiber.StatusNoContent)
}

// revokePeer permanently revokes one peer's access to a project (Federation v1
// F5.4, US-6.2). It flips revoked=1 (US-6.2 AC1), enqueues + delivers a signed
// federation_revoke control event to the now-revoked peer (special-cased past the
// fan-out's revoked-skip), and halts: a revoked peer is dropped from fan-out and
// its inbound traffic is rejected 403 federation_revoked. The peer URL rides in
// the body. Revoke is IRREVERSIBLE (US-6.2 AC5). → 204. Unknown project/peer → 404.
func (h *FederationAdminHandler) revokePeer(c fiber.Ctx) error {
	const op = "handler.FederationAdmin.RevokePeer"
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, op, slog.Int64("project_id", id))

	if h.svc == nil {
		logValidation(c, op, "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	var req dto.RevokePeerRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, op, "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.InstanceURL == "" {
		logValidation(c, op, "missing instanceUrl")
		return httpapi.ErrValidation("instanceUrl is required")
	}

	if err := h.svc.RevokePeer(c.Context(), id, req.InstanceURL); err != nil {
		if errors.Is(err, fedsvc.ErrProjectNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		if errors.Is(err, fedsvc.ErrPeerNotFound) {
			return httpapi.ErrNotFound("peer not found")
		}
		if errors.Is(err, fedsvc.ErrKeyMissing) {
			return httpapi.ErrFederationKeyMissing()
		}
		return httpapi.ErrInternal("revoke peer").WithCause(err)
	}

	logMutation(c, op, slog.Int64("project_id", id), slog.String("peer", req.InstanceURL))
	return c.SendStatus(fiber.StatusNoContent)
}

// trustKey manually re-trusts a peer's NEW key after a key-change incident
// (Federation v1 F5.6b, US-6.4 AC3). When a peer rotated its key, this instance
// rejected its events 401 and recorded a sticky incident (US-6.4 AC1/AC2 — NO
// auto-refetch). An operator with out-of-band confidence the rotation is genuine
// POSTs here; the server fetches the peer's CURRENT .well-known key, overwrites the
// pinned key (durable + in-memory), clears the sticky marker, and resolves the
// incident with an audit log. The peer URL rides in the body. → 204. Unknown
// project/peer → 404; a federation-off build → CodeFederationKeyMissing.
func (h *FederationAdminHandler) trustKey(c fiber.Ctx) error {
	const op = "handler.FederationAdmin.TrustKey"
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, op, slog.Int64("project_id", id))

	if h.svc == nil {
		logValidation(c, op, "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	var req dto.TrustPeerKeyRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, op, "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.InstanceURL == "" {
		logValidation(c, op, "missing instanceUrl")
		return httpapi.ErrValidation("instanceUrl is required")
	}

	if err := h.svc.TrustPeerKey(c.Context(), id, req.InstanceURL); err != nil {
		if errors.Is(err, fedsvc.ErrProjectNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		if errors.Is(err, fedsvc.ErrPeerNotFound) {
			return httpapi.ErrNotFound("peer not found")
		}
		if errors.Is(err, fedsvc.ErrTrustKeyUnavailable) {
			return httpapi.ErrFederationKeyMissing()
		}
		// A failed .well-known fetch (peer unreachable) is a transient upstream fault,
		// not a client error: surface it as a retryable 502 so the operator retries
		// rather than assuming the trust succeeded.
		return httpapi.ErrFederationUpstream().WithCause(err)
	}

	logMutation(c, op, slog.Int64("project_id", id), slog.String("peer", req.InstanceURL))
	return c.SendStatus(fiber.StatusNoContent)
}

// leave voluntarily leaves a JOINED federated project (Federation v1 F5.5,
// US-6.3). The joiner sends the owner a signed federation_leave control event (so
// the owner marks it "left" and stops fanning out, US-6.3 AC2) and marks its own
// local copy federation_lost with reason="left" — a plain editable local project
// with no further outbound sync (US-6.3 AC1/AC3). It is idempotent (re-leaving is a
// no-op success). The project id rides in the path; there is no body. → 204.
// Leaving the owner's OWN project (or a non-federated project) → 409
// federation_not_joined; an unknown project → 404.
//
// The optional ?delete=true query selects "delete" over the default "keep
// locally": the local copy is soft-deleted (cascading to tasks/sections) in the
// same transaction that marks it left. The delete is local-only — the owner still
// only receives the leave event, never an op=delete (US-6.3).
func (h *FederationAdminHandler) leave(c fiber.Ctx) error {
	const op = "handler.FederationAdmin.Leave"
	id, err := parseID(c)
	if err != nil {
		return err
	}
	deleteLocal := c.Query("delete") == "true"
	logEntry(c, op, slog.Int64("project_id", id), slog.Bool("delete", deleteLocal))

	if h.svc == nil {
		logValidation(c, op, "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	if err := h.svc.LeaveProject(c.Context(), id, deleteLocal); err != nil {
		if errors.Is(err, fedsvc.ErrProjectNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		if errors.Is(err, fedsvc.ErrNotJoined) {
			logValidation(c, op, "project is not a joined federated copy")
			return httpapi.ErrFederationNotJoined()
		}
		if errors.Is(err, fedsvc.ErrKeyMissing) {
			return httpapi.ErrFederationKeyMissing()
		}
		return httpapi.ErrInternal("leave project").WithCause(err)
	}

	logMutation(c, op, slog.Int64("project_id", id))
	return c.SendStatus(fiber.StatusNoContent)
}

// status returns the per-project federation sync-status for every shared project
// (Federation v1 F4.3, US-4.3): synced / pending / unreachable / key_mismatch.
// It is a pure server read (there is no client outbox) — the owner UI renders a
// badge on each federated project's header. Non-federated projects are absent
// (the badge is hidden for them). The response is a stable JSON array even when
// federation is off (CodeFederationKeyMissing then).
func (h *FederationAdminHandler) status(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.Status")
	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.Status", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	statuses, err := h.svc.Status(c.Context())
	if err != nil {
		return httpapi.ErrInternal("federation status").WithCause(err)
	}

	out := make([]dto.SyncStatusDTO, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, dto.SyncStatusDTO{
			ProjectId:       s.LocalProjectID,
			Status:          string(s.Status),
			PendingCount:    s.PendingCount,
			UnreachablePeer: s.UnreachablePeer,
			KeyMismatchPeer: s.KeyMismatchPeer,
		})
	}
	return c.JSON(out)
}

// overview returns the privacy/federation overview (Federation v1 F6.4, US-7.1
// AC1): every federated project with this instance's role (owner|peer|read-only)
// and the named peer list it is visible to (instanceUrl + displayName, US-7.1 AC3).
// Non-federated projects are absent. It is a pure JWT-only owner server read. A
// federation-off build surfaces CodeFederationKeyMissing; otherwise the response is
// a stable shape (an empty projects array when nothing is federated).
func (h *FederationAdminHandler) overview(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.Overview")
	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.Overview", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	rows, err := h.svc.Overview(c.Context())
	if err != nil {
		return httpapi.ErrInternal("federation overview").WithCause(err)
	}

	resp := dto.OverviewResponseDTO{Projects: make([]dto.OverviewProjectDTO, 0, len(rows))}
	for _, r := range rows {
		peers := make([]dto.PeerInstanceDTO, 0, len(r.Peers))
		for _, p := range r.Peers {
			peers = append(peers, dto.PeerInstanceDTO{InstanceUrl: p.InstanceURL, DisplayName: p.DisplayName})
		}
		resp.Projects = append(resp.Projects, dto.OverviewProjectDTO{
			ProjectId: r.LocalProjectID,
			Title:     r.Title,
			Role:      string(r.Role),
			Peers:     peers,
		})
	}
	return c.JSON(resp)
}

// deadLetter returns the parked, permanently-failed outbound events the owner can
// inspect (Federation v1 F4.4, US-4.4 AC3). These are events a peer rejected with
// a 4xx (≠429) the worker did not retry; they are excluded from the per-peer
// pending count. The response is a stable JSON array (empty when nothing failed).
func (h *FederationAdminHandler) deadLetter(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.DeadLetter")
	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.DeadLetter", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	views, err := h.svc.DeadLetter(c.Context(), 0)
	if err != nil {
		return httpapi.ErrInternal("federation dead-letter").WithCause(err)
	}

	out := make([]dto.DeadLetterDTO, 0, len(views))
	for _, v := range views {
		out = append(out, dto.DeadLetterDTO{
			EventId:         v.EventID,
			PeerInstanceUrl: v.PeerInstanceURL,
			ProjectId:       v.LocalProjectID,
			StatusCode:      v.StatusCode,
			Reason:          v.Reason,
			FailedAt:        v.FailedAt,
		})
	}
	return c.JSON(out)
}

// health returns the federation liveness report WITH per-peer detail (Federation
// v1 F6.5, US-8.1). Unlike the public /federation/health probe it includes the
// peers array, since this route is JWT-only owner access. A federation-off build
// surfaces CodeFederationKeyMissing.
func (h *FederationAdminHandler) health(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.Health")
	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.Health", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	rep, err := h.svc.Health(c.Context())
	if err != nil {
		return httpapi.ErrInternal("federation health").WithCause(err)
	}

	resp := dto.HealthResponse{
		InstanceUrl:      rep.InstanceURL,
		ProtocolVersions: rep.ProtocolVersions,
		OutboxDepth:      rep.OutboxDepth,
		Status:           string(rep.Status),
		Peers:            make([]dto.HealthPeerDTO, 0, len(rep.Peers)),
	}
	for _, p := range rep.Peers {
		hp := dto.HealthPeerDTO{
			InstanceUrl: p.InstanceURL,
			DisplayName: p.DisplayName,
			Status:      string(p.Status),
		}
		if p.LastContactAt != nil {
			hp.LastContactAt = model.FormatUTC(*p.LastContactAt)
		}
		resp.Peers = append(resp.Peers, hp)
	}
	return c.JSON(resp)
}

// getRetention returns the current retention overrides + the resolved effective
// windows (Federation v1 F6.5, US-8.4). A federation-off build (no retention
// service wired) surfaces CodeFederationKeyMissing.
func (h *FederationAdminHandler) getRetention(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.GetRetention")
	if h.retention == nil {
		logValidation(c, "handler.FederationAdmin.GetRetention", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}
	return c.JSON(h.retentionDTO())
}

// patchRetention runtime-updates the retention overrides (Federation v1 F6.5,
// US-8.4). Omitted fields keep their current value; a sent 0 reverts that window
// to its default. The change is persisted and reflected by the next GC pass
// without a restart. The outbox window's EFFECTIVE value stays clamped at 30 days
// (§16.3) even if a larger value is stored.
func (h *FederationAdminHandler) patchRetention(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.PatchRetention")
	if h.retention == nil {
		logValidation(c, "handler.FederationAdmin.PatchRetention", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	var req dto.UpdateRetentionRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.FederationAdmin.PatchRetention", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}

	// Start from the current overrides so an omitted field is preserved; apply the
	// sent fields. A negative value is a client error.
	cur := h.retention.Get()
	next := fedsvc.RetentionWindows{
		TombstoneDays: cur.TombstoneDays,
		OutboxDays:    cur.OutboxDays,
		InboxDays:     cur.InboxDays,
	}
	if req.TombstoneDays != nil {
		if *req.TombstoneDays < 0 {
			return httpapi.ErrValidation("tombstoneDays must be >= 0")
		}
		next.TombstoneDays = *req.TombstoneDays
	}
	if req.OutboxDays != nil {
		if *req.OutboxDays < 0 {
			return httpapi.ErrValidation("outboxDays must be >= 0")
		}
		next.OutboxDays = *req.OutboxDays
	}
	if req.InboxDays != nil {
		if *req.InboxDays < 0 {
			return httpapi.ErrValidation("inboxDays must be >= 0")
		}
		next.InboxDays = *req.InboxDays
	}

	if err := h.retention.Update(c.Context(), next); err != nil {
		return httpapi.ErrInternal("update retention").WithCause(err)
	}

	logMutation(c, "handler.FederationAdmin.PatchRetention",
		slog.Int("tombstoneDays", next.TombstoneDays),
		slog.Int("outboxDays", next.OutboxDays),
		slog.Int("inboxDays", next.InboxDays))
	return c.JSON(h.retentionDTO())
}

// retentionDTO renders the current overrides + the resolved effective windows.
func (h *FederationAdminHandler) retentionDTO() dto.RetentionSettingsDTO {
	w := h.retention.Get()
	cfg := h.retention.GCConfig()
	const day = 24 * time.Hour
	return dto.RetentionSettingsDTO{
		TombstoneDays:          w.TombstoneDays,
		OutboxDays:             w.OutboxDays,
		InboxDays:              w.InboxDays,
		OutboxHardcapDays:      int(fedsvc.OutboxRetentionHardcap / day),
		EffectiveTombstoneDays: int(cfg.TombstoneRetention / day),
		EffectiveOutboxDays:    int(cfg.OutboxRetention / day),
		EffectiveInboxDays:     int(cfg.InboxRetention / day),
	}
}

// backupDownload streams a federation-aware physical backup (Federation v1 F6.5,
// US-8.5): a VACUUM INTO copy of the WHOLE database — including the federation
// tables + keypair — so a restore under the same BASE_URL keeps this instance's
// federation identity (no re-handshake). It is an off-peak admin action (VACUUM
// holds the lone connection, §16 / R1). The file downloads as a .db SQLite file.
func (h *FederationAdminHandler) backupDownload(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.BackupDownload")
	if h.backup == nil {
		logValidation(c, "handler.FederationAdmin.BackupDownload", "federation backup not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	data, err := h.backup.VacuumIntoBytes(c.Context(), h.backupDir)
	if err != nil {
		return httpapi.ErrInternal("federation backup").WithCause(err)
	}

	filename := "turboist-federation-backup-" + time.Now().UTC().Format("20060102") + ".db"
	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Set("Cache-Control", "no-store")
	logMutation(c, "handler.FederationAdmin.BackupDownload", slog.Int("bytes", len(data)))
	return c.Send(data)
}

// audit returns the federation audit log (Federation v1 F6.3, US-7.4 AC1): the
// security-relevant federation events the owner can browse to investigate
// anomalies (handshake/revoke/sig-fail/digest/author-origin/clock-skew/replay/
// key-change), newest-first, with an optional ?peer= filter and limit/offset
// pagination. It also returns the "possible attack on peer X" alerts derived from
// the recent signature-failure counts (US-7.4 AC3). A federation-off build
// surfaces CodeFederationKeyMissing; otherwise the response is a stable shape
// (empty entries/alerts arrays).
func (h *FederationAdminHandler) audit(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.Audit")
	if h.svc == nil {
		logValidation(c, "handler.FederationAdmin.Audit", "federation not configured")
		return httpapi.ErrFederationKeyMissing()
	}

	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	entries, err := h.svc.Audit(c.Context(), fedsvc.AuditQuery{
		PeerInstanceURL: c.Query("peer"),
		Kind:            c.Query("kind"),
		Limit:           pp.Limit,
		Offset:          pp.Offset,
	})
	if err != nil {
		return httpapi.ErrInternal("federation audit").WithCause(err)
	}
	alerts, err := h.svc.SignatureFailureAlerts(c.Context())
	if err != nil {
		return httpapi.ErrInternal("federation audit alerts").WithCause(err)
	}

	resp := dto.AuditResponseDTO{
		Entries: make([]dto.AuditEntryDTO, 0, len(entries)),
		Alerts:  make([]dto.SignatureFailureAlertDTO, 0, len(alerts)),
	}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, dto.AuditEntryDTO{
			Id:              e.ID,
			Kind:            e.Kind,
			Outcome:         e.Outcome,
			PeerInstanceUrl: e.PeerInstanceURL,
			Detail:          e.Detail,
			CreatedAt:       e.CreatedAt,
		})
	}
	for _, a := range alerts {
		resp.Alerts = append(resp.Alerts, dto.SignatureFailureAlertDTO{
			PeerInstanceUrl: a.PeerInstanceURL,
			Count:           a.Count,
			Threshold:       a.Threshold,
		})
	}
	return c.JSON(resp)
}

// preview resolves the owner identity behind an invite WITHOUT consuming it
// (Federation v1 F2.1 preview backing, US-2.1 AC3). Our instance fetches the
// owner's public .well-known server-side so the secret never travels
// browser→owner. The project name is not known until Accept (join) consumes the
// invite, so the preview surfaces the owner identity + the locally-advertised
// protocol version.
func (h *FederationAdminHandler) preview(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.Preview")
	if h.svc == nil {
		return httpapi.ErrFederationKeyMissing()
	}

	var req dto.JoinInviteRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.FederationAdmin.Preview", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}

	pv, err := h.svc.Preview(c.Context(), req.OwnerInstanceURL)
	if err != nil {
		return mapJoinError(err)
	}
	return c.JSON(dto.JoinPreviewResponse{
		ProjectName:      "",
		OwnerInstanceURL: pv.OwnerInstanceURL,
		OwnerDisplayName: pv.OwnerDisplayName,
		Permissions:      "",
		ProtocolVersion:  pv.ProtocolVersion,
	})
}

// join accepts an invite: it signs + sends the handshake to the owner, validates
// the owner key against an independent .well-known fetch (US-2.2 AC2), persists
// the owner identity + the joiner-side mapping, warms the peer-key cache (US-2.2
// AC6), and returns the locally-mapped federated project (US-2.2 AC3). The secret
// rides in the request body to our instance only; it is never logged.
func (h *FederationAdminHandler) join(c fiber.Ctx) error {
	logEntry(c, "handler.FederationAdmin.Join")
	if h.svc == nil {
		return httpapi.ErrFederationKeyMissing()
	}

	var req dto.JoinInviteRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.FederationAdmin.Join", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}

	res, err := h.svc.Join(c.Context(), req.OwnerInstanceURL,
		fedsvc.ParsedInvite{InviteID: req.InviteID, Secret: req.Secret}, "")
	if err != nil {
		return mapJoinError(err)
	}

	logMutation(c, "handler.FederationAdmin.Join",
		slog.Int64("project_id", res.ProjectID),
		slog.String("owner", res.OwnerInstanceURL),
	)
	return c.JSON(dto.JoinResultResponse{
		ProjectID:   res.ProjectID,
		ProjectName: res.ProjectName,
		Permissions: string(res.Permissions),
	})
}

// mapJoinError translates joiner-side service errors to typed wire errors,
// including re-surfacing the owner's federation error code on a rejected
// handshake (US-2.2 AC4/AC5, US-9.1 AC2).
func mapJoinError(err error) error {
	if errors.Is(err, fedsvc.ErrKeyMissing) {
		return httpapi.ErrFederationKeyMissing()
	}
	if errors.Is(err, fedsvc.ErrOwnerInstanceMissing) {
		return httpapi.ErrValidation("owner instance url required")
	}
	if errors.Is(err, fedsvc.ErrOwnerUntrusted) {
		return httpapi.ErrFederationUntrusted("owner key not corroborated by .well-known")
	}
	var remote *fedsvc.RemoteHandshakeError
	if errors.As(err, &remote) {
		return mapRemoteHandshakeError(remote)
	}
	return httpapi.ErrInternal("federation join").WithCause(err)
}

// mapRemoteHandshakeError re-surfaces the owner's federation error to the joiner
// UI: the owner's own status/code is preserved so the join page can map a
// wrong-secret 401, a key-mismatch 409, a no-version 400, or a stale-invite 410
// to the matching typed frontend error.
func mapRemoteHandshakeError(remote *fedsvc.RemoteHandshakeError) error {
	// A genuine owner 5xx (e.g. a mid-build DB failure) is a transient upstream
	// fault, not an auth/invite rejection — surface it as a retryable 502 so the
	// user retries instead of chasing a fresh invite (F2.3 #8). Branch on this
	// FIRST so a 5xx with an unrecognized code never collapses to the generic 401.
	if remote.StatusCode >= 500 {
		return httpapi.ErrFederationUpstream()
	}
	switch remote.Code {
	case httpapi.CodeFederationVersionUnsupported:
		return httpapi.ErrFederationVersionUnsupported()
	case httpapi.CodeFederationKeyMismatch:
		return httpapi.ErrFederationKeyMismatch()
	case httpapi.CodeFederationNotEnabled:
		return httpapi.ErrFederationNotEnabled()
	case httpapi.CodeGone:
		return httpapi.ErrGone("invite no longer valid")
	default:
		// Wrong secret / unknown invite collapse to the owner's generic 401
		// (US-2.2 AC4 — no id-vs-secret disclosure leaks to the joiner either).
		return httpapi.ErrFederationSignatureInvalid("handshake rejected by owner")
	}
}

// peerViewToDTO maps a service PeerView to the wire DTO. Optional timestamps
// render as empty strings when unset so the JSON shape is stable.
func peerViewToDTO(v fedsvc.PeerView) dto.PeerDTO {
	out := dto.PeerDTO{
		InstanceURL:     v.PeerInstanceURL,
		DisplayName:     v.DisplayName,
		Permissions:     string(v.Permissions),
		Status:          string(v.Status),
		LastSentHLC:     v.LastSentHLC,
		JoinedAt:        model.FormatUTC(v.JoinedAt),
		PendingDelivery: v.PendingDelivery,
		KeyMismatchAt:   v.KeyMismatchAt,
	}
	if v.LastContactAt != nil {
		out.LastContactAt = model.FormatUTC(*v.LastContactAt)
	}
	return out
}

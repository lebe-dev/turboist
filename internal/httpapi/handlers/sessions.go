package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

// SessionsHandler exposes the current user's active sessions.
//
//	GET    /api/v1/sessions      -> list active sessions, with isCurrent flag
//	DELETE /api/v1/sessions/:id  -> revoke one session (must belong to the caller)
//
// API-token auth is rejected by RequireJWTAuth on the subgroup so a leaked
// long-lived token cannot drop the user's web sessions.
type SessionsHandler struct {
	sessions *repo.SessionRepo
}

func NewSessionsHandler(sessions *repo.SessionRepo) *SessionsHandler {
	return &SessionsHandler{sessions: sessions}
}

func (h *SessionsHandler) Register(r fiber.Router) {
	r.Get("/", h.list)
	r.Delete("/:id", h.revoke)
}

func (h *SessionsHandler) list(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	logEntry(c, "handler.Sessions.List", slog.Int64("user_id", claims.UserID))
	rows, err := h.sessions.ListActiveForUser(c.Context(), claims.UserID)
	if err != nil {
		return httpapi.ErrInternal("list sessions").WithCause(err)
	}
	out := make([]dto.SessionDTO, 0, len(rows))
	for i := range rows {
		out = append(out, dto.SessionFromModel(rows[i], claims.SessionID))
	}
	return c.JSON(out)
}

func (h *SessionsHandler) revoke(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Sessions.Revoke",
		slog.Int64("user_id", claims.UserID),
		slog.Int64("session_id", id),
	)
	if err := h.sessions.RevokeForUser(c.Context(), id, claims.UserID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("session not found")
		}
		return httpapi.ErrInternal("revoke session").WithCause(err)
	}
	logMutation(c, "handler.Sessions.Revoke",
		slog.Int64("user_id", claims.UserID),
		slog.Int64("session_id", id),
	)
	return c.SendStatus(fiber.StatusNoContent)
}

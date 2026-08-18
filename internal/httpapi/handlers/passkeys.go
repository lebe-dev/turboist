package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/service/passkey"
)

// PasskeysHandler manages the caller's registered passkeys.
//
//	GET    /api/v1/passkeys                  -> list
//	POST   /api/v1/passkeys/register/begin   -> creation options + ceremony id
//	POST   /api/v1/passkeys/register/finish  -> verify attestation, store credential
//	PATCH  /api/v1/passkeys/:id              -> rename
//	DELETE /api/v1/passkeys/:id              -> remove
//
// Enrolling a passkey grants a password-equivalent login, so the group is
// mounted behind RequireJWTAuth: a leaked API token must not be able to bolt a
// new credential onto the account.
type PasskeysHandler struct {
	svc *passkey.Service
}

func NewPasskeysHandler(svc *passkey.Service) *PasskeysHandler {
	return &PasskeysHandler{svc: svc}
}

func (h *PasskeysHandler) Register(r fiber.Router) {
	r.Get("/", h.list)
	r.Post("/register/begin", h.registerBegin)
	r.Post("/register/finish", h.registerFinish)
	r.Patch("/:id", h.rename)
	r.Delete("/:id", h.remove)
}

func (h *PasskeysHandler) list(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	logEntry(c, "handler.Passkeys.List", slog.Int64("user_id", claims.UserID))
	rows, err := h.svc.List(c.Context(), claims.UserID)
	if err != nil {
		return httpapi.ErrInternal("list passkeys").WithCause(err)
	}
	out := make([]dto.PasskeyDTO, 0, len(rows))
	for i := range rows {
		out = append(out, dto.PasskeyFromModel(rows[i]))
	}
	return c.JSON(out)
}

func (h *PasskeysHandler) registerBegin(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	logEntry(c, "handler.Passkeys.RegisterBegin", slog.Int64("user_id", claims.UserID))
	begin, err := h.svc.BeginRegistration(c.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, passkey.ErrLimitReached) {
			return httpapi.ErrLimitExceeded("passkey limit reached",
				map[string]any{"max": passkey.MaxCredentialsPerUser})
		}
		return httpapi.ErrInternal("begin passkey registration").WithCause(err)
	}
	return c.JSON(dto.PasskeyCeremonyResponse{CeremonyID: begin.CeremonyID, Options: begin.Options})
}

func (h *PasskeysHandler) registerFinish(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	var req dto.PasskeyRegisterFinishRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.CeremonyID == "" {
		return httpapi.ErrValidation("ceremonyId is required")
	}
	if len(req.Credential) == 0 {
		return httpapi.ErrValidation("credential is required")
	}
	logEntry(c, "handler.Passkeys.RegisterFinish", slog.Int64("user_id", claims.UserID))

	stored, err := h.svc.FinishRegistration(c.Context(), claims.UserID, req.CeremonyID, req.Name, req.Credential)
	if err != nil {
		switch {
		case errors.Is(err, passkey.ErrCeremonyExpired):
			return httpapi.ErrPasskeyCeremonyInvalid()
		case errors.Is(err, passkey.ErrInvalidResponse):
			return httpapi.ErrValidation("invalid authenticator response")
		case errors.Is(err, passkey.ErrAlreadyExists):
			return httpapi.ErrPasskeyExists()
		case errors.Is(err, passkey.ErrLimitReached):
			return httpapi.ErrLimitExceeded("passkey limit reached",
				map[string]any{"max": passkey.MaxCredentialsPerUser})
		default:
			return httpapi.ErrInternal("finish passkey registration").WithCause(err)
		}
	}
	return c.Status(fiber.StatusCreated).JSON(dto.PasskeyFromModel(*stored))
}

func (h *PasskeysHandler) rename(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req dto.PasskeyRenameRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	logEntry(c, "handler.Passkeys.Rename",
		slog.Int64("user_id", claims.UserID), slog.Int64("passkey_id", id))

	updated, err := h.svc.Rename(c.Context(), id, claims.UserID, req.Name)
	if err != nil {
		if errors.Is(err, passkey.ErrNotFound) {
			return httpapi.ErrNotFound("passkey not found")
		}
		return httpapi.ErrInternal("rename passkey").WithCause(err)
	}
	return c.JSON(dto.PasskeyFromModel(*updated))
}

func (h *PasskeysHandler) remove(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Passkeys.Delete",
		slog.Int64("user_id", claims.UserID), slog.Int64("passkey_id", id))

	if err := h.svc.Delete(c.Context(), id, claims.UserID); err != nil {
		if errors.Is(err, passkey.ErrNotFound) {
			return httpapi.ErrNotFound("passkey not found")
		}
		return httpapi.ErrInternal("delete passkey").WithCause(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

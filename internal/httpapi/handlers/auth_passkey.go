package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service/passkey"
)

const (
	opPasskeyLoginBegin  = "handler.Auth.PasskeyLoginBegin"
	opPasskeyLoginFinish = "handler.Auth.PasskeyLoginFinish"
)

// WithPasskeys enables the passkey login endpoints. Pass nil (or skip the call)
// to leave them unregistered.
func (h *AuthHandler) WithPasskeys(svc *passkey.Service) *AuthHandler {
	h.passkeys = svc
	return h
}

// passkeyLoginBegin issues a discoverable-login challenge. It is deliberately
// usernameless: the authenticator answers with the user handle it stored at
// registration time, so the login page needs no field at all.
func (h *AuthHandler) passkeyLoginBegin(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	if !h.limiter.Allow(c.IP()) {
		log.WarnContext(ctx, "auth: passkey login begin rate limited",
			slog.String("op", opPasskeyLoginBegin),
			slog.String("ip", c.IP()),
		)
		return httpapi.ErrAuthRateLimited()
	}

	var req dto.PasskeyLoginBeginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if !req.ClientKind.IsValid() {
		return httpapi.ErrValidation("clientKind must be web, ios, android, or cli")
	}

	begin, err := h.passkeys.BeginLogin(ctx)
	if err != nil {
		return httpapi.ErrInternal("begin passkey login").WithCause(err)
	}
	log.InfoContext(ctx, "auth: passkey login challenge issued",
		slog.String("op", opPasskeyLoginBegin),
		slog.String("client_kind", string(req.ClientKind)),
	)
	return c.JSON(dto.PasskeyCeremonyResponse{CeremonyID: begin.CeremonyID, Options: begin.Options})
}

// passkeyLoginFinish verifies the assertion and issues a session.
//
// No TOTP step follows: a passkey assertion is itself multi-factor (the
// authenticator holds the key and verifies the user), so requiring a code on
// top would add friction without adding a factor.
func (h *AuthHandler) passkeyLoginFinish(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	if !h.limiter.Allow(c.IP()) {
		log.WarnContext(ctx, "auth: passkey login finish rate limited",
			slog.String("op", opPasskeyLoginFinish),
			slog.String("ip", c.IP()),
		)
		return httpapi.ErrAuthRateLimited()
	}

	var req dto.PasskeyLoginFinishRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.CeremonyID == "" {
		return httpapi.ErrValidation("ceremonyId is required")
	}
	if len(req.Credential) == 0 {
		return httpapi.ErrValidation("credential is required")
	}
	if !req.ClientKind.IsValid() {
		return httpapi.ErrValidation("clientKind must be web, ios, android, or cli")
	}

	userID, err := h.passkeys.FinishLogin(ctx, req.CeremonyID, req.Credential)
	if err != nil {
		switch {
		case errors.Is(err, passkey.ErrCeremonyExpired):
			log.WarnContext(ctx, "auth: passkey login ceremony expired",
				slog.String("op", opPasskeyLoginFinish),
			)
			return httpapi.ErrPasskeyCeremonyInvalid()
		case errors.Is(err, passkey.ErrInvalidResponse), errors.Is(err, repo.ErrNotFound):
			log.WarnContext(ctx, "auth: passkey login rejected",
				slog.String("op", opPasskeyLoginFinish),
				slog.String("err", err.Error()),
			)
			return httpapi.ErrAuthInvalid(msgInvalidCredentials)
		default:
			return httpapi.ErrInternal("finish passkey login").WithCause(err)
		}
	}

	user, err := h.users.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrAuthInvalid(msgInvalidCredentials)
		}
		return httpapi.ErrInternal("lookup user").WithCause(err)
	}
	log.InfoContext(ctx, "auth: passkey login ok",
		slog.String("op", opPasskeyLoginFinish),
		slog.Int64("user_id", user.ID),
		slog.String("client_kind", string(req.ClientKind)),
	)
	return h.issueSession(c, user, req.ClientKind)
}

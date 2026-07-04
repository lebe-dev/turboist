package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service/totp"
)

const (
	opTOTPConfirm = "handler.TOTP.Confirm"
	opTOTPDisable = "handler.TOTP.Disable"
)

// TOTPHandler implements /auth/totp/* endpoints.
type TOTPHandler struct {
	users   *repo.UserRepo
	svc     *totp.Service
	limiter *auth.IPLimiter
}

func NewTOTPHandler(users *repo.UserRepo, svc *totp.Service, limiter *auth.IPLimiter) *TOTPHandler {
	return &TOTPHandler{users: users, svc: svc, limiter: limiter}
}

// RegisterTOTP wires /auth/totp/* under the supplied router; the router is
// expected to already enforce JWT authentication.
func (h *TOTPHandler) RegisterTOTP(r fiber.Router) {
	r.Post("/totp/setup", h.setup)
	r.Post("/totp/confirm", h.confirm)
	r.Post("/totp/disable", h.disable)
}

func (h *TOTPHandler) setup(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	user, err := h.users.Get(ctx, claims.UserID)
	if err != nil {
		return httpapi.ErrInternal("get user").WithCause(err)
	}
	info, err := h.svc.BeginSetup(ctx, user.ID, user.Username)
	if err != nil {
		if errors.Is(err, totp.ErrAlreadyEnabled) {
			log.WarnContext(ctx, "totp: setup already enabled",
				slog.String("op", "handler.TOTP.Setup"),
				slog.Int64("user_id", user.ID),
			)
			return httpapi.ErrTOTPAlreadyEnabled()
		}
		return httpapi.ErrInternal("totp setup").WithCause(err)
	}
	log.InfoContext(ctx, "totp: setup begun",
		slog.String("op", "handler.TOTP.Setup"),
		slog.Int64("user_id", user.ID),
	)
	return c.JSON(dto.TOTPSetupResponse{
		Secret:      info.Secret,
		OtpauthURL:  info.OtpauthURL,
		QRPngBase64: base64.StdEncoding.EncodeToString(info.QRPNG),
	})
}

func (h *TOTPHandler) confirm(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	if !h.limiter.Allow(c.IP()) {
		log.WarnContext(ctx, "totp: confirm rate limited",
			slog.String("op", opTOTPConfirm),
			slog.String("ip", c.IP()),
		)
		return httpapi.ErrAuthRateLimited()
	}
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	var req dto.TOTPCodeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return httpapi.ErrValidation("code is required")
	}
	codes, err := h.svc.ConfirmSetup(ctx, claims.UserID, code)
	if err != nil {
		return mapTOTPError(ctx, log, opTOTPConfirm, claims.UserID, err)
	}
	log.InfoContext(ctx, "totp: enabled",
		slog.String("op", opTOTPConfirm),
		slog.Int64("user_id", claims.UserID),
	)
	return c.JSON(dto.TOTPConfirmResponse{RecoveryCodes: codes})
}

func (h *TOTPHandler) disable(c fiber.Ctx) error {
	ctx := c.Context()
	log := logging.FromContext(ctx)
	if !h.limiter.Allow(c.IP()) {
		log.WarnContext(ctx, "totp: disable rate limited",
			slog.String("op", opTOTPDisable),
			slog.String("ip", c.IP()),
		)
		return httpapi.ErrAuthRateLimited()
	}
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	var req dto.TOTPCodeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return httpapi.ErrValidation("code is required")
	}
	if err := h.svc.Disable(ctx, claims.UserID, code); err != nil {
		return mapTOTPError(ctx, log, opTOTPDisable, claims.UserID, err)
	}
	log.InfoContext(ctx, "totp: disabled",
		slog.String("op", opTOTPDisable),
		slog.Int64("user_id", claims.UserID),
	)
	return c.SendStatus(fiber.StatusNoContent)
}

func mapTOTPError(ctx context.Context, log *slog.Logger, op string, userID int64, err error) error {
	switch {
	case errors.Is(err, totp.ErrInvalidCode):
		log.WarnContext(ctx, "totp: invalid code",
			slog.String("op", op),
			slog.Int64("user_id", userID),
		)
		return httpapi.ErrTOTPInvalidCode()
	case errors.Is(err, totp.ErrAlreadyEnabled):
		return httpapi.ErrTOTPAlreadyEnabled()
	case errors.Is(err, totp.ErrNotEnabled), errors.Is(err, totp.ErrNoPendingSetup):
		return httpapi.ErrTOTPNotEnabled()
	default:
		return httpapi.ErrInternal(op).WithCause(err)
	}
}

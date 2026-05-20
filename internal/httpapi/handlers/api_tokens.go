package handlers

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// APITokensHandler manages long-lived API tokens for external integrations.
//
//	POST   /api/v1/api-tokens      -> create a token; plaintext value returned only here
//	GET    /api/v1/api-tokens      -> list user's tokens (metadata only)
//	DELETE /api/v1/api-tokens/:id  -> revoke a token
//
// All routes require a JWT session; API-token authentication is rejected by
// the RequireJWTAuth middleware applied to the subgroup in main.go.
type APITokensHandler struct {
	repo *repo.APITokenRepo
	salt []byte
}

func NewAPITokensHandler(r *repo.APITokenRepo, salt []byte) *APITokensHandler {
	return &APITokensHandler{repo: r, salt: salt}
}

func (h *APITokensHandler) Register(r fiber.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Delete("/:id", h.delete)
}

const apiTokenNameMaxLen = 64

type apiTokenResp struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type apiTokenCreateResp struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Token     string `json:"token"`
	CreatedAt string `json:"createdAt"`
}

type apiTokenCreateReq struct {
	Name string `json:"name"`
}

func toAPITokenResp(t *model.APIToken) apiTokenResp {
	return apiTokenResp{
		ID:        t.ID,
		Name:      t.Name,
		CreatedAt: model.FormatUTC(t.CreatedAt),
	}
}

func (h *APITokensHandler) create(c fiber.Ctx) error {
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	logEntry(c, "handler.APIToken.Create", slog.Int64("user_id", userID))
	var req apiTokenCreateReq
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.APIToken.Create", "invalid JSON")
		return httpapi.ErrValidation("invalid JSON")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		logValidation(c, "handler.APIToken.Create", "name required")
		return httpapi.ErrValidation("name is required")
	}
	if len(name) > apiTokenNameMaxLen {
		logValidation(c, "handler.APIToken.Create", "name too long", slog.Int("len", len(name)))
		return httpapi.ErrValidation("name is too long")
	}

	plain, err := auth.GenerateAPIToken()
	if err != nil {
		return httpapi.ErrInternal("generate token").WithCause(err)
	}
	hash := auth.HashAPIToken(plain, h.salt)
	created, err := h.repo.Create(c.Context(), userID, name, hash)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			return httpapi.ErrConflict("token already exists")
		}
		return httpapi.ErrInternal("create api token").WithCause(err)
	}
	// Never log the plaintext token value — only the row id.
	logMutation(c, "handler.APIToken.Create",
		slog.Int64("token_id", created.ID),
		slog.Int64("user_id", userID),
		slog.String("name", created.Name),
	)
	return c.Status(fiber.StatusCreated).JSON(apiTokenCreateResp{
		ID:        created.ID,
		Name:      created.Name,
		Token:     plain,
		CreatedAt: model.FormatUTC(created.CreatedAt),
	})
}

func (h *APITokensHandler) list(c fiber.Ctx) error {
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	tokens, err := h.repo.ListByUser(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("list api tokens").WithCause(err)
	}
	out := make([]apiTokenResp, 0, len(tokens))
	for i := range tokens {
		out = append(out, toAPITokenResp(&tokens[i]))
	}
	return c.JSON(out)
}

func (h *APITokensHandler) delete(c fiber.Ctx) error {
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		logValidation(c, "handler.APIToken.Delete", "invalid id")
		return httpapi.ErrValidation("invalid id")
	}
	logEntry(c, "handler.APIToken.Delete", slog.Int64("token_id", id), slog.Int64("user_id", userID))
	if err := h.repo.Delete(c.Context(), id, userID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("api token not found")
		}
		return httpapi.ErrInternal("delete api token").WithCause(err)
	}
	logMutation(c, "handler.APIToken.Delete", slog.Int64("token_id", id), slog.Int64("user_id", userID))
	return c.SendStatus(fiber.StatusNoContent)
}

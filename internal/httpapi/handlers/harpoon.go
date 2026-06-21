package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// HarpoonHandler exposes the user's "jump pair" of harpooned task/project
// references.
//
//	GET    /api/v1/harpoon          -> returns the current pair (hydrated)
//	POST   /api/v1/harpoon/attach   -> add a reference, returns the pair
//	POST   /api/v1/harpoon/detach   -> remove a reference, returns the pair
type HarpoonHandler struct {
	harpoon *service.HarpoonService
}

func NewHarpoonHandler(harpoon *service.HarpoonService) *HarpoonHandler {
	return &HarpoonHandler{harpoon: harpoon}
}

func (h *HarpoonHandler) Register(r fiber.Router) {
	r.Get("/harpoon", httpapi.RequireScope("settings:read"), h.get)
	r.Post("/harpoon/attach", httpapi.RequireScope("settings:write"), h.attach)
	r.Post("/harpoon/detach", httpapi.RequireScope("settings:write"), h.detach)
}

type harpoonSlotResp struct {
	Kind  string `json:"kind"`
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type harpoonResp struct {
	Slots []harpoonSlotResp `json:"slots"`
}

type harpoonRefReq struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

func harpoonToResp(slots []service.HarpoonSlot) harpoonResp {
	out := make([]harpoonSlotResp, len(slots))
	for i, s := range slots {
		out[i] = harpoonSlotResp{Kind: string(s.Kind), ID: s.ID, Title: s.Title}
	}
	return harpoonResp{Slots: out}
}

// parseHarpoonRef validates and converts the request body into a model ref.
func parseHarpoonRef(c fiber.Ctx, op string) (model.HarpoonRef, error) {
	var req harpoonRefReq
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, op, "invalid JSON")
		return model.HarpoonRef{}, httpapi.ErrValidation("invalid JSON")
	}
	kind := model.HarpoonKind(req.Kind)
	if kind != model.HarpoonKindTask && kind != model.HarpoonKindProject {
		logValidation(c, op, "invalid kind", slog.String("kind", req.Kind))
		return model.HarpoonRef{}, httpapi.ErrValidation("kind must be 'task' or 'project'")
	}
	if req.ID <= 0 {
		logValidation(c, op, "invalid id", slog.Int64("id", req.ID))
		return model.HarpoonRef{}, httpapi.ErrValidation("id must be positive")
	}
	return model.HarpoonRef{Kind: kind, ID: req.ID}, nil
}

func (h *HarpoonHandler) get(c fiber.Ctx) error {
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	slots, err := h.harpoon.Get(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("load harpoon").WithCause(err)
	}
	return c.JSON(harpoonToResp(slots))
}

func (h *HarpoonHandler) attach(c fiber.Ctx) error {
	const op = "handler.Harpoon.Attach"
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	ref, err := parseHarpoonRef(c, op)
	if err != nil {
		return err
	}
	logEntry(c, op, slog.String("kind", string(ref.Kind)), slog.Int64("id", ref.ID))
	slots, err := h.harpoon.Attach(c.Context(), userID, ref)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("harpoon target not found")
		}
		return httpapi.ErrInternal("attach harpoon").WithCause(err)
	}
	logMutation(c, op, slog.String("kind", string(ref.Kind)), slog.Int64("id", ref.ID))
	return c.JSON(harpoonToResp(slots))
}

func (h *HarpoonHandler) detach(c fiber.Ctx) error {
	const op = "handler.Harpoon.Detach"
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid(msgMissingAuthClaims)
	}
	ref, err := parseHarpoonRef(c, op)
	if err != nil {
		return err
	}
	logEntry(c, op, slog.String("kind", string(ref.Kind)), slog.Int64("id", ref.ID))
	slots, err := h.harpoon.Detach(c.Context(), userID, ref)
	if err != nil {
		return httpapi.ErrInternal("detach harpoon").WithCause(err)
	}
	logMutation(c, op, slog.String("kind", string(ref.Kind)), slog.Int64("id", ref.ID))
	return c.JSON(harpoonToResp(slots))
}

package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/repo"
)

// StateHandler exposes the per-user UI state blob.
//
//	GET   /api/v1/state         -> returns the JSON object verbatim
//	PATCH /api/v1/state         -> shallow-merges the incoming object into stored state;
//	                               keys with `null` values are removed
type StateHandler struct {
	users *repo.UserRepo
}

func NewStateHandler(users *repo.UserRepo) *StateHandler {
	return &StateHandler{users: users}
}

func (h *StateHandler) Register(r fiber.Router) {
	r.Get("/state", h.get)
	r.Patch("/state", h.patch)
}

func (h *StateHandler) get(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	raw, err := h.users.GetState(c.Context(), claims.UserID)
	if err != nil {
		return httpapi.ErrInternal("load state")
	}
	c.Set("Content-Type", "application/json")
	if raw == "" {
		raw = "{}"
	}
	return c.SendString(raw)
}

func (h *StateHandler) patch(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(c.Body(), &patch); err != nil {
		return httpapi.ErrValidation("invalid JSON object")
	}

	rawCurrent, err := h.users.GetState(c.Context(), claims.UserID)
	if err != nil {
		return httpapi.ErrInternal("load state")
	}
	current := map[string]json.RawMessage{}
	if rawCurrent != "" {
		if err := json.Unmarshal([]byte(rawCurrent), &current); err != nil {
			current = map[string]json.RawMessage{}
		}
	}
	for k, v := range patch {
		if string(v) == "null" {
			delete(current, k)
			continue
		}
		current[k] = v
	}
	merged, err := json.Marshal(current)
	if err != nil {
		return httpapi.ErrInternal("encode state")
	}
	if err := h.users.SetState(c.Context(), claims.UserID, string(merged)); err != nil {
		return httpapi.ErrInternal("save state")
	}
	c.Set("Content-Type", "application/json")
	return c.Send(merged)
}

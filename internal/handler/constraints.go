package handler

import (
	"math/rand/v2"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/storage"
)

// ConstraintsHandler handles daily constraints endpoints.
type ConstraintsHandler struct {
	store *storage.Store
	cfg   *config.AppConfig
}

// NewConstraintsHandler creates a new ConstraintsHandler.
func NewConstraintsHandler(store *storage.Store, cfg *config.AppConfig) *ConstraintsHandler {
	return &ConstraintsHandler{store: store, cfg: cfg}
}

type dailyConstraintsResponse struct {
	NeedsSelection bool     `json:"needs_selection"`
	Items          []string `json:"items"`
	RerollsUsed    int      `json:"rerolls_used"`
	MaxRerolls     int      `json:"max_rerolls"`
	PoolSize       int      `json:"pool_size"`
	Confirmed      bool     `json:"confirmed"`
}

// Daily handles GET /api/constraints/daily.
func (h *ConstraintsHandler) Daily(c fiber.Ctx) error {
	if !h.cfg.Constraints.Enabled {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "constraints are disabled"})
	}

	pool, err := h.store.GetConstraintPool()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load constraint pool"})
	}

	state, err := h.store.GetDailyConstraints()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load daily constraints"})
	}

	today := time.Now().In(h.cfg.Location).Format("2006-01-02")

	if state == nil || state.Date != today {
		return c.JSON(dailyConstraintsResponse{
			NeedsSelection: true,
			Items:          []string{},
			MaxRerolls:     h.cfg.Constraints.Daily.MaxRerolls,
			PoolSize:       len(pool),
		})
	}

	return c.JSON(dailyConstraintsResponse{
		NeedsSelection: false,
		Items:          state.Items,
		RerollsUsed:    state.RerollsUsed,
		MaxRerolls:     h.cfg.Constraints.Daily.MaxRerolls,
		PoolSize:       len(pool),
		Confirmed:      state.Confirmed,
	})
}

// Roll handles POST /api/constraints/daily/roll.
func (h *ConstraintsHandler) Roll(c fiber.Ctx) error {
	if !h.cfg.Constraints.Enabled {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "constraints are disabled"})
	}

	pool, err := h.store.GetConstraintPool()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load constraint pool"})
	}
	if len(pool) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "constraint pool is empty"})
	}

	today := time.Now().In(h.cfg.Location).Format("2006-01-02")
	state, err := h.store.GetDailyConstraints()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load daily constraints"})
	}

	rerollsUsed := 0
	if state != nil && state.Date == today {
		if state.Confirmed {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "constraints already confirmed"})
		}
		// This is a reroll
		if state.RerollsUsed >= h.cfg.Constraints.Daily.MaxRerolls {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no rerolls remaining"})
		}
		rerollsUsed = state.RerollsUsed + 1
	}
	// First roll of the day: rerollsUsed stays 0

	picked := pickRandom(pool, h.cfg.Constraints.Daily.MaxConstraints)

	newState := &storage.DailyConstraintsState{
		Date:        today,
		Items:       picked,
		RerollsUsed: rerollsUsed,
		Confirmed:   false,
	}
	if err := h.store.SetDailyConstraints(newState); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save daily constraints"})
	}

	return c.JSON(dailyConstraintsResponse{
		NeedsSelection: false,
		Items:          newState.Items,
		RerollsUsed:    newState.RerollsUsed,
		MaxRerolls:     h.cfg.Constraints.Daily.MaxRerolls,
		PoolSize:       len(pool),
		Confirmed:      false,
	})
}

type swapRequest struct {
	Index int `json:"index"`
}

// Swap handles POST /api/constraints/daily/swap.
func (h *ConstraintsHandler) Swap(c fiber.Ctx) error {
	if !h.cfg.Constraints.Enabled {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "constraints are disabled"})
	}

	var req swapRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}

	today := time.Now().In(h.cfg.Location).Format("2006-01-02")
	state, err := h.store.GetDailyConstraints()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load daily constraints"})
	}
	if state == nil || state.Date != today {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no daily constraints for today"})
	}
	if state.Confirmed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "constraints already confirmed"})
	}
	if req.Index < 0 || req.Index >= len(state.Items) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "index out of bounds"})
	}
	if state.RerollsUsed >= h.cfg.Constraints.Daily.MaxRerolls {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no rerolls remaining"})
	}

	pool, err := h.store.GetConstraintPool()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load constraint pool"})
	}

	// Find items not in current selection
	currentSet := make(map[string]bool, len(state.Items))
	for _, item := range state.Items {
		currentSet[item] = true
	}
	var candidates []string
	for _, item := range pool {
		if !currentSet[item] {
			candidates = append(candidates, item)
		}
	}
	if len(candidates) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no alternative constraints available"})
	}

	replacement := candidates[rand.IntN(len(candidates))]
	state.Items[req.Index] = replacement
	state.RerollsUsed++

	if err := h.store.SetDailyConstraints(state); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save daily constraints"})
	}

	return c.JSON(dailyConstraintsResponse{
		NeedsSelection: false,
		Items:          state.Items,
		RerollsUsed:    state.RerollsUsed,
		MaxRerolls:     h.cfg.Constraints.Daily.MaxRerolls,
		PoolSize:       len(pool),
		Confirmed:      false,
	})
}

// Confirm handles POST /api/constraints/daily/confirm.
func (h *ConstraintsHandler) Confirm(c fiber.Ctx) error {
	if !h.cfg.Constraints.Enabled {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "constraints are disabled"})
	}

	today := time.Now().In(h.cfg.Location).Format("2006-01-02")
	state, err := h.store.GetDailyConstraints()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load daily constraints"})
	}
	if state == nil || state.Date != today {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no daily constraints for today"})
	}

	pool, err := h.store.GetConstraintPool()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load constraint pool"})
	}

	state.Confirmed = true
	if err := h.store.SetDailyConstraints(state); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save daily constraints"})
	}

	return c.JSON(dailyConstraintsResponse{
		NeedsSelection: false,
		Items:          state.Items,
		RerollsUsed:    state.RerollsUsed,
		MaxRerolls:     h.cfg.Constraints.Daily.MaxRerolls,
		PoolSize:       len(pool),
		Confirmed:      true,
	})
}

// pickRandom selects up to n random items from the slice without repeats.
func pickRandom(items []string, n int) []string {
	if n >= len(items) {
		result := make([]string, len(items))
		copy(result, items)
		rand.Shuffle(len(result), func(i, j int) { result[i], result[j] = result[j], result[i] })
		return result
	}

	// Fisher-Yates partial shuffle
	work := make([]string, len(items))
	copy(work, items)
	for i := 0; i < n; i++ {
		j := i + rand.IntN(len(work)-i)
		work[i], work[j] = work[j], work[i]
	}
	return work[:n]
}

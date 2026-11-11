package handlers

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/service"
)

const opTodayLock = "handler.TodayLock.Lock"

// TodayLockHandler turns on the Today focus lock.
//
//	POST /api/v1/today/lock -> starts the lock until the next midnight
//
// There is deliberately no way to lift the lock early: it ends on its own at
// midnight in the server timezone. Its state is read from the user settings
// (`todayLockUntil`), which every client already holds.
type TodayLockHandler struct {
	lock *service.TodayLockService
}

// NewTodayLockHandler constructs a TodayLockHandler.
func NewTodayLockHandler(lock *service.TodayLockService) *TodayLockHandler {
	return &TodayLockHandler{lock: lock}
}

// Register wires the route onto r (the /api/v1 group).
func (h *TodayLockHandler) Register(r fiber.Router) {
	r.Post("/today/lock", httpapi.RequireScope("settings:write"), h.lockToday)
}

type todayLockResp struct {
	// LockedUntil is the moment the lock ends: the next midnight in the server
	// timezone, or the end of the lock that was already running.
	LockedUntil string `json:"lockedUntil"`
}

func (h *TodayLockHandler) lockToday(c fiber.Ctx) error {
	logEntry(c, opTodayLock)
	until, err := h.lock.Lock(c.Context())
	if err != nil {
		return httpapi.ErrInternal("lock today").WithCause(err)
	}
	logMutation(c, opTodayLock, slog.String("until", model.FormatUTC(until)))
	return c.JSON(todayLockResp{LockedUntil: model.FormatUTC(until)})
}

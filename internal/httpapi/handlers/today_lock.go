package handlers

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/service"
)

const (
	opTodayLock      = "handler.TodayLock.Lock"
	opTodayLockState = "handler.TodayLock.State"
)

// TodayLockHandler turns on the Today focus lock.
//
//	POST /api/v1/today/lock -> starts the lock until the next midnight
//	GET  /api/v1/today/lock -> the running lock and the projects it lets into Today
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
	r.Get("/today/lock", httpapi.RequireScope("settings:read"), h.lockState)
}

type todayLockStateResp struct {
	// Active tells whether a lock runs right now.
	Active bool `json:"active"`
	// LockedUntil is the end of the running lock; null when none runs.
	LockedUntil *string `json:"lockedUntil"`
	// ProjectIDs are the projects that may take work for today while the lock
	// runs: those with open work on Today plus the always-allowed ones. Empty
	// when no lock runs.
	ProjectIDs []int64 `json:"projectIds"`
}

func (h *TodayLockHandler) lockState(c fiber.Ctx) error {
	logEntry(c, opTodayLockState)
	until, active, ids, err := h.lock.AllowedProjects(c.Context())
	if err != nil {
		return httpapi.ErrInternal("read today lock").WithCause(err)
	}
	resp := todayLockStateResp{Active: active, ProjectIDs: ids}
	if active {
		end := model.FormatUTC(until)
		resp.LockedUntil = &end
	}
	return c.JSON(resp)
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

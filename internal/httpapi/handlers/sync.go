package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/service"
)

// SyncHandler exposes POST /api/v1/sync/pull — the entry point for offline
// clients reconciling their local cache with the server.
type SyncHandler struct {
	svc     *service.SyncService
	baseURL string
}

func NewSyncHandler(svc *service.SyncService, baseURL string) *SyncHandler {
	return &SyncHandler{svc: svc, baseURL: baseURL}
}

func (h *SyncHandler) Register(r fiber.Router) {
	r.Post("/sync/pull", httpapi.RequireScope("tasks:read"), h.pull)
}

// SyncPullResponse is the body returned by POST /sync/pull.
type SyncPullResponse struct {
	Now      string           `json:"now"`
	Tasks    []dto.TaskDTO    `json:"tasks"`
	Projects []dto.ProjectDTO `json:"projects"`
	Sections []dto.SectionDTO `json:"sections"`
	Labels   []dto.LabelDTO   `json:"labels"`
	Contexts []dto.ContextDTO `json:"contexts"`
}

func (h *SyncHandler) pull(c fiber.Ctx) error {
	logEntry(c, "handler.Sync.Pull")
	var since *time.Time
	if s := c.Query("since"); s != "" {
		ts, err := model.ParseUTC(s)
		if err != nil {
			logValidation(c, "handler.Sync.Pull", "invalid since")
			return httpapi.ErrValidation("invalid since")
		}
		since = &ts
	}
	bundle, err := h.svc.Pull(c.Context(), since)
	if err != nil {
		return httpapi.ErrInternal("sync pull").WithCause(err)
	}
	resp := SyncPullResponse{
		Now:      dto.FormatTime(bundle.Now),
		Tasks:    make([]dto.TaskDTO, len(bundle.Tasks)),
		Projects: make([]dto.ProjectDTO, len(bundle.Projects)),
		Sections: make([]dto.SectionDTO, len(bundle.Sections)),
		Labels:   make([]dto.LabelDTO, len(bundle.Labels)),
		Contexts: make([]dto.ContextDTO, len(bundle.Contexts)),
	}
	for i, t := range bundle.Tasks {
		resp.Tasks[i] = dto.TaskFromModel(t, h.baseURL)
	}
	for i, p := range bundle.Projects {
		resp.Projects[i] = dto.ProjectFromModel(p)
	}
	for i, s := range bundle.Sections {
		resp.Sections[i] = dto.SectionFromModel(s)
	}
	for i, l := range bundle.Labels {
		resp.Labels[i] = dto.LabelFromModel(l)
	}
	for i, ctxRow := range bundle.Contexts {
		resp.Contexts[i] = dto.ContextFromModel(ctxRow)
	}
	return c.JSON(resp)
}

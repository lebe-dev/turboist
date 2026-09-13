package handlers

import (
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service/inboxproc"
)

const (
	opInboxProcessingStatus  = "handler.InboxProcessing.Status"
	opInboxProcessingRun     = "handler.InboxProcessing.Run"
	opInboxProcessingPreview = "handler.InboxProcessing.Preview"
	opInboxProcessingLog     = "handler.InboxProcessing.Log"
	opInboxProcessingRevert  = "handler.InboxProcessing.Revert"
	opInboxProcessingPut     = "handler.InboxProcessing.PutSettings"
)

// InboxProcessingHandler exposes the LLM Inbox processor. It is mounted even when
// the feature is disabled, so the settings page gets an honest `enabled: false`
// instead of a 404, and the journal and prompt editor keep working.
//
//	GET  /api/v1/inbox/processing                  -> status
//	POST /api/v1/inbox/processing/run              -> 202, queues an immediate run
//	POST /api/v1/inbox/processing/preview          -> {rendered}
//	GET  /api/v1/inbox/processing/log              -> paged journal
//	POST /api/v1/inbox/processing/log/:id/revert   -> TaskDTO
//	PUT  /api/v1/app-settings/inbox-processing     -> AppSettings (prompt and/or paused)
type InboxProcessingHandler struct {
	proc     *inboxproc.Processor
	journal  *repo.InboxProcessingRepo
	settings *repo.AppSettingsRepo
	baseURL  string
}

func NewInboxProcessingHandler(proc *inboxproc.Processor, journal *repo.InboxProcessingRepo, settings *repo.AppSettingsRepo, baseURL string) *InboxProcessingHandler {
	return &InboxProcessingHandler{proc: proc, journal: journal, settings: settings, baseURL: baseURL}
}

func (h *InboxProcessingHandler) Register(r fiber.Router) {
	r.Get("/inbox/processing", httpapi.RequireScope(auth.ScopeSettingsRead), h.status)
	r.Post("/inbox/processing/run", httpapi.RequireScope(auth.ScopeTasksWrite), h.run)
	r.Post("/inbox/processing/preview", httpapi.RequireScope(auth.ScopeSettingsRead), h.preview)
	r.Get("/inbox/processing/log", httpapi.RequireScope(auth.ScopeTasksRead), h.log)
	r.Post("/inbox/processing/log/:id/revert", httpapi.RequireScope(auth.ScopeTasksWrite), h.revert)
	r.Put("/app-settings/inbox-processing", httpapi.RequireScope(auth.ScopeSettingsWrite), h.putSettings)
}

type inboxRunSummaryDTO struct {
	Sorted int `json:"sorted"`
	Kept   int `json:"kept"`
	Failed int `json:"failed"`
}

type inboxProcessingStatusDTO struct {
	Enabled        bool                `json:"enabled"`
	Model          string              `json:"model"`
	APIHost        string              `json:"apiHost"`
	Interval       string              `json:"interval"`
	BatchLimit     int                 `json:"batchLimit"`
	Running        bool                `json:"running"`
	PendingCount   int                 `json:"pendingCount"`
	UndecidedCount int                 `json:"undecidedCount"`
	LastRunAt      *string             `json:"lastRunAt"`
	LastRunSummary *inboxRunSummaryDTO `json:"lastRunSummary"`
	LastError      *string             `json:"lastError"`
	BackoffUntil   *string             `json:"backoffUntil"`
	Paused         bool                `json:"paused"`
	DefaultPrompt  string              `json:"defaultPrompt"`
}

func (h *InboxProcessingHandler) status(c fiber.Ctx) error {
	logEntry(c, opInboxProcessingStatus)
	cfg := h.proc.Config()
	pending, err := h.proc.PendingCount(c.Context())
	if err != nil {
		return httpapi.ErrInternal("count pending inbox tasks").WithCause(err)
	}
	undecided, err := h.journal.UndecidedCount(c.Context())
	if err != nil {
		return httpapi.ErrInternal("count undecided inbox tasks").WithCause(err)
	}
	settings, err := h.settings.Get(c.Context())
	if err != nil {
		return httpapi.ErrInternal("load app settings").WithCause(err)
	}
	st := h.proc.Status()
	resp := inboxProcessingStatusDTO{
		Paused:         settings.InboxProcessing.Paused,
		Enabled:        h.proc.Enabled(),
		Model:          cfg.Model,
		APIHost:        apiHost(cfg.APIURL),
		Interval:       formatDuration(cfg.Interval),
		BatchLimit:     cfg.BatchLimit,
		Running:        st.Running,
		PendingCount:   pending,
		UndecidedCount: undecided,
		LastRunAt:      dto.FormatTimePtr(st.LastRunAt),
		LastError:      st.LastError,
		BackoffUntil:   dto.FormatTimePtr(st.BackoffUntil),
		DefaultPrompt:  inboxproc.DefaultPrompt,
	}
	if st.LastRunSummary != nil {
		resp.LastRunSummary = &inboxRunSummaryDTO{
			Sorted: st.LastRunSummary.Sorted,
			Kept:   st.LastRunSummary.Kept,
			Failed: st.LastRunSummary.Failed,
		}
	}
	return c.JSON(resp)
}

func (h *InboxProcessingHandler) run(c fiber.Ctx) error {
	logEntry(c, opInboxProcessingRun)
	if !h.proc.Enabled() {
		logValidation(c, opInboxProcessingRun, "inbox processing disabled")
		return httpapi.ErrInboxProcessingDisabled()
	}
	pending, err := h.proc.PendingCount(c.Context())
	if err != nil {
		return httpapi.ErrInternal("count pending inbox tasks").WithCause(err)
	}
	if pending == 0 {
		logValidation(c, opInboxProcessingRun, "nothing to process")
		return httpapi.ErrInboxNothingPending()
	}
	if !h.proc.TriggerNow() {
		logValidation(c, opInboxProcessingRun, "inbox processing disabled")
		return httpapi.ErrInboxProcessingDisabled()
	}
	logMutation(c, opInboxProcessingRun)
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"running": true})
}

type inboxPromptReq struct {
	Prompt *string `json:"prompt"`
}

func (h *InboxProcessingHandler) preview(c fiber.Ctx) error {
	logEntry(c, opInboxProcessingPreview)
	var req inboxPromptReq
	if len(c.Body()) > 0 {
		if err := c.Bind().JSON(&req); err != nil {
			logValidation(c, opInboxProcessingPreview, msgInvalidBody)
			return httpapi.ErrValidation(msgInvalidRequestBody)
		}
	}
	rendered, err := h.proc.Preview(c.Context(), req.Prompt)
	if err != nil {
		return templateErr(c, opInboxProcessingPreview, err, "render prompt preview")
	}
	return c.JSON(fiber.Map{"rendered": rendered})
}

type inboxSettingsPutReq struct {
	Prompt *string `json:"prompt"`
	Paused *bool   `json:"paused"`
}

// putSettings updates the prompt, the pause flag, or both; a field left out is
// kept as stored.
func (h *InboxProcessingHandler) putSettings(c fiber.Ctx) error {
	logEntry(c, opInboxProcessingPut)
	var req inboxSettingsPutReq
	if err := c.Bind().JSON(&req); err != nil || (req.Prompt == nil && req.Paused == nil) {
		logValidation(c, opInboxProcessingPut, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	var prompt string
	if req.Prompt != nil {
		prompt = normalizePrompt(*req.Prompt)
		if err := h.proc.ValidatePrompt(c.Context(), prompt); err != nil {
			return templateErr(c, opInboxProcessingPut, err, "validate prompt")
		}
	}
	current, err := h.settings.Get(c.Context())
	if err != nil {
		return httpapi.ErrInternal("load app settings").WithCause(err)
	}
	if req.Prompt != nil {
		current.InboxProcessing.Prompt = prompt
	}
	if req.Paused != nil {
		current.InboxProcessing.Paused = *req.Paused
	}
	if err := h.settings.Set(c.Context(), current); err != nil {
		return httpapi.ErrInternal("save app settings").WithCause(err)
	}
	attrs := []any{slog.Bool("paused", current.InboxProcessing.Paused)}
	if req.Prompt != nil {
		attrs = append(attrs, slog.Bool("default_prompt", prompt == ""), slog.Int("prompt_length", len(prompt)))
	}
	logMutation(c, opInboxProcessingPut, attrs...)
	return c.JSON(toAppSettingsResp(current))
}

// normalizePrompt stores the built-in default as the empty string, so an
// untouched prompt keeps following the default across releases.
func normalizePrompt(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	if strings.TrimSpace(prompt) == strings.TrimSpace(inboxproc.DefaultPrompt) {
		return ""
	}
	return prompt
}

func templateErr(c fiber.Ctx, op string, err error, internalMsg string) error {
	if errors.Is(err, inboxproc.ErrTemplate) {
		logValidation(c, op, "invalid prompt template", slog.String("err", err.Error()))
		detail := strings.TrimPrefix(err.Error(), inboxproc.ErrTemplate.Error()+": ")
		return httpapi.ErrUnprocessable("invalid prompt template", map[string]any{"error": detail})
	}
	return httpapi.ErrInternal(internalMsg).WithCause(err)
}

type inboxTaskBeforeDTO struct {
	LabelIDs   []int64 `json:"labelIds"`
	Priority   string  `json:"priority"`
	DueAt      *string `json:"dueAt"`
	DueHasTime bool    `json:"dueHasTime"`
}

type inboxTaskAfterDTO struct {
	ContextID int64   `json:"contextId"`
	ProjectID int64   `json:"projectId"`
	LabelIDs  []int64 `json:"labelIds"`
	Priority  string  `json:"priority"`
	DueAt     *string `json:"dueAt"`
}

type inboxProcessingLogDTO struct {
	ID               int64              `json:"id"`
	TaskID           *int64             `json:"taskId"`
	TaskTitle        string             `json:"taskTitle"`
	Outcome          string             `json:"outcome"`
	Model            string             `json:"model"`
	Reason           string             `json:"reason"`
	Confidence       *float64           `json:"confidence"`
	Before           inboxTaskBeforeDTO `json:"before"`
	After            *inboxTaskAfterDTO `json:"after"`
	Error            *string            `json:"error"`
	PromptTokens     *int               `json:"promptTokens"`
	CompletionTokens *int               `json:"completionTokens"`
	RevertedAt       *string            `json:"revertedAt"`
	CreatedAt        string             `json:"createdAt"`
}

func inboxLogFromModel(e model.InboxProcessingLogEntry) inboxProcessingLogDTO {
	out := inboxProcessingLogDTO{
		ID:        e.ID,
		TaskID:    e.TaskID,
		TaskTitle: e.TaskTitle,
		Outcome:   string(e.Outcome),
		Model:     e.Model,
		Reason:    e.Reason,
		Before: inboxTaskBeforeDTO{
			LabelIDs:   nonNilIDs(e.Before.LabelIDs),
			Priority:   string(e.Before.Priority),
			DueAt:      dto.FormatTimePtr(e.Before.DueAt),
			DueHasTime: e.Before.DueHasTime,
		},
		Confidence:       e.Confidence,
		Error:            e.Error,
		PromptTokens:     e.PromptTokens,
		CompletionTokens: e.CompletionTokens,
		RevertedAt:       dto.FormatTimePtr(e.RevertedAt),
		CreatedAt:        dto.FormatTime(e.CreatedAt),
	}
	if e.After != nil {
		out.After = &inboxTaskAfterDTO{
			ContextID: e.After.ContextID,
			ProjectID: e.After.ProjectID,
			LabelIDs:  nonNilIDs(e.After.LabelIDs),
			Priority:  string(e.After.Priority),
			DueAt:     dto.FormatTimePtr(e.After.DueAt),
		}
	}
	return out
}

func nonNilIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

func (h *InboxProcessingHandler) log(c fiber.Ctx) error {
	logEntry(c, opInboxProcessingLog)
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	entries, total, err := h.journal.ListLog(c.Context(), repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list inbox processing log").WithCause(err)
	}
	items := make([]inboxProcessingLogDTO, 0, len(entries))
	for _, e := range entries {
		items = append(items, inboxLogFromModel(e))
	}
	return c.JSON(dto.NewPagedResponse(items, total, pp.Limit, pp.Offset))
}

func (h *InboxProcessingHandler) revert(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opInboxProcessingRevert, slog.Int64("log_id", id))
	task, err := h.proc.Revert(c.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrNotFound):
			logValidation(c, opInboxProcessingRevert, "entry or task not found", slog.Int64("log_id", id))
			return httpapi.ErrNotFound("inbox processing entry or its task not found")
		case errors.Is(err, repo.ErrConflict):
			logValidation(c, opInboxProcessingRevert, "entry cannot be reverted", slog.Int64("log_id", id))
			return httpapi.ErrConflict("inbox processing entry is not a filed task or was already reverted")
		case errors.Is(err, repo.ErrInvalidPlacement), errors.Is(err, repo.ErrCycle):
			logValidation(c, opInboxProcessingRevert, msgInvalidPlacement, slog.Int64("log_id", id))
			return httpapi.ErrForbiddenPlacement("the task has become a subtask and cannot return to the inbox")
		}
		return httpapi.ErrInternal("revert inbox processing decision").WithCause(err)
	}
	logMutation(c, opInboxProcessingRevert, slog.Int64("log_id", id), slog.Int64("task_id", task.ID))
	return c.JSON(dto.TaskFromModel(*task, h.baseURL))
}

// apiHost shows where task text is sent without exposing the full URL.
func apiHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// formatDuration renders "3m" rather than Go's "3m0s".
func formatDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

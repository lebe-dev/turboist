package handlers

import (
	"encoding/json"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// syncSnapshotResp is a complete replica seed.
//
// Every collection carries the same DTO its own REST endpoint serves and the
// same entity vocabulary the delta feed uses, one field per entity:
//
//	tasks → task · projects → project · sections → section · contexts → context
//	labels → label · taskRelations → task_relation · taskTemplates → task_template
//	userSettings → user_settings · userState → user_state · appSettings → app_settings
//
// so an applier written for the delta feed needs no second vocabulary, and a
// snapshot row and the matching delta row are byte-identical.
//
// `cursor` is the log position the whole payload was read at. A client stores it
// alongside the data and continues with GET /sync/changes?since=<cursor>.
// `epoch` stamps which history that cursor belongs to.
type syncSnapshotResp struct {
	Epoch  int64 `json:"epoch"`
	Cursor int64 `json:"cursor"`

	// CompletedSince is the start of the completed-task window this payload was
	// cut at. Tasks completed before it are absent on purpose and no delta will
	// ever deliver them; a client that wants older history reads it online.
	CompletedSince string `json:"completedSince"`

	Tasks         []dto.TaskDTO             `json:"tasks"`
	Projects      []dto.ProjectDTO          `json:"projects"`
	Sections      []dto.SectionDTO          `json:"sections"`
	Contexts      []dto.ContextDTO          `json:"contexts"`
	Labels        []dto.LabelDTO            `json:"labels"`
	TaskRelations []dto.TaskRelationEdgeDTO `json:"taskRelations"`
	TaskTemplates []dto.TaskTemplateDTO     `json:"taskTemplates"`

	UserSettings settingsResp    `json:"userSettings"`
	AppSettings  appSettingsResp `json:"appSettings"`
	UserState    json.RawMessage `json:"userState"`
}

// snapshot answers GET /api/v1/sync/snapshot: the bootstrap a replica starts
// from on first launch, and the recovery it falls back to when the delta feed
// refuses to resume.
//
// There is no paging in this version. The dataset belongs to one person, and the
// completed-task window keeps the only collection that grows without bound in
// check; a client that has to page a bootstrap would need resumable paging to be
// worth anything, which is a contract this does not have to carry yet.
func (h *SyncHandler) snapshot(c fiber.Ctx) error {
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	logEntry(c, opSyncSnapshot, slog.Int64("user_id", userID))

	snap, err := h.changes.Snapshot(c.Context(), repo.SyncSnapshotQuery{UserID: userID})
	if err != nil {
		return httpapi.ErrInternal("read sync snapshot").WithCause(err)
	}

	slog.DebugContext(c.Context(), "sync snapshot composed",
		slog.String("op", opSyncSnapshot),
		slog.Int64("cursor", snap.Cursor),
		slog.Int64("epoch", snap.Epoch),
		slog.Int("tasks", len(snap.Tasks)))

	return c.JSON(syncSnapshotResp{
		Epoch:          snap.Epoch,
		Cursor:         snap.Cursor,
		CompletedSince: model.FormatUTC(snap.CompletedSince),
		Tasks:          renderSnapshot(snap.Tasks, func(t *model.Task) dto.TaskDTO { return dto.TaskFromModel(*t, h.baseURL) }),
		Projects:       renderSnapshot(snap.Projects, func(p *model.Project) dto.ProjectDTO { return dto.ProjectFromModel(*p) }),
		Sections:       renderSnapshot(snap.Sections, func(s *model.ProjectSection) dto.SectionDTO { return dto.SectionFromModel(*s) }),
		Contexts:       renderSnapshot(snap.Contexts, func(x *model.Context) dto.ContextDTO { return dto.ContextFromModel(*x) }),
		Labels:         renderSnapshot(snap.Labels, func(l *model.Label) dto.LabelDTO { return dto.LabelFromModel(*l) }),
		TaskRelations:  renderSnapshot(snap.TaskRelations, func(r *model.TaskRelation) dto.TaskRelationEdgeDTO { return dto.TaskRelationEdgeFromModel(*r) }),
		TaskTemplates:  renderSnapshot(snap.TaskTemplates, func(t *model.TaskTemplate) dto.TaskTemplateDTO { return dto.TaskTemplateFromModel(*t) }),
		UserSettings:   toResp(snap.UserSettings),
		AppSettings:    toAppSettingsResp(snap.AppSettings),
		UserState:      json.RawMessage(snap.UserState),
	})
}

// renderSnapshot maps one collection onto its DTO. It is sized up front, so an
// empty collection marshals to [] rather than null — a client decoding into a
// non-nullable list must not have to special-case an empty workspace.
func renderSnapshot[M any, D any](items []M, render func(M) D) []D {
	out := make([]D, len(items))
	for i, item := range items {
		out[i] = render(item)
	}
	return out
}

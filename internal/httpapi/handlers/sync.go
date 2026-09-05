package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

const (
	opSyncChanges  = "handler.Sync.Changes"
	opSyncSnapshot = "handler.Sync.Snapshot"
)

// SyncHandler serves the delta feed a full local replica catches up from.
//
//	GET /api/v1/sync/changes?since=<seq>&epoch=<n>&limit=<..500>
//
// A caller applies one page, stores the returned cursor with it, and asks again
// with since=cursor until hasMore is false. Applying is idempotent — every
// change is the entity's current row, or a tombstone — so a page replayed after
// a crash costs nothing but the work of writing it twice.
//
// The endpoint adds no reads that the REST API does not already offer: each
// payload is the very DTO the corresponding GET serves, so a replica assembled
// from deltas holds exactly what it would have fetched endpoint by endpoint.
//
// The same handler serves the bootstrap the delta loop starts from:
//
//	GET /api/v1/sync/snapshot
//
// which answers with every syncable entity plus the {epoch, cursor} to continue
// at. A client takes it on first launch and whenever the delta feed refuses to
// resume — the epoch moved, or the history it needed was pruned.
type SyncHandler struct {
	changes *repo.ChangeLogRepo
	baseURL string
}

func NewSyncHandler(changes *repo.ChangeLogRepo, baseURL string) *SyncHandler {
	return &SyncHandler{changes: changes, baseURL: baseURL}
}

// Register wires the delta feed onto the given router.
//
// The scope set mirrors what the payload can expose. One page can carry tasks,
// projects, sections, contexts, labels, templates and both settings blobs, so an
// API token must hold every one of those read scopes — reading the whole
// workspace through this endpoint must not be cheaper than reading it through
// the endpoints it mirrors. JWT sessions bypass scope checks, so the apps are
// unaffected.
func (h *SyncHandler) Register(r fiber.Router) {
	guard := httpapi.RequireAllScopes(
		auth.ScopeTasksRead,
		auth.ScopeProjectsRead,
		auth.ScopeSectionsRead,
		auth.ScopeContextsRead,
		auth.ScopeLabelsRead,
		auth.ScopeTemplatesRead,
		auth.ScopeSettingsRead,
	)
	r.Get("/changes", guard, h.changesPage)
	r.Get("/snapshot", guard, h.snapshot)
}

// syncChangeResp is one entity's net change. `id` is always present — it is the
// only thing a delete carries, and the settings blobs have no id of their own
// inside their payload — while `data` appears on upserts only.
type syncChangeResp struct {
	Entity string `json:"entity"`
	Op     string `json:"op"`
	Seq    int64  `json:"seq"`
	ID     int64  `json:"id"`
	Data   any    `json:"data,omitempty"`
}

type syncChangesResp struct {
	Epoch   int64            `json:"epoch"`
	Cursor  int64            `json:"cursor"`
	HasMore bool             `json:"hasMore"`
	Changes []syncChangeResp `json:"changes"`
}

func (h *SyncHandler) changesPage(c fiber.Ctx) error {
	since, err := parseSyncInt(c, "since", 0)
	if err != nil {
		return err
	}
	limit, err := parseSyncInt(c, "limit", 0)
	if err != nil {
		return err
	}
	if limit > repo.MaxSyncChangeLimit {
		logValidation(c, opSyncChanges, "limit above maximum", slog.Int64("limit", limit))
		return httpapi.ErrValidation(fmt.Sprintf("limit must not exceed %d", repo.MaxSyncChangeLimit))
	}
	query := repo.SyncChangesQuery{Since: since, Limit: int(limit)}
	if raw := c.Query("epoch"); raw != "" {
		epoch, err := parseSyncInt(c, "epoch", 0)
		if err != nil {
			return err
		}
		query.Epoch = &epoch
	}
	logEntry(c, opSyncChanges,
		slog.Int64("since", since),
		slog.Int64("limit", limit))

	page, err := h.changes.Changes(c.Context(), query)
	if err != nil {
		return h.mapChangesError(c, err)
	}

	out := make([]syncChangeResp, len(page.Changes))
	for i, ch := range page.Changes {
		data, err := h.render(ch.Payload)
		if err != nil {
			return httpapi.ErrInternal("render change payload").WithCause(err)
		}
		out[i] = syncChangeResp{Entity: ch.Entity, Op: ch.Op, Seq: ch.Seq, ID: ch.EntityID, Data: data}
	}
	return c.JSON(syncChangesResp{
		Epoch:   page.Epoch,
		Cursor:  page.Cursor,
		HasMore: page.HasMore,
		Changes: out,
	})
}

// mapChangesError turns the two "start over from a snapshot" conditions into
// their wire codes. Both are expected states a replica handles, not failures, so
// they are logged at WARN with the numbers that explain them rather than
// bubbling up as internal errors.
func (h *SyncHandler) mapChangesError(c fiber.Ctx, err error) error {
	var mismatch *repo.SyncEpochMismatchError
	if errors.As(err, &mismatch) {
		logValidation(c, opSyncChanges, "sync epoch mismatch", slog.Int64("current_epoch", mismatch.Current))
		return httpapi.ErrSyncEpochMismatch(mismatch.Current)
	}
	var expired *repo.SyncCursorExpiredError
	if errors.As(err, &expired) {
		logValidation(c, opSyncChanges, "sync cursor expired",
			slog.Int64("since", expired.Since),
			slog.Int64("oldest_retained", expired.OldestRetained))
		return httpapi.ErrSyncCursorExpired(expired.Epoch, expired.OldestRetained)
	}
	return httpapi.ErrInternal("read changes").WithCause(err)
}

// render maps a hydrated row onto the DTO its own REST endpoint serves, so the
// two can never drift apart.
func (h *SyncHandler) render(payload any) (any, error) {
	switch v := payload.(type) {
	case nil:
		return nil, nil
	case *model.Task:
		return dto.TaskFromModel(*v, h.baseURL), nil
	case *model.Project:
		return dto.ProjectFromModel(*v), nil
	case *model.ProjectSection:
		return dto.SectionFromModel(*v), nil
	case *model.Context:
		return dto.ContextFromModel(*v), nil
	case *model.Label:
		return dto.LabelFromModel(*v), nil
	case *model.TaskRelation:
		return dto.TaskRelationEdgeFromModel(*v), nil
	case *model.TaskTemplate:
		return dto.TaskTemplateFromModel(*v), nil
	case *model.UserSettings:
		return toResp(v), nil
	case *model.AppSettings:
		return toAppSettingsResp(v), nil
	case repo.SyncUserState:
		return json.RawMessage(v), nil
	}
	return nil, fmt.Errorf("no DTO for sync payload %T", payload)
}

// parseSyncInt reads a non-negative integer query parameter, defaulting when it
// is absent. A malformed or negative value is rejected rather than clamped: a
// cursor is the client's own bookkeeping, and silently reinterpreting it would
// hand back a page that does not answer the question asked.
func parseSyncInt(c fiber.Ctx, name string, def int64) (int64, error) {
	raw := c.Query(name)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		logValidation(c, opSyncChanges, "invalid query parameter", slog.String("param", name), slog.String("value", raw))
		return 0, httpapi.ErrValidation(name + " must be a non-negative integer")
	}
	return v, nil
}

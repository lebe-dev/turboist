package snapshot

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ReApplyParams carry the per-re-bootstrap inputs (Federation v1 F4.2, US-4.2).
// Unlike ApplyParams (initial bootstrap, which CREATES a brand-new local
// project), ReApply OVERWRITES an EXISTING local project the joiner already holds
// — identified by LocalProjectID — when the joiner has fallen behind retention
// and the owner answered its pull with a 410 stale_pull (the consume half of
// US-3.7 AC4).
type ReApplyParams struct {
	// LocalProjectID is the joiner's EXISTING local project (mapped to the owner)
	// whose state is overwritten in place. It is NOT recreated.
	LocalProjectID int64
	// OwnerInstanceURL is the owner the joined mapping row points at; it scopes
	// which federated_projects row carries the re-bootstrap marker.
	OwnerInstanceURL string
	// Reader streams the NDJSON snapshot body the owner returned.
	Reader io.Reader
	// Now is injectable for a deterministic re-bootstrap cutoff X; nil → time.Now.
	Now func() time.Time
}

// ReApplyResult is the outcome of a successful re-bootstrap. CutoffHLC is the
// snapshot's as_of_hlc (the causal cutoff X); RebootstrappedAt is the wall-clock
// the overwrite committed at (the human-readable X the re-sync banner renders).
type ReApplyResult struct {
	LocalProjectID   int64
	CutoffHLC        string
	RebootstrappedAt string
}

// ReApply overwrites an EXISTING joined federated project from a fresh owner
// snapshot, preserving the joiner's unsent outbox (Federation v1 F4.2, US-4.2).
//
// The whole overwrite runs in ONE transaction so a mid-stream failure rolls
// everything back (no half-overwritten project). Within the tx it, by
// cross-instance client_id:
//
//  1. upserts the project's federated fields (title/desc/color/status), clearing
//     any local tombstone;
//  2. upserts the snapshot's sections + live tasks (insert missing, update
//     existing — no duplicate rows);
//  3. converges the project: any LIVE local task with a client_id that is NOT in
//     the snapshot's live set is soft-deleted (it was removed upstream) — EXCEPT a
//     task the joiner created locally that still has an unsent federation_outbox
//     event (its own offline work, which the owner never received and so cannot be
//     in the snapshot; it survives, US-4.2 AC2/AC3) — and every snapshot tombstone
//     is applied as a soft-delete (no resurrection, US-4.2 AC2);
//  4. writes each per-field HLC with the higher-wins upsert, so a field the joiner
//     has locally advanced PAST the snapshot is NOT regressed (US-4.2 AC3 — an
//     old-HLC value loses LWW gracefully). It also records the synthetic _deleted
//     field HLC for every tombstone so a later stale update cannot resurrect it;
//  5. advances last_received_hlc to as_of and stamps the re-bootstrap marker
//     (cutoff X) on the joined mapping row (US-4.2 AC4).
//
// CRITICALLY (R3 — the highest-impact F4.2 bug), ReApply NEVER touches
// federation_outbox: the joiner's unsent local edits survive the re-bootstrap and
// are flushed afterwards (events with HLC < as_of still flush; peer LWW resolves).
func ReApply(ctx context.Context, deps ApplyDeps, params ReApplyParams) (*ReApplyResult, error) {
	now := params.Now
	if now == nil {
		now = time.Now
	}
	if deps.Snapshot == nil && deps.DB != nil {
		deps.Snapshot = repo.NewFederationSnapshotRepo(deps.DB)
	}

	snap, err := parseStream(params.Reader)
	if err != nil {
		return nil, err
	}
	// Reject a snapshot carrying a malformed HLC before persisting anything (same
	// guard the initial Apply uses, F2.3 #7): a bad as_of/field HLC would corrupt
	// the pull cursor / LWW ordering, so abort and roll back.
	if err := validateHLCs(snap); err != nil {
		return nil, err
	}

	nowWall := model.FormatUTC(now())
	res := ReApplyResult{
		LocalProjectID:   params.LocalProjectID,
		CutoffHLC:        snap.AsOfHLC,
		RebootstrappedAt: nowWall,
	}

	// Index the snapshot's per-field HLCs by entity client_id → field → HLC so the
	// re-bootstrap can gate each federated COLUMN write by the SAME per-field HLC
	// the inbox Applier uses. A field with no explicit snapshot HLC line falls back
	// to as_of (the snapshot's causal moment), so a field the joiner has NOT
	// advanced still converges, while a field the joiner advanced PAST the snapshot
	// keeps both its local column AND its local HLC (US-4.2 AC3 — the field that won
	// LWW shows the value that won, never silently regressed to the losing value).
	fieldHLCByEntity := indexFieldHLCs(snap)

	err = db.WithTx(ctx, deps.DB, func(tx *sql.Tx) error {
		localProjectID := params.LocalProjectID

		// (1) Overwrite the existing project's federated fields in place, gating each
		// column by the per-field HLC so a locally-advanced field is not regressed.
		projectWon := wonFunc(ctx, tx, deps.Snapshot, entityTypeProject, snap.Project.ClientID, fieldHLCByEntity, snap.AsOfHLC)
		if err := deps.Snapshot.UpsertProjectTx(ctx, tx, localProjectID, repo.SnapshotProject{
			ClientID:    snap.Project.ClientID,
			Title:       snap.Project.Title,
			Description: snap.Project.Description,
			Color:       snap.Project.Color,
			Status:      snap.Project.Status,
			UpdatedAt:   snap.Project.UpdatedAt,
		}, projectWon); err != nil {
			return err
		}

		contextID, err := projectContextIDTx(ctx, tx, localProjectID)
		if err != nil {
			return err
		}

		// (2) Upsert sections, building the client_id → local int64 map for task links.
		// Converge sections BEFORE upserting: soft-delete any local live section
		// whose client_id is not in the snapshot's live OR tombstoned section set
		// (removed upstream), mirroring the task sweep. Preserves the joiner's own
		// unsent-create sections (R3). Without this, an owner-deleted section lingers
		// as a ghost on the joiner after re-bootstrap (Federation v1 F4.2).
		if err := deps.Snapshot.SoftDeleteLiveSectionsNotInTx(ctx, tx, localProjectID, keepSectionClientIDs(snap), nowWall); err != nil {
			return err
		}
		sectionLocalByClient := map[string]int64{}
		for _, s := range snap.Sections {
			sid, err := deps.Snapshot.UpsertSectionTx(ctx, tx, localProjectID, repo.SnapshotSection{
				ClientID:  s.ClientID,
				Title:     s.Title,
				Position:  s.Position,
				CreatedAt: s.CreatedAt,
				UpdatedAt: s.UpdatedAt,
			})
			if err != nil {
				return err
			}
			if s.ClientID != "" {
				sectionLocalByClient[s.ClientID] = sid
			}
		}

		// (3a) Converge live tasks: soft-delete any local live task whose client_id is
		// not in the snapshot's live OR tombstoned set (removed upstream). Do this
		// BEFORE upserting so a task that simply moved section/parent is re-upserted,
		// not double-handled.
		keep := keepClientIDs(snap)
		if err := deps.Snapshot.SoftDeleteLiveTasksNotInTx(ctx, tx, localProjectID, keep, nowWall); err != nil {
			return err
		}

		// (3b) Upsert the snapshot's live tasks, resolving section + parent links.
		taskLocalByClient := map[string]int64{}
		for _, tk := range snap.Tasks {
			var sectionID *int64
			if tk.SectionClientID != "" {
				if sid, ok := sectionLocalByClient[tk.SectionClientID]; ok {
					sectionID = &sid
				}
			}
			var parentID *int64
			if tk.ParentClientID != "" {
				if pid, ok := taskLocalByClient[tk.ParentClientID]; ok {
					parentID = &pid
				}
			}
			taskWon := wonFunc(ctx, tx, deps.Snapshot, entityTypeTask, tk.ClientID, fieldHLCByEntity, snap.AsOfHLC)
			localTaskID, err := deps.Snapshot.UpsertTaskTx(ctx, tx, localProjectID, repo.SnapshotTask{
				ClientID:        tk.ClientID,
				ContextID:       contextID,
				Title:           tk.Title,
				Description:     tk.Description,
				Priority:        tk.Priority,
				Status:          tk.Status,
				DueAt:           tk.DueAt,
				DueHasTime:      tk.DueHasTime,
				DeadlineAt:      tk.DeadlineAt,
				DeadlineHasTime: tk.DeadlineHasTime,
				CompletedAt:     tk.CompletedAt,
				SectionID:       sectionID,
				ParentID:        parentID,
				CreatedAt:       tk.CreatedAt,
				UpdatedAt:       tk.UpdatedAt,
			}, taskWon)
			if err != nil {
				return err
			}
			if tk.ClientID != "" {
				taskLocalByClient[tk.ClientID] = localTaskID
			}
		}

		// (3c) Apply every snapshot tombstone as a soft-delete + record the synthetic
		// _deleted field HLC so a later stale update cannot resurrect it.
		for _, tomb := range snap.Tombstones {
			table := tableForEntityType(tomb.EntityType)
			if table != "" {
				if err := deps.Snapshot.SoftDeleteByClientIDTx(ctx, tx, table, tomb.EntityID, nowWall); err != nil {
					return err
				}
			}
			if err := deps.Snapshot.InsertFieldHLCTx(ctx, tx, tomb.EntityType, tomb.EntityID, "_deleted", snap.AsOfHLC); err != nil {
				return err
			}
		}

		// (4) Rewrite per-field HLC with the higher-wins upsert (forward-safe): a
		// field the joiner advanced past the snapshot keeps its higher HLC (and thus
		// its local value), so an old-HLC snapshot value loses LWW gracefully.
		for _, fh := range snap.FieldHLCs {
			if err := deps.Snapshot.InsertFieldHLCTx(ctx, tx, fh.EntityType, fh.EntityID, fh.Field, fh.HLC); err != nil {
				return err
			}
		}

		// (5) Advance the cursor to as_of + stamp the re-bootstrap marker (cutoff X).
		// This NEVER touches federation_outbox (R3).
		return deps.FedProjects.MarkReBootstrapTx(ctx, tx, localProjectID, params.OwnerInstanceURL, snap.AsOfHLC, nowWall)
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// keepClientIDs gathers the client_ids of every entity the snapshot still carries
// (live tasks + tombstones) so the convergence sweep does not soft-delete a task
// the snapshot does describe. A tombstoned task is in the keep set (it is handled
// explicitly by the tombstone pass) so the sweep does not double-touch it.
func keepClientIDs(snap *Snapshot) []string {
	out := make([]string, 0, len(snap.Tasks)+len(snap.Tombstones))
	for _, tk := range snap.Tasks {
		if tk.ClientID != "" {
			out = append(out, tk.ClientID)
		}
	}
	for _, tomb := range snap.Tombstones {
		if tomb.EntityType == entityTypeTask && tomb.EntityID != "" {
			out = append(out, tomb.EntityID)
		}
	}
	return out
}

// keepSectionClientIDs is the section analogue of keepClientIDs: the client_ids
// of every section the snapshot still describes (live + tombstoned), so the
// section convergence sweep does not soft-delete a section the snapshot carries.
func keepSectionClientIDs(snap *Snapshot) []string {
	out := make([]string, 0, len(snap.Sections)+len(snap.Tombstones))
	for _, s := range snap.Sections {
		if s.ClientID != "" {
			out = append(out, s.ClientID)
		}
	}
	for _, tomb := range snap.Tombstones {
		if tomb.EntityType == entityTypeSection && tomb.EntityID != "" {
			out = append(out, tomb.EntityID)
		}
	}
	return out
}

// indexFieldHLCs groups the snapshot's per-field HLC lines by entity client_id
// then field name, so the column-write gate can look up the snapshot HLC for any
// (entity, field) in O(1).
func indexFieldHLCs(snap *Snapshot) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, fh := range snap.FieldHLCs {
		byField := out[fh.EntityID]
		if byField == nil {
			byField = map[string]string{}
			out[fh.EntityID] = byField
		}
		byField[fh.Field] = fh.HLC
	}
	return out
}

// wonFunc builds the per-field HLC gate ReApply hands to the gated upserts: it
// reports, for one entity, whether the snapshot's value for a field should
// overwrite the live column. The snapshot wins ONLY when its field HLC strictly
// exceeds the joiner's STORED HLC for that field (the same higher-wins compare the
// inbox Applier uses via CASFieldHLC). A field the joiner advanced locally past
// the snapshot loses here, so its column keeps the local value — consistent with
// the local HLC the higher-wins merge preserves (US-4.2 AC3). A field with no
// explicit snapshot HLC line falls back to as_of (the snapshot's causal moment).
// The stored HLC is read once per field and cached for the duration of the upsert.
func wonFunc(ctx context.Context, tx *sql.Tx, snapRepo *repo.FederationSnapshotRepo, entityType, clientID string, byEntity map[string]map[string]string, asOf string) repo.FieldWonFunc {
	cache := map[string]bool{}
	return func(field string) bool {
		if decided, ok := cache[field]; ok {
			return decided
		}
		snapHLC := asOf
		if byField, ok := byEntity[clientID]; ok {
			if h, ok := byField[field]; ok && h != "" {
				snapHLC = h
			}
		}
		stored, err := snapRepo.StoredFieldHLCTx(ctx, tx, entityType, clientID, field)
		if err != nil {
			// On a read error, refuse to overwrite the local value — fail safe toward
			// preserving the joiner's data (the surrounding tx will surface the error
			// elsewhere if it is real; a missing row returns "" here, not an error).
			cache[field] = false
			return false
		}
		won := hlc.CompareString(snapHLC, stored) > 0
		cache[field] = won
		return won
	}
}

// tableForEntityType maps a snapshot entity_type to its local table for the
// tombstone soft-delete. Only the federated entity types that carry a deleted_at
// (task/section) are supported; project tombstones never appear in a snapshot
// (the snapshot IS the live project).
func tableForEntityType(entityType string) string {
	switch entityType {
	case entityTypeTask:
		return "tasks"
	case entityTypeSection:
		return "project_sections"
	default:
		return ""
	}
}

// projectContextIDTx returns the context the existing local project hangs off, so
// a task the re-bootstrap newly materialises (one the joiner never saw) carries a
// valid context_id (the tasks placement CHECK requires a project task to have a
// non-NULL context_id; it is local, not synced).
func projectContextIDTx(ctx context.Context, tx *sql.Tx, localProjectID int64) (int64, error) {
	var contextID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT context_id FROM projects WHERE id = ?`, localProjectID).Scan(&contextID); err != nil {
		return 0, fmt.Errorf("re-apply read project context: %w", err)
	}
	return contextID, nil
}

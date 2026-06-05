package federation

import (
	"context"
	"database/sql"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ProjectMutator routes user-originated project mutations through the federation
// Emitter so an UPDATE or DELETE of a FEDERATED project emits the per-field HLC
// bump + signed outbox event the sync workers fan out (Federation v1 F3.1,
// US-3.2 AC1). A project is its own federated entity (events.EntityProject): the
// project's own client_id is the event EntityID and its own id is LocalProjectID.
//
// A mutation on a NON-federated project incurs zero federation overhead — the
// Emitter gate is keyed on projects.is_federated. When federation is OFF the
// handler holds a nil *ProjectMutator and bypasses it entirely.
type ProjectMutator struct {
	emitter  *Emitter
	projects *repo.ProjectRepo
}

// NewProjectMutator constructs the project federation facade over the Emitter +
// project repo. Wire emitter.WithCommitPing(worker.Ping) before passing it so a
// federated mutation pushes immediately (NFR-1.1).
func NewProjectMutator(emitter *Emitter, projects *repo.ProjectRepo) *ProjectMutator {
	return &ProjectMutator{emitter: emitter, projects: projects}
}

// Create inserts a project. A freshly created project is NEVER born federated —
// federation is enabled later via the owner admin flow (Service.EnableForProject),
// and a joined peer's project is materialised from the bootstrap SNAPSHOT, not an
// op=create event. So there is no federated project to emit into at create time;
// this is a plain insert by design (the Emitter gate would short-circuit anyway,
// the new row not yet existing). It is provided for symmetry with the task path
// and so the handler has one mutator seam for all project writes. clientID is the
// project's cross-instance client_id (a fresh ULID). Returns the new local id.
func (m *ProjectMutator) Create(ctx context.Context, in repo.CreateProject, clientID string) (int64, error) {
	var newID int64
	err := db.WithTx(ctx, m.emitter.db, func(tx *sql.Tx) error {
		id, err := m.projects.CreateTx(ctx, tx, in, clientID)
		if err != nil {
			return err
		}
		newID = id
		return nil
	})
	if err != nil {
		return 0, err
	}
	return newID, nil
}

// Update applies a project field update and, when the project is federated, emits
// a signed op=update event carrying ONLY the changed federated fields (per-field
// LWW, US-3.3 AC1) — in ONE transaction with the row update (NFR-2). When the
// update touches no federated field (e.g. only context_id / project_type / pin),
// the domain write still runs but no event is emitted.
func (m *ProjectMutator) Update(ctx context.Context, projectID int64, u repo.ProjectUpdate) error {
	fields := projectUpdateFields(u)
	if len(fields) == 0 {
		_, err := m.projects.Update(ctx, projectID, u)
		return err
	}
	return m.emitter.EmitMutation(ctx, MutationSpec{
		LocalProjectID: projectID,
		EntityType:     events.EntityProject,
		EntityID:       m.projectClientID(ctx, projectID),
		Op:             events.OpUpdate,
		Fields:         fields,
	}, func(tx *sql.Tx) error {
		return m.projects.UpdateTx(ctx, tx, projectID, u)
	})
}

// UpdateStatus changes the project status (archive/complete/cancel/open) and,
// when the project is federated, emits a signed op=update event carrying ONLY the
// status field — in ONE transaction with the row update (NFR-2, US-3.2 AC1). The
// domain write also clears troiki_category when leaving 'open', but that is a
// turboist-local side effect and is NOT emitted (the federated set is just
// status; the receiver maps status→projects.status). A non-federated project runs
// the bare repo status update with zero federation overhead.
func (m *ProjectMutator) UpdateStatus(ctx context.Context, projectID int64, status model.ProjectStatus) error {
	return m.emitter.EmitMutation(ctx, MutationSpec{
		LocalProjectID: projectID,
		EntityType:     events.EntityProject,
		EntityID:       m.projectClientID(ctx, projectID),
		Op:             events.OpUpdate,
		Fields:         projectStatusFields(status),
	}, func(tx *sql.Tx) error {
		return m.projects.UpdateStatusTx(ctx, tx, projectID, status)
	})
}

// Delete soft-deletes the project (and cascade-tombstones its sections + tasks)
// and, when the project is federated, emits an op=delete tombstone for the project
// entity — in ONE transaction (NFR-2). The receiver soft-deletes its local copy of
// the project by client_id (inbox.applyDelete); child tasks/sections are not
// cross-emitted here (a project-level tombstone is the unit, matching the receiver
// which cascades child tombstones only for a TASK delete). This reconciles with the
// F2.4 read-path filter: the project row is tombstoned, so the surface resolver
// (which filters deleted_at IS NULL) simply stops returning it.
func (m *ProjectMutator) Delete(ctx context.Context, projectID int64) error {
	clientID := m.projectClientID(ctx, projectID)
	return m.emitter.EmitDeleteCascade(ctx, MutationSpec{
		LocalProjectID: projectID,
		EntityType:     events.EntityProject,
		EntityID:       clientID,
		Op:             events.OpDelete,
	}, nil, func(tx *sql.Tx) error {
		return m.projects.DeleteTx(ctx, tx, projectID)
	})
}

// projectClientID resolves the project's cross-instance client_id. A lookup
// failure (or a missing/tombstoned project) yields the empty string: the Emitter
// gate then treats the project as non-federated, so the emit sidecar is skipped
// and only the domain write runs — the same outcome the gate would reach itself.
func (m *ProjectMutator) projectClientID(ctx context.Context, projectID int64) string {
	var clientID string
	_ = m.emitter.db.QueryRowContext(ctx,
		`SELECT client_id FROM projects WHERE id = ? AND deleted_at IS NULL`, projectID).Scan(&clientID)
	return clientID
}

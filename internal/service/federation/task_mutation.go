package federation

import (
	"context"
	"database/sql"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TaskMutator is the production wiring that routes user-originated task mutations
// through the federation Emitter so a mutation on a FEDERATED task actually emits
// the per-field HLC bump + signed outbox event the sync workers fan out
// (Federation v1 F3.1 EmitMutation, F3.3 EmitDeleteCascade). It is the "service
// layer the handler calls" the F3.3 review finding asks for: without it the task
// delete/patch handlers wrote straight to the repo (soft-delete only), so a
// federated delete emitted NO op=delete tombstone and US-3.7 AC3's origin-cascade
// emit was unreachable in production.
//
// A mutation on a NON-federated task incurs zero federation overhead: the Emitter
// runs the domain write only (the gate is keyed on projects.is_federated), so the
// single-user hot path is untouched (§3 scoped overlay). When federation is OFF
// (no FEDERATION_KEY) the handler holds a nil *TaskMutator and bypasses it entirely.
type TaskMutator struct {
	emitter *Emitter
	tasks   *repo.TaskRepo
}

// NewTaskMutator constructs the task federation facade over the Emitter + task
// repo. Wire emitter.WithCommitPing(worker.Ping) before passing it so a federated
// mutation pushes immediately (NFR-1.1).
func NewTaskMutator(emitter *Emitter, tasks *repo.TaskRepo) *TaskMutator {
	return &TaskMutator{emitter: emitter, tasks: tasks}
}

// Delete soft-deletes a task and, when its project is federated, emits an
// op=delete tombstone for the task PLUS one per child comment / checklist item,
// all in ONE transaction (§8.4, US-3.7 AC3). The whole subtree is soft-deleted by
// the write closure (matching repo.TaskRepo.Delete); the children's cross-instance
// tombstones carry their own _deleted field HLC so a peer cannot resurrect them.
//
// task is the already-loaded task (the handler fetched it for the 404/410
// disambiguation). A task with no ProjectID or no ClientID cannot be federated, so
// it routes through the plain repo delete. Returns repo.ErrNotFound for an
// already-tombstoned task (the handler maps it to 410/404).
func (m *TaskMutator) Delete(ctx context.Context, task *model.Task) error {
	if task.ProjectID == nil || task.ClientID == "" {
		// Inbox / non-project / unidentified task — never federated.
		return m.tasks.Delete(ctx, task.ID)
	}

	children, err := m.tasks.ListFederatedDeleteChildren(ctx, task.ID)
	if err != nil {
		return err
	}
	tombstones := make([]ChildTombstone, 0, len(children))
	for _, c := range children {
		tombstones = append(tombstones, ChildTombstone{
			EntityType: events.EntityType(c.Kind),
			EntityID:   c.ClientID,
		})
	}

	return m.emitter.EmitDeleteCascade(ctx, MutationSpec{
		LocalProjectID: *task.ProjectID,
		EntityType:     events.EntityTask,
		EntityID:       task.ClientID,
		Op:             events.OpDelete,
	}, tombstones, func(tx *sql.Tx) error {
		if err := m.tasks.DeleteTx(ctx, tx, task.ID); err != nil {
			return err
		}
		return m.tasks.CascadeDeleteChildrenTx(ctx, tx, task.ID)
	})
}

// Create inserts a task and, when its project is federated, emits a signed
// op=create event carrying the task's federated field set (US-3.2 AC1, US-3.1
// AC1) — all in ONE transaction with the row insert (NFR-2). It returns the new
// task's local id. clientID is the task's cross-instance client_id (a fresh
// ULID); the same id is the event's EntityID so a peer resolves the create to it.
//
// A task placed in the inbox / a non-project container, or in a NON-federated
// project, incurs zero federation overhead: EmitMutation runs the bare CreateTx
// (the gate is keyed on projects.is_federated). Label application and other
// non-federated side effects run in the caller AFTER this returns — they are not
// part of the federated field set.
func (m *TaskMutator) Create(ctx context.Context, in repo.CreateTask, clientID string) (int64, error) {
	if in.ProjectID == nil {
		// No project → never federated (inbox / context-only task). Plain insert.
		t, err := m.tasks.Create(ctx, in)
		if err != nil {
			return 0, err
		}
		return t.ID, nil
	}

	var newID int64
	err := m.emitter.EmitMutation(ctx, MutationSpec{
		LocalProjectID: *in.ProjectID,
		EntityType:     events.EntityTask,
		EntityID:       clientID,
		Op:             events.OpCreate,
		Fields:         taskCreateFields(in),
	}, func(tx *sql.Tx) error {
		id, err := m.tasks.CreateTx(ctx, tx, in, clientID)
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

// Update applies a task field update and, when its project is federated, emits a
// signed op=update event carrying ONLY the changed federated fields (per-field
// LWW, US-3.3 AC1) — in ONE transaction with the row update (NFR-2).
//
// task is the already-loaded task (the handler fetched it for the 404/410
// disambiguation + invariant checks). A task with no ProjectID / ClientID, or in
// a non-federated project, routes through the bare repo update (zero overhead).
// When the update touches no federated field (e.g. only day_part/plan_state
// changed), the domain write still runs but no event is emitted — the receiver
// has nothing to merge.
func (m *TaskMutator) Update(ctx context.Context, task *model.Task, u repo.TaskUpdate) error {
	fields := taskUpdateFields(u)
	if task.ProjectID == nil || task.ClientID == "" || len(fields) == 0 {
		// Not federatable, or no federated field changed → bare repo update.
		_, err := m.tasks.Update(ctx, task.ID, u)
		return err
	}

	return m.emitter.EmitMutation(ctx, MutationSpec{
		LocalProjectID: *task.ProjectID,
		EntityType:     events.EntityTask,
		EntityID:       task.ClientID,
		Op:             events.OpUpdate,
		Fields:         fields,
	}, func(tx *sql.Tx) error {
		return m.tasks.UpdateTx(ctx, tx, task.ID, u)
	})
}

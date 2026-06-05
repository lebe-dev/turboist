package federation

import (
	"context"
	"database/sql"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// CompleteMutator routes a user-originated task STATUS change (complete /
// uncomplete / cancel, and the recurring advance) through the federation Emitter
// so a change on a task in a FEDERATED project emits the per-field HLC bump +
// signed outbox event the sync workers fan out (Federation v1 F3.1, US-3.2 AC1,
// TASK A). It implements service.TaskStatusFederator so service.CompleteService
// keeps the recurrence / troiki invariants and only delegates the transactional
// emit, mirroring how TaskMutator backs the delete/patch handler seam.
//
// A change on a task with no ProjectID / no ClientID, or in a NON-federated
// project, incurs zero federation overhead: the Emitter gate (keyed on
// projects.is_federated) runs the domain write closure only. When federation is
// OFF the CompleteService holds a nil federator and bypasses this entirely.
type CompleteMutator struct {
	emitter *Emitter
	tasks   *repo.TaskRepo
}

// NewCompleteMutator constructs the complete-action federation facade over the
// Emitter + task repo. Wire emitter.WithCommitPing(worker.Ping) before passing it
// so a federated status change pushes immediately (NFR-1.1).
func NewCompleteMutator(emitter *Emitter, tasks *repo.TaskRepo) *CompleteMutator {
	return &CompleteMutator{emitter: emitter, tasks: tasks}
}

// StatusChange runs write in ONE transaction and, when the task's project is
// federated, emits a signed op=update event carrying the changed status fields
// (status / completed_at / priority). A task with no project or no client_id is
// not federatable — the write runs through db.WithTx with no event (zero overhead).
func (m *CompleteMutator) StatusChange(ctx context.Context, task *model.Task, fields map[string]any, write func(tx *sql.Tx) error) error {
	if task.ProjectID == nil || task.ClientID == "" {
		return db.WithTx(ctx, m.emitter.db, write)
	}
	return m.emitter.EmitMutation(ctx, MutationSpec{
		LocalProjectID: *task.ProjectID,
		EntityType:     events.EntityTask,
		EntityID:       task.ClientID,
		Op:             events.OpUpdate,
		Fields:         fields,
	}, write)
}

// RecurrenceAdvance runs write in ONE transaction and, when the parent task's
// project is federated, emits TWO signed events together: op=update for the parent
// (the advanced due_at / reset fields, status stays open) AND op=create for the
// new occurrence snapshot (its own snapClientID, full federated field set). Both
// stamp the same HLC moment so a peer applies the advance and the new occurrence
// from one consistent point (TASK A recurrence). A non-federatable parent runs the
// write closure only.
func (m *CompleteMutator) RecurrenceAdvance(ctx context.Context, parent *model.Task, advanceFields map[string]any, snapClientID string, snapFields map[string]any, write func(tx *sql.Tx) error) error {
	if parent.ProjectID == nil || parent.ClientID == "" {
		return db.WithTx(ctx, m.emitter.db, write)
	}
	return m.emitter.EmitMutations(ctx, []MutationSpec{
		{
			LocalProjectID: *parent.ProjectID,
			EntityType:     events.EntityTask,
			EntityID:       parent.ClientID,
			Op:             events.OpUpdate,
			Fields:         advanceFields,
		},
		{
			LocalProjectID: *parent.ProjectID,
			EntityType:     events.EntityTask,
			EntityID:       snapClientID,
			Op:             events.OpCreate,
			Fields:         snapFields,
		},
	}, write)
}

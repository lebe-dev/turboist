package inboxproc

import (
	"context"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// Revert puts a filed task back into the Inbox and undoes what the decision
// added. Only what the processor itself changed is undone: a label, priority or
// due date the user set after the task was filed stays.
//
// Refusals: an unknown row or a deleted task is repo.ErrNotFound; a row that is
// not a "sorted" decision or was already reverted is repo.ErrConflict; a task
// that has since become a subtask is repo.ErrInvalidPlacement — pulling it out
// of its parent is not a revert any more.
func (p *Processor) Revert(ctx context.Context, logID int64) (*model.Task, error) {
	const op = "inboxproc.Processor.Revert"
	entry, err := p.d.State.GetLog(ctx, logID)
	if err != nil {
		return nil, err
	}
	if entry.Outcome != model.InboxOutcomeSorted || entry.RevertedAt != nil || entry.After == nil {
		return nil, repo.ErrConflict
	}
	if entry.TaskID == nil {
		return nil, repo.ErrNotFound
	}
	task, err := p.d.Tasks.Get(ctx, *entry.TaskID)
	if err != nil {
		return nil, err
	}
	if task.ParentID != nil {
		return nil, repo.ErrInvalidPlacement
	}

	inbox := inboxID
	if _, err := p.d.Move.Move(ctx, task.ID, repo.Placement{InboxID: &inbox}); err != nil {
		return nil, err
	}

	var removedLabels []string
	if added := missingIDs(entry.Before.LabelIDs, entry.After.LabelIDs); len(added) > 0 {
		drop := make(map[int64]struct{}, len(added))
		for _, id := range added {
			drop[id] = struct{}{}
		}
		kept := make([]int64, 0, len(task.Labels))
		for _, l := range task.Labels {
			if _, ok := drop[l.ID]; !ok {
				kept = append(kept, l.ID)
				continue
			}
			removedLabels = append(removedLabels, l.Name)
		}
		if len(kept) != len(task.Labels) {
			if err := p.d.TaskLabels.SetForTask(ctx, task.ID, kept); err != nil {
				return nil, err
			}
		}
	}

	var update repo.TaskUpdate
	if task.Priority == entry.After.Priority && task.Priority != entry.Before.Priority {
		before := entry.Before.Priority
		update.Priority = &before
	}
	if sameInstant(task.DueAt, entry.After.DueAt) && !sameInstant(task.DueAt, entry.Before.DueAt) {
		if entry.Before.DueAt == nil {
			update.DueAtClear = true
		} else {
			hasTime := entry.Before.DueHasTime
			update.DueAt = entry.Before.DueAt
			update.DueHasTime = &hasTime
		}
	}
	restored, err := p.d.Tasks.Update(ctx, task.ID, update)
	if err != nil {
		return nil, err
	}

	now := p.now()
	if err := p.d.State.MarkReverted(ctx, logID, now); err != nil {
		return nil, err
	}
	// Without this row the next run would file the task straight back.
	if err := p.d.State.UpsertState(ctx, model.InboxProcessingState{
		TaskID:      restored.ID,
		Fingerprint: model.InboxFingerprint(restored.Title, restored.Description),
		Status:      model.InboxStateReverted,
		UpdatedAt:   now,
	}); err != nil {
		return nil, err
	}
	attrs := []any{
		slog.String("op", op),
		slog.Int64("journal_id", logID),
		slog.Int64("task_id", restored.ID),
		slog.String("task_title", restored.Title),
		slog.Int64("from_context_id", derefID(task.ContextID)),
		slog.String("from_context", p.contextName(ctx, task.ContextID)),
		slog.Int64("from_project_id", derefID(task.ProjectID)),
		slog.String("from_project", p.projectTitle(ctx, task.ProjectID)),
		slog.String("to", "inbox"),
		slog.Any("labels_removed", nonNilStrings(removedLabels)),
	}
	// Only what was actually put back is named; "" for due_restored means the
	// model-set due date was cleared.
	if update.Priority != nil {
		attrs = append(attrs, slog.String("priority_restored", string(*update.Priority)))
	}
	if update.DueAtClear || update.DueAt != nil {
		attrs = append(attrs, slog.String("due_restored", formatDue(restored.DueAt, restored.DueHasTime, p.d.Location)))
	}
	p.log.InfoContext(ctx, "inbox processing decision reverted", attrs...)
	return restored, nil
}

// projectTitle names the project a task was in, or "" once it is gone.
func (p *Processor) projectTitle(ctx context.Context, id *int64) string {
	if id == nil {
		return ""
	}
	project, err := p.d.Projects.Get(ctx, *id)
	if err != nil {
		return ""
	}
	return project.Title
}

// contextName names the context a task was in, or "" once it is gone.
func (p *Processor) contextName(ctx context.Context, id *int64) string {
	if id == nil {
		return ""
	}
	c, err := p.d.Contexts.Get(ctx, *id)
	if err != nil {
		return ""
	}
	return c.Name
}

func derefID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

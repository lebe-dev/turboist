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

	if added := missingIDs(entry.Before.LabelIDs, entry.After.LabelIDs); len(added) > 0 {
		drop := make(map[int64]struct{}, len(added))
		for _, id := range added {
			drop[id] = struct{}{}
		}
		kept := make([]int64, 0, len(task.Labels))
		for _, l := range task.Labels {
			if _, ok := drop[l.ID]; !ok {
				kept = append(kept, l.ID)
			}
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
	p.log.InfoContext(ctx, "inbox processing decision reverted", slog.String("op", op),
		slog.Int64("log_id", logID), slog.Int64("task_id", restored.ID))
	return restored, nil
}

func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrPriorityManagedByTroiki is returned when a caller tries to set a priority
// on a task that lives in a Troiki-categorised project. The project's category
// pins every open task's priority, so a direct override is rejected to keep the
// two in sync — the same invariant PATCH /tasks enforces at the handler layer.
var ErrPriorityManagedByTroiki = errors.New("service: priority is managed by Troiki category")

// SetPriority updates a single task's priority. It rejects the change with
// ErrPriorityManagedByTroiki when the task belongs to a Troiki-categorised
// project and the requested priority does not match the category-derived one.
func (s *TaskService) SetPriority(ctx context.Context, taskID int64, p model.Priority) (*model.Task, error) {
	const op = "service.TaskService.SetPriority"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))

	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		logRepoErr(ctx, op+": get task", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	if t.ProjectID != nil && s.projects != nil {
		proj, err := s.projects.Get(ctx, *t.ProjectID)
		if err != nil && !errors.Is(err, repo.ErrNotFound) {
			logRepoErr(ctx, op+": get project", err, slog.Int64("project_id", *t.ProjectID))
			return nil, err
		}
		if proj != nil && proj.TroikiCategory != nil && p != PriorityForCategory(*proj.TroikiCategory) {
			log.WarnContext(ctx, op+": priority managed by troiki", slog.Int64("task_id", taskID))
			return nil, ErrPriorityManagedByTroiki
		}
	}

	updated, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{Priority: &p})
	if err != nil {
		logRepoErr(ctx, op+": update task", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	log.InfoContext(ctx, "task priority set", slog.String("op", op), slog.Int64("task_id", taskID))
	return updated, nil
}

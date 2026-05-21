package service

import (
	"context"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TaskService orchestrates task creation and label management.
type TaskService struct {
	tasks      *repo.TaskRepo
	projects   *repo.ProjectRepo
	taskLabels *repo.TaskLabelsRepo
	autoLabels *AutoLabelsService
}

// NewTaskService constructs a TaskService.
func NewTaskService(tasks *repo.TaskRepo, projects *repo.ProjectRepo, taskLabels *repo.TaskLabelsRepo, autoLabels *AutoLabelsService) *TaskService {
	return &TaskService{tasks: tasks, projects: projects, taskLabels: taskLabels, autoLabels: autoLabels}
}

// Create creates a task and applies explicit labels and auto-label rules.
// If the task is created in a project with a Troiki category, the task's
// priority is coerced to the category-derived priority — the same invariant
// PATCH /tasks enforces at the handler layer.
func (s *TaskService) Create(ctx context.Context, in repo.CreateTask, explicitLabels []string, removedAutoLabels []string) (*model.Task, error) {
	const op = "service.TaskService.Create"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op)
	if in.ProjectID != nil && s.projects != nil {
		p, err := s.projects.Get(ctx, *in.ProjectID)
		if err != nil {
			logRepoErr(ctx, op+": get project", err, slog.Int64("project_id", *in.ProjectID))
			return nil, err
		}
		if p.TroikiCategory != nil {
			in.Priority = PriorityForCategory(*p.TroikiCategory)
		}
	}
	t, err := s.tasks.Create(ctx, in)
	if err != nil {
		logRepoErr(ctx, op+": create task", err)
		return nil, err
	}
	finalIDs, err := s.autoLabels.Apply(ctx, in.Title, nil, &explicitLabels, removedAutoLabels)
	if err != nil {
		// UnknownLabelError is a business-rule rejection from the user's
		// explicit label set; everything else is unexpected.
		if _, ok := err.(*UnknownLabelError); ok {
			log.WarnContext(ctx, op+": unknown label", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
		} else {
			log.ErrorContext(ctx, op+": apply labels", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
		}
		return nil, err
	}
	if len(finalIDs) > 0 {
		if err := s.taskLabels.SetForTask(ctx, t.ID, finalIDs); err != nil {
			logRepoErr(ctx, op+": set labels", err, slog.Int64("task_id", t.ID))
			return nil, err
		}
		out, err := s.tasks.Get(ctx, t.ID)
		if err != nil {
			logRepoErr(ctx, op+": refetch", err, slog.Int64("task_id", t.ID))
			return nil, err
		}
		log.InfoContext(ctx, "task created", slog.String("op", op), slog.Int64("task_id", t.ID))
		return out, nil
	}
	log.InfoContext(ctx, "task created", slog.String("op", op), slog.Int64("task_id", t.ID))
	return t, nil
}

// PatchLabels applies label changes to an existing task.
// It re-evaluates auto-labels against newTitle and merges with the explicit / current label set.
func (s *TaskService) PatchLabels(ctx context.Context, task *model.Task, newTitle string, explicitLabels *[]string, removedAutoLabels []string) error {
	const op = "service.TaskService.PatchLabels"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", task.ID))
	currentIDs := make([]int64, len(task.Labels))
	for i, l := range task.Labels {
		currentIDs[i] = l.ID
	}
	finalIDs, err := s.autoLabels.Apply(ctx, newTitle, currentIDs, explicitLabels, removedAutoLabels)
	if err != nil {
		if _, ok := err.(*UnknownLabelError); ok {
			log.WarnContext(ctx, op+": unknown label", slog.Int64("task_id", task.ID), slog.String("err", err.Error()))
		} else {
			log.ErrorContext(ctx, op+": apply labels", slog.Int64("task_id", task.ID), slog.String("err", err.Error()))
		}
		return err
	}
	if err := s.taskLabels.SetForTask(ctx, task.ID, finalIDs); err != nil {
		log.ErrorContext(ctx, op+": set labels", slog.Int64("task_id", task.ID), slog.String("err", err.Error()))
		return err
	}
	return nil
}

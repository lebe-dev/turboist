package service

import (
	"context"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TaskService orchestrates task creation and label management.
type TaskService struct {
	tasks      *repo.TaskRepo
	taskLabels *repo.TaskLabelsRepo
	autoLabels *AutoLabelsService
}

// NewTaskService constructs a TaskService.
func NewTaskService(tasks *repo.TaskRepo, taskLabels *repo.TaskLabelsRepo, autoLabels *AutoLabelsService) *TaskService {
	return &TaskService{tasks: tasks, taskLabels: taskLabels, autoLabels: autoLabels}
}

// Create creates a task and applies explicit labels and auto-label rules.
func (s *TaskService) Create(ctx context.Context, in repo.CreateTask, explicitLabels []string, removedAutoLabels []string) (*model.Task, error) {
	t, err := s.tasks.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	finalIDs, err := s.autoLabels.Apply(ctx, in.Title, nil, &explicitLabels, removedAutoLabels)
	if err != nil {
		return nil, err
	}
	if len(finalIDs) > 0 {
		if err := s.taskLabels.SetForTask(ctx, t.ID, finalIDs); err != nil {
			return nil, err
		}
		return s.tasks.Get(ctx, t.ID)
	}
	return t, nil
}

// PatchLabels applies label changes to an existing task.
// It re-evaluates auto-labels against newTitle and merges with the explicit / current label set.
func (s *TaskService) PatchLabels(ctx context.Context, task *model.Task, newTitle string, explicitLabels *[]string, removedAutoLabels []string) error {
	currentIDs := make([]int64, len(task.Labels))
	for i, l := range task.Labels {
		currentIDs[i] = l.ID
	}
	finalIDs, err := s.autoLabels.Apply(ctx, newTitle, currentIDs, explicitLabels, removedAutoLabels)
	if err != nil {
		return err
	}
	return s.taskLabels.SetForTask(ctx, task.ID, finalIDs)
}

package service

import (
	"context"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TaskCreator routes a task insert through the federation Emitter so a create in
// a FEDERATED project emits a signed op=create event in the SAME tx as the row
// insert (Federation v1 F3.1, US-3.1 AC1 / US-3.2 AC1). It returns the new task's
// local id. A create in a non-federated project (or with no project) incurs zero
// federation overhead. service.TaskService depends on this narrow interface so it
// does not import the federation service package (avoids an import cycle).
type TaskCreator interface {
	Create(ctx context.Context, in repo.CreateTask, clientID string) (int64, error)
}

// TaskService orchestrates task creation and label management.
type TaskService struct {
	tasks      *repo.TaskRepo
	projects   *repo.ProjectRepo
	taskLabels *repo.TaskLabelsRepo
	autoLabels *AutoLabelsService

	// fedCreate routes the row insert through the federation Emitter when wired
	// (production, FEDERATION_KEY set). nil → the service inserts via the plain
	// repo, so the single-user / federation-off path is untouched.
	fedCreate TaskCreator
}

// NewTaskService constructs a TaskService.
func NewTaskService(tasks *repo.TaskRepo, projects *repo.ProjectRepo, taskLabels *repo.TaskLabelsRepo, autoLabels *AutoLabelsService) *TaskService {
	return &TaskService{tasks: tasks, projects: projects, taskLabels: taskLabels, autoLabels: autoLabels}
}

// WithFederation wires the federation task creator so a create in a federated
// project emits its op=create event (US-3.1 AC1). Returns the service for
// chaining. A nil creator leaves the service on the plain repo insert path.
func (s *TaskService) WithFederation(c TaskCreator) *TaskService {
	s.fedCreate = c
	return s
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
	t, err := s.create(ctx, in)
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

// create inserts the task row, routing through the federation creator when wired
// (so a federated-project create emits its op=create event atomically with the
// insert) and falling back to the plain repo otherwise. The federation path mints
// the cross-instance client_id; the plain path lets the repo mint it. Either way
// the hydrated task is re-read so callers see the assigned client_id + defaults.
func (s *TaskService) create(ctx context.Context, in repo.CreateTask) (*model.Task, error) {
	if s.fedCreate == nil {
		return s.tasks.Create(ctx, in)
	}
	id, err := s.fedCreate.Create(ctx, in, model.NewClientID())
	if err != nil {
		return nil, err
	}
	return s.tasks.Get(ctx, id)
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

package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TemplateService instantiates task templates into a project. Plain template
// CRUD has no cross-repo invariants and is served directly from TemplateRepo by
// the handler; only instantiation lives here because it spawns tasks (and must
// go through TaskService for auto-labels, troiki coercion, and placement).
type TemplateService struct {
	templates *repo.TemplateRepo
	projects  *repo.ProjectRepo
	tasks     *TaskService
}

func NewTemplateService(templates *repo.TemplateRepo, projects *repo.ProjectRepo, tasks *TaskService) *TemplateService {
	return &TemplateService{templates: templates, projects: projects, tasks: tasks}
}

// ErrTemplateNotFound / ErrProjectNotFound are returned by Instantiate so the
// handler can map them to 404 without depending on repo error identity.
var (
	ErrTemplateNotFound = errors.New("template not found")
	ErrProjectNotFound  = errors.New("project not found")
)

// InstantiateResult is the outcome of creating tasks from a template: the root
// task plus its created subtasks (in order).
type InstantiateResult struct {
	Root     *model.Task
	Subtasks []model.Task
}

// Instantiate creates the template's root task in the given project and then
// each subtask under it. It is best-effort and not transactional across tasks:
// a subtask failure surfaces an error but does not roll back the root or
// already-created siblings (mirrors GroupService semantics).
func (s *TemplateService) Instantiate(ctx context.Context, templateID, projectID int64) (*InstantiateResult, error) {
	const op = "service.templates.Instantiate"

	tmpl, err := s.templates.Get(ctx, templateID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	proj, err := s.projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}

	rootPlacement := repo.Placement{ContextID: &proj.ContextID, ProjectID: &proj.ID}
	root, err := s.tasks.Create(ctx, repo.CreateTask{
		Placement:   rootPlacement,
		Title:       tmpl.Name,
		Description: tmpl.Description,
		Priority:    tmpl.Priority,
		DayPart:     tmpl.DayPart,
	}, labelNames(tmpl.Labels), nil)
	if err != nil {
		return nil, err
	}

	out := &InstantiateResult{Root: root, Subtasks: make([]model.Task, 0, len(tmpl.Subtasks))}
	for _, st := range tmpl.Subtasks {
		sub, err := s.tasks.Create(ctx, repo.CreateTask{
			Placement:   repo.Placement{ContextID: &proj.ContextID, ProjectID: &proj.ID, ParentID: &root.ID},
			Title:       st.Title,
			Description: st.Description,
			Priority:    st.Priority,
			DayPart:     st.DayPart,
		}, labelNames(st.Labels), nil)
		if err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, op+": subtask create failed",
				slog.String("op", op), slog.Int64("template_id", templateID),
				slog.Int64("root_id", root.ID), slog.String("err", err.Error()))
			return out, err
		}
		out.Subtasks = append(out.Subtasks, *sub)
	}
	return out, nil
}

func labelNames(labels []model.Label) []string {
	if len(labels) == 0 {
		// Non-nil empty slice: tells TaskService.Create "no explicit labels"
		// without inheriting (auto-labels still apply by title).
		return []string{}
	}
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names
}

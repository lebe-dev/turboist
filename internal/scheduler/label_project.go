package scheduler

import (
	"context"
	"slices"

	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

// ProjectMover is the subset of todoist.Cache that LabelProjectSync needs.
type ProjectMover interface {
	Tasks() []*todoist.Task
	Projects() []*todoist.Project
	InboxProjectID() string
	MoveTaskToProject(ctx context.Context, id string, projectID string) error
}

// LabelProjectSync moves root tasks to projects based on label-to-project mappings.
// First matching label wins; tasks with no match go to Inbox.
// Subtasks (tasks with a parent_id) are skipped entirely.
type LabelProjectSync struct {
	mover    ProjectMover
	mappings []config.LabelProjectMapping
}

// NewLabelProjectSync creates a LabelProjectSync job.
func NewLabelProjectSync(mover ProjectMover, mappings []config.LabelProjectMapping) *LabelProjectSync {
	return &LabelProjectSync{mover: mover, mappings: mappings}
}

// Job implements scheduler.Job. Register it with the Scheduler.
func (lp *LabelProjectSync) Job(ctx context.Context) {
	tasks := lp.mover.Tasks()
	projects := lp.mover.Projects()
	inboxID := lp.mover.InboxProjectID()

	projectByName := make(map[string]string, len(projects))
	for _, p := range projects {
		projectByName[p.Name] = p.ID
	}

	for _, task := range tasks {
		if task.ParentID != nil {
			continue
		}

		targetProjectID := lp.resolveProject(task.Labels, projectByName, inboxID)
		if targetProjectID == "" {
			continue
		}
		if task.ProjectID == targetProjectID {
			continue
		}

		log.Info("label_project: moving task",
			"task", task.ID,
			"content", task.Content,
			"from", task.ProjectID,
			"to", targetProjectID,
		)

		if err := lp.mover.MoveTaskToProject(ctx, task.ID, targetProjectID); err != nil {
			log.Error("label_project: failed to move task", "task", task.ID, "err", err)
		}
	}
}

// resolveProject returns the target project ID for the given labels.
// First matching mapping wins. Falls back to inboxID when no label matches.
// Returns "" if inboxID is also empty (Inbox project not found in cache).
func (lp *LabelProjectSync) resolveProject(labels []string, projectByName map[string]string, inboxID string) string {
	for _, m := range lp.mappings {
		if slices.Contains(labels, m.Label) {
			if id, ok := projectByName[m.Project]; ok {
				return id
			}
			log.Warn("label_project: mapped project not found in cache", "project", m.Project)
		}
	}
	return inboxID
}

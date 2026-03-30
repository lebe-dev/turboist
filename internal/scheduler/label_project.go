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
	Sections() []*todoist.Section
	InboxProjectID() string
	BatchMoveTasks(ctx context.Context, moves map[string]todoist.MoveTarget) error
}

// LabelProjectSync moves root tasks to projects (and optionally sections) based on
// label-to-project mappings. First matching label wins; tasks with no match go to Inbox.
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
	sections := lp.mover.Sections()
	inboxID := lp.mover.InboxProjectID()

	projectByName := make(map[string]string, len(projects))
	for _, p := range projects {
		projectByName[p.Name] = p.ID
	}

	sectionByProjectAndName := make(map[string]map[string]string)
	for _, s := range sections {
		if _, ok := sectionByProjectAndName[s.ProjectID]; !ok {
			sectionByProjectAndName[s.ProjectID] = make(map[string]string)
		}
		sectionByProjectAndName[s.ProjectID][s.Name] = s.ID
	}

	moves := make(map[string]todoist.MoveTarget)
	for _, task := range tasks {
		if task.ParentID != nil {
			continue
		}

		target := lp.resolveTarget(task.Labels, projectByName, sectionByProjectAndName, inboxID)
		if target.ProjectID == "" {
			continue
		}

		if task.ProjectID == target.ProjectID {
			if target.SectionID == "" {
				continue
			}
			if task.SectionID != nil && *task.SectionID == target.SectionID {
				continue
			}
		}

		log.Info("label_project: moving task",
			"task", task.ID,
			"content", task.Content,
			"from_project", task.ProjectID,
			"to_project", target.ProjectID,
			"to_section", target.SectionID,
		)
		moves[task.ID] = target
	}

	if len(moves) == 0 {
		return
	}

	if err := lp.mover.BatchMoveTasks(ctx, moves); err != nil {
		log.Error("label_project: batch move failed", "count", len(moves), "err", err)
	}
}

// resolveTarget returns the target for the given labels.
// First matching mapping wins. Falls back to inbox (project-level) when no label matches.
// Returns empty MoveTarget if inboxID is also empty (Inbox project not found in cache).
func (lp *LabelProjectSync) resolveTarget(
	labels []string,
	projectByName map[string]string,
	sectionByProjectAndName map[string]map[string]string,
	inboxID string,
) todoist.MoveTarget {
	for _, m := range lp.mappings {
		if slices.Contains(labels, m.Label) {
			projectID, ok := projectByName[m.Project]
			if !ok {
				log.Warn("label_project: mapped project not found in cache", "project", m.Project)
				continue
			}
			if m.Section != "" {
				if sections, ok := sectionByProjectAndName[projectID]; ok {
					if sectionID, ok := sections[m.Section]; ok {
						return todoist.MoveTarget{ProjectID: projectID, SectionID: sectionID}
					}
				}
				log.Warn("label_project: mapped section not found in cache",
					"project", m.Project, "section", m.Section)
			}
			return todoist.MoveTarget{ProjectID: projectID}
		}
	}
	return todoist.MoveTarget{ProjectID: inboxID}
}

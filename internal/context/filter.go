package context

import (
	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

// FilterTasks returns tasks matching the given context filters.
// projects and sections are used to resolve names to IDs.
// Filters: projects (OR), sections (OR), labels (OR) — all active filters combined with AND.
// Empty filters → all tasks returned.
func FilterTasks(
	tasks []*todoist.Task,
	filters config.ContextFilters,
	projects []*todoist.Project,
	sections []*todoist.Section,
) []*todoist.Task {
	if len(filters.Projects) == 0 && len(filters.Sections) == 0 && len(filters.Labels) == 0 {
		log.Debug("filter: no filters defined, returning all tasks", "count", len(tasks))
		return tasks
	}

	allowedProjects := nameSetToIDs(filters.Projects, projects)
	allowedSections := nameSetToSectionIDs(filters.Sections, sections)
	allowedLabels := toSet(filters.Labels)

	log.Debug("filter: applying filters",
		"projects", filters.Projects,
		"sections", filters.Sections,
		"labels", filters.Labels,
		"input_tasks", len(tasks),
	)

	result := make([]*todoist.Task, 0)
	for _, t := range tasks {
		if !matchesFilters(t, filters, allowedProjects, allowedSections, allowedLabels) {
			continue
		}
		result = append(result, t)
	}

	log.Debug("filter: done", "matched", len(result), "total", len(tasks))
	return result
}

func matchesFilters(
	t *todoist.Task,
	filters config.ContextFilters,
	allowedProjects map[string]struct{},
	allowedSections map[string]struct{},
	allowedLabels map[string]struct{},
) bool {
	if len(filters.Projects) > 0 {
		if _, ok := allowedProjects[t.ProjectID]; !ok {
			return false
		}
	}

	if len(filters.Sections) > 0 {
		sectionID := ""
		if t.SectionID != nil {
			sectionID = *t.SectionID
		}
		if _, ok := allowedSections[sectionID]; !ok {
			return false
		}
	}

	if len(filters.Labels) > 0 {
		if !hasAnyLabel(t.Labels, allowedLabels) {
			return false
		}
	}

	return true
}

func hasAnyLabel(taskLabels []string, allowed map[string]struct{}) bool {
	for _, l := range taskLabels {
		if _, ok := allowed[l]; ok {
			return true
		}
	}
	return false
}

func nameSetToIDs(names []string, projects []*todoist.Project) map[string]struct{} {
	wanted := toSet(names)
	ids := make(map[string]struct{})
	for _, p := range projects {
		if _, ok := wanted[p.Name]; ok {
			ids[p.ID] = struct{}{}
		}
	}
	return ids
}

func nameSetToSectionIDs(names []string, sections []*todoist.Section) map[string]struct{} {
	wanted := toSet(names)
	ids := make(map[string]struct{})
	for _, s := range sections {
		if _, ok := wanted[s.Name]; ok {
			ids[s.ID] = struct{}{}
		}
	}
	return ids
}

func toSet(items []string) map[string]struct{} {
	s := make(map[string]struct{}, len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return s
}

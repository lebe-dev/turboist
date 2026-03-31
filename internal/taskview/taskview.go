package taskview

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/config"
	ctxfilter "github.com/lebe-dev/turboist/internal/context"
	"github.com/lebe-dev/turboist/internal/todoist"
)

// ViewParams specifies which view and context to compute.
type ViewParams struct {
	View    string // "all", "inbox", "today", "tomorrow", "weekly", "backlog", "completed"
	Context string
}

// TasksMeta holds counters returned alongside a task list.
type TasksMeta struct {
	Context      string `json:"context"`
	WeeklyLimit  int    `json:"weekly_limit"`
	WeeklyCount  int    `json:"weekly_count"`
	BacklogLimit int    `json:"backlog_limit"`
	BacklogCount int    `json:"backlog_count"`
	InboxCount   int    `json:"inbox_count"`
	LastSyncedAt string `json:"last_synced_at,omitempty"`
}

// TasksResult is the output of ComputeTasks.
type TasksResult struct {
	Tasks []*todoist.Task
	Meta  TasksMeta
}

// PlanningResult is the output of ComputePlanning.
type PlanningResult struct {
	Backlog []*todoist.Task
	Weekly  []*todoist.Task
	Meta    TasksMeta
}

// ComputeTasks computes the task tree for a given view and context.
func ComputeTasks(cache *todoist.Cache, cfg *config.AppConfig, params ViewParams) TasksResult {
	tasks := filterByContext(cache, cfg, params.Context)
	weeklyCount := CountWithLabel(tasks, cfg.Weekly.Label)

	var result TasksResult
	switch params.View {
	case "inbox":
		result = computeInbox(cache, tasks, cfg, params.Context, weeklyCount)
	case "today":
		result = computeToday(tasks, cfg, params.Context, weeklyCount)
	case "tomorrow":
		result = computeTomorrow(tasks, cfg, params.Context, weeklyCount)
	case "weekly":
		result = computeWeekly(tasks, cfg, params.Context)
	case "backlog":
		result = computeBacklog(cache, tasks, cfg, params.Context)
	default: // "all"
		result = computeAll(tasks, cfg, params.Context, weeklyCount)
	}

	result.Meta.InboxCount = countInbox(cache, tasks)
	return result
}

// ComputePlanning computes backlog + weekly lists for the planning view.
func ComputePlanning(cache *todoist.Cache, cfg *config.AppConfig, context string) PlanningResult {
	tasks := filterByContext(cache, cfg, context)
	weeklyCount := CountWithLabel(cache.Tasks(), cfg.Weekly.Label)

	backlog := FilterByLabel(tasks, cfg.Backlog.Label)
	backlogTree := BuildTree(backlog)
	SortBacklogTasks(backlogTree, cfg.Backlog.TaskSort)

	// Weekly panel has no context filter per original behavior
	allTasks := cache.Tasks()
	weekly := FilterByLabel(allTasks, cfg.Weekly.Label)
	weeklyTree := BuildTree(weekly)
	SortTasks(weeklyTree, cfg.TaskSort)

	return PlanningResult{
		Backlog: backlogTree,
		Weekly:  weeklyTree,
		Meta: TasksMeta{
			Context:      context,
			WeeklyLimit:  cfg.Weekly.MaxTasks,
			WeeklyCount:  weeklyCount,
			BacklogLimit: cfg.Backlog.MaxLimit,
			BacklogCount: len(backlog),
		},
	}
}

func computeAll(tasks []*todoist.Task, cfg *config.AppConfig, context string, weeklyCount int) TasksResult {
	tree := BuildTree(tasks)
	SortTasks(tree, config.TaskSortPriority)
	return TasksResult{
		Tasks: tree,
		Meta: TasksMeta{
			Context:     context,
			WeeklyLimit: cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	}
}

func countInbox(cache *todoist.Cache, tasks []*todoist.Task) int {
	inboxProjectID := cache.InboxProjectID()
	if inboxProjectID == "" {
		return 0
	}
	count := 0
	for _, t := range tasks {
		if t.ProjectID == inboxProjectID && t.ParentID == nil {
			count++
		}
	}
	return count
}

func computeInbox(cache *todoist.Cache, tasks []*todoist.Task, cfg *config.AppConfig, context string, weeklyCount int) TasksResult {
	inboxProjectID := cache.InboxProjectID()
	inbox := make([]*todoist.Task, 0)
	for _, t := range tasks {
		if t.ProjectID == inboxProjectID {
			inbox = append(inbox, t)
		}
	}
	tree := BuildTree(inbox)
	SortTasksByAddedAt(tree)
	return TasksResult{
		Tasks: tree,
		Meta: TasksMeta{
			Context:     context,
			WeeklyLimit: cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	}
}

func computeToday(tasks []*todoist.Task, cfg *config.AppConfig, context string, weeklyCount int) TasksResult {
	today := FilterByDueDate(tasks, time.Now(), cfg.Today.IncludeOverdue)
	tree := BuildTree(today)
	SortTasks(tree, cfg.TaskSort)
	return TasksResult{
		Tasks: tree,
		Meta: TasksMeta{
			Context:     context,
			WeeklyLimit: cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	}
}

func computeTomorrow(tasks []*todoist.Task, cfg *config.AppConfig, context string, weeklyCount int) TasksResult {
	tomorrow := FilterByDueDate(tasks, time.Now().AddDate(0, 0, 1), false)
	tree := BuildTree(tomorrow)
	SortTasks(tree, cfg.TaskSort)
	return TasksResult{
		Tasks: tree,
		Meta: TasksMeta{
			Context:     context,
			WeeklyLimit: cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	}
}

func computeWeekly(tasks []*todoist.Task, cfg *config.AppConfig, context string) TasksResult {
	weekly := FilterByLabel(tasks, cfg.Weekly.Label)
	tree := BuildTree(weekly)
	SortTasks(tree, cfg.TaskSort)
	return TasksResult{
		Tasks: tree,
		Meta: TasksMeta{
			Context:     context,
			WeeklyLimit: cfg.Weekly.MaxTasks,
			WeeklyCount: len(weekly),
		},
	}
}

func computeBacklog(cache *todoist.Cache, tasks []*todoist.Task, cfg *config.AppConfig, context string) TasksResult {
	weeklyCount := CountWithLabel(cache.Tasks(), cfg.Weekly.Label)
	backlog := FilterByLabel(tasks, cfg.Backlog.Label)
	tree := BuildTree(backlog)
	SortBacklogTasks(tree, cfg.Backlog.TaskSort)
	return TasksResult{
		Tasks: tree,
		Meta: TasksMeta{
			Context:     context,
			WeeklyLimit: cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	}
}

func filterByContext(cache *todoist.Cache, cfg *config.AppConfig, contextKey string) []*todoist.Task {
	tasks := cache.Tasks()
	if contextKey == "" {
		return tasks
	}
	ctx := cfg.FindContext(contextKey)
	if ctx == nil {
		return tasks
	}
	return ctxfilter.FilterTasks(tasks, ctx.Filters, cache.Projects(), cache.Sections())
}

// BuildTree builds a parent/child tree from a flat task list.
// Tasks are cloned to avoid mutating shared cache state.
// Children whose parent is not in the set are treated as roots.
func BuildTree(tasks []*todoist.Task) []*todoist.Task {
	byID := make(map[string]*todoist.Task, len(tasks))
	clones := make([]*todoist.Task, len(tasks))
	for i, t := range tasks {
		c := *t
		c.Children = make([]*todoist.Task, 0)
		clones[i] = &c
		byID[t.ID] = &c
	}

	roots := make([]*todoist.Task, 0)
	for _, t := range clones {
		if t.ParentID == nil {
			roots = append(roots, t)
			continue
		}
		if parent, ok := byID[*t.ParentID]; ok {
			parent.Children = append(parent.Children, t)
		} else {
			roots = append(roots, t)
		}
	}

	for _, t := range roots {
		populateSubtaskCounts(t)
	}

	return roots
}

// SortTasks sorts tasks in place according to the configured sort mode.
// Also recursively sorts children.
func SortTasks(tasks []*todoist.Task, mode config.TaskSort) {
	slices.SortStableFunc(tasks, func(a, b *todoist.Task) int {
		switch mode {
		case config.TaskSortDueDate:
			return CompareDueDate(a, b)
		case config.TaskSortContent:
			return cmp.Compare(strings.ToLower(a.Content), strings.ToLower(b.Content))
		case config.TaskSortAddedAt:
			return cmp.Compare(b.AddedAt, a.AddedAt)
		default: // priority
			if c := cmp.Compare(b.Priority, a.Priority); c != 0 {
				return c
			}
			return CompareDueDate(a, b)
		}
	})
	for _, t := range tasks {
		if len(t.Children) > 1 {
			SortTasks(t.Children, mode)
		}
	}
}

// SortBacklogTasks sorts backlog tasks using the backlog-specific sort mode.
func SortBacklogTasks(tasks []*todoist.Task, mode config.TaskSort) {
	SortTasks(tasks, mode)
}

// SortTasksByAddedAt sorts tasks by creation date descending (newest first).
// Also recursively sorts children.
func SortTasksByAddedAt(tasks []*todoist.Task) {
	slices.SortStableFunc(tasks, func(a, b *todoist.Task) int {
		return cmp.Compare(b.AddedAt, a.AddedAt)
	})
	for _, t := range tasks {
		if len(t.Children) > 1 {
			SortTasksByAddedAt(t.Children)
		}
	}
}

// CompareDueDate compares two tasks by due date. Tasks without due date go last.
func CompareDueDate(a, b *todoist.Task) int {
	switch {
	case a.Due == nil && b.Due == nil:
		return 0
	case a.Due == nil:
		return 1
	case b.Due == nil:
		return -1
	default:
		return cmp.Compare(a.Due.Date, b.Due.Date)
	}
}

func populateSubtaskCounts(t *todoist.Task) {
	t.SubTaskCount = len(t.Children)
	for _, child := range t.Children {
		populateSubtaskCounts(child)
	}
}

// FilterByLabel returns tasks that have the given label.
func FilterByLabel(tasks []*todoist.Task, label string) []*todoist.Task {
	if label == "" {
		return tasks
	}
	result := make([]*todoist.Task, 0)
	for _, t := range tasks {
		if slices.Contains(t.Labels, label) {
			result = append(result, t)
		}
	}
	return result
}

// CountWithLabel counts tasks that have the given label.
func CountWithLabel(tasks []*todoist.Task, label string) int {
	if label == "" {
		return 0
	}
	count := 0
	for _, t := range tasks {
		if slices.Contains(t.Labels, label) {
			count++
		}
	}
	return count
}

// ExcludeByLabel returns tasks that do NOT have the given label.
func ExcludeByLabel(tasks []*todoist.Task, label string) []*todoist.Task {
	if label == "" {
		return tasks
	}
	result := make([]*todoist.Task, 0)
	for _, t := range tasks {
		if !slices.Contains(t.Labels, label) {
			result = append(result, t)
		}
	}
	return result
}

// FilterByDueDate returns tasks due on the given date.
// If includeOverdue is true, tasks with due dates before the target date are also included.
func FilterByDueDate(tasks []*todoist.Task, target time.Time, includeOverdue bool) []*todoist.Task {
	targetDate := target.Format("2006-01-02")
	result := make([]*todoist.Task, 0)
	for _, t := range tasks {
		if t.Due == nil {
			continue
		}
		if t.Due.Date == targetDate {
			result = append(result, t)
			continue
		}
		if includeOverdue && t.Due.Date < targetDate {
			result = append(result, t)
		}
	}
	return result
}

// FindInTree recursively searches for a task by ID in a tree.
func FindInTree(tasks []*todoist.Task, id string) *todoist.Task {
	for _, t := range tasks {
		if t.ID == id {
			return t
		}
		if found := FindInTree(t.Children, id); found != nil {
			return found
		}
	}
	return nil
}

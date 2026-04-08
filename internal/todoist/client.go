package todoist

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/CnTeng/todoist-api-go/rest"
	"github.com/CnTeng/todoist-api-go/sync"
	extclient "github.com/CnTeng/todoist-api-go/todoist"
	"github.com/charmbracelet/log"
	"github.com/google/uuid"
)

// APIError wraps errors returned by the Todoist API.
type APIError struct {
	Op  string
	Err error
}

func (e *APIError) Error() string {
	return fmt.Sprintf("todoist %s: %v", e.Op, e.Err)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// SyncResult holds all data from a single Todoist sync call.
type SyncResult struct {
	Tasks    []*Task
	Projects []*Project
	Sections []*Section
	Labels   []*Label
}

// Client wraps the todoist-api-go client.
type Client struct {
	cli     *extclient.Client
	taskSvc *extclient.TaskService
}

// NewClient creates a new Todoist API client with the given API key.
func NewClient(apiKey string) *Client {
	cli := extclient.NewClient(http.DefaultClient, apiKey, extclient.DefaultHandler)
	return &Client{
		cli:     cli,
		taskSvc: extclient.NewTaskService(cli),
	}
}

// FetchAll fetches tasks, projects, sections and labels via a full sync (sync_token=*).
func (c *Client) FetchAll(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	resp, err := c.cli.SyncWithAutoToken(ctx, true)
	if err != nil {
		log.Debug("todoist FetchAll failed", "err", err, "elapsed", time.Since(start))
		return nil, &APIError{Op: "FetchAll", Err: err}
	}

	result := parseSyncResponse(resp)
	log.Debug("todoist FetchAll done",
		"tasks", len(result.Tasks),
		"projects", len(result.Projects),
		"sections", len(result.Sections),
		"labels", len(result.Labels),
		"elapsed", time.Since(start),
	)
	return result, nil
}

// FetchIncremental fetches only changes since the last sync using a stored sync token.
// Returns a DeltaResult containing items to upsert and IDs to remove.
// If the server returns a full sync (token expired), FullSync is set to true and
// the Result field contains the complete dataset.
func (c *Client) FetchIncremental(ctx context.Context) (*DeltaResult, error) {
	start := time.Now()
	resp, err := c.cli.SyncWithAutoToken(ctx, false)
	if err != nil {
		log.Debug("todoist FetchIncremental failed", "err", err, "elapsed", time.Since(start))
		return nil, &APIError{Op: "FetchIncremental", Err: err}
	}

	if resp.FullSync {
		result := parseSyncResponse(resp)
		log.Debug("todoist FetchIncremental got full sync",
			"tasks", len(result.Tasks),
			"elapsed", time.Since(start),
		)
		return &DeltaResult{FullSync: true, Result: result}, nil
	}

	delta := &DeltaResult{}
	for _, t := range resp.Tasks {
		if t.IsDeleted || t.Checked {
			delta.RemovedTaskIDs = append(delta.RemovedTaskIDs, t.ID)
		} else {
			delta.UpsertedTasks = append(delta.UpsertedTasks, TaskFromSync(t))
		}
	}
	for _, p := range resp.Projects {
		if p.IsDeleted || p.IsArchived {
			delta.RemovedProjectIDs = append(delta.RemovedProjectIDs, p.ID)
		} else {
			delta.UpsertedProjects = append(delta.UpsertedProjects, ProjectFromSync(p))
		}
	}
	for _, s := range resp.Sections {
		if s.IsDeleted {
			delta.RemovedSectionIDs = append(delta.RemovedSectionIDs, s.ID)
		} else {
			delta.UpsertedSections = append(delta.UpsertedSections, SectionFromSync(s))
		}
	}
	for _, l := range resp.Labels {
		if l.IsDeleted {
			delta.RemovedLabelIDs = append(delta.RemovedLabelIDs, l.ID)
		} else {
			delta.UpsertedLabels = append(delta.UpsertedLabels, LabelFromSync(l))
		}
	}

	log.Debug("todoist FetchIncremental done",
		"upserted_tasks", len(delta.UpsertedTasks),
		"removed_tasks", len(delta.RemovedTaskIDs),
		"elapsed", time.Since(start),
	)
	return delta, nil
}

func parseSyncResponse(resp *sync.SyncResponse) *SyncResult {
	result := &SyncResult{
		Tasks:    make([]*Task, 0, len(resp.Tasks)),
		Projects: make([]*Project, 0, len(resp.Projects)),
		Sections: make([]*Section, 0, len(resp.Sections)),
		Labels:   make([]*Label, 0, len(resp.Labels)),
	}

	for _, t := range resp.Tasks {
		if t.IsDeleted || t.Checked {
			continue
		}
		result.Tasks = append(result.Tasks, TaskFromSync(t))
	}

	for _, p := range resp.Projects {
		if p.IsDeleted || p.IsArchived {
			continue
		}
		result.Projects = append(result.Projects, ProjectFromSync(p))
	}

	for _, s := range resp.Sections {
		result.Sections = append(result.Sections, SectionFromSync(s))
	}

	for _, l := range resp.Labels {
		if l.IsDeleted {
			continue
		}
		result.Labels = append(result.Labels, LabelFromSync(l))
	}

	return result
}

// GetTasks returns all active tasks.
func (c *Client) GetTasks(ctx context.Context) ([]*Task, error) {
	result, err := c.FetchAll(ctx)
	if err != nil {
		return nil, err
	}
	return result.Tasks, nil
}

// GetProjects returns all active projects.
func (c *Client) GetProjects(ctx context.Context) ([]*Project, error) {
	result, err := c.FetchAll(ctx)
	if err != nil {
		return nil, err
	}
	return result.Projects, nil
}

// GetSections returns all sections.
func (c *Client) GetSections(ctx context.Context) ([]*Section, error) {
	result, err := c.FetchAll(ctx)
	if err != nil {
		return nil, err
	}
	return result.Sections, nil
}

// GetLabels returns all personal labels.
func (c *Client) GetLabels(ctx context.Context) ([]*Label, error) {
	result, err := c.FetchAll(ctx)
	if err != nil {
		return nil, err
	}
	return result.Labels, nil
}

// AddTask creates a new task via the Todoist API and returns the new task ID.
func (c *Client) AddTask(ctx context.Context, args *sync.TaskAddArgs) (string, error) {
	log.Debug("todoist AddTask", "content", args.Content)
	start := time.Now()
	resp, err := c.taskSvc.AddTask(ctx, args)
	if err != nil {
		log.Debug("todoist AddTask failed", "err", err, "elapsed", time.Since(start))
		return "", &APIError{Op: "AddTask", Err: err}
	}
	for _, id := range resp.TempIDMapping {
		log.Debug("todoist AddTask done", "id", id, "elapsed", time.Since(start))
		return id, nil
	}
	return "", nil
}

// UpdateTask updates an existing task via the Todoist API.
func (c *Client) UpdateTask(ctx context.Context, args *sync.TaskUpdateArgs) error {
	log.Debug("todoist UpdateTask", "id", args.ID)
	start := time.Now()
	_, err := c.taskSvc.UpdateTask(ctx, args)
	if err != nil {
		log.Debug("todoist UpdateTask failed", "id", args.ID, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "UpdateTask", Err: err}
	}
	log.Debug("todoist UpdateTask done", "id", args.ID, "elapsed", time.Since(start))
	return nil
}

// SetTasksLabels updates labels for multiple tasks in a single sync call.
// Unlike UpdateTask, this always sends the labels field (even when empty)
// to work around the omitempty tag on TaskUpdateArgs.Labels.
func (c *Client) SetTasksLabels(ctx context.Context, updates map[string][]string) error {
	cmds := make(sync.Commands, 0, len(updates))
	for id, labels := range updates {
		cmds = append(cmds, &sync.Command{
			Type: "item_update",
			UUID: uuid.New(),
			Args: map[string]any{"id": id, "labels": labels},
		})
	}
	_, err := c.cli.ExecuteCommands(ctx, cmds)
	if err != nil {
		return &APIError{Op: "SetTasksLabels", Err: err}
	}
	return nil
}

// FetchCompletedTasks returns tasks completed between since and until.
func (c *Client) FetchCompletedTasks(ctx context.Context, since, until time.Time) ([]*Task, error) {
	items, err := c.taskSvc.GetAllCompletedTasksByCompletionDate(ctx, &rest.TaskGetCompletedByCompletionDateParams{
		Since: since,
		Until: until,
	})
	if err != nil {
		return nil, &APIError{Op: "FetchCompletedTasks", Err: err}
	}

	tasks := make([]*Task, 0, len(items))
	for _, t := range items {
		tasks = append(tasks, TaskFromSync(t))
	}
	return tasks, nil
}

// FetchCompletedSubtasks returns subtasks of the given parent completed in the last 90 days.
func (c *Client) FetchCompletedSubtasks(ctx context.Context, parentID string) ([]*Task, error) {
	now := time.Now()
	since := now.AddDate(0, -3, 0) // 3 months back

	items, err := c.taskSvc.GetAllCompletedTasksByCompletionDate(ctx, &rest.TaskGetCompletedByCompletionDateParams{
		Since: since,
		Until: now,
	})
	if err != nil {
		return nil, &APIError{Op: "FetchCompletedSubtasks", Err: err}
	}

	tasks := make([]*Task, 0)
	for _, t := range items {
		if t.ParentID != nil && *t.ParentID == parentID {
			tasks = append(tasks, TaskFromSync(t))
		}
	}
	return tasks, nil
}

// MoveTask moves a task to be a subtask of the given parent via the Todoist API.
func (c *Client) MoveTask(ctx context.Context, id string, parentID string) error {
	log.Debug("todoist MoveTask", "id", id, "parent_id", parentID)
	start := time.Now()
	_, err := c.taskSvc.MoveTask(ctx, &sync.TaskMoveArgs{
		ID:       id,
		ParentID: &parentID,
	})
	if err != nil {
		log.Debug("todoist MoveTask failed", "id", id, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "MoveTask", Err: err}
	}
	log.Debug("todoist MoveTask done", "id", id, "elapsed", time.Since(start))
	return nil
}

// MoveTaskToProject moves a task to the given project via the Todoist API.
func (c *Client) MoveTaskToProject(ctx context.Context, id string, projectID string) error {
	log.Debug("todoist MoveTaskToProject", "id", id, "project_id", projectID)
	start := time.Now()
	_, err := c.taskSvc.MoveTask(ctx, &sync.TaskMoveArgs{
		ID:        id,
		ProjectID: &projectID,
	})
	if err != nil {
		log.Debug("todoist MoveTaskToProject failed", "id", id, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "MoveTaskToProject", Err: err}
	}
	log.Debug("todoist MoveTaskToProject done", "id", id, "elapsed", time.Since(start))
	return nil
}

// CompleteTask archives a non-recurring task using item_complete.
func (c *Client) CompleteTask(ctx context.Context, id string) error {
	log.Debug("todoist CompleteTask", "id", id)
	start := time.Now()
	_, err := c.taskSvc.CompleteTask(ctx, &sync.TaskCompleteArgs{ID: id})
	if err != nil {
		log.Debug("todoist CompleteTask failed", "id", id, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "CompleteTask", Err: err}
	}
	log.Debug("todoist CompleteTask done", "id", id, "elapsed", time.Since(start))
	return nil
}

// CloseTask advances a recurring task to its next occurrence using item_close.
func (c *Client) CloseTask(ctx context.Context, id string) error {
	log.Debug("todoist CloseTask", "id", id)
	start := time.Now()
	_, err := c.taskSvc.CloseTask(ctx, &sync.TaskCloseArgs{ID: id})
	if err != nil {
		log.Debug("todoist CloseTask failed", "id", id, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "CloseTask", Err: err}
	}
	log.Debug("todoist CloseTask done", "id", id, "elapsed", time.Since(start))
	return nil
}

// DeleteTask deletes a task and all its sub-tasks via the Todoist API.
func (c *Client) DeleteTask(ctx context.Context, id string) error {
	log.Debug("todoist DeleteTask", "id", id)
	start := time.Now()
	_, err := c.taskSvc.DeleteTask(ctx, &sync.TaskDeleteArgs{ID: id})
	if err != nil {
		log.Debug("todoist DeleteTask failed", "id", id, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "DeleteTask", Err: err}
	}
	log.Debug("todoist DeleteTask done", "id", id, "elapsed", time.Since(start))
	return nil
}

// DecomposeTask creates N new tasks (inheriting properties from src) and deletes the source
// task in a single Todoist Sync API batch call.
func (c *Client) DecomposeTask(ctx context.Context, src *Task, newContents []string) error {
	cmds := make(sync.Commands, 0, len(newContents)+1)

	for _, content := range newContents {
		args := map[string]any{
			"content":    content,
			"project_id": src.ProjectID,
			"priority":   src.Priority,
		}
		if src.SectionID != nil {
			args["section_id"] = *src.SectionID
		}
		if src.ParentID != nil {
			args["parent_id"] = *src.ParentID
		}
		if len(src.Labels) > 0 {
			args["labels"] = src.Labels
		}
		if src.Due != nil {
			args["due"] = map[string]any{"date": src.Due.Date}
		}
		cmds = append(cmds, &sync.Command{
			Type:   "item_add",
			UUID:   uuid.New(),
			TempID: uuid.New(),
			Args:   args,
		})
	}

	cmds = append(cmds, &sync.Command{
		Type: "item_delete",
		UUID: uuid.New(),
		Args: map[string]any{"id": src.ID},
	})

	log.Debug("todoist DecomposeTask", "src", src.ID, "new_tasks", len(newContents))
	_, err := c.cli.ExecuteCommands(ctx, cmds)
	if err != nil {
		return &APIError{Op: "DecomposeTask", Err: err}
	}
	return nil
}

// BatchMoveTasksToProject moves multiple tasks to their target projects in a single sync call.
// The moves map is taskID → projectID.
func (c *Client) BatchMoveTasksToProject(ctx context.Context, moves map[string]string) error {
	if len(moves) == 0 {
		return nil
	}
	cmds := make(sync.Commands, 0, len(moves))
	for id, projectID := range moves {
		cmds = append(cmds, &sync.Command{
			Type: "item_move",
			UUID: uuid.New(),
			Args: map[string]any{"id": id, "project_id": projectID},
		})
	}
	log.Debug("todoist BatchMoveTasksToProject", "count", len(moves))
	_, err := c.cli.ExecuteCommands(ctx, cmds)
	if err != nil {
		return &APIError{Op: "BatchMoveTasksToProject", Err: err}
	}
	return nil
}

// BatchMoveTasks moves multiple tasks to their targets in a single sync call.
// When target has SectionID, the command uses section_id (project is implicit in Todoist API).
// When target has no SectionID, the command uses project_id.
func (c *Client) BatchMoveTasks(ctx context.Context, moves map[string]MoveTarget) error {
	if len(moves) == 0 {
		return nil
	}
	cmds := make(sync.Commands, 0, len(moves))
	for id, target := range moves {
		args := map[string]any{"id": id}
		if target.SectionID != "" {
			args["section_id"] = target.SectionID
		} else {
			args["project_id"] = target.ProjectID
		}
		cmds = append(cmds, &sync.Command{
			Type: "item_move",
			UUID: uuid.New(),
			Args: args,
		})
	}
	log.Debug("todoist BatchMoveTasks", "count", len(moves))
	_, err := c.cli.ExecuteCommands(ctx, cmds)
	if err != nil {
		return &APIError{Op: "BatchMoveTasks", Err: err}
	}
	return nil
}

// IsRateLimited reports whether the error indicates a Todoist API rate limit (HTTP 429).
// The external library returns errors.New(http.StatusText(429)) for non-200 responses.
func IsRateLimited(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Too Many Requests")
}

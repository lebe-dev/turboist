package todoist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	gosync "sync"
	"time"

	todoist "github.com/lebe-dev/go-todoist-api"

	"github.com/charmbracelet/log"
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

// Client wraps the lebe-dev/go-todoist-api client.
type Client struct {
	cli       *todoist.Client
	mu        gosync.Mutex
	syncToken string
}

// NewClient creates a new Todoist API client with the given API key.
func NewClient(apiKey string) *Client {
	return &Client{
		cli:       todoist.NewClient(apiKey),
		syncToken: "*",
	}
}

// FetchAll fetches tasks, projects, sections and labels via a full sync (sync_token=*).
func (c *Client) FetchAll(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	resp, err := c.cli.Sync(ctx, &todoist.SyncRequest{
		SyncToken: "*",
		ResourceTypes: []todoist.SyncResourceType{
			todoist.SyncResourceItems,
			todoist.SyncResourceProjects,
			todoist.SyncResourceSections,
			todoist.SyncResourceLabels,
		},
	})
	if err != nil {
		log.Debug("todoist FetchAll failed", "err", err, "elapsed", time.Since(start))
		return nil, &APIError{Op: "FetchAll", Err: err}
	}

	result, err := parseSyncResponse(resp)
	if err != nil {
		return nil, &APIError{Op: "FetchAll", Err: err}
	}

	c.mu.Lock()
	c.syncToken = resp.SyncToken
	c.mu.Unlock()

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

	c.mu.Lock()
	token := c.syncToken
	c.mu.Unlock()

	resp, err := c.cli.Sync(ctx, &todoist.SyncRequest{
		SyncToken: token,
		ResourceTypes: []todoist.SyncResourceType{
			todoist.SyncResourceItems,
			todoist.SyncResourceProjects,
			todoist.SyncResourceSections,
			todoist.SyncResourceLabels,
		},
	})
	if err != nil {
		log.Debug("todoist FetchIncremental failed", "err", err, "elapsed", time.Since(start))
		return nil, &APIError{Op: "FetchIncremental", Err: err}
	}

	// Only update sync token for data syncs (no SyncStatus entries).
	// Command responses include SyncStatus; by keeping the pre-mutation token,
	// the next incremental sync picks up the mutation's effects.
	if len(resp.SyncStatus) == 0 {
		c.mu.Lock()
		c.syncToken = resp.SyncToken
		c.mu.Unlock()
	}

	if resp.FullSync {
		result, err := parseSyncResponse(resp)
		if err != nil {
			return nil, &APIError{Op: "FetchIncremental", Err: err}
		}
		log.Debug("todoist FetchIncremental got full sync",
			"tasks", len(result.Tasks),
			"elapsed", time.Since(start),
		)
		return &DeltaResult{FullSync: true, Result: result}, nil
	}

	delta, err := parseSyncDelta(resp)
	if err != nil {
		return nil, &APIError{Op: "FetchIncremental", Err: err}
	}

	log.Debug("todoist FetchIncremental done",
		"upserted_tasks", len(delta.UpsertedTasks),
		"removed_tasks", len(delta.RemovedTaskIDs),
		"elapsed", time.Since(start),
	)
	return delta, nil
}

func parseSyncResponse(resp *todoist.SyncResponse) (*SyncResult, error) {
	result := &SyncResult{
		Tasks:    make([]*Task, 0),
		Projects: make([]*Project, 0),
		Sections: make([]*Section, 0),
		Labels:   make([]*Label, 0),
	}

	for _, raw := range resp.Items {
		var item syncItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("unmarshal sync item: %w", err)
		}
		if item.IsDeleted || item.Checked {
			continue
		}
		result.Tasks = append(result.Tasks, TaskFromSync(&item))
	}

	for _, raw := range resp.Projects {
		var proj syncProject
		if err := json.Unmarshal(raw, &proj); err != nil {
			return nil, fmt.Errorf("unmarshal sync project: %w", err)
		}
		if proj.IsDeleted || proj.IsArchived {
			continue
		}
		result.Projects = append(result.Projects, ProjectFromSync(&proj))
	}

	for _, raw := range resp.Sections {
		var sec syncSection
		if err := json.Unmarshal(raw, &sec); err != nil {
			return nil, fmt.Errorf("unmarshal sync section: %w", err)
		}
		if sec.IsDeleted {
			continue
		}
		result.Sections = append(result.Sections, SectionFromSync(&sec))
	}

	for _, raw := range resp.Labels {
		var lbl syncLabel
		if err := json.Unmarshal(raw, &lbl); err != nil {
			return nil, fmt.Errorf("unmarshal sync label: %w", err)
		}
		if lbl.IsDeleted {
			continue
		}
		result.Labels = append(result.Labels, LabelFromSync(&lbl))
	}

	return result, nil
}

func parseSyncDelta(resp *todoist.SyncResponse) (*DeltaResult, error) {
	delta := &DeltaResult{}

	for _, raw := range resp.Items {
		var item syncItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("unmarshal sync item: %w", err)
		}
		if item.IsDeleted || item.Checked {
			delta.RemovedTaskIDs = append(delta.RemovedTaskIDs, item.ID)
		} else {
			delta.UpsertedTasks = append(delta.UpsertedTasks, TaskFromSync(&item))
		}
	}

	for _, raw := range resp.Projects {
		var proj syncProject
		if err := json.Unmarshal(raw, &proj); err != nil {
			return nil, fmt.Errorf("unmarshal sync project: %w", err)
		}
		if proj.IsDeleted || proj.IsArchived {
			delta.RemovedProjectIDs = append(delta.RemovedProjectIDs, proj.ID)
		} else {
			delta.UpsertedProjects = append(delta.UpsertedProjects, ProjectFromSync(&proj))
		}
	}

	for _, raw := range resp.Sections {
		var sec syncSection
		if err := json.Unmarshal(raw, &sec); err != nil {
			return nil, fmt.Errorf("unmarshal sync section: %w", err)
		}
		if sec.IsDeleted {
			delta.RemovedSectionIDs = append(delta.RemovedSectionIDs, sec.ID)
		} else {
			delta.UpsertedSections = append(delta.UpsertedSections, SectionFromSync(&sec))
		}
	}

	for _, raw := range resp.Labels {
		var lbl syncLabel
		if err := json.Unmarshal(raw, &lbl); err != nil {
			return nil, fmt.Errorf("unmarshal sync label: %w", err)
		}
		if lbl.IsDeleted {
			delta.RemovedLabelIDs = append(delta.RemovedLabelIDs, lbl.ID)
		} else {
			delta.UpsertedLabels = append(delta.UpsertedLabels, LabelFromSync(&lbl))
		}
	}

	return delta, nil
}

// AddTask creates a new task via the REST API and returns the new task ID.
func (c *Client) AddTask(ctx context.Context, args *TaskAddArgs) (string, error) {
	log.Debug("todoist AddTask", "content", args.Content)
	start := time.Now()

	restArgs := &todoist.AddTaskArgs{
		Content: args.Content,
	}
	if args.Description != "" {
		restArgs.Description = &args.Description
	}
	if args.ProjectID != "" {
		restArgs.ProjectID = &args.ProjectID
	}
	if args.SectionID != "" {
		restArgs.SectionID = &args.SectionID
	}
	if args.ParentID != "" {
		restArgs.ParentID = &args.ParentID
	}
	if len(args.Labels) > 0 {
		restArgs.Labels = args.Labels
	}
	if args.Priority != 0 {
		restArgs.Priority = &args.Priority
	}
	if args.DueDate != "" {
		restArgs.DueDate = &args.DueDate
	}
	if args.DueString != "" {
		restArgs.DueString = &args.DueString
	}
	if args.DueLang != "" {
		restArgs.DueLang = &args.DueLang
	}

	task, err := c.cli.AddTask(ctx, restArgs)
	if err != nil {
		log.Debug("todoist AddTask failed", "err", err, "elapsed", time.Since(start))
		return "", &APIError{Op: "AddTask", Err: err}
	}
	log.Debug("todoist AddTask done", "id", task.ID, "elapsed", time.Since(start))
	return task.ID, nil
}

// UpdateTask updates an existing task via the REST API.
func (c *Client) UpdateTask(ctx context.Context, args *TaskUpdateArgs) error {
	log.Debug("todoist UpdateTask", "id", args.ID)
	start := time.Now()

	restArgs := &todoist.UpdateTaskArgs{
		Content:     args.Content,
		Description: args.Description,
		Labels:      args.Labels,
		Priority:    args.Priority,
		DueDate:     args.DueDate,
		DueString:   args.DueString,
		DueLang:     args.DueLang,
	}

	_, err := c.cli.UpdateTask(ctx, args.ID, restArgs)
	if err != nil {
		log.Debug("todoist UpdateTask failed", "id", args.ID, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "UpdateTask", Err: err}
	}
	log.Debug("todoist UpdateTask done", "id", args.ID, "elapsed", time.Since(start))
	return nil
}

// SetTasksLabels updates labels for multiple tasks in a single sync call.
// Unlike UpdateTask, this always sends the labels field (even when empty)
// to work around the omitempty tag on UpdateTaskArgs.Labels.
func (c *Client) SetTasksLabels(ctx context.Context, updates map[string][]string) error {
	cmds := make([]todoist.SyncCommand, 0, len(updates))
	for id, labels := range updates {
		cmds = append(cmds, todoist.CreateCommand(
			todoist.SyncCmdItemUpdate,
			map[string]any{"id": id, "labels": labels},
		))
	}
	_, err := c.cli.Sync(ctx, &todoist.SyncRequest{
		Commands:  cmds,
		SyncToken: "*",
	})
	if err != nil {
		return &APIError{Op: "SetTasksLabels", Err: err}
	}
	return nil
}

// FetchCompletedTasks returns tasks completed between since and until.
func (c *Client) FetchCompletedTasks(ctx context.Context, since, until time.Time) ([]*Task, error) {
	tasks, err := c.fetchAllCompletedByDate(ctx, &todoist.GetCompletedTasksByCompletionDateArgs{
		Since: since.Format(time.RFC3339),
		Until: until.Format(time.RFC3339),
	})
	if err != nil {
		return nil, &APIError{Op: "FetchCompletedTasks", Err: err}
	}
	return tasks, nil
}

// FetchCompletedBySection returns completed root tasks in a specific project section (last 30 days).
func (c *Client) FetchCompletedBySection(ctx context.Context, projectID, sectionID string) ([]*Task, error) {
	now := time.Now()
	since := now.AddDate(0, -1, 0) // 1 month back

	allTasks, err := c.fetchAllCompletedByDate(ctx, &todoist.GetCompletedTasksByCompletionDateArgs{
		Since:     since.Format(time.RFC3339),
		Until:     now.Format(time.RFC3339),
		ProjectID: &projectID,
		SectionID: &sectionID,
	})
	if err != nil {
		return nil, &APIError{Op: "FetchCompletedBySection", Err: err}
	}

	tasks := make([]*Task, 0, len(allTasks))
	for _, t := range allTasks {
		if t.ParentID == nil && t.SectionID != nil && *t.SectionID == sectionID {
			tasks = append(tasks, t)
		}
	}
	return tasks, nil
}

// FetchCompletedSubtasks returns subtasks of the given parent completed in the last 90 days.
func (c *Client) FetchCompletedSubtasks(ctx context.Context, parentID string) ([]*Task, error) {
	now := time.Now()
	since := now.AddDate(0, -3, 0) // 3 months back

	tasks, err := c.fetchAllCompletedByDate(ctx, &todoist.GetCompletedTasksByCompletionDateArgs{
		Since:    since.Format(time.RFC3339),
		Until:    now.Format(time.RFC3339),
		ParentID: &parentID,
	})
	if err != nil {
		return nil, &APIError{Op: "FetchCompletedSubtasks", Err: err}
	}
	return tasks, nil
}

// fetchAllCompletedByDate fetches all pages of completed tasks matching the given args.
func (c *Client) fetchAllCompletedByDate(ctx context.Context, args *todoist.GetCompletedTasksByCompletionDateArgs) ([]*Task, error) {
	var all []*Task
	for {
		page, err := c.cli.GetCompletedTasksByCompletionDate(ctx, args)
		if err != nil {
			return nil, err
		}
		for i := range page.Results {
			all = append(all, taskFromREST(&page.Results[i]))
		}
		if !page.HasNextPage() {
			break
		}
		args.Cursor = page.NextCursor
	}
	return all, nil
}

// taskFromREST converts a lebe-dev/go-todoist-api Task to our internal Task model.
func taskFromREST(t *todoist.Task) *Task {
	task := &Task{
		ID:          t.ID,
		Content:     t.Content,
		Description: t.Description,
		ProjectID:   t.ProjectID,
		SectionID:   t.SectionID,
		ParentID:    t.ParentID,
		Labels:      t.Labels,
		Priority:    t.Priority,
		CompletedAt: t.CompletedAt,
		Children:    []*Task{},
	}

	if t.AddedAt != nil {
		task.AddedAt = *t.AddedAt
	}

	if t.Due != nil && t.Due.Date != "" {
		date := t.Due.Date
		if len(date) > 10 {
			date = date[:10]
		}
		task.Due = &Due{
			Date:      date,
			Recurring: t.Due.IsRecurring,
		}
	}

	if task.Labels == nil {
		task.Labels = []string{}
	}

	return task
}

// MoveTask moves a task to be a subtask of the given parent via the REST API.
func (c *Client) MoveTask(ctx context.Context, id string, parentID string) error {
	log.Debug("todoist MoveTask", "id", id, "parent_id", parentID)
	start := time.Now()
	_, err := c.cli.MoveTask(ctx, id, &todoist.MoveTaskArgs{ParentID: &parentID})
	if err != nil {
		log.Debug("todoist MoveTask failed", "id", id, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "MoveTask", Err: err}
	}
	log.Debug("todoist MoveTask done", "id", id, "elapsed", time.Since(start))
	return nil
}

// MoveTaskToProject moves a task to the given project via the REST API.
func (c *Client) MoveTaskToProject(ctx context.Context, id string, projectID string) error {
	log.Debug("todoist MoveTaskToProject", "id", id, "project_id", projectID)
	start := time.Now()
	_, err := c.cli.MoveTask(ctx, id, &todoist.MoveTaskArgs{ProjectID: &projectID})
	if err != nil {
		log.Debug("todoist MoveTaskToProject failed", "id", id, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "MoveTaskToProject", Err: err}
	}
	log.Debug("todoist MoveTaskToProject done", "id", id, "elapsed", time.Since(start))
	return nil
}

// CompleteTask closes a task via the REST API.
// REST close handles both recurring (advances to next occurrence) and
// non-recurring (archives permanently) tasks.
func (c *Client) CompleteTask(ctx context.Context, id string) error {
	log.Debug("todoist CompleteTask", "id", id)
	start := time.Now()
	if err := c.cli.CloseTask(ctx, id); err != nil {
		log.Debug("todoist CompleteTask failed", "id", id, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "CompleteTask", Err: err}
	}
	log.Debug("todoist CompleteTask done", "id", id, "elapsed", time.Since(start))
	return nil
}

// DeleteTask deletes a task and all its sub-tasks via the REST API.
func (c *Client) DeleteTask(ctx context.Context, id string) error {
	log.Debug("todoist DeleteTask", "id", id)
	start := time.Now()
	if err := c.cli.DeleteTask(ctx, id); err != nil {
		log.Debug("todoist DeleteTask failed", "id", id, "err", err, "elapsed", time.Since(start))
		return &APIError{Op: "DeleteTask", Err: err}
	}
	log.Debug("todoist DeleteTask done", "id", id, "elapsed", time.Since(start))
	return nil
}

// DecomposeTask creates N new tasks (inheriting properties from src) and deletes the source
// task in a single Todoist Sync API batch call.
func (c *Client) DecomposeTask(ctx context.Context, src *Task, newContents []string) error {
	cmds := make([]todoist.SyncCommand, 0, len(newContents)+1)

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
		cmds = append(cmds, todoist.CreateCommand(todoist.SyncCmdItemAdd, args))
	}

	cmds = append(cmds, todoist.CreateCommand(
		todoist.SyncCmdItemDelete,
		map[string]any{"id": src.ID},
	))

	log.Debug("todoist DecomposeTask", "src", src.ID, "new_tasks", len(newContents))
	_, err := c.cli.Sync(ctx, &todoist.SyncRequest{
		Commands:  cmds,
		SyncToken: "*",
	})
	if err != nil {
		return &APIError{Op: "DecomposeTask", Err: err}
	}
	return nil
}

// BatchMoveTasksToProject moves multiple tasks to their target projects in a single sync call.
// The moves map is taskID -> projectID.
func (c *Client) BatchMoveTasksToProject(ctx context.Context, moves map[string]string) error {
	if len(moves) == 0 {
		return nil
	}
	cmds := make([]todoist.SyncCommand, 0, len(moves))
	for id, projectID := range moves {
		cmds = append(cmds, todoist.CreateCommand(
			todoist.SyncCmdItemMove,
			map[string]any{"id": id, "project_id": projectID},
		))
	}
	log.Debug("todoist BatchMoveTasksToProject", "count", len(moves))
	_, err := c.cli.Sync(ctx, &todoist.SyncRequest{
		Commands:  cmds,
		SyncToken: "*",
	})
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
	cmds := make([]todoist.SyncCommand, 0, len(moves))
	for id, target := range moves {
		args := map[string]any{"id": id}
		if target.SectionID != "" {
			args["section_id"] = target.SectionID
		} else {
			args["project_id"] = target.ProjectID
		}
		cmds = append(cmds, todoist.CreateCommand(todoist.SyncCmdItemMove, args))
	}
	log.Debug("todoist BatchMoveTasks", "count", len(moves))
	_, err := c.cli.Sync(ctx, &todoist.SyncRequest{
		Commands:  cmds,
		SyncToken: "*",
	})
	if err != nil {
		return &APIError{Op: "BatchMoveTasks", Err: err}
	}
	return nil
}

// AddSection creates a new section in a project via the REST API and returns the new section ID.
func (c *Client) AddSection(ctx context.Context, name string, projectID string) (string, error) {
	log.Debug("todoist AddSection", "name", name, "project_id", projectID)
	start := time.Now()

	section, err := c.cli.AddSection(ctx, &todoist.AddSectionArgs{
		Name:      name,
		ProjectID: projectID,
	})
	if err != nil {
		log.Debug("todoist AddSection failed", "err", err, "elapsed", time.Since(start))
		return "", &APIError{Op: "AddSection", Err: err}
	}
	log.Debug("todoist AddSection done", "id", section.ID, "elapsed", time.Since(start))
	return section.ID, nil
}

// IsRateLimited reports whether the error indicates a Todoist API rate limit (HTTP 429).
func IsRateLimited(err error) bool {
	var reqErr *todoist.TodoistRequestError
	if errors.As(err, &reqErr) {
		return reqErr.HTTPStatusCode == 429
	}
	return false
}

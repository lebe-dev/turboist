package todoist

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/CnTeng/todoist-api-go/rest"
	"github.com/CnTeng/todoist-api-go/sync"
	extclient "github.com/CnTeng/todoist-api-go/todoist"
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

func (c *Client) fullSync(ctx context.Context) (*sync.SyncResponse, error) {
	resp, err := c.cli.SyncWithAutoToken(ctx, true)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// FetchAll fetches tasks, projects, sections and labels in a single API call.
func (c *Client) FetchAll(ctx context.Context) (*SyncResult, error) {
	resp, err := c.fullSync(ctx)
	if err != nil {
		return nil, &APIError{Op: "FetchAll", Err: err}
	}

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

	return result, nil
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
	resp, err := c.taskSvc.AddTask(ctx, args)
	if err != nil {
		return "", &APIError{Op: "AddTask", Err: err}
	}
	for _, id := range resp.TempIDMapping {
		return id, nil
	}
	return "", nil
}

// UpdateTask updates an existing task via the Todoist API.
func (c *Client) UpdateTask(ctx context.Context, args *sync.TaskUpdateArgs) error {
	_, err := c.taskSvc.UpdateTask(ctx, args)
	if err != nil {
		return &APIError{Op: "UpdateTask", Err: err}
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

// CompleteTask closes a task via the Todoist API.
func (c *Client) CompleteTask(ctx context.Context, id string) error {
	_, err := c.taskSvc.CloseTask(ctx, &sync.TaskCloseArgs{ID: id})
	if err != nil {
		return &APIError{Op: "CompleteTask", Err: err}
	}
	return nil
}

// DeleteTask deletes a task and all its sub-tasks via the Todoist API.
func (c *Client) DeleteTask(ctx context.Context, id string) error {
	_, err := c.taskSvc.DeleteTask(ctx, &sync.TaskDeleteArgs{ID: id})
	if err != nil {
		return &APIError{Op: "DeleteTask", Err: err}
	}
	return nil
}

package todoist

import (
	"context"
	"sync"
	"time"

	synctodoist "github.com/CnTeng/todoist-api-go/sync"
	"github.com/charmbracelet/log"
	"golang.org/x/sync/singleflight"
)

const (
	backoffInitial    = 5 * time.Second
	backoffMax        = 5 * time.Minute
	coldStartRetries  = 10
	coldStartInterval = 10 * time.Second
	refreshDebounce   = 1 * time.Second
)

// cacheClient abstracts the Todoist client methods used by Cache, enabling testing.
type cacheClient interface {
	FetchAll(ctx context.Context) (*SyncResult, error)
	AddTask(ctx context.Context, args *synctodoist.TaskAddArgs) (string, error)
	UpdateTask(ctx context.Context, args *synctodoist.TaskUpdateArgs) error
	MoveTask(ctx context.Context, id string, parentID string) error
	MoveTaskToProject(ctx context.Context, id string, projectID string) error
	CompleteTask(ctx context.Context, id string) error
	DeleteTask(ctx context.Context, id string) error
	DecomposeTask(ctx context.Context, src *Task, newContents []string) error
	BatchMoveTasksToProject(ctx context.Context, moves map[string]string) error
	BatchMoveTasks(ctx context.Context, moves map[string]MoveTarget) error
}

// Cache holds an in-memory snapshot of Todoist data and keeps it fresh via Refresh.
type Cache struct {
	mu           sync.RWMutex
	tasks        []*Task
	projects     []*Project
	sections     []*Section
	labels       []*Label
	lastSyncedAt time.Time
	warmed       bool

	client         cacheClient
	sfg            singleflight.Group
	onRefresh      func()
	taskEnricher   func(tasks []*Task)
	refreshTimer   *time.Timer
	refreshTimerMu sync.Mutex
}

// NewCache creates a Cache, performs a synchronous cold-start Refresh with retries, and panics
// if all attempts fail. Rate-limited responses are retried with a fixed interval.
func NewCache(client *Client) *Cache {
	c := &Cache{client: client}
	var lastErr error
	for attempt := range coldStartRetries {
		if err := c.Refresh(context.Background()); err != nil {
			lastErr = err
			log.Warn("cache cold start attempt failed, retrying",
				"attempt", attempt+1,
				"max", coldStartRetries,
				"err", err,
				"retry_in", coldStartInterval,
			)
			time.Sleep(coldStartInterval)
			continue
		}
		return c
	}
	panic("todoist cache cold start failed after retries: " + lastErr.Error())
}

// Refresh fetches all data from Todoist and atomically replaces the cached snapshot.
func (c *Cache) Refresh(ctx context.Context) error {
	start := time.Now()
	result, err := c.client.FetchAll(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.tasks = result.Tasks
	c.projects = result.Projects
	c.sections = result.Sections
	c.labels = result.Labels
	c.lastSyncedAt = time.Now()
	c.warmed = true
	if c.taskEnricher != nil {
		c.taskEnricher(c.tasks)
	}
	c.mu.Unlock()

	log.Debug("cache refreshed",
		"tasks", len(result.Tasks),
		"projects", len(result.Projects),
		"elapsed", time.Since(start),
	)

	if c.onRefresh != nil {
		c.onRefresh()
	}

	return nil
}

// Warmed reports whether the cache has been successfully populated at least once.
func (c *Cache) Warmed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.warmed
}

// Tasks returns the cached task list.
func (c *Cache) Tasks() []*Task {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tasks
}

// Projects returns the cached project list.
func (c *Cache) Projects() []*Project {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.projects
}

// Sections returns the cached section list.
func (c *Cache) Sections() []*Section {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sections
}

// Labels returns the cached label list.
func (c *Cache) Labels() []*Label {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.labels
}

// LastSyncedAt returns the time of the most recent successful sync.
func (c *Cache) LastSyncedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSyncedAt
}

// InboxProjectID returns the ID of the Todoist Inbox project, or empty string if not found.
func (c *Cache) InboxProjectID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, p := range c.projects {
		if p.IsInbox {
			return p.ID
		}
	}
	return ""
}

// Client returns the underlying Todoist API client.
func (c *Cache) Client() *Client {
	return c.client.(*Client)
}

// SetOnRefresh sets a callback that is invoked after every successful cache refresh.
func (c *Cache) SetOnRefresh(fn func()) {
	c.onRefresh = fn
}

// SetTaskEnricher sets a callback that enriches tasks with additional data (e.g. postpone counts)
// after each cache refresh, while still holding the write lock.
func (c *Cache) SetTaskEnricher(fn func(tasks []*Task)) {
	c.taskEnricher = fn
}

// ScheduleRefresh schedules a debounced cache refresh after a mutation.
// Multiple calls within the debounce window are coalesced into a single refresh.
func (c *Cache) ScheduleRefresh() {
	c.refreshTimerMu.Lock()
	defer c.refreshTimerMu.Unlock()

	if c.refreshTimer != nil {
		c.refreshTimer.Stop()
	}
	c.refreshTimer = time.AfterFunc(refreshDebounce, func() {
		_, err, _ := c.sfg.Do("refresh", func() (any, error) {
			return nil, c.Refresh(context.Background())
		})
		if err != nil {
			log.Warn("debounced cache refresh failed", "err", err)
		}
	})
}

// RefreshAfterMutation schedules a debounced cache refresh.
// Fire-and-forget: errors are logged, not returned, since the mutation
// has already succeeded at Todoist.
func (c *Cache) RefreshAfterMutation() {
	c.ScheduleRefresh()
}

// AddTask creates a task via the Todoist API and schedules a cache refresh.
// Returns the new task ID.
func (c *Cache) AddTask(ctx context.Context, args *synctodoist.TaskAddArgs) (string, error) {
	newID, err := c.client.AddTask(ctx, args)
	if err != nil {
		return "", err
	}
	c.ScheduleRefresh()
	return newID, nil
}

// UpdateTask updates a task via the Todoist API and schedules a cache refresh.
func (c *Cache) UpdateTask(ctx context.Context, args *synctodoist.TaskUpdateArgs) error {
	if err := c.client.UpdateTask(ctx, args); err != nil {
		return err
	}
	c.ScheduleRefresh()
	return nil
}

// MoveTask moves a task to be a subtask of the given parent and schedules a cache refresh.
func (c *Cache) MoveTask(ctx context.Context, id string, parentID string) error {
	if err := c.client.MoveTask(ctx, id, parentID); err != nil {
		return err
	}
	c.ScheduleRefresh()
	return nil
}

// MoveTaskToProject moves a task to the given project and schedules a cache refresh.
func (c *Cache) MoveTaskToProject(ctx context.Context, id string, projectID string) error {
	if err := c.client.MoveTaskToProject(ctx, id, projectID); err != nil {
		return err
	}
	c.ScheduleRefresh()
	return nil
}

// CompleteTask closes a task via the Todoist API and schedules a cache refresh.
func (c *Cache) CompleteTask(ctx context.Context, id string) error {
	if err := c.client.CompleteTask(ctx, id); err != nil {
		return err
	}
	c.ScheduleRefresh()
	return nil
}

// DeleteTask deletes a task via the Todoist API and schedules a cache refresh.
func (c *Cache) DeleteTask(ctx context.Context, id string) error {
	if err := c.client.DeleteTask(ctx, id); err != nil {
		return err
	}
	c.ScheduleRefresh()
	return nil
}

// DecomposeTask creates new tasks from the source task and deletes the original,
// then schedules a cache refresh.
func (c *Cache) DecomposeTask(ctx context.Context, src *Task, newContents []string) error {
	if err := c.client.DecomposeTask(ctx, src, newContents); err != nil {
		return err
	}
	c.ScheduleRefresh()
	return nil
}

// BatchMoveTasksToProject moves multiple tasks to their target projects in a single API call
// and schedules a cache refresh.
func (c *Cache) BatchMoveTasksToProject(ctx context.Context, moves map[string]string) error {
	if err := c.client.BatchMoveTasksToProject(ctx, moves); err != nil {
		return err
	}
	c.ScheduleRefresh()
	return nil
}

// BatchMoveTasks moves multiple tasks to their targets (project or section) in a single API call
// and schedules a cache refresh.
func (c *Cache) BatchMoveTasks(ctx context.Context, moves map[string]MoveTarget) error {
	if err := c.client.BatchMoveTasks(ctx, moves); err != nil {
		return err
	}
	c.ScheduleRefresh()
	return nil
}

// StartPolling launches a background goroutine that refreshes the cache every interval.
// On error it retries with exponential backoff (5s → 10s → 20s → … → 5min).
// On recovery it resets the delay back to interval.
// Stale data continues to be served from the cache during retries.
// The goroutine stops when ctx is cancelled.
func (c *Cache) StartPolling(ctx context.Context, interval time.Duration) {
	go func() {
		delay := interval
		backoff := time.Duration(0)

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}

			if err := c.Refresh(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				if backoff == 0 {
					backoff = backoffInitial
				} else {
					backoff = min(backoff*2, backoffMax)
				}
				log.Error("cache refresh failed, will retry", "err", err, "in", backoff)
				delay = backoff
				continue
			}

			if backoff > 0 {
				log.Info("cache refresh recovered")
				backoff = 0
			}
			delay = interval
		}
	}()
}

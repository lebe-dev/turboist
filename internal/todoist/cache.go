package todoist

import (
	"context"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"golang.org/x/sync/singleflight"
)

const (
	backoffInitial    = 5 * time.Second
	backoffMax        = 5 * time.Minute
	coldStartRetries  = 10
	coldStartInterval = 10 * time.Second
	refreshDebounce   = 1 * time.Second
	evictGracePeriod  = 45 * time.Second
)

// cacheClient abstracts the Todoist client methods used by Cache, enabling testing.
type cacheClient interface {
	FetchAll(ctx context.Context) (*SyncResult, error)
	FetchIncremental(ctx context.Context) (*DeltaResult, error)
	AddTask(ctx context.Context, args *TaskAddArgs) (string, error)
	UpdateTask(ctx context.Context, args *TaskUpdateArgs) error
	MoveTask(ctx context.Context, id string, parentID string) error
	MoveTaskToProject(ctx context.Context, id string, projectID string) error
	CompleteTask(ctx context.Context, id string) error
	DeleteTask(ctx context.Context, id string) error
	DecomposeTask(ctx context.Context, src *Task, newContents []string, opts DecomposeOpts) error
	BatchMoveTasksToProject(ctx context.Context, moves map[string]string) error
	BatchMoveTasks(ctx context.Context, moves map[string]MoveTarget) error
	AddSection(ctx context.Context, name string, projectID string) (string, error)
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

	// Recently evicted task IDs — prevents Todoist eventual-consistency lag
	// from re-adding tasks that were just completed/deleted.
	evictedMu sync.Mutex
	evicted   map[string]time.Time
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

// Refresh fetches data from Todoist and updates the cache.
// On cold start (first call), it does a full sync. Subsequent calls use incremental
// sync to work around a Todoist Sync v1 API issue where full sync returns stale data.
func (c *Cache) Refresh(ctx context.Context) error {
	start := time.Now()

	if !c.Warmed() {
		return c.fullRefresh(ctx, start)
	}
	return c.incrementalRefresh(ctx, start)
}

func (c *Cache) fullRefresh(ctx context.Context, start time.Time) error {
	result, err := c.client.FetchAll(ctx)
	if err != nil {
		return err
	}

	result.Tasks = c.filterEvicted(result.Tasks)

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

	log.Debug("cache refreshed (full)",
		"tasks", len(result.Tasks),
		"projects", len(result.Projects),
		"elapsed", time.Since(start),
	)

	if c.onRefresh != nil {
		c.onRefresh()
	}
	return nil
}

func (c *Cache) incrementalRefresh(ctx context.Context, start time.Time) error {
	delta, err := c.client.FetchIncremental(ctx)
	if err != nil {
		return err
	}

	// Server may return a full sync if the token expired.
	if delta.FullSync {
		delta.Result.Tasks = c.filterEvicted(delta.Result.Tasks)

		c.mu.Lock()
		c.tasks = delta.Result.Tasks
		c.projects = delta.Result.Projects
		c.sections = delta.Result.Sections
		c.labels = delta.Result.Labels
		c.lastSyncedAt = time.Now()
		if c.taskEnricher != nil {
			c.taskEnricher(c.tasks)
		}
		c.mu.Unlock()

		log.Debug("cache refreshed (full via incremental)",
			"tasks", len(delta.Result.Tasks),
			"elapsed", time.Since(start),
		)
		if c.onRefresh != nil {
			c.onRefresh()
		}
		return nil
	}

	// Apply delta to existing cache.
	c.mu.Lock()
	c.tasks = c.filterEvicted(applyDelta(c.tasks, delta.UpsertedTasks, delta.RemovedTaskIDs))
	c.projects = applyDeltaProjects(c.projects, delta.UpsertedProjects, delta.RemovedProjectIDs)
	c.sections = applyDeltaSections(c.sections, delta.UpsertedSections, delta.RemovedSectionIDs)
	c.labels = applyDeltaLabels(c.labels, delta.UpsertedLabels, delta.RemovedLabelIDs)
	c.lastSyncedAt = time.Now()
	if c.taskEnricher != nil {
		c.taskEnricher(c.tasks)
	}
	totalTasks := len(c.tasks)
	c.mu.Unlock()

	log.Debug("cache refreshed (incremental)",
		"upserted_tasks", len(delta.UpsertedTasks),
		"removed_tasks", len(delta.RemovedTaskIDs),
		"total_tasks", totalTasks,
		"elapsed", time.Since(start),
	)

	if c.onRefresh != nil {
		c.onRefresh()
	}
	return nil
}

// applyDelta merges upserted tasks and removes deleted ones from the existing list.
func applyDelta(existing []*Task, upserted []*Task, removedIDs []string) []*Task {
	if len(upserted) == 0 && len(removedIDs) == 0 {
		return existing
	}

	remove := make(map[string]bool, len(removedIDs))
	for _, id := range removedIDs {
		remove[id] = true
	}
	// Also remove upserted IDs so we can re-add them fresh.
	for _, t := range upserted {
		remove[t.ID] = true
	}

	result := make([]*Task, 0, len(existing))
	for _, t := range existing {
		if !remove[t.ID] {
			result = append(result, t)
		}
	}
	result = append(result, upserted...)
	return result
}

func applyDeltaProjects(existing []*Project, upserted []*Project, removedIDs []string) []*Project {
	if len(upserted) == 0 && len(removedIDs) == 0 {
		return existing
	}
	remove := make(map[string]bool, len(removedIDs)+len(upserted))
	for _, id := range removedIDs {
		remove[id] = true
	}
	for _, p := range upserted {
		remove[p.ID] = true
	}
	result := make([]*Project, 0, len(existing))
	for _, p := range existing {
		if !remove[p.ID] {
			result = append(result, p)
		}
	}
	return append(result, upserted...)
}

func applyDeltaSections(existing []*Section, upserted []*Section, removedIDs []string) []*Section {
	if len(upserted) == 0 && len(removedIDs) == 0 {
		return existing
	}
	remove := make(map[string]bool, len(removedIDs)+len(upserted))
	for _, id := range removedIDs {
		remove[id] = true
	}
	for _, s := range upserted {
		remove[s.ID] = true
	}
	result := make([]*Section, 0, len(existing))
	for _, s := range existing {
		if !remove[s.ID] {
			result = append(result, s)
		}
	}
	return append(result, upserted...)
}

func applyDeltaLabels(existing []*Label, upserted []*Label, removedIDs []string) []*Label {
	if len(upserted) == 0 && len(removedIDs) == 0 {
		return existing
	}
	remove := make(map[string]bool, len(removedIDs)+len(upserted))
	for _, id := range removedIDs {
		remove[id] = true
	}
	for _, l := range upserted {
		remove[l.ID] = true
	}
	result := make([]*Label, 0, len(existing))
	for _, l := range existing {
		if !remove[l.ID] {
			result = append(result, l)
		}
	}
	return append(result, upserted...)
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

// FetchCompletedBySection returns completed root tasks for a specific project section.
func (c *Cache) FetchCompletedBySection(ctx context.Context, projectID, sectionID string) ([]*Task, error) {
	return c.Client().FetchCompletedBySection(ctx, projectID, sectionID)
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
func (c *Cache) AddTask(ctx context.Context, args *TaskAddArgs) (string, error) {
	newID, err := c.client.AddTask(ctx, args)
	if err != nil {
		return "", err
	}
	c.ScheduleRefresh()
	return newID, nil
}

// UpdateTask updates a task via the Todoist API and schedules a cache refresh.
func (c *Cache) UpdateTask(ctx context.Context, args *TaskUpdateArgs) error {
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

// CompleteTask completes a task via the Todoist API and schedules a cache refresh.
// REST close handles both recurring (advances to next occurrence) and
// non-recurring (archives permanently) tasks.
func (c *Cache) CompleteTask(ctx context.Context, id string) error {
	if err := c.client.CompleteTask(ctx, id); err != nil {
		return err
	}
	c.evictTask(id)
	c.ScheduleRefresh()
	return nil
}

// DeleteTask deletes a task via the Todoist API and schedules a cache refresh.
func (c *Cache) DeleteTask(ctx context.Context, id string) error {
	if err := c.client.DeleteTask(ctx, id); err != nil {
		return err
	}
	c.evictTask(id)
	c.ScheduleRefresh()
	return nil
}

// evictTask optimistically removes a task (and its subtasks) from the in-memory
// cache and broadcasts the change. This prevents stale data from appearing if the
// deferred Todoist sync hasn't caught up yet (eventual consistency).
func (c *Cache) evictTask(id string) {
	c.mu.Lock()
	// Collect the target and all its descendants
	evict := map[string]bool{id: true}
	changed := true
	for changed {
		changed = false
		for _, t := range c.tasks {
			if t.ParentID != nil && evict[*t.ParentID] && !evict[t.ID] {
				evict[t.ID] = true
				changed = true
			}
		}
	}
	filtered := make([]*Task, 0, len(c.tasks))
	for _, t := range c.tasks {
		if !evict[t.ID] {
			filtered = append(filtered, t)
		}
	}
	c.tasks = filtered
	c.mu.Unlock()

	// Record evicted IDs so filterEvicted suppresses them in subsequent refreshes
	now := time.Now()
	c.evictedMu.Lock()
	if c.evicted == nil {
		c.evicted = make(map[string]time.Time)
	}
	for id := range evict {
		c.evicted[id] = now
	}
	c.evictedMu.Unlock()

	if c.onRefresh != nil {
		c.onRefresh()
	}
}

// ClearEvicted removes all entries from the evicted set, allowing
// a subsequent Refresh to return the full Todoist state.
func (c *Cache) ClearEvicted() {
	c.evictedMu.Lock()
	c.evicted = nil
	c.evictedMu.Unlock()
}

// filterEvicted removes tasks that were recently completed/deleted from the
// fetched result, preventing Todoist eventual-consistency lag from resurrecting them.
// Expired entries are cleaned up on each call.
func (c *Cache) filterEvicted(tasks []*Task) []*Task {
	c.evictedMu.Lock()
	defer c.evictedMu.Unlock()

	if len(c.evicted) == 0 {
		return tasks
	}

	// Expire old entries
	now := time.Now()
	for id, t := range c.evicted {
		if now.Sub(t) > evictGracePeriod {
			delete(c.evicted, id)
		}
	}
	if len(c.evicted) == 0 {
		return tasks
	}

	filtered := make([]*Task, 0, len(tasks))
	for _, t := range tasks {
		if _, ok := c.evicted[t.ID]; ok {
			log.Debug("filterEvicted: suppressing task", "id", t.ID, "content", t.Content)
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// DecomposeTask creates new tasks from the source task and deletes the original,
// then schedules a cache refresh.
func (c *Cache) DecomposeTask(ctx context.Context, src *Task, newContents []string, opts DecomposeOpts) error {
	if err := c.client.DecomposeTask(ctx, src, newContents, opts); err != nil {
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

// AddSection creates a section in a project via the Todoist API and schedules a cache refresh.
// Returns the new section ID.
func (c *Cache) AddSection(ctx context.Context, name string, projectID string) (string, error) {
	newID, err := c.client.AddSection(ctx, name, projectID)
	if err != nil {
		return "", err
	}
	c.ScheduleRefresh()
	return newID, nil
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

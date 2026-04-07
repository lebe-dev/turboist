package todoist

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	synctodoist "github.com/CnTeng/todoist-api-go/sync"
)

// mockCacheClient implements cacheClient for tests.
type mockCacheClient struct {
	fetchAllFn                func(ctx context.Context) (*SyncResult, error)
	addTaskFn                 func(ctx context.Context, args *synctodoist.TaskAddArgs) (string, error)
	updateTaskFn              func(ctx context.Context, args *synctodoist.TaskUpdateArgs) error
	deleteTaskFn              func(ctx context.Context, id string) error
	moveTaskFn                func(ctx context.Context, id string, parentID string) error
	moveTaskToProjectFn       func(ctx context.Context, id string, projectID string) error
	completeTaskFn            func(ctx context.Context, id string) error
	decomposeTaskFn           func(ctx context.Context, src *Task, newContents []string) error
	batchMoveTasksToProjectFn func(ctx context.Context, moves map[string]string) error
	batchMoveTasksFn          func(ctx context.Context, moves map[string]MoveTarget) error
}

func (m *mockCacheClient) FetchAll(ctx context.Context) (*SyncResult, error) {
	if m.fetchAllFn != nil {
		return m.fetchAllFn(ctx)
	}
	return &SyncResult{}, nil
}

func (m *mockCacheClient) AddTask(ctx context.Context, args *synctodoist.TaskAddArgs) (string, error) {
	if m.addTaskFn != nil {
		return m.addTaskFn(ctx, args)
	}
	return "new-id", nil
}

func (m *mockCacheClient) UpdateTask(ctx context.Context, args *synctodoist.TaskUpdateArgs) error {
	if m.updateTaskFn != nil {
		return m.updateTaskFn(ctx, args)
	}
	return nil
}

func (m *mockCacheClient) DeleteTask(ctx context.Context, id string) error {
	if m.deleteTaskFn != nil {
		return m.deleteTaskFn(ctx, id)
	}
	return nil
}

func (m *mockCacheClient) MoveTask(ctx context.Context, id string, parentID string) error {
	if m.moveTaskFn != nil {
		return m.moveTaskFn(ctx, id, parentID)
	}
	return nil
}

func (m *mockCacheClient) MoveTaskToProject(ctx context.Context, id string, projectID string) error {
	if m.moveTaskToProjectFn != nil {
		return m.moveTaskToProjectFn(ctx, id, projectID)
	}
	return nil
}

func (m *mockCacheClient) CompleteTask(ctx context.Context, id string) error {
	if m.completeTaskFn != nil {
		return m.completeTaskFn(ctx, id)
	}
	return nil
}

func (m *mockCacheClient) CloseTask(ctx context.Context, id string) error {
	if m.completeTaskFn != nil {
		return m.completeTaskFn(ctx, id)
	}
	return nil
}

func (m *mockCacheClient) DecomposeTask(ctx context.Context, src *Task, newContents []string) error {
	if m.decomposeTaskFn != nil {
		return m.decomposeTaskFn(ctx, src, newContents)
	}
	return nil
}

func (m *mockCacheClient) BatchMoveTasksToProject(ctx context.Context, moves map[string]string) error {
	if m.batchMoveTasksToProjectFn != nil {
		return m.batchMoveTasksToProjectFn(ctx, moves)
	}
	return nil
}

func (m *mockCacheClient) BatchMoveTasks(ctx context.Context, moves map[string]MoveTarget) error {
	if m.batchMoveTasksFn != nil {
		return m.batchMoveTasksFn(ctx, moves)
	}
	return nil
}

func newTestCache(client cacheClient) *Cache {
	return &Cache{
		client: client,
		warmed: true,
	}
}

func TestAddTask_SucceedsWhenFetchAllFails(t *testing.T) {
	mock := &mockCacheClient{
		addTaskFn: func(_ context.Context, _ *synctodoist.TaskAddArgs) (string, error) {
			return "task-123", nil
		},
		fetchAllFn: func(_ context.Context) (*SyncResult, error) {
			return nil, errors.New("Too Many Requests")
		},
	}
	cache := newTestCache(mock)

	id, err := cache.AddTask(context.Background(), &synctodoist.TaskAddArgs{Content: "test"})
	if err != nil {
		t.Fatalf("got err %v, want nil (FetchAll failure should not propagate)", err)
	}
	if id != "task-123" {
		t.Errorf("got id %q, want %q", id, "task-123")
	}
}

func TestAddTask_PropagatesMutationError(t *testing.T) {
	mutationErr := errors.New("network error")
	mock := &mockCacheClient{
		addTaskFn: func(_ context.Context, _ *synctodoist.TaskAddArgs) (string, error) {
			return "", mutationErr
		},
	}
	cache := newTestCache(mock)

	_, err := cache.AddTask(context.Background(), &synctodoist.TaskAddArgs{Content: "test"})
	if err == nil {
		t.Fatal("got nil error, want mutation error to propagate")
	}
	if !errors.Is(err, mutationErr) {
		t.Errorf("got err %v, want %v", err, mutationErr)
	}
}

func TestDeleteTask_SucceedsWhenFetchAllFails(t *testing.T) {
	mock := &mockCacheClient{
		deleteTaskFn: func(_ context.Context, _ string) error {
			return nil
		},
		fetchAllFn: func(_ context.Context) (*SyncResult, error) {
			return nil, errors.New("Too Many Requests")
		},
	}
	cache := newTestCache(mock)

	err := cache.DeleteTask(context.Background(), "task-123")
	if err != nil {
		t.Fatalf("got err %v, want nil (FetchAll failure should not propagate)", err)
	}
}

func TestDeleteTask_PropagatesMutationError(t *testing.T) {
	mutationErr := errors.New("not found")
	mock := &mockCacheClient{
		deleteTaskFn: func(_ context.Context, _ string) error {
			return mutationErr
		},
	}
	cache := newTestCache(mock)

	err := cache.DeleteTask(context.Background(), "task-123")
	if err == nil {
		t.Fatal("got nil error, want mutation error to propagate")
	}
}

func TestUpdateTask_SucceedsWhenFetchAllFails(t *testing.T) {
	mock := &mockCacheClient{
		updateTaskFn: func(_ context.Context, _ *synctodoist.TaskUpdateArgs) error {
			return nil
		},
		fetchAllFn: func(_ context.Context) (*SyncResult, error) {
			return nil, errors.New("Too Many Requests")
		},
	}
	cache := newTestCache(mock)

	err := cache.UpdateTask(context.Background(), &synctodoist.TaskUpdateArgs{ID: "task-123"})
	if err != nil {
		t.Fatalf("got err %v, want nil (FetchAll failure should not propagate)", err)
	}
}

func TestScheduleRefresh_Debounces(t *testing.T) {
	var refreshCount atomic.Int32
	mock := &mockCacheClient{
		fetchAllFn: func(_ context.Context) (*SyncResult, error) {
			refreshCount.Add(1)
			return &SyncResult{}, nil
		},
	}
	cache := newTestCache(mock)

	// Schedule 5 rapid refreshes — should coalesce into 1
	for range 5 {
		cache.ScheduleRefresh()
	}

	// Wait for debounce + execution
	time.Sleep(refreshDebounce + 500*time.Millisecond)

	count := refreshCount.Load()
	if count != 1 {
		t.Errorf("got %d refreshes, want 1 (should debounce)", count)
	}
}

func TestScheduleRefresh_MultipleWavesFireSeparately(t *testing.T) {
	var refreshCount atomic.Int32
	mock := &mockCacheClient{
		fetchAllFn: func(_ context.Context) (*SyncResult, error) {
			refreshCount.Add(1)
			return &SyncResult{}, nil
		},
	}
	cache := newTestCache(mock)

	// First wave
	cache.ScheduleRefresh()
	time.Sleep(refreshDebounce + 200*time.Millisecond)

	// Second wave (after first debounce completed)
	cache.ScheduleRefresh()
	time.Sleep(refreshDebounce + 200*time.Millisecond)

	count := refreshCount.Load()
	if count != 2 {
		t.Errorf("got %d refreshes, want 2 (separate waves should fire independently)", count)
	}
}

func TestCompleteTask_EvictsFromCache(t *testing.T) {
	parentID := "parent"
	mock := &mockCacheClient{
		completeTaskFn: func(_ context.Context, _ string) error { return nil },
		fetchAllFn:     func(_ context.Context) (*SyncResult, error) { return &SyncResult{}, nil },
	}
	cache := newTestCache(mock)
	cache.mu.Lock()
	cache.tasks = []*Task{
		{ID: "parent", Content: "Parent"},
		{ID: "child", Content: "Child", ParentID: &parentID},
		{ID: "other", Content: "Other"},
	}
	cache.mu.Unlock()

	var broadcastCount int
	cache.onRefresh = func() { broadcastCount++ }

	err := cache.CompleteTask(context.Background(), "parent")
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}

	tasks := cache.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1 (parent + child evicted)", len(tasks))
	}
	if tasks[0].ID != "other" {
		t.Errorf("remaining task id: got %q, want %q", tasks[0].ID, "other")
	}
	if broadcastCount == 0 {
		t.Error("onRefresh not called after evict")
	}
}

func TestDeleteTask_EvictsFromCache(t *testing.T) {
	mock := &mockCacheClient{
		deleteTaskFn: func(_ context.Context, _ string) error { return nil },
		fetchAllFn:   func(_ context.Context) (*SyncResult, error) { return &SyncResult{}, nil },
	}
	cache := newTestCache(mock)
	cache.mu.Lock()
	cache.tasks = []*Task{
		{ID: "1", Content: "Task 1"},
		{ID: "2", Content: "Task 2"},
	}
	cache.mu.Unlock()

	err := cache.DeleteTask(context.Background(), "1")
	if err != nil {
		t.Fatalf("got err %v, want nil", err)
	}

	tasks := cache.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].ID != "2" {
		t.Errorf("remaining task id: got %q, want %q", tasks[0].ID, "2")
	}
}

func TestFilterEvicted_SuppressesDuringRefresh(t *testing.T) {
	mock := &mockCacheClient{
		completeTaskFn: func(_ context.Context, _ string) error { return nil },
		fetchAllFn: func(_ context.Context) (*SyncResult, error) {
			// Todoist returns the task as still active (eventual consistency lag)
			return &SyncResult{
				Tasks: []*Task{
					{ID: "completed", Content: "Done task"},
					{ID: "active", Content: "Active task"},
				},
			}, nil
		},
	}
	cache := newTestCache(mock)
	cache.mu.Lock()
	cache.tasks = []*Task{
		{ID: "completed", Content: "Done task"},
		{ID: "active", Content: "Active task"},
	}
	cache.mu.Unlock()

	// Complete the task — evicts immediately
	if err := cache.CompleteTask(context.Background(), "completed"); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Now refresh — FetchAll returns the task (stale), but filterEvicted suppresses it
	if err := cache.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	tasks := cache.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks after refresh, want 1 (evicted task suppressed)", len(tasks))
	}
	if tasks[0].ID != "active" {
		t.Errorf("remaining task: got %q, want %q", tasks[0].ID, "active")
	}
}

func TestFilterEvicted_SurvivesFullPollCycle(t *testing.T) {
	mock := &mockCacheClient{
		completeTaskFn: func(_ context.Context, _ string) error { return nil },
	}
	cache := newTestCache(mock)
	cache.mu.Lock()
	cache.tasks = []*Task{
		{ID: "recurring", Content: "Daily standup"},
		{ID: "active", Content: "Active task"},
	}
	cache.mu.Unlock()

	// Complete the task — evicts immediately
	if err := cache.CompleteTask(context.Background(), "recurring"); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Simulate 30s passing (one full default poll cycle).
	// The eviction must still suppress the task at this point.
	cache.evictedMu.Lock()
	for id := range cache.evicted {
		cache.evicted[id] = time.Now().Add(-30 * time.Second)
	}
	cache.evictedMu.Unlock()

	// FetchAll returns the task again (recurring task with next occurrence)
	mock.fetchAllFn = func(_ context.Context) (*SyncResult, error) {
		return &SyncResult{
			Tasks: []*Task{
				{ID: "recurring", Content: "Daily standup"},
				{ID: "active", Content: "Active task"},
			},
		}, nil
	}

	if err := cache.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	tasks := cache.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1 (evicted task should still be suppressed after 30s)", len(tasks))
	}
	if tasks[0].ID != "active" {
		t.Errorf("remaining task: got %q, want %q", tasks[0].ID, "active")
	}
}

func TestFilterEvicted_ExpiresAfterGracePeriod(t *testing.T) {
	mock := &mockCacheClient{
		completeTaskFn: func(_ context.Context, _ string) error { return nil },
	}
	cache := newTestCache(mock)
	cache.mu.Lock()
	cache.tasks = []*Task{
		{ID: "recurring", Content: "Daily standup"},
	}
	cache.mu.Unlock()

	if err := cache.CompleteTask(context.Background(), "recurring"); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Simulate grace period expiring (46s > 45s)
	cache.evictedMu.Lock()
	for id := range cache.evicted {
		cache.evicted[id] = time.Now().Add(-46 * time.Second)
	}
	cache.evictedMu.Unlock()

	// FetchAll returns the task (recurring next occurrence)
	mock.fetchAllFn = func(_ context.Context) (*SyncResult, error) {
		return &SyncResult{
			Tasks: []*Task{
				{ID: "recurring", Content: "Daily standup (next)"},
			},
		}, nil
	}

	if err := cache.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	tasks := cache.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1 (eviction should have expired, task should reappear)", len(tasks))
	}
	if tasks[0].ID != "recurring" {
		t.Errorf("task id: got %q, want %q", tasks[0].ID, "recurring")
	}
}

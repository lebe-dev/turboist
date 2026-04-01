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

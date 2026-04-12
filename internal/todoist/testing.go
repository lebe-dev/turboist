package todoist

import (
	"context"
	"time"
)

// NewTestCache creates a Cache pre-populated with the given data, for use in tests.
// The cache is marked as warmed and has no backing client.
func NewTestCache(tasks []*Task, projects []*Project, sections []*Section, labels []*Label) *Cache {
	return &Cache{
		tasks:        tasks,
		projects:     projects,
		sections:     sections,
		labels:       labels,
		lastSyncedAt: time.Now(),
		warmed:       true,
	}
}

// NewTestCacheWithMock creates a Cache pre-populated with data and a no-op stub client,
// allowing mutation methods (UpdateTask, etc.) to succeed in tests.
func NewTestCacheWithMock(tasks []*Task, projects []*Project, sections []*Section, labels []*Label) *Cache {
	return &Cache{
		client:       &noopCacheClient{},
		tasks:        tasks,
		projects:     projects,
		sections:     sections,
		labels:       labels,
		lastSyncedAt: time.Now(),
		warmed:       true,
	}
}

// noopCacheClient is a no-op implementation of cacheClient for handler tests.
type noopCacheClient struct{}

func (m *noopCacheClient) FetchAll(context.Context) (*SyncResult, error)          { return nil, nil }
func (m *noopCacheClient) FetchIncremental(context.Context) (*DeltaResult, error) { return nil, nil }
func (m *noopCacheClient) AddTask(context.Context, *TaskAddArgs) (string, error) {
	return "test-id", nil
}
func (m *noopCacheClient) UpdateTask(context.Context, *TaskUpdateArgs) error       { return nil }
func (m *noopCacheClient) MoveTask(context.Context, string, string) error          { return nil }
func (m *noopCacheClient) MoveTaskToProject(context.Context, string, string) error { return nil }
func (m *noopCacheClient) CompleteTask(context.Context, string) error              { return nil }
func (m *noopCacheClient) DeleteTask(context.Context, string) error                { return nil }
func (m *noopCacheClient) DecomposeTask(context.Context, *Task, []string, DecomposeOpts) error {
	return nil
}
func (m *noopCacheClient) BatchMoveTasksToProject(context.Context, map[string]string) error {
	return nil
}
func (m *noopCacheClient) BatchMoveTasks(context.Context, map[string]MoveTarget) error { return nil }
func (m *noopCacheClient) AddSection(context.Context, string, string) (string, error) {
	return "test-section-id", nil
}

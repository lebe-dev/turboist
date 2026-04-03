package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

type mockDeleter struct {
	tasks   []*todoist.Task
	deleted []string
	err     error
}

func (m *mockDeleter) Tasks() []*todoist.Task { return m.tasks }

func (m *mockDeleter) DeleteTask(_ context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	m.deleted = append(m.deleted, id)
	return nil
}

type mockAutoRemoveStore struct {
	firstSeen map[string]time.Time
	upserted  [][3]string // taskID, label, time
	deletedFS [][2]string // taskID, label
	cleaned   []map[string]bool
}

func newMockStore() *mockAutoRemoveStore {
	return &mockAutoRemoveStore{firstSeen: make(map[string]time.Time)}
}

func (m *mockAutoRemoveStore) GetAutoRemoveFirstSeen() (map[string]time.Time, error) {
	result := make(map[string]time.Time, len(m.firstSeen))
	for k, v := range m.firstSeen {
		result[k] = v
	}
	return result, nil
}

func (m *mockAutoRemoveStore) UpsertAutoRemoveFirstSeen(taskID, label string, firstSeen time.Time) error {
	key := taskID + ":" + label
	if _, exists := m.firstSeen[key]; !exists {
		m.firstSeen[key] = firstSeen
	}
	m.upserted = append(m.upserted, [3]string{taskID, label, firstSeen.Format(time.RFC3339)})
	return nil
}

func (m *mockAutoRemoveStore) DeleteAutoRemoveFirstSeen(taskID, label string) error {
	delete(m.firstSeen, taskID+":"+label)
	m.deletedFS = append(m.deletedFS, [2]string{taskID, label})
	return nil
}

func (m *mockAutoRemoveStore) CleanupAutoRemoveFirstSeen(activeKeys map[string]bool) error {
	m.cleaned = append(m.cleaned, activeKeys)
	for key := range m.firstSeen {
		if !activeKeys[key] {
			delete(m.firstSeen, key)
		}
	}
	return nil
}

func defaultCfg(rules ...config.AutoRemoveRuleConfig) config.AutoRemoveConfig {
	return config.AutoRemoveConfig{
		MinTTL:     time.Hour,
		MaxPerTick: 10,
		MaxPercent: 50,
		Rules:      rules,
	}
}

func rule(label string, ttl time.Duration) config.AutoRemoveRuleConfig {
	return config.AutoRemoveRuleConfig{Label: label, TTL: ttl}
}

func TestAutoRemove_DeletesTask(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("42", "urgent")}}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, store)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background())

	// Not yet expired
	if len(d.deleted) != 0 {
		t.Fatal("task should not be deleted before TTL")
	}

	// Advance past TTL
	ar.now = func() time.Time { return t0.Add(time.Hour + time.Second) }
	ar.Job(context.Background())

	if len(d.deleted) != 1 || d.deleted[0] != "42" {
		t.Fatalf("expected task 42 deleted, got %v", d.deleted)
	}
}

func TestAutoRemove_PersistsFirstSeen(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("1", "urgent")}}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, store)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background())

	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(store.upserted))
	}
	if store.upserted[0][0] != "1" || store.upserted[0][1] != "urgent" {
		t.Errorf("upserted: got %v", store.upserted[0])
	}
}

func TestAutoRemove_LoadsFromDB(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("99", "urgent")}}
	store := newMockStore()
	// Pre-populate: task was first seen 2 hours ago
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	store.firstSeen["99:urgent"] = twoHoursAgo

	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, store)

	ar.now = func() time.Time { return time.Now() }
	ar.Job(context.Background())

	// Should be deleted immediately since TTL already expired in DB
	if len(d.deleted) != 1 || d.deleted[0] != "99" {
		t.Fatalf("expected task 99 deleted from DB timestamp, got %v", d.deleted)
	}
}

func TestAutoRemove_SurvivesRestart(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("99", "urgent")}}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))

	// First instance records first_seen
	t0 := time.Now()
	ar1 := NewAutoRemove(d, cfg, store)
	ar1.now = func() time.Time { return t0 }
	ar1.Job(context.Background())

	// Simulate restart: create new instance (loads from store)
	ar2 := NewAutoRemove(d, cfg, store)
	ar2.now = func() time.Time { return t0.Add(time.Hour + time.Second) }
	ar2.Job(context.Background())

	// Should delete using the original first_seen from DB
	if len(d.deleted) != 1 || d.deleted[0] != "99" {
		t.Fatalf("expected task 99 deleted after restart, got %v", d.deleted)
	}
}

func TestAutoRemove_PerTickLimit(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{
		task("1", "urgent"),
		task("2", "urgent"),
		task("3", "urgent"),
		task("4", "urgent"),
		task("5", "urgent"),
	}}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	cfg.MaxPerTick = 2
	cfg.MaxPercent = 100 // disable circuit breaker for this test

	ar := NewAutoRemove(d, cfg, store)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background()) // record first_seen

	ar.now = func() time.Time { return t0.Add(2 * time.Hour) }
	ar.Job(context.Background()) // should delete only 2

	if len(d.deleted) != 2 {
		t.Fatalf("expected 2 deletions (per-tick limit), got %d", len(d.deleted))
	}
}

func TestAutoRemove_CircuitBreaker(t *testing.T) {
	// 10 tasks total, 2 match for deletion -> 20%, threshold is 10%
	tasks := make([]*todoist.Task, 10)
	for i := range tasks {
		if i < 2 {
			tasks[i] = task(string(rune('a'+i)), "urgent")
		} else {
			tasks[i] = task(string(rune('a'+i)), "other")
		}
	}
	d := &mockDeleter{tasks: tasks}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	cfg.MaxPercent = 10

	ar := NewAutoRemove(d, cfg, store)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background()) // record first_seen

	ar.now = func() time.Time { return t0.Add(2 * time.Hour) }
	ar.Job(context.Background()) // circuit breaker should trip

	if len(d.deleted) != 0 {
		t.Fatalf("expected 0 deletions when circuit breaker trips, got %d", len(d.deleted))
	}
	if !ar.Paused() {
		t.Fatal("expected paused=true after circuit breaker trips")
	}
}

func TestAutoRemove_CircuitBreakerResets(t *testing.T) {
	// Start with circuit breaker tripped
	tasks := make([]*todoist.Task, 10)
	for i := range tasks {
		if i < 2 {
			tasks[i] = task(string(rune('a'+i)), "urgent")
		} else {
			tasks[i] = task(string(rune('a'+i)), "other")
		}
	}
	d := &mockDeleter{tasks: tasks}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	cfg.MaxPercent = 10

	ar := NewAutoRemove(d, cfg, store)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background())
	ar.now = func() time.Time { return t0.Add(2 * time.Hour) }
	ar.Job(context.Background())

	if !ar.Paused() {
		t.Fatal("expected paused=true")
	}

	// Remove excess tasks so only 1 matches (10%)
	d.tasks = []*todoist.Task{
		task("a", "urgent"),
		task("c", "other"),
		task("d", "other"),
		task("e", "other"),
		task("f", "other"),
		task("g", "other"),
		task("h", "other"),
		task("i", "other"),
		task("j", "other"),
		task("k", "other"),
	}
	ar.Job(context.Background())

	if ar.Paused() {
		t.Fatal("expected paused=false after reducing matching tasks")
	}
}

func TestAutoRemove_CleansUpStaleEntries(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("1", "urgent")}}
	store := newMockStore()
	// Pre-populate stale entry for a task that no longer exists
	store.firstSeen["old-task:urgent"] = time.Now()

	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, store)

	ar.now = func() time.Time { return time.Now() }
	ar.Job(context.Background())

	// "old-task:urgent" should be cleaned up
	if _, exists := store.firstSeen["old-task:urgent"]; exists {
		t.Fatal("stale entry should be cleaned up")
	}
	// "1:urgent" should still exist
	if _, exists := store.firstSeen["1:urgent"]; !exists {
		t.Fatal("active entry should be preserved")
	}
}

func TestAutoRemove_DeleteError_KeepsFirstSeen(t *testing.T) {
	d := &mockDeleter{
		tasks: []*todoist.Task{task("5", "urgent")},
		err:   errors.New("api error"),
	}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, store)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background())

	ar.now = func() time.Time { return t0.Add(2 * time.Hour) }
	ar.Job(context.Background())

	// first_seen should be kept when DeleteTask fails
	ar.mu.Lock()
	_, stillTracked := ar.firstSeen["5:urgent"]
	ar.mu.Unlock()
	if !stillTracked {
		t.Fatal("first_seen should be kept when DeleteTask fails")
	}
}

func TestAutoRemove_IgnoresTasksWithoutLabel(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("3")}}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, store)

	ar.now = func() time.Time { return time.Now() }
	ar.Job(context.Background())

	ar.mu.Lock()
	n := len(ar.firstSeen)
	ar.mu.Unlock()

	if n != 0 {
		t.Fatalf("expected empty firstSeen, got %d entries", n)
	}
}

func TestAutoRemove_FirstSeenNotOverwritten(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("1", "urgent")}}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, store)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background())

	t1 := t0.Add(10 * time.Minute)
	ar.now = func() time.Time { return t1 }
	ar.Job(context.Background())

	ar.mu.Lock()
	seenAt := ar.firstSeen["1:urgent"]
	ar.mu.Unlock()

	if !seenAt.Equal(t0) {
		t.Fatalf("first_seen should not be overwritten: got %v, want %v", seenAt, t0)
	}
}

func TestAutoRemove_TTLNotExpiredYet(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("7", "urgent")}}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, store)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background())

	ar.now = func() time.Time { return t0.Add(59 * time.Minute) }
	ar.Job(context.Background())

	if len(d.deleted) != 0 {
		t.Fatal("task should not be deleted before TTL expires")
	}
}

func TestAutoRemove_NilStore(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("1", "urgent")}}
	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, nil)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background())

	ar.now = func() time.Time { return t0.Add(2 * time.Hour) }
	ar.Job(context.Background())

	if len(d.deleted) != 1 || d.deleted[0] != "1" {
		t.Fatalf("should work without store, got %v", d.deleted)
	}
}

func TestAutoRemove_FirstSeenRemovedAfterDelete(t *testing.T) {
	d := &mockDeleter{tasks: []*todoist.Task{task("42", "urgent")}}
	store := newMockStore()
	cfg := defaultCfg(rule("urgent", time.Hour))
	ar := NewAutoRemove(d, cfg, store)

	t0 := time.Now()
	ar.now = func() time.Time { return t0 }
	ar.Job(context.Background())

	ar.now = func() time.Time { return t0.Add(2 * time.Hour) }
	ar.Job(context.Background())

	ar.mu.Lock()
	_, stillTracked := ar.firstSeen["42:urgent"]
	ar.mu.Unlock()
	if stillTracked {
		t.Fatal("first_seen should be deleted after successful deletion")
	}

	if len(store.deletedFS) < 1 {
		t.Fatal("store.DeleteAutoRemoveFirstSeen should be called")
	}
}

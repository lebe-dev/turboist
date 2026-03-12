package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

type mockCompleter struct {
	tasks     []*todoist.Task
	completed []string
	err       error
}

func (m *mockCompleter) Tasks() []*todoist.Task { return m.tasks }

func (m *mockCompleter) CompleteTask(_ context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	m.completed = append(m.completed, id)
	return nil
}

func task(id string, labels ...string) *todoist.Task {
	return &todoist.Task{ID: id, Content: "task " + id, Labels: labels}
}

func rules(label string, ttl time.Duration) []config.AutoExpireConfig {
	return []config.AutoExpireConfig{{Label: label, TTL: ttl}}
}

func TestAutoExpire_FirstSeenTracking(t *testing.T) {
	c := &mockCompleter{tasks: []*todoist.Task{task("1", "urgent")}}
	ae := NewAutoExpire(c, rules("urgent", time.Hour))

	t0 := time.Now()
	ae.now = func() time.Time { return t0 }

	ae.Job(context.Background())

	ae.mu.Lock()
	seenAt, ok := ae.firstSeen["1:urgent"]
	ae.mu.Unlock()

	if !ok {
		t.Fatal("expected first_seen to be recorded")
	}
	if !seenAt.Equal(t0) {
		t.Fatalf("expected seenAt=%v, got %v", t0, seenAt)
	}
	if len(c.completed) != 0 {
		t.Fatal("task should not be completed before TTL")
	}
}

func TestAutoExpire_FirstSeenNotOverwritten(t *testing.T) {
	c := &mockCompleter{tasks: []*todoist.Task{task("1", "urgent")}}
	ae := NewAutoExpire(c, rules("urgent", time.Hour))

	t0 := time.Now()
	ae.now = func() time.Time { return t0 }
	ae.Job(context.Background())

	t1 := t0.Add(10 * time.Minute)
	ae.now = func() time.Time { return t1 }
	ae.Job(context.Background())

	ae.mu.Lock()
	seenAt := ae.firstSeen["1:urgent"]
	ae.mu.Unlock()

	if !seenAt.Equal(t0) {
		t.Fatalf("first_seen should not be overwritten: got %v, want %v", seenAt, t0)
	}
}

func TestAutoExpire_TTLExpiry(t *testing.T) {
	c := &mockCompleter{tasks: []*todoist.Task{task("42", "urgent")}}
	ae := NewAutoExpire(c, rules("urgent", time.Hour))

	t0 := time.Now()
	ae.now = func() time.Time { return t0 }
	ae.Job(context.Background()) // records first_seen

	// advance past TTL
	ae.now = func() time.Time { return t0.Add(time.Hour + time.Second) }
	ae.Job(context.Background()) // should complete

	if len(c.completed) != 1 || c.completed[0] != "42" {
		t.Fatalf("expected task 42 to be completed, got %v", c.completed)
	}

	// first_seen should be removed after completion
	ae.mu.Lock()
	_, stillTracked := ae.firstSeen["42:urgent"]
	ae.mu.Unlock()
	if stillTracked {
		t.Fatal("first_seen should be deleted after completion")
	}
}

func TestAutoExpire_TTLNotExpiredYet(t *testing.T) {
	c := &mockCompleter{tasks: []*todoist.Task{task("7", "urgent")}}
	ae := NewAutoExpire(c, rules("urgent", time.Hour))

	t0 := time.Now()
	ae.now = func() time.Time { return t0 }
	ae.Job(context.Background())

	ae.now = func() time.Time { return t0.Add(59 * time.Minute) }
	ae.Job(context.Background())

	if len(c.completed) != 0 {
		t.Fatal("task should not be completed before TTL")
	}
}

func TestAutoExpire_ResetAfterRestart(t *testing.T) {
	task1 := task("99", "urgent")
	c := &mockCompleter{tasks: []*todoist.Task{task1}}
	ttl := time.Hour

	// First "instance" (before restart): records first_seen at t0
	ae1 := NewAutoExpire(c, rules("urgent", ttl))
	t0 := time.Now()
	ae1.now = func() time.Time { return t0 }
	ae1.Job(context.Background())

	// Simulate restart: new instance, time is now past TTL of the old first_seen
	ae2 := NewAutoExpire(c, rules("urgent", ttl))
	ae2.now = func() time.Time { return t0.Add(2 * time.Hour) }

	// First run of new instance: records fresh first_seen, no completion yet
	ae2.Job(context.Background())

	if len(c.completed) != 0 {
		t.Fatal("after restart, task should get fresh TTL and not be completed immediately")
	}

	ae2.mu.Lock()
	seenAt, ok := ae2.firstSeen["99:urgent"]
	ae2.mu.Unlock()
	if !ok {
		t.Fatal("new instance should record first_seen")
	}
	// seenAt should be the new instance's first run time, not t0
	if seenAt.Equal(t0) {
		t.Fatal("new instance should record new first_seen, not the old one")
	}
}

func TestAutoExpire_CompleteError(t *testing.T) {
	c := &mockCompleter{
		tasks: []*todoist.Task{task("5", "urgent")},
		err:   errors.New("api error"),
	}
	ae := NewAutoExpire(c, rules("urgent", time.Hour))

	t0 := time.Now()
	ae.now = func() time.Time { return t0 }
	ae.Job(context.Background())

	// advance past TTL, error on complete — first_seen should NOT be deleted
	ae.now = func() time.Time { return t0.Add(time.Hour + time.Second) }
	ae.Job(context.Background())

	ae.mu.Lock()
	_, stillTracked := ae.firstSeen["5:urgent"]
	ae.mu.Unlock()
	if !stillTracked {
		t.Fatal("first_seen should be kept when CompleteTask fails")
	}
}

func TestAutoExpire_NoLabelNoTracking(t *testing.T) {
	c := &mockCompleter{tasks: []*todoist.Task{task("3")}} // no labels
	ae := NewAutoExpire(c, rules("urgent", time.Hour))

	ae.now = func() time.Time { return time.Now() }
	ae.Job(context.Background())

	ae.mu.Lock()
	n := len(ae.firstSeen)
	ae.mu.Unlock()

	if n != 0 {
		t.Fatalf("expected empty firstSeen, got %d entries", n)
	}
}

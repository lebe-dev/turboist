package scheduler

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

type mockTaskReader struct {
	tasks []*todoist.Task
}

func (m *mockTaskReader) Tasks() []*todoist.Task { return m.tasks }

func TestWeeklyLimit_UnderLimit(t *testing.T) {
	reader := &mockTaskReader{
		tasks: []*todoist.Task{
			task("1", "weekly"),
			task("2", "weekly"),
			task("3"),
		},
	}
	wl := NewWeeklyLimit(reader, config.WeeklyConfig{Label: "weekly", MaxTasks: 5})
	// Should not panic or error
	wl.Job(context.Background())
}

func TestWeeklyLimit_ExceedsLimit(t *testing.T) {
	reader := &mockTaskReader{
		tasks: []*todoist.Task{
			task("1", "weekly"),
			task("2", "weekly"),
			task("3", "weekly"),
		},
	}
	wl := NewWeeklyLimit(reader, config.WeeklyConfig{Label: "weekly", MaxTasks: 2})
	// Should log warning but not panic
	wl.Job(context.Background())
}

func TestWeeklyLimit_Disabled_ZeroMax(t *testing.T) {
	reader := &mockTaskReader{
		tasks: []*todoist.Task{task("1", "weekly")},
	}
	wl := NewWeeklyLimit(reader, config.WeeklyConfig{Label: "weekly", MaxTasks: 0})
	// Should early-return without reading tasks
	wl.Job(context.Background())
}

func TestWeeklyLimit_Disabled_EmptyLabel(t *testing.T) {
	reader := &mockTaskReader{
		tasks: []*todoist.Task{task("1", "weekly")},
	}
	wl := NewWeeklyLimit(reader, config.WeeklyConfig{Label: "", MaxTasks: 5})
	// Should early-return without reading tasks
	wl.Job(context.Background())
}

func TestWeeklyLimit_ExactlyAtLimit(t *testing.T) {
	reader := &mockTaskReader{
		tasks: []*todoist.Task{
			task("1", "weekly"),
			task("2", "weekly"),
		},
	}
	wl := NewWeeklyLimit(reader, config.WeeklyConfig{Label: "weekly", MaxTasks: 2})
	// At limit (not over), should not warn
	wl.Job(context.Background())
}

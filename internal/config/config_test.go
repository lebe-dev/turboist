package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const archMdExample = `
poll_interval: "30s"

contexts:
  - id: dev
    display_name: "Разработка"
    filters:
      projects: ["Проекты"]
      sections: ["Категория - Разработка"]
  - id: personal
    display_name: "Личное"
    filters:
      projects: ["Личное"]

weekly:
  label: "на неделе"
  max_tasks: 15

next_week:
  label: "на след неделе"

auto_expire:
  - label: "срочное"
    ttl: "24h"
  - label: "горит"
    ttl: "4h"
`

func TestParseAppConfig(t *testing.T) {
	app, err := ParseAppConfig([]byte(archMdExample))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if app.PollInterval != 30*time.Second {
		t.Errorf("poll_interval: got %v, want 30s", app.PollInterval)
	}

	if len(app.Contexts) != 2 {
		t.Errorf("contexts: got %d, want 2", len(app.Contexts))
	}

	dev := app.FindContext("dev")
	if dev == nil {
		t.Fatal("context 'dev' not found")
	}
	if dev.DisplayName != "Разработка" {
		t.Errorf("dev.display_name: got %q, want 'Разработка'", dev.DisplayName)
	}
	if len(dev.Filters.Projects) != 1 || dev.Filters.Projects[0] != "Проекты" {
		t.Errorf("dev.filters.projects: got %v", dev.Filters.Projects)
	}
	if len(dev.Filters.Sections) != 1 || dev.Filters.Sections[0] != "Категория - Разработка" {
		t.Errorf("dev.filters.sections: got %v", dev.Filters.Sections)
	}

	if app.Weekly.Label != "на неделе" {
		t.Errorf("weekly.label: got %q", app.Weekly.Label)
	}
	if app.Weekly.MaxTasks != 15 {
		t.Errorf("weekly.max_tasks: got %d, want 15", app.Weekly.MaxTasks)
	}

	if app.NextWeek.Label != "на след неделе" {
		t.Errorf("next_week.label: got %q", app.NextWeek.Label)
	}

	if len(app.AutoExpire) != 2 {
		t.Fatalf("auto_expire: got %d, want 2", len(app.AutoExpire))
	}
	if app.AutoExpire[0].Label != "срочное" || app.AutoExpire[0].TTL != 24*time.Hour {
		t.Errorf("auto_expire[0]: got %+v", app.AutoExpire[0])
	}
	if app.AutoExpire[1].Label != "горит" || app.AutoExpire[1].TTL != 4*time.Hour {
		t.Errorf("auto_expire[1]: got %+v", app.AutoExpire[1])
	}
}

func TestParseAppConfig_TaskSort(t *testing.T) {
	app, err := ParseAppConfig([]byte(`task_sort: "due_date"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.TaskSort != TaskSortDueDate {
		t.Errorf("task_sort: got %q, want %q", app.TaskSort, TaskSortDueDate)
	}
}

func TestParseAppConfig_TaskSortDefault(t *testing.T) {
	app, err := ParseAppConfig([]byte(`weekly: {label: "x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.TaskSort != TaskSortPriority {
		t.Errorf("task_sort default: got %q, want %q", app.TaskSort, TaskSortPriority)
	}
}

func TestParseAppConfig_TaskSortInvalid(t *testing.T) {
	_, err := ParseAppConfig([]byte(`task_sort: "unknown"`))
	if err == nil {
		t.Fatal("expected error for invalid task_sort")
	}
}

func TestParseAppConfig_DefaultPollInterval(t *testing.T) {
	app, err := ParseAppConfig([]byte(`weekly: {label: "x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.PollInterval != 30*time.Second {
		t.Errorf("default poll_interval: got %v, want 30s", app.PollInterval)
	}
}

func TestParseAppConfig_DayParts(t *testing.T) {
	yaml := `
today:
  include_overdue: true
  day_parts:
    - label: "morning"
      start: 6
      end: 12
    - label: "afternoon"
      start: 12
      end: 18
    - label: "evening"
      start: 18
      end: 23
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.Today.DayParts) != 3 {
		t.Fatalf("expected 3 day parts, got %d", len(app.Today.DayParts))
	}
	if app.Today.DayParts[0].Label != "morning" {
		t.Errorf("day_parts[0].label: got %q", app.Today.DayParts[0].Label)
	}
	if app.Today.DayParts[1].Start != 12 || app.Today.DayParts[1].End != 18 {
		t.Errorf("day_parts[1]: got start=%d end=%d", app.Today.DayParts[1].Start, app.Today.DayParts[1].End)
	}
}

func TestParseAppConfig_DayPartsEmpty(t *testing.T) {
	yaml := `today: {include_overdue: true}`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.Today.DayParts) != 0 {
		t.Errorf("expected 0 day parts, got %d", len(app.Today.DayParts))
	}
}

func TestParseAppConfig_DayPartsInvalidRange(t *testing.T) {
	yaml := `
today:
  day_parts:
    - label: "bad"
      start: 18
      end: 6
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid range")
	}
}

func TestParseAppConfig_DayPartsOverlapping(t *testing.T) {
	yaml := `
today:
  day_parts:
    - label: "a"
      start: 6
      end: 14
    - label: "b"
      start: 12
      end: 18
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for overlapping ranges")
	}
}

func TestParseAppConfig_DayPartsEmptyLabel(t *testing.T) {
	yaml := `
today:
  day_parts:
    - label: ""
      start: 6
      end: 12
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty label")
	}
}

func TestParseAppConfig_CompletedDefault(t *testing.T) {
	app, err := ParseAppConfig([]byte(`weekly: {label: "x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Completed.Days != 3 {
		t.Errorf("completed.days default: got %d, want 3", app.Completed.Days)
	}
}

func TestParseAppConfig_ContextColor(t *testing.T) {
	yaml := `
contexts:
  - id: work
    display_name: "Work"
    color: "#FF5733"
    filters:
      labels: ["work"]
  - id: personal
    display_name: "Personal"
    color: green
    filters:
      labels: ["personal"]
  - id: misc
    display_name: "Misc"
    filters:
      labels: ["misc"]
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.Contexts) != 3 {
		t.Fatalf("contexts: got %d, want 3", len(app.Contexts))
	}
	if app.Contexts[0].Color != "#FF5733" {
		t.Errorf("contexts[0].color: got %q, want %q", app.Contexts[0].Color, "#FF5733")
	}
	if app.Contexts[1].Color != "green" {
		t.Errorf("contexts[1].color: got %q, want %q", app.Contexts[1].Color, "green")
	}
	if app.Contexts[2].Color != "" {
		t.Errorf("contexts[2].color: got %q, want empty", app.Contexts[2].Color)
	}
}

func TestParseAppConfig_CompletedCustom(t *testing.T) {
	app, err := ParseAppConfig([]byte(`completed: {days: 7}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Completed.Days != 7 {
		t.Errorf("completed.days: got %d, want 7", app.Completed.Days)
	}
}

func TestLoadDotEnv_SetsVars(t *testing.T) {
	f := filepath.Join(t.TempDir(), ".env")
	content := `
# comment
TURBOIST_TEST_KEY=hello
TURBOIST_TEST_QUOTED="world"
`
	if err := os.WriteFile(f, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TURBOIST_TEST_KEY", "")
	t.Setenv("TURBOIST_TEST_QUOTED", "")

	if err := loadDotEnv(f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := os.Getenv("TURBOIST_TEST_KEY"); got != "hello" {
		t.Errorf("TURBOIST_TEST_KEY: got %q, want %q", got, "hello")
	}
	if got := os.Getenv("TURBOIST_TEST_QUOTED"); got != "world" {
		t.Errorf("TURBOIST_TEST_QUOTED: got %q, want %q", got, "world")
	}
}

func TestLoadDotEnv_NoOverride(t *testing.T) {
	t.Setenv("TURBOIST_TEST_NOOVERRIDE", "original")

	f := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(f, []byte("TURBOIST_TEST_NOOVERRIDE=replaced\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := loadDotEnv(f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := os.Getenv("TURBOIST_TEST_NOOVERRIDE"); got != "original" {
		t.Errorf("TURBOIST_TEST_NOOVERRIDE: got %q, want %q", got, "original")
	}
}

func TestLoadDotEnv_MissingFile(t *testing.T) {
	err := loadDotEnv("/nonexistent/.env")
	if err != nil {
		t.Errorf("expected no error for missing file, got %v", err)
	}
}

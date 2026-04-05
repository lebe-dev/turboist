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

backlog:
  label: "на след неделе"

auto_remove:
  min_ttl: "1h"
  max_per_tick: 1
  max_percent: 10
  rules:
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

	if app.Backlog.Label != "на след неделе" {
		t.Errorf("backlog.label: got %q", app.Backlog.Label)
	}

	if len(app.AutoRemove.Rules) != 2 {
		t.Fatalf("auto_remove.rules: got %d, want 2", len(app.AutoRemove.Rules))
	}
	if app.AutoRemove.Rules[0].Label != "срочное" || app.AutoRemove.Rules[0].TTL != 24*time.Hour {
		t.Errorf("auto_remove.rules[0]: got %+v", app.AutoRemove.Rules[0])
	}
	if app.AutoRemove.Rules[1].Label != "горит" || app.AutoRemove.Rules[1].TTL != 4*time.Hour {
		t.Errorf("auto_remove.rules[1]: got %+v", app.AutoRemove.Rules[1])
	}
	if app.AutoRemove.MinTTL != time.Hour {
		t.Errorf("auto_remove.min_ttl: got %v, want 1h", app.AutoRemove.MinTTL)
	}
	if app.AutoRemove.MaxPerTick != 1 {
		t.Errorf("auto_remove.max_per_tick: got %d, want 1", app.AutoRemove.MaxPerTick)
	}
	if app.AutoRemove.MaxPercent != 10 {
		t.Errorf("auto_remove.max_percent: got %d, want 10", app.AutoRemove.MaxPercent)
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

func TestContextConfig_ShouldInheritLabels(t *testing.T) {
	t.Run("default (nil) returns true", func(t *testing.T) {
		ctx := ContextConfig{ID: "test"}
		if !ctx.ShouldInheritLabels() {
			t.Error("expected true when InheritLabels is nil")
		}
	})

	t.Run("explicit true", func(t *testing.T) {
		v := true
		ctx := ContextConfig{ID: "test", InheritLabels: &v}
		if !ctx.ShouldInheritLabels() {
			t.Error("expected true")
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		v := false
		ctx := ContextConfig{ID: "test", InheritLabels: &v}
		if ctx.ShouldInheritLabels() {
			t.Error("expected false")
		}
	})
}

func TestParseAppConfig_InheritLabels(t *testing.T) {
	yaml := `
contexts:
  - id: with_inherit
    display_name: "With"
    filters:
      labels: ["a"]
  - id: no_inherit
    display_name: "Without"
    inherit_labels: false
    filters:
      labels: ["b"]
  - id: explicit_inherit
    display_name: "Explicit"
    inherit_labels: true
    filters:
      labels: ["c"]
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	with := app.FindContext("with_inherit")
	if with == nil || !with.ShouldInheritLabels() {
		t.Error("with_inherit: expected inherit_labels=true by default")
	}

	no := app.FindContext("no_inherit")
	if no == nil || no.ShouldInheritLabels() {
		t.Error("no_inherit: expected inherit_labels=false")
	}

	explicit := app.FindContext("explicit_inherit")
	if explicit == nil || !explicit.ShouldInheritLabels() {
		t.Error("explicit_inherit: expected inherit_labels=true")
	}
}

func TestLabelConfig_ShouldInheritToSubtasks(t *testing.T) {
	t.Run("default (nil) returns true", func(t *testing.T) {
		lc := LabelConfig{Name: "test"}
		if !lc.ShouldInheritToSubtasks() {
			t.Error("expected true when InheritToSubtasks is nil")
		}
	})

	t.Run("explicit true", func(t *testing.T) {
		v := true
		lc := LabelConfig{Name: "test", InheritToSubtasks: &v}
		if !lc.ShouldInheritToSubtasks() {
			t.Error("expected true")
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		v := false
		lc := LabelConfig{Name: "test", InheritToSubtasks: &v}
		if lc.ShouldInheritToSubtasks() {
			t.Error("expected false")
		}
	})
}

func TestParseAppConfig_Labels(t *testing.T) {
	yaml := `
labels:
  - name: "weekly"
    inherit_to_subtasks: false
  - name: "backlog"
    inherit_to_subtasks: false
  - name: "important"
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.Labels) != 3 {
		t.Fatalf("labels: got %d, want 3", len(app.Labels))
	}

	weekly := app.FindLabel("weekly")
	if weekly == nil {
		t.Fatal("label 'weekly' not found")
	}
	if weekly.ShouldInheritToSubtasks() {
		t.Error("weekly: expected inherit_to_subtasks=false")
	}

	important := app.FindLabel("important")
	if important == nil {
		t.Fatal("label 'important' not found")
	}
	if !important.ShouldInheritToSubtasks() {
		t.Error("important: expected inherit_to_subtasks=true by default")
	}

	if app.FindLabel("nonexistent") != nil {
		t.Error("expected nil for nonexistent label")
	}
}

func TestParseAppConfig_LabelsEmpty(t *testing.T) {
	app, err := ParseAppConfig([]byte(`weekly: {label: "x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.Labels) != 0 {
		t.Errorf("labels: got %d, want 0", len(app.Labels))
	}
}

func TestParseAppConfig_LabelsDuplicateName(t *testing.T) {
	yaml := `
labels:
  - name: "weekly"
  - name: "weekly"
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate label name")
	}
}

func TestParseAppConfig_LabelsEmptyName(t *testing.T) {
	yaml := `
labels:
  - name: ""
    inherit_to_subtasks: false
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty label name")
	}
}

func TestParseAppConfig_AutoLabels(t *testing.T) {
	yaml := `
auto_labels:
  - mask: "купить"
    label: "покупки"
  - mask: "встреча|созвон"
    label: "работа"
    ignore_case: false
  - mask: "^Проект -"
    label: "проект"
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.AutoLabels) != 3 {
		t.Fatalf("auto_labels: got %d, want 3", len(app.AutoLabels))
	}
	if app.AutoLabels[0].Mask != "купить" || app.AutoLabels[0].Label != "покупки" {
		t.Errorf("auto_labels[0]: got %+v", app.AutoLabels[0])
	}
	if len(app.CompiledAutoLabels) != 3 {
		t.Fatalf("compiled auto_labels: got %d, want 3", len(app.CompiledAutoLabels))
	}
}

func TestParseAppConfig_AutoLabelsDefaultIgnoreCase(t *testing.T) {
	yaml := `
auto_labels:
  - mask: "купить"
    label: "покупки"
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	at := app.AutoLabels[0]
	if !at.ShouldIgnoreCase() {
		t.Error("expected ignore_case=true by default")
	}
	// Compiled mask should be lowercased for case-insensitive matching
	compiled := app.CompiledAutoLabels[0]
	if !compiled.IgnoreCase {
		t.Error("expected IgnoreCase=true")
	}
	if compiled.Mask != "купить" {
		t.Errorf("mask: got %q, want %q", compiled.Mask, "купить")
	}
}

func TestParseAppConfig_AutoLabelsIgnoreCaseFalse(t *testing.T) {
	yaml := `
auto_labels:
  - mask: "Купить"
    label: "покупки"
    ignore_case: false
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	compiled := app.CompiledAutoLabels[0]
	if compiled.IgnoreCase {
		t.Error("expected IgnoreCase=false")
	}
	// Mask should be preserved as-is when ignore_case=false
	if compiled.Mask != "Купить" {
		t.Errorf("mask: got %q, want %q", compiled.Mask, "Купить")
	}
}

func TestParseAppConfig_AutoLabelsEmptyMask(t *testing.T) {
	yaml := `
auto_labels:
  - mask: ""
    label: "test"
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty mask")
	}
}

func TestParseAppConfig_AutoLabelsEmptyLabel(t *testing.T) {
	yaml := `
auto_labels:
  - mask: "купить"
    label: ""
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty label")
	}
}

func TestParseAppConfig_AutoLabelsEmpty(t *testing.T) {
	app, err := ParseAppConfig([]byte(`weekly: {label: "x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.AutoLabels) != 0 {
		t.Errorf("expected 0 auto_labels, got %d", len(app.AutoLabels))
	}
	if len(app.CompiledAutoLabels) != 0 {
		t.Errorf("expected 0 compiled auto_labels, got %d", len(app.CompiledAutoLabels))
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

// setupLoadTest creates a temporary directory with a minimal config.yml and .env,
// sets required env vars, and chdir's into it. Cleanup restores the original CWD.
func setupLoadTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(`weekly: {label: "x"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TODOIST_API_KEY", "test-key")
	t.Setenv("TURBOIST_ADMIN_PASSWORD", "test-pass")
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("failed to restore CWD: %v", err)
		}
	})
}

func TestParseAppConfig_MaxDayPartNoteLengthDefault(t *testing.T) {
	app, err := ParseAppConfig([]byte(`weekly: {label: "x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Today.MaxDayPartNoteLength != 200 {
		t.Errorf("max_day_part_note_length default: got %d, want 200", app.Today.MaxDayPartNoteLength)
	}
}

func TestParseAppConfig_MaxDayPartNoteLengthCustom(t *testing.T) {
	yaml := `
today:
  max_day_part_note_length: 100
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Today.MaxDayPartNoteLength != 100 {
		t.Errorf("max_day_part_note_length: got %d, want 100", app.Today.MaxDayPartNoteLength)
	}
}

func TestLoad_SyncIntervalDefault(t *testing.T) {
	setupLoadTest(t)
	t.Setenv("TODOIST_API_SYNC_INTERVAL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.App.SyncInterval != 60*time.Second {
		t.Errorf("sync_interval default: got %v, want 60s", cfg.App.SyncInterval)
	}
}

func TestLoad_SyncIntervalCustom(t *testing.T) {
	setupLoadTest(t)
	t.Setenv("TODOIST_API_SYNC_INTERVAL", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.App.SyncInterval != 30*time.Second {
		t.Errorf("sync_interval: got %v, want 30s", cfg.App.SyncInterval)
	}
}

func TestLoad_SyncIntervalMinimum(t *testing.T) {
	setupLoadTest(t)
	t.Setenv("TODOIST_API_SYNC_INTERVAL", "1s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.App.SyncInterval != 5*time.Second {
		t.Errorf("sync_interval minimum: got %v, want 5s", cfg.App.SyncInterval)
	}
}

func TestParseAppConfig_AutoRemoveDefaults(t *testing.T) {
	yaml := `
auto_remove:
  rules:
    - label: "urgent"
      ttl: "2h"
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.AutoRemove.MinTTL != time.Hour {
		t.Errorf("min_ttl default: got %v, want 1h", app.AutoRemove.MinTTL)
	}
	if app.AutoRemove.MaxPerTick != 1 {
		t.Errorf("max_per_tick default: got %d, want 1", app.AutoRemove.MaxPerTick)
	}
	if app.AutoRemove.MaxPercent != 10 {
		t.Errorf("max_percent default: got %d, want 10", app.AutoRemove.MaxPercent)
	}
}

func TestParseAppConfig_AutoRemoveTTLBelowMin(t *testing.T) {
	yaml := `
auto_remove:
  min_ttl: "1h"
  rules:
    - label: "fast"
      ttl: "30m"
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for TTL below min_ttl")
	}
}

func TestParseAppConfig_AutoRemoveEmptyLabel(t *testing.T) {
	yaml := `
auto_remove:
  rules:
    - label: ""
      ttl: "2h"
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty label")
	}
}

func TestParseAppConfig_AutoRemoveEmptyTTL(t *testing.T) {
	yaml := `
auto_remove:
  rules:
    - label: "urgent"
      ttl: ""
`
	_, err := ParseAppConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty ttl")
	}
}

func TestParseAppConfig_AutoRemoveEmpty(t *testing.T) {
	app, err := ParseAppConfig([]byte(`weekly: {label: "x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(app.AutoRemove.Rules) != 0 {
		t.Errorf("expected 0 auto_remove rules, got %d", len(app.AutoRemove.Rules))
	}
}

func TestParseAppConfig_AutoRemoveEnabledTrue(t *testing.T) {
	yaml := `
auto_remove:
  enabled: true
  rules:
    - label: "urgent"
      ttl: "2h"
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !app.AutoRemove.Enabled {
		t.Error("auto_remove.enabled: got false, want true")
	}
}

func TestParseAppConfig_AutoRemoveEnabledDefaultFalse(t *testing.T) {
	yaml := `
auto_remove:
  rules:
    - label: "urgent"
      ttl: "2h"
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.AutoRemove.Enabled {
		t.Error("auto_remove.enabled: got true, want false (default)")
	}
}

func TestParseAppConfig_LabelProjectMapEnabledTrue(t *testing.T) {
	yaml := `
label_project_map:
  enabled: true
  mappings:
    - label: "work"
      project: "Work"
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !app.LabelProjectMap.Enabled {
		t.Error("label_project_map.enabled: got false, want true")
	}
	if len(app.LabelProjectMap.Mappings) != 1 {
		t.Fatalf("label_project_map.mappings: got %d, want 1", len(app.LabelProjectMap.Mappings))
	}
	if app.LabelProjectMap.Mappings[0].Label != "work" {
		t.Errorf("label_project_map.mappings[0].label: got %q, want %q", app.LabelProjectMap.Mappings[0].Label, "work")
	}
}

func TestParseAppConfig_LabelProjectMapEnabledDefaultFalse(t *testing.T) {
	yaml := `
label_project_map:
  mappings:
    - label: "work"
      project: "Work"
`
	app, err := ParseAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.LabelProjectMap.Enabled {
		t.Error("label_project_map.enabled: got true, want false (default)")
	}
}

func TestParseAppConfig_LabelProjectMapOmitted(t *testing.T) {
	app, err := ParseAppConfig([]byte(`weekly: {label: "x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.LabelProjectMap.Enabled {
		t.Error("label_project_map.enabled: got true, want false")
	}
	if len(app.LabelProjectMap.Mappings) != 0 {
		t.Errorf("label_project_map.mappings: got %d, want 0", len(app.LabelProjectMap.Mappings))
	}
}

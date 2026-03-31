package storage

import (
	"encoding/json"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestGetStateDefaults(t *testing.T) {
	s := newTestStore(t)

	state, err := s.GetState()
	if err != nil {
		t.Fatalf("get state: %v", err)
	}

	if len(state.PinnedTasks) != 0 {
		t.Errorf("expected empty pinned_tasks, got %d", len(state.PinnedTasks))
	}
	if state.ActiveContextID != "" {
		t.Errorf("expected empty active_context_id, got %q", state.ActiveContextID)
	}
	if state.ActiveView != "all" {
		t.Errorf("expected active_view 'all', got %q", state.ActiveView)
	}
	if len(state.CollapsedIDs) != 0 {
		t.Errorf("expected empty collapsed_ids, got %d", len(state.CollapsedIDs))
	}
	if state.SidebarCollapsed {
		t.Error("expected sidebar_collapsed false")
	}
	if state.PlanningOpen {
		t.Error("expected planning_open false")
	}
}

func TestSetValueAndGetState(t *testing.T) {
	s := newTestStore(t)

	pinned := []PinnedTask{{ID: "t1", Content: "Task 1"}, {ID: "t2", Content: "Task 2"}}
	pinnedJSON, _ := json.Marshal(pinned)

	if err := s.SetValue("pinned_tasks", string(pinnedJSON)); err != nil {
		t.Fatalf("set pinned_tasks: %v", err)
	}
	if err := s.SetValue("active_context_id", "work"); err != nil {
		t.Fatalf("set active_context_id: %v", err)
	}
	if err := s.SetValue("active_view", "today"); err != nil {
		t.Fatalf("set active_view: %v", err)
	}
	collapsed, _ := json.Marshal([]string{"g1", "g2"})
	if err := s.SetValue("collapsed_ids", string(collapsed)); err != nil {
		t.Fatalf("set collapsed_ids: %v", err)
	}
	if err := s.SetValue("sidebar_collapsed", "true"); err != nil {
		t.Fatalf("set sidebar_collapsed: %v", err)
	}
	if err := s.SetValue("planning_open", "true"); err != nil {
		t.Fatalf("set planning_open: %v", err)
	}

	state, err := s.GetState()
	if err != nil {
		t.Fatalf("get state: %v", err)
	}

	if len(state.PinnedTasks) != 2 {
		t.Fatalf("expected 2 pinned_tasks, got %d", len(state.PinnedTasks))
	}
	if state.PinnedTasks[0].ID != "t1" || state.PinnedTasks[1].Content != "Task 2" {
		t.Errorf("unexpected pinned_tasks: %+v", state.PinnedTasks)
	}
	if state.ActiveContextID != "work" {
		t.Errorf("expected active_context_id 'work', got %q", state.ActiveContextID)
	}
	if state.ActiveView != "today" {
		t.Errorf("expected active_view 'today', got %q", state.ActiveView)
	}
	if len(state.CollapsedIDs) != 2 {
		t.Fatalf("expected 2 collapsed_ids, got %d", len(state.CollapsedIDs))
	}
	if state.CollapsedIDs[0] != "g1" {
		t.Errorf("expected collapsed_ids[0] 'g1', got %q", state.CollapsedIDs[0])
	}
	if !state.SidebarCollapsed {
		t.Error("expected sidebar_collapsed true")
	}
	if !state.PlanningOpen {
		t.Error("expected planning_open true")
	}
}

func TestSetValueOverwrite(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetValue("active_view", "today"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.SetValue("active_view", "weekly"); err != nil {
		t.Fatalf("set: %v", err)
	}

	state, err := s.GetState()
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.ActiveView != "weekly" {
		t.Errorf("expected 'weekly', got %q", state.ActiveView)
	}
}

func TestGetStateDefaults_NewFields(t *testing.T) {
	s := newTestStore(t)

	state, err := s.GetState()
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.Locale != "" {
		t.Errorf("expected empty locale, got %q", state.Locale)
	}
	if state.AllFilters != nil {
		t.Errorf("expected nil all_filters, got %+v", state.AllFilters)
	}
}

func TestSetValueAndGetState_Locale(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetValue("locale", "ru"); err != nil {
		t.Fatalf("set locale: %v", err)
	}

	state, err := s.GetState()
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.Locale != "ru" {
		t.Errorf("expected locale 'ru', got %q", state.Locale)
	}
}

func TestSetValueAndGetState_AllFilters(t *testing.T) {
	s := newTestStore(t)

	af := AllFiltersState{
		SelectedPriorities: []int{4, 3},
		SelectedLabels:     []string{"work", "urgent"},
		LinksOnly:          true,
		FiltersExpanded:    true,
	}
	data, _ := json.Marshal(af)
	if err := s.SetValue("all_filters", string(data)); err != nil {
		t.Fatalf("set all_filters: %v", err)
	}

	state, err := s.GetState()
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.AllFilters == nil {
		t.Fatal("expected non-nil all_filters")
	}
	if len(state.AllFilters.SelectedPriorities) != 2 || state.AllFilters.SelectedPriorities[0] != 4 {
		t.Errorf("unexpected selected_priorities: %v", state.AllFilters.SelectedPriorities)
	}
	if len(state.AllFilters.SelectedLabels) != 2 || state.AllFilters.SelectedLabels[0] != "work" {
		t.Errorf("unexpected selected_labels: %v", state.AllFilters.SelectedLabels)
	}
	if !state.AllFilters.LinksOnly {
		t.Error("expected links_only true")
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	s := newTestStore(t)

	// Run migrate again — should be a no-op
	if err := s.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	state, err := s.GetState()
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.ActiveView != "all" {
		t.Errorf("expected 'all' after idempotent migrate, got %q", state.ActiveView)
	}
}

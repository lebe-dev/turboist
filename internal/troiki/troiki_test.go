package troiki

import (
	"context"
	"fmt"
	"testing"

	synctodoist "github.com/CnTeng/todoist-api-go/sync"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

func ptr(s string) *string { return &s }

// --- mocks ---

type mockCache struct {
	projects      []*todoist.Project
	sections      []*todoist.Section
	tasks         []*todoist.Task
	addedTasks    []synctodoist.TaskAddArgs
	addedSections []struct{ name, projectID string }
	nextTaskID    string
	nextSectionID string
	addTaskErr    error
}

func (m *mockCache) Projects() []*todoist.Project { return m.projects }
func (m *mockCache) Sections() []*todoist.Section { return m.sections }
func (m *mockCache) Tasks() []*todoist.Task       { return m.tasks }

func (m *mockCache) AddTask(_ context.Context, args *synctodoist.TaskAddArgs) (string, error) {
	if m.addTaskErr != nil {
		return "", m.addTaskErr
	}
	m.addedTasks = append(m.addedTasks, *args)
	id := m.nextTaskID
	if id == "" {
		id = fmt.Sprintf("task-%d", len(m.addedTasks))
	}
	return id, nil
}

func (m *mockCache) AddSection(_ context.Context, name string, projectID string) (string, error) {
	m.addedSections = append(m.addedSections, struct{ name, projectID string }{name, projectID})
	id := m.nextSectionID
	if id == "" {
		id = fmt.Sprintf("sec-%d", len(m.addedSections))
	}
	return id, nil
}

type mockStore struct {
	capacity map[string]int
}

func newMockStore() *mockStore {
	return &mockStore{capacity: make(map[string]int)}
}

func (m *mockStore) GetAllTroikiCapacity() (map[string]int, error) {
	result := make(map[string]int)
	for k, v := range m.capacity {
		result[k] = v
	}
	return result, nil
}

func (m *mockStore) IncrementTroikiCapacity(sectionClass string) error {
	m.capacity[sectionClass]++
	return nil
}

func (m *mockStore) EnsureMinTroikiCapacity(sectionClass string, min int) error {
	if m.capacity[sectionClass] < min {
		m.capacity[sectionClass] = min
	}
	return nil
}

// --- helpers ---

func defaultCfg() config.TroikiConfig {
	return config.TroikiConfig{
		Enabled:            true,
		ProjectName:        "Troiki",
		MaxTasksPerSection: 3,
		InitialCapacity:    3,
		Sections: config.TroikiSectionsConfig{
			Important: "Важное",
			Medium:    "Среднее",
			Rest:      "Остальное",
		},
	}
}

func newTestService(mc *mockCache, store *mockStore) *Service {
	return NewService(mc, defaultCfg(), store)
}

func setupInitialized(mc *mockCache, store *mockStore) *Service {
	svc := newTestService(mc, store)
	svc.projectID = "proj-1"
	svc.sectionIDs[Important] = "sec-imp"
	svc.sectionIDs[Medium] = "sec-med"
	svc.sectionIDs[Rest] = "sec-rest"
	return svc
}

func task(id, projectID string, sectionID *string, parentID *string) *todoist.Task {
	return &todoist.Task{
		ID:        id,
		ProjectID: projectID,
		SectionID: sectionID,
		ParentID:  parentID,
		Labels:    []string{},
		Children:  []*todoist.Task{},
	}
}

// --- Init tests ---

func TestInit_FindsExistingSections(t *testing.T) {
	mc := &mockCache{
		projects: []*todoist.Project{{ID: "proj-1", Name: "Troiki"}},
		sections: []*todoist.Section{
			{ID: "sec-1", Name: "Важное", ProjectID: "proj-1"},
			{ID: "sec-2", Name: "Среднее", ProjectID: "proj-1"},
			{ID: "sec-3", Name: "Остальное", ProjectID: "proj-1"},
		},
	}
	svc := newTestService(mc, newMockStore())

	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if svc.projectID != "proj-1" {
		t.Errorf("projectID: got %q, want %q", svc.projectID, "proj-1")
	}
	if svc.sectionIDs[Important] != "sec-1" {
		t.Errorf("important section: got %q, want %q", svc.sectionIDs[Important], "sec-1")
	}
	if svc.sectionIDs[Medium] != "sec-2" {
		t.Errorf("medium section: got %q, want %q", svc.sectionIDs[Medium], "sec-2")
	}
	if svc.sectionIDs[Rest] != "sec-3" {
		t.Errorf("rest section: got %q, want %q", svc.sectionIDs[Rest], "sec-3")
	}
	if len(mc.addedSections) != 0 {
		t.Errorf("expected no sections created, got %d", len(mc.addedSections))
	}
}

func TestInit_CreatesMissingSections(t *testing.T) {
	mc := &mockCache{
		projects: []*todoist.Project{{ID: "proj-1", Name: "Troiki"}},
		sections: []*todoist.Section{
			{ID: "sec-1", Name: "Важное", ProjectID: "proj-1"},
		},
	}
	svc := newTestService(mc, newMockStore())

	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(mc.addedSections) != 2 {
		t.Fatalf("expected 2 sections created, got %d", len(mc.addedSections))
	}
	if mc.addedSections[0].name != "Среднее" {
		t.Errorf("first created section: got %q, want %q", mc.addedSections[0].name, "Среднее")
	}
	if mc.addedSections[1].name != "Остальное" {
		t.Errorf("second created section: got %q, want %q", mc.addedSections[1].name, "Остальное")
	}
}

func TestInit_ProjectNotFound(t *testing.T) {
	mc := &mockCache{
		projects: []*todoist.Project{{ID: "proj-1", Name: "Other"}},
	}
	svc := newTestService(mc, newMockStore())

	err := svc.Init(context.Background())
	if err == nil {
		t.Fatal("expected error for missing project")
	}
}

func TestInit_IgnoresSectionsFromOtherProjects(t *testing.T) {
	mc := &mockCache{
		projects: []*todoist.Project{{ID: "proj-1", Name: "Troiki"}},
		sections: []*todoist.Section{
			{ID: "sec-other", Name: "Важное", ProjectID: "proj-999"},
		},
	}
	svc := newTestService(mc, newMockStore())

	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// All 3 sections should be created since the existing one is from a different project
	if len(mc.addedSections) != 3 {
		t.Errorf("expected 3 sections created, got %d", len(mc.addedSections))
	}
}

func TestInit_SetsInitialCapacityForMediumAndRest(t *testing.T) {
	mc := &mockCache{
		projects: []*todoist.Project{{ID: "proj-1", Name: "Troiki"}},
		sections: []*todoist.Section{
			{ID: "sec-1", Name: "Важное", ProjectID: "proj-1"},
			{ID: "sec-2", Name: "Среднее", ProjectID: "proj-1"},
			{ID: "sec-3", Name: "Остальное", ProjectID: "proj-1"},
		},
	}
	store := newMockStore()
	svc := newTestService(mc, store)

	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if store.capacity["medium"] != 3 {
		t.Errorf("medium initial capacity: got %d, want 3", store.capacity["medium"])
	}
	if store.capacity["rest"] != 3 {
		t.Errorf("rest initial capacity: got %d, want 3", store.capacity["rest"])
	}
}

func TestInit_DoesNotDecreaseExistingCapacity(t *testing.T) {
	mc := &mockCache{
		projects: []*todoist.Project{{ID: "proj-1", Name: "Troiki"}},
		sections: []*todoist.Section{
			{ID: "sec-1", Name: "Важное", ProjectID: "proj-1"},
			{ID: "sec-2", Name: "Среднее", ProjectID: "proj-1"},
			{ID: "sec-3", Name: "Остальное", ProjectID: "proj-1"},
		},
	}
	store := newMockStore()
	store.capacity["medium"] = 7 // accumulated over time
	svc := newTestService(mc, store)

	if err := svc.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if store.capacity["medium"] != 7 {
		t.Errorf("medium capacity: got %d, want 7 (should not decrease)", store.capacity["medium"])
	}
}

// --- CanAddTask tests ---

func TestCanAddTask_ImportantOpen(t *testing.T) {
	mc := &mockCache{
		tasks: []*todoist.Task{
			task("t1", "proj-1", ptr("sec-imp"), nil),
			task("t2", "proj-1", ptr("sec-imp"), nil),
		},
	}
	svc := setupInitialized(mc, newMockStore())

	can, err := svc.CanAddTask(Important)
	if err != nil {
		t.Fatalf("CanAddTask: %v", err)
	}
	if !can {
		t.Error("expected CanAdd=true for important with 2/3 slots")
	}
}

func TestCanAddTask_ImportantFull(t *testing.T) {
	mc := &mockCache{
		tasks: []*todoist.Task{
			task("t1", "proj-1", ptr("sec-imp"), nil),
			task("t2", "proj-1", ptr("sec-imp"), nil),
			task("t3", "proj-1", ptr("sec-imp"), nil),
		},
	}
	svc := setupInitialized(mc, newMockStore())

	can, err := svc.CanAddTask(Important)
	if err != nil {
		t.Fatalf("CanAddTask: %v", err)
	}
	if can {
		t.Error("expected CanAdd=false for important with 3/3 slots")
	}
}

func TestCanAddTask_MediumWithCapacity(t *testing.T) {
	mc := &mockCache{
		tasks: []*todoist.Task{
			task("t1", "proj-1", ptr("sec-med"), nil),
		},
	}
	store := newMockStore()
	store.capacity["medium"] = 2
	svc := setupInitialized(mc, store)

	can, err := svc.CanAddTask(Medium)
	if err != nil {
		t.Fatalf("CanAddTask: %v", err)
	}
	if !can {
		t.Error("expected CanAdd=true for medium with 1 task and capacity 2")
	}
}

func TestCanAddTask_MediumNoCapacity(t *testing.T) {
	mc := &mockCache{
		tasks: []*todoist.Task{},
	}
	store := newMockStore()
	// capacity is 0 by default
	svc := setupInitialized(mc, store)

	can, err := svc.CanAddTask(Medium)
	if err != nil {
		t.Fatalf("CanAddTask: %v", err)
	}
	if can {
		t.Error("expected CanAdd=false for medium with 0 capacity")
	}
}

func TestCanAddTask_MediumFull(t *testing.T) {
	mc := &mockCache{
		tasks: []*todoist.Task{
			task("t1", "proj-1", ptr("sec-med"), nil),
			task("t2", "proj-1", ptr("sec-med"), nil),
			task("t3", "proj-1", ptr("sec-med"), nil),
		},
	}
	store := newMockStore()
	store.capacity["medium"] = 5 // capacity > max, but still capped at maxTasks
	svc := setupInitialized(mc, store)

	can, err := svc.CanAddTask(Medium)
	if err != nil {
		t.Fatalf("CanAddTask: %v", err)
	}
	if can {
		t.Error("expected CanAdd=false for medium at maxTasks even with high capacity")
	}
}

// --- OnTaskCompleted tests ---

func TestOnCompleted_ImportantUnlocksMedium(t *testing.T) {
	store := newMockStore()
	mc := &mockCache{}
	svc := setupInitialized(mc, store)

	svc.OnTaskCompleted(task("t1", "proj-1", ptr("sec-imp"), nil))

	if store.capacity["medium"] != 1 {
		t.Errorf("medium capacity: got %d, want 1", store.capacity["medium"])
	}
}

func TestOnCompleted_MediumUnlocksRest(t *testing.T) {
	store := newMockStore()
	mc := &mockCache{}
	svc := setupInitialized(mc, store)

	svc.OnTaskCompleted(task("t1", "proj-1", ptr("sec-med"), nil))

	if store.capacity["rest"] != 1 {
		t.Errorf("rest capacity: got %d, want 1", store.capacity["rest"])
	}
}

func TestOnCompleted_RestNoOp(t *testing.T) {
	store := newMockStore()
	mc := &mockCache{}
	svc := setupInitialized(mc, store)

	svc.OnTaskCompleted(task("t1", "proj-1", ptr("sec-rest"), nil))

	if len(store.capacity) != 0 {
		t.Errorf("expected no capacity changes, got %v", store.capacity)
	}
}

func TestOnCompleted_SubtaskNoOp(t *testing.T) {
	store := newMockStore()
	mc := &mockCache{}
	svc := setupInitialized(mc, store)

	// Subtask in important section — should NOT unlock medium
	svc.OnTaskCompleted(task("sub1", "proj-1", ptr("sec-imp"), ptr("t1")))

	if len(store.capacity) != 0 {
		t.Errorf("expected no capacity changes for subtask, got %v", store.capacity)
	}
}

func TestOnCompleted_DifferentProjectNoOp(t *testing.T) {
	store := newMockStore()
	mc := &mockCache{}
	svc := setupInitialized(mc, store)

	svc.OnTaskCompleted(task("t1", "other-proj", ptr("sec-imp"), nil))

	if len(store.capacity) != 0 {
		t.Errorf("expected no capacity changes for different project, got %v", store.capacity)
	}
}

// --- AddTask tests ---

func TestAddTask_Success(t *testing.T) {
	mc := &mockCache{
		tasks:      []*todoist.Task{},
		nextTaskID: "new-task-1",
	}
	store := newMockStore()
	store.capacity["medium"] = 2
	svc := setupInitialized(mc, store)

	id, err := svc.AddTask(context.Background(), Medium, "Test task", "Description")
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if id != "new-task-1" {
		t.Errorf("task ID: got %q, want %q", id, "new-task-1")
	}
	if len(mc.addedTasks) != 1 {
		t.Fatalf("expected 1 added task, got %d", len(mc.addedTasks))
	}
	added := mc.addedTasks[0]
	if added.Content != "Test task" {
		t.Errorf("content: got %q, want %q", added.Content, "Test task")
	}
	if added.ProjectID == nil || *added.ProjectID != "proj-1" {
		t.Errorf("project ID: got %v, want %q", added.ProjectID, "proj-1")
	}
	if added.SectionID == nil || *added.SectionID != "sec-med" {
		t.Errorf("section ID: got %v, want %q", added.SectionID, "sec-med")
	}
}

func TestAddTask_NoCapacity(t *testing.T) {
	mc := &mockCache{tasks: []*todoist.Task{}}
	store := newMockStore()
	// Medium has 0 capacity
	svc := setupInitialized(mc, store)

	_, err := svc.AddTask(context.Background(), Medium, "Test task", "")
	if err != ErrNoCapacity {
		t.Errorf("expected ErrNoCapacity, got %v", err)
	}
}

func TestAddTask_NoCapacitySpent(t *testing.T) {
	mc := &mockCache{
		tasks: []*todoist.Task{
			task("t1", "proj-1", ptr("sec-med"), nil),
		},
		nextTaskID: "new-task-1",
	}
	store := newMockStore()
	store.capacity["medium"] = 3
	svc := setupInitialized(mc, store)

	_, err := svc.AddTask(context.Background(), Medium, "Task 2", "")
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Capacity should remain unchanged — AddTask does not decrement capacity
	if store.capacity["medium"] != 3 {
		t.Errorf("medium capacity: got %d, want 3 (should not be spent)", store.capacity["medium"])
	}
}

// --- ComputeState tests ---

func TestComputeState_Empty(t *testing.T) {
	mc := &mockCache{tasks: []*todoist.Task{}}
	store := newMockStore()
	svc := setupInitialized(mc, store)

	state, err := svc.ComputeState()
	if err != nil {
		t.Fatalf("ComputeState: %v", err)
	}
	if state.ProjectID != "proj-1" {
		t.Errorf("project ID: got %q, want %q", state.ProjectID, "proj-1")
	}
	if len(state.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(state.Sections))
	}

	imp := state.Sections[0]
	if imp.Class != Important {
		t.Errorf("section 0 class: got %q, want %q", imp.Class, Important)
	}
	if imp.RootCount != 0 {
		t.Errorf("important root count: got %d, want 0", imp.RootCount)
	}
	if imp.Capacity != 3 {
		t.Errorf("important capacity: got %d, want 3 (always maxTasks)", imp.Capacity)
	}
	if !imp.CanAdd {
		t.Error("important CanAdd should be true when empty")
	}

	med := state.Sections[1]
	if med.Capacity != 0 {
		t.Errorf("medium capacity: got %d, want 0", med.Capacity)
	}
	if med.CanAdd {
		t.Error("medium CanAdd should be false with 0 capacity")
	}
}

func TestComputeState_SubtasksNotCounted(t *testing.T) {
	mc := &mockCache{
		tasks: []*todoist.Task{
			task("t1", "proj-1", ptr("sec-imp"), nil),
			task("sub1", "proj-1", ptr("sec-imp"), ptr("t1")),
			task("sub2", "proj-1", ptr("sec-imp"), ptr("t1")),
		},
	}
	svc := setupInitialized(mc, newMockStore())

	state, err := svc.ComputeState()
	if err != nil {
		t.Fatalf("ComputeState: %v", err)
	}

	imp := state.Sections[0]
	if imp.RootCount != 1 {
		t.Errorf("important root count: got %d, want 1 (subtasks not counted)", imp.RootCount)
	}
	if len(imp.Tasks) != 3 {
		t.Errorf("important total tasks: got %d, want 3 (all tasks including subtasks)", len(imp.Tasks))
	}
	if !imp.CanAdd {
		t.Error("important CanAdd should be true with 1/3 root tasks")
	}
}

func TestComputeState_TasksFromOtherProjectIgnored(t *testing.T) {
	mc := &mockCache{
		tasks: []*todoist.Task{
			task("t1", "proj-1", ptr("sec-imp"), nil),
			task("t-other", "proj-999", ptr("sec-other"), nil),
		},
	}
	svc := setupInitialized(mc, newMockStore())

	state, err := svc.ComputeState()
	if err != nil {
		t.Fatalf("ComputeState: %v", err)
	}

	imp := state.Sections[0]
	if imp.RootCount != 1 {
		t.Errorf("important root count: got %d, want 1", imp.RootCount)
	}
}

// --- Capacity accumulation ---

func TestCapacityAccumulation(t *testing.T) {
	mc := &mockCache{tasks: []*todoist.Task{}}
	store := newMockStore()
	svc := setupInitialized(mc, store)

	// Complete 3 important tasks — should give medium 3 capacity
	for i := range 3 {
		svc.OnTaskCompleted(task(fmt.Sprintf("imp-%d", i), "proj-1", ptr("sec-imp"), nil))
	}
	if store.capacity["medium"] != 3 {
		t.Errorf("medium capacity after 3 completions: got %d, want 3", store.capacity["medium"])
	}

	// Complete 2 medium tasks — should give rest 2 capacity
	for i := range 2 {
		svc.OnTaskCompleted(task(fmt.Sprintf("med-%d", i), "proj-1", ptr("sec-med"), nil))
	}
	if store.capacity["rest"] != 2 {
		t.Errorf("rest capacity after 2 completions: got %d, want 2", store.capacity["rest"])
	}

	// Medium capacity unchanged by medium completions
	if store.capacity["medium"] != 3 {
		t.Errorf("medium capacity should not change on medium completion: got %d, want 3", store.capacity["medium"])
	}
}

// --- Delete frees slot ---

func TestDeleteFreesSlot_NoCapacityChange(t *testing.T) {
	store := newMockStore()
	store.capacity["medium"] = 2

	mc := &mockCache{
		tasks: []*todoist.Task{
			task("t1", "proj-1", ptr("sec-med"), nil),
			task("t2", "proj-1", ptr("sec-med"), nil),
		},
	}
	svc := setupInitialized(mc, store)

	// Verify we can't add (2 root tasks, capacity 2 → 2 < 2 = false)
	can, err := svc.CanAddTask(Medium)
	if err != nil {
		t.Fatalf("CanAddTask: %v", err)
	}
	if can {
		t.Error("should not be able to add when rootCount == capacity")
	}

	// Simulate deletion by removing a task from cache
	mc.tasks = []*todoist.Task{
		task("t1", "proj-1", ptr("sec-med"), nil),
	}

	// Now should be able to add (1 root task, capacity 2 → 1 < 2 = true)
	can, err = svc.CanAddTask(Medium)
	if err != nil {
		t.Fatalf("CanAddTask: %v", err)
	}
	if !can {
		t.Error("should be able to add after deletion freed a slot")
	}

	// Capacity stays the same — no decrement on delete
	if store.capacity["medium"] != 2 {
		t.Errorf("medium capacity: got %d, want 2 (unchanged after delete)", store.capacity["medium"])
	}
}

// --- Priority tests ---

func TestAddTask_SetsCorrectPriority(t *testing.T) {
	cases := []struct {
		class    SectionClass
		wantPrio int
	}{
		{Important, 4},
		{Medium, 3},
		{Rest, 2},
	}

	for _, tc := range cases {
		mc := &mockCache{tasks: []*todoist.Task{}, nextTaskID: "t1"}
		store := newMockStore()
		store.capacity["medium"] = 3
		store.capacity["rest"] = 3
		svc := setupInitialized(mc, store)

		_, err := svc.AddTask(context.Background(), tc.class, "task", "")
		if err != nil {
			t.Fatalf("class=%s AddTask: %v", tc.class, err)
		}
		if len(mc.addedTasks) != 1 {
			t.Fatalf("class=%s: expected 1 added task, got %d", tc.class, len(mc.addedTasks))
		}
		added := mc.addedTasks[0]
		if added.Priority == nil {
			t.Errorf("class=%s: priority is nil, want %d", tc.class, tc.wantPrio)
		} else if *added.Priority != tc.wantPrio {
			t.Errorf("class=%s: priority got %d, want %d", tc.class, *added.Priority, tc.wantPrio)
		}
	}
}

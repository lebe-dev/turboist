package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	synctodoist "github.com/CnTeng/todoist-api-go/sync"
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
	"github.com/lebe-dev/turboist/internal/troiki"
)

// --- mocks ---

type troikiMockCache struct {
	projects      []*todoist.Project
	sections      []*todoist.Section
	tasks         []*todoist.Task
	addedTasks    []synctodoist.TaskAddArgs
	addedSections []struct{ name, projectID string }
	nextTaskID    string
	addTaskErr    error
}

func (m *troikiMockCache) Projects() []*todoist.Project { return m.projects }
func (m *troikiMockCache) Sections() []*todoist.Section { return m.sections }
func (m *troikiMockCache) Tasks() []*todoist.Task       { return m.tasks }

func (m *troikiMockCache) AddTask(_ context.Context, args *synctodoist.TaskAddArgs) (string, error) {
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

func (m *troikiMockCache) AddSection(_ context.Context, name string, projectID string) (string, error) {
	m.addedSections = append(m.addedSections, struct{ name, projectID string }{name, projectID})
	return fmt.Sprintf("sec-%d", len(m.addedSections)), nil
}

type troikiMockStore struct {
	capacity map[string]int
}

func newTroikiMockStore() *troikiMockStore {
	return &troikiMockStore{capacity: make(map[string]int)}
}

func (m *troikiMockStore) GetAllTroikiCapacity() (map[string]int, error) {
	result := make(map[string]int)
	for k, v := range m.capacity {
		result[k] = v
	}
	return result, nil
}

func (m *troikiMockStore) IncrementTroikiCapacity(sectionClass string) error {
	m.capacity[sectionClass]++
	return nil
}

// --- helpers ---

func troikiCfg() config.TroikiConfig {
	return config.TroikiConfig{
		Enabled:            true,
		ProjectName:        "Troiki",
		MaxTasksPerSection: 3,
		Sections: config.TroikiSectionsConfig{
			Important: "Важное",
			Medium:    "Среднее",
			Rest:      "Остальное",
		},
	}
}

func troikiTask(id string, sectionID *string, parentID *string) *todoist.Task {
	return &todoist.Task{
		ID:        id,
		ProjectID: "proj-1",
		SectionID: sectionID,
		ParentID:  parentID,
		Labels:    []string{},
		Children:  []*todoist.Task{},
	}
}

func setupTroikiService(mc *troikiMockCache, store *troikiMockStore) *troiki.Service {
	svc := troiki.NewService(mc, troikiCfg(), store)
	// Manually set resolved IDs to avoid needing Init
	svc.SetTestState("proj-1", map[troiki.SectionClass]string{
		troiki.Important: "sec-imp",
		troiki.Medium:    "sec-med",
		troiki.Rest:      "sec-rest",
	})
	return svc
}

func newTroikiApp(svc *troiki.Service) *fiber.App {
	h := NewTroikiHandler(svc)
	app := fiber.New()
	app.Get("/api/troiki", h.State)
	app.Post("/api/troiki/tasks", h.CreateTask)
	return app
}

// --- GET /api/troiki tests ---

func TestTroikiState_Empty(t *testing.T) {
	mc := &troikiMockCache{tasks: []*todoist.Task{}}
	svc := setupTroikiService(mc, newTroikiMockStore())
	app := newTroikiApp(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/troiki", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}

	var state troiki.State
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if state.ProjectID != "proj-1" {
		t.Errorf("project_id: got %q, want %q", state.ProjectID, "proj-1")
	}
	if len(state.Sections) != 3 {
		t.Fatalf("sections: got %d, want 3", len(state.Sections))
	}
	if state.Sections[0].Class != troiki.Important {
		t.Errorf("section[0] class: got %q, want %q", state.Sections[0].Class, troiki.Important)
	}
	if !state.Sections[0].CanAdd {
		t.Error("important should be addable when empty")
	}
	if state.Sections[1].CanAdd {
		t.Error("medium should not be addable with 0 capacity")
	}
}

func TestTroikiState_WithTasks(t *testing.T) {
	mc := &troikiMockCache{
		tasks: []*todoist.Task{
			troikiTask("t1", ptr("sec-imp"), nil),
			troikiTask("t2", ptr("sec-imp"), nil),
			troikiTask("t3", ptr("sec-med"), nil),
		},
	}
	store := newTroikiMockStore()
	store.capacity["medium"] = 2
	svc := setupTroikiService(mc, store)
	app := newTroikiApp(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/troiki", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	var state troiki.State
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if state.Sections[0].RootCount != 2 {
		t.Errorf("important root_count: got %d, want 2", state.Sections[0].RootCount)
	}
	if state.Sections[1].RootCount != 1 {
		t.Errorf("medium root_count: got %d, want 1", state.Sections[1].RootCount)
	}
	if !state.Sections[1].CanAdd {
		t.Error("medium should be addable with capacity 2 and 1 task")
	}
}

// --- POST /api/troiki/tasks tests ---

func TestTroikiCreateTask_Success(t *testing.T) {
	mc := &troikiMockCache{
		tasks:      []*todoist.Task{},
		nextTaskID: "new-1",
	}
	store := newTroikiMockStore()
	store.capacity["medium"] = 2
	svc := setupTroikiService(mc, store)
	app := newTroikiApp(svc)

	body, _ := json.Marshal(map[string]string{
		"section_class": "medium",
		"content":       "New task",
		"description":   "Details",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/troiki/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["id"] != "new-1" {
		t.Errorf("id: got %v, want %q", result["id"], "new-1")
	}
}

func TestTroikiCreateTask_NoCapacity(t *testing.T) {
	mc := &troikiMockCache{tasks: []*todoist.Task{}}
	store := newTroikiMockStore()
	// medium capacity is 0
	svc := setupTroikiService(mc, store)
	app := newTroikiApp(svc)

	body, _ := json.Marshal(map[string]string{
		"section_class": "medium",
		"content":       "Task",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/troiki/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("got %d, want 409", resp.StatusCode)
	}
}

func TestTroikiCreateTask_ImportantAlwaysOpen(t *testing.T) {
	mc := &troikiMockCache{
		tasks:      []*todoist.Task{},
		nextTaskID: "imp-1",
	}
	svc := setupTroikiService(mc, newTroikiMockStore())
	app := newTroikiApp(svc)

	body, _ := json.Marshal(map[string]string{
		"section_class": "important",
		"content":       "Important task",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/troiki/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
}

func TestTroikiCreateTask_EmptyContent(t *testing.T) {
	mc := &troikiMockCache{tasks: []*todoist.Task{}}
	svc := setupTroikiService(mc, newTroikiMockStore())
	app := newTroikiApp(svc)

	body, _ := json.Marshal(map[string]string{
		"section_class": "important",
		"content":       "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/troiki/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestTroikiCreateTask_InvalidSectionClass(t *testing.T) {
	mc := &troikiMockCache{tasks: []*todoist.Task{}}
	svc := setupTroikiService(mc, newTroikiMockStore())
	app := newTroikiApp(svc)

	body, _ := json.Marshal(map[string]string{
		"section_class": "invalid",
		"content":       "Task",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/troiki/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestTroikiCreateTask_ImportantFull(t *testing.T) {
	mc := &troikiMockCache{
		tasks: []*todoist.Task{
			troikiTask("t1", ptr("sec-imp"), nil),
			troikiTask("t2", ptr("sec-imp"), nil),
			troikiTask("t3", ptr("sec-imp"), nil),
		},
	}
	svc := setupTroikiService(mc, newTroikiMockStore())
	app := newTroikiApp(svc)

	body, _ := json.Marshal(map[string]string{
		"section_class": "important",
		"content":       "Overflow",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/troiki/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("got %d, want 409", resp.StatusCode)
	}
}

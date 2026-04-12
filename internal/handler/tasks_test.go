package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/storage"
	"github.com/lebe-dev/turboist/internal/taskview"
	"github.com/lebe-dev/turboist/internal/todoist"
)

func ptr(s string) *string { return &s }

func TestBuildTree_flat(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Children: []*todoist.Task{}},
	}
	roots := taskview.BuildTree(tasks)
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}
}

func TestBuildTree_parentChild(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "parent", Children: []*todoist.Task{}},
		{ID: "2", Content: "child", ParentID: ptr("1"), Children: []*todoist.Task{}},
		{ID: "3", Content: "child2", ParentID: ptr("1"), Children: []*todoist.Task{}},
	}
	roots := taskview.BuildTree(tasks)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].SubTaskCount != 2 {
		t.Errorf("expected SubTaskCount=2, got %d", roots[0].SubTaskCount)
	}
	if len(roots[0].Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(roots[0].Children))
	}
}

func TestBuildTree_orphanBecomesRoot(t *testing.T) {
	tasks := []*todoist.Task{
		// parent not in set
		{ID: "2", Content: "child", ParentID: ptr("999"), Children: []*todoist.Task{}},
	}
	roots := taskview.BuildTree(tasks)
	if len(roots) != 1 {
		t.Fatalf("expected orphan to be root, got %d roots", len(roots))
	}
}

func TestBuildTree_doesNotMutateCached(t *testing.T) {
	original := &todoist.Task{ID: "1", Content: "p", Children: []*todoist.Task{}}
	child := &todoist.Task{ID: "2", Content: "c", ParentID: ptr("1"), Children: []*todoist.Task{}}

	taskview.BuildTree([]*todoist.Task{original, child})

	if len(original.Children) != 0 {
		t.Error("buildTree mutated cached task Children")
	}
}

func TestFilterByLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"на неделе", "другой"}},
		{ID: "2", Labels: []string{"другой"}},
		{ID: "3", Labels: []string{"на неделе"}},
	}
	got := taskview.FilterByLabel(tasks, "на неделе")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestFilterByLabel_emptyLabel(t *testing.T) {
	tasks := []*todoist.Task{{ID: "1"}, {ID: "2"}}
	got := taskview.FilterByLabel(tasks, "")
	if len(got) != 2 {
		t.Fatalf("expected all tasks returned for empty label, got %d", len(got))
	}
}

func TestFilterByDueDate_exactMatch(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Due: &todoist.Due{Date: "2026-03-13"}},
		{ID: "2", Due: &todoist.Due{Date: "2026-03-14"}},
		{ID: "3"},
	}
	target, _ := time.Parse("2006-01-02", "2026-03-13")
	got := taskview.FilterByDueDate(tasks, target, false)
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("expected 1 task (id=1), got %d", len(got))
	}
}

func TestFilterByDueDate_includeOverdue(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Due: &todoist.Due{Date: "2026-03-11"}}, // overdue
		{ID: "2", Due: &todoist.Due{Date: "2026-03-13"}}, // today
		{ID: "3", Due: &todoist.Due{Date: "2026-03-14"}}, // future
		{ID: "4"}, // no due
	}
	target, _ := time.Parse("2006-01-02", "2026-03-13")
	got := taskview.FilterByDueDate(tasks, target, true)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks (overdue + today), got %d", len(got))
	}
}

func TestSortTasks_Priority(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "low", Priority: 1, Children: []*todoist.Task{}},
		{ID: "2", Content: "high", Priority: 4, Children: []*todoist.Task{}},
		{ID: "3", Content: "med", Priority: 2, Children: []*todoist.Task{}},
	}
	taskview.SortTasks(tasks, "priority")
	if tasks[0].ID != "2" || tasks[1].ID != "3" || tasks[2].ID != "1" {
		t.Errorf("expected order [2,3,1], got [%s,%s,%s]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_PriorityThenDueDate(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Priority: 4, Due: &todoist.Due{Date: "2026-03-15"}, Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Priority: 4, Due: &todoist.Due{Date: "2026-03-10"}, Children: []*todoist.Task{}},
		{ID: "3", Content: "c", Priority: 4, Children: []*todoist.Task{}}, // no due date
	}
	taskview.SortTasks(tasks, "priority")
	if tasks[0].ID != "2" || tasks[1].ID != "1" || tasks[2].ID != "3" {
		t.Errorf("expected order [2,1,3], got [%s,%s,%s]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_DueDate(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Due: &todoist.Due{Date: "2026-03-15"}, Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Children: []*todoist.Task{}}, // no due
		{ID: "3", Content: "c", Due: &todoist.Due{Date: "2026-03-10"}, Children: []*todoist.Task{}},
	}
	taskview.SortTasks(tasks, "due_date")
	if tasks[0].ID != "3" || tasks[1].ID != "1" || tasks[2].ID != "2" {
		t.Errorf("expected order [3,1,2], got [%s,%s,%s]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_Content(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "Charlie", Children: []*todoist.Task{}},
		{ID: "2", Content: "alpha", Children: []*todoist.Task{}},
		{ID: "3", Content: "Bravo", Children: []*todoist.Task{}},
	}
	taskview.SortTasks(tasks, "content")
	if tasks[0].ID != "2" || tasks[1].ID != "3" || tasks[2].ID != "1" {
		t.Errorf("expected order [2,3,1], got [%s,%s,%s]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_Recursive(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "parent", Priority: 1, Children: []*todoist.Task{
			{ID: "c1", Content: "child-low", Priority: 1, Children: []*todoist.Task{}},
			{ID: "c2", Content: "child-high", Priority: 4, Children: []*todoist.Task{}},
		}},
	}
	taskview.SortTasks(tasks, "priority")
	if tasks[0].Children[0].ID != "c2" {
		t.Errorf("expected children sorted, got first child %s", tasks[0].Children[0].ID)
	}
}

func TestExcludeByLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"weekly", "other"}},
		{ID: "2", Labels: []string{"other"}},
		{ID: "3", Labels: []string{"weekly"}},
		{ID: "4", Labels: []string{}},
	}
	got := taskview.ExcludeByLabel(tasks, "weekly")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].ID != "2" || got[1].ID != "4" {
		t.Errorf("expected IDs [2,4], got [%s,%s]", got[0].ID, got[1].ID)
	}
}

func TestExcludeByLabel_emptyLabel(t *testing.T) {
	tasks := []*todoist.Task{{ID: "1"}, {ID: "2"}}
	got := taskview.ExcludeByLabel(tasks, "")
	if len(got) != 2 {
		t.Fatalf("expected all tasks returned for empty label, got %d", len(got))
	}
}

func autoLabel(mask, label string, ignoreCase bool) config.CompiledAutoLabel {
	m := mask
	if ignoreCase {
		m = strings.ToLower(m)
	}
	return config.CompiledAutoLabel{Label: label, Mask: m, IgnoreCase: ignoreCase}
}

func TestApplyAutoLabels_Match(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("Купить молоко", []string{}, tags)
	if len(got) != 1 || got[0] != "покупки" {
		t.Errorf("expected [покупки], got %v", got)
	}
}

func TestApplyAutoLabels_NoMatch(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("Позвонить другу", []string{}, tags)
	if len(got) != 0 {
		t.Errorf("expected no labels, got %v", got)
	}
}

func TestApplyAutoLabels_NoDuplicate(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("Купить молоко", []string{"покупки"}, tags)
	if len(got) != 1 {
		t.Errorf("expected 1 label (no duplicate), got %v", got)
	}
}

func TestApplyAutoLabels_CaseInsensitive(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("КУПИТЬ ХЛЕБ", []string{}, tags)
	if len(got) != 1 || got[0] != "покупки" {
		t.Errorf("expected [покупки], got %v", got)
	}
}

func TestApplyAutoLabels_CaseSensitive(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", false)}
	if got := applyAutoLabels("купить молоко", []string{}, tags); len(got) != 1 {
		t.Errorf("expected match for exact case, got %v", got)
	}
	if got := applyAutoLabels("КУПИТЬ молоко", []string{}, tags); len(got) != 0 {
		t.Errorf("expected no match for wrong case, got %v", got)
	}
}

func TestApplyAutoLabels_MultipleMatches(t *testing.T) {
	tags := []config.CompiledAutoLabel{
		autoLabel("купить", "покупки", true),
		autoLabel("встреча", "работа", true),
	}
	got := applyAutoLabels("Встреча и купить кофе", []string{}, tags)
	if len(got) != 2 {
		t.Errorf("expected 2 labels, got %v", got)
	}
}

func TestApplyAutoLabels_PreservesExisting(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("Купить молоко", []string{"важное"}, tags)
	if len(got) != 2 {
		t.Errorf("expected 2 labels, got %v", got)
	}
	if got[0] != "важное" {
		t.Errorf("expected existing label first, got %v", got[0])
	}
}

func TestCountWithLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"на неделе"}},
		{ID: "2", Labels: []string{}},
		{ID: "3", Labels: []string{"на неделе"}},
	}
	if n := taskview.CountWithLabel(tasks, "на неделе"); n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
	if n := taskview.CountWithLabel(tasks, ""); n != 0 {
		t.Errorf("expected 0 for empty label, got %d", n)
	}
}

// --- Postpone budget enforcement tests ---

func newTestTasksApp(t *testing.T, cfg *config.AppConfig, store *storage.Store, tasks []*todoist.Task) *fiber.App {
	t.Helper()
	cache := todoist.NewTestCacheWithMock(tasks, nil, nil, nil)

	sessStore := auth.NewSessionStore()
	token, _ := sessStore.CreateSession()
	mw := auth.NewMiddleware(sessStore)

	h := NewTasksHandler(cache, cfg, store, nil)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Request().Header.SetCookie(cookieName, token)
		return c.Next()
	})
	app.Use(mw)
	app.Patch("/api/tasks/:id", h.Update)

	return app
}

func patchTask(t *testing.T, app *fiber.App, id string, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/tasks/"+id, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func TestUpdate_PostponeBudgetAllowed(t *testing.T) {
	store := newTestStore(t)
	today := time.Now().Format("2006-01-02")

	// Set budget with 1 used out of 3
	if err := store.SetPostponeBudget(&storage.PostponeBudgetState{Date: today, Used: 1}); err != nil {
		t.Fatalf("set postpone budget: %v", err)
	}

	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled:        true,
			PostponeBudget: 3,
		},
	}

	tasks := []*todoist.Task{
		{ID: "task1", Due: &todoist.Due{Date: today}},
	}

	app := newTestTasksApp(t, cfg, store, tasks)
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	resp := patchTask(t, app, "task1", map[string]any{"due_date": tomorrow})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200 (under budget)", resp.StatusCode)
	}

	// Verify budget was incremented
	budgetState, err := store.GetPostponeBudget()
	if err != nil {
		t.Fatalf("get postpone budget: %v", err)
	}
	if budgetState == nil {
		t.Fatal("postpone budget state is nil after postpone")
	}
	if budgetState.Used != 2 {
		t.Errorf("got used %d, want 2", budgetState.Used)
	}
}

func TestUpdate_PostponeBudgetExhausted(t *testing.T) {
	store := newTestStore(t)
	today := time.Now().Format("2006-01-02")

	// Set budget to max
	if err := store.SetPostponeBudget(&storage.PostponeBudgetState{Date: today, Used: 3}); err != nil {
		t.Fatalf("set postpone budget: %v", err)
	}

	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled:        true,
			PostponeBudget: 3,
		},
	}

	tasks := []*todoist.Task{
		{ID: "task1", Due: &todoist.Due{Date: today}},
	}

	app := newTestTasksApp(t, cfg, store, tasks)
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	resp := patchTask(t, app, "task1", map[string]any{"due_date": tomorrow})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (budget exhausted)", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "Daily postpone limit reached" {
		t.Errorf("got error %q, want %q", body["error"], "Daily postpone limit reached")
	}
}

func TestUpdate_PostponeBudgetResetsOnNewDay(t *testing.T) {
	store := newTestStore(t)
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// Set budget exhausted for yesterday
	if err := store.SetPostponeBudget(&storage.PostponeBudgetState{Date: yesterday, Used: 3}); err != nil {
		t.Fatalf("set postpone budget: %v", err)
	}

	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled:        true,
			PostponeBudget: 3,
		},
	}

	tasks := []*todoist.Task{
		{ID: "task1", Due: &todoist.Due{Date: today}},
	}

	app := newTestTasksApp(t, cfg, store, tasks)
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	resp := patchTask(t, app, "task1", map[string]any{"due_date": tomorrow})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200 (budget resets on new day)", resp.StatusCode)
	}

	// Verify budget was set to 1 for today
	budgetState, err := store.GetPostponeBudget()
	if err != nil {
		t.Fatalf("get postpone budget: %v", err)
	}
	if budgetState.Date != today {
		t.Errorf("got date %q, want %q", budgetState.Date, today)
	}
	if budgetState.Used != 1 {
		t.Errorf("got used %d, want 1", budgetState.Used)
	}
}

func TestUpdate_PostponeBudgetUnlimitedWhenZero(t *testing.T) {
	store := newTestStore(t)
	today := time.Now().Format("2006-01-02")

	cfg := &config.AppConfig{
		Constraints: config.ConstraintsConfig{
			PostponeBudget: 0, // unlimited
		},
	}

	tasks := []*todoist.Task{
		{ID: "task1", Due: &todoist.Due{Date: today}},
	}

	app := newTestTasksApp(t, cfg, store, tasks)
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	resp := patchTask(t, app, "task1", map[string]any{"due_date": tomorrow})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200 (unlimited when budget=0)", resp.StatusCode)
	}

	// Budget state should NOT be set when PostponeBudget=0
	budgetState, err := store.GetPostponeBudget()
	if err != nil {
		t.Fatalf("get postpone budget: %v", err)
	}
	if budgetState != nil {
		t.Errorf("got budget state %+v, want nil (no tracking when unlimited)", budgetState)
	}
}

// --- Decompose handler tests ---

func newDecomposeApp(t *testing.T, tasks []*todoist.Task) *fiber.App {
	t.Helper()
	cache := todoist.NewTestCacheWithMock(tasks, nil, nil, nil)
	store := newTestStore(t)

	sessStore := auth.NewSessionStore()
	token, _ := sessStore.CreateSession()
	mw := auth.NewMiddleware(sessStore)

	h := NewTasksHandler(cache, &config.AppConfig{}, store, nil)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Request().Header.SetCookie(cookieName, token)
		return c.Next()
	})
	app.Use(mw)
	app.Post("/api/tasks/:id/decompose", h.Decompose)

	return app
}

func postDecompose(t *testing.T, app *fiber.App, id string, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/"+id+"/decompose", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func TestDecompose_WithPriorityAndDueDate(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "src-1", Content: "Big task", Priority: 2, Due: &todoist.Due{Date: "2026-04-10"}, Labels: []string{"work"}, Children: []*todoist.Task{}},
	}

	app := newDecomposeApp(t, tasks)
	resp := postDecompose(t, app, "src-1", map[string]any{
		"tasks":    []string{"Sub A", "Sub B"},
		"priority": 4,
		"due_date": "2026-04-15",
	})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
}

func TestDecompose_WithoutOverrides(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "src-1", Content: "Big task", Priority: 2, Children: []*todoist.Task{}},
	}

	app := newDecomposeApp(t, tasks)
	resp := postDecompose(t, app, "src-1", map[string]any{
		"tasks": []string{"Sub A"},
	})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
}

func TestDecompose_InvalidPriority(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "src-1", Content: "Big task", Priority: 2, Children: []*todoist.Task{}},
	}

	app := newDecomposeApp(t, tasks)
	resp := postDecompose(t, app, "src-1", map[string]any{
		"tasks":    []string{"Sub A"},
		"priority": 5,
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (invalid priority)", resp.StatusCode)
	}
}

func TestDecompose_PriorityZero(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "src-1", Content: "Big task", Priority: 2, Children: []*todoist.Task{}},
	}

	app := newDecomposeApp(t, tasks)
	resp := postDecompose(t, app, "src-1", map[string]any{
		"tasks":    []string{"Sub A"},
		"priority": 0,
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (priority 0 is below range)", resp.StatusCode)
	}
}

func TestDecompose_PriorityOnly(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "src-1", Content: "Big task", Priority: 2, Children: []*todoist.Task{}},
	}

	app := newDecomposeApp(t, tasks)
	resp := postDecompose(t, app, "src-1", map[string]any{
		"tasks":    []string{"Sub A"},
		"priority": 3,
	})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
}

func TestDecompose_DueDateOnly(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "src-1", Content: "Big task", Priority: 2, Children: []*todoist.Task{}},
	}

	app := newDecomposeApp(t, tasks)
	resp := postDecompose(t, app, "src-1", map[string]any{
		"tasks":    []string{"Sub A"},
		"due_date": "2026-04-15",
	})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want 201", resp.StatusCode)
	}
}

func TestDecompose_InvalidDueDate(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "src-1", Content: "Big task", Priority: 2, Children: []*todoist.Task{}},
	}

	app := newDecomposeApp(t, tasks)
	resp := postDecompose(t, app, "src-1", map[string]any{
		"tasks":    []string{"Sub A"},
		"due_date": "not-a-date",
	})

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (invalid due_date)", resp.StatusCode)
	}
}

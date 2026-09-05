package repo

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// The list views are re-implemented as local queries by the native mobile
// replica, which renders its lists offline instead of asking the server. Two
// implementations of the same predicates drift silently unless something pins
// them together, so both sides load one shared fixture dataset and must produce
// the orderings stored under testdata/sync-contract/golden. This test is the Go
// half of that contract: it replays the fixture through the repositories, runs
// every view in views.go over it, and diffs the ordered task lists against the
// goldens. The mobile client runs the same fixture through its own queries and
// diffs against the same files.
//
// Rerun with `go test ./internal/repo -run TestViewContract -update` to rewrite
// the goldens after a deliberate change to a view predicate or to the sort, and
// review the diff — every changed line is a behaviour change the mobile client
// has to follow.
var updateViewContractGoldens = flag.Bool("update", false, "rewrite the shared view-contract golden files")

const viewContractDir = "../../testdata/sync-contract"

// --- fixture shape -------------------------------------------------------

type contractFixture struct {
	Clock     contractClock      `json:"clock"`
	Contexts  []contractContext  `json:"contexts"`
	Projects  []contractProject  `json:"projects"`
	Sections  []contractSection  `json:"sections"`
	Labels    []contractLabel    `json:"labels"`
	Tasks     []contractTask     `json:"tasks"`
	Relations []contractRelation `json:"relations"`
	Views     []contractView     `json:"views"`
}

type contractClock struct {
	Now                  string `json:"now"`
	TodayStart           string `json:"todayStart"`
	WeekStart            string `json:"weekStart"`
	WeekEnd              string `json:"weekEnd"`
	CompletedWindowStart string `json:"completedWindowStart"`
	CompletedWindowEnd   string `json:"completedWindowEnd"`
}

type contractContext struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	IsFavourite bool   `json:"isFavourite"`
}

type contractProject struct {
	Key            string `json:"key"`
	ContextKey     string `json:"contextKey"`
	Title          string `json:"title"`
	Color          string `json:"color"`
	Type           string `json:"type"`
	Status         string `json:"status"`
	TroikiCategory string `json:"troikiCategory"`
}

type contractSection struct {
	Key        string `json:"key"`
	ProjectKey string `json:"projectKey"`
	Title      string `json:"title"`
}

type contractLabel struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type contractPlacement struct {
	Inbox      bool   `json:"inbox"`
	ContextKey string `json:"contextKey"`
	ProjectKey string `json:"projectKey"`
	SectionKey string `json:"sectionKey"`
	ParentKey  string `json:"parentKey"`
}

type contractTask struct {
	Key                    string            `json:"key"`
	Title                  string            `json:"title"`
	Placement              contractPlacement `json:"placement"`
	Priority               string            `json:"priority"`
	Status                 string            `json:"status"`
	DueAt                  string            `json:"dueAt"`
	DueHasTime             bool              `json:"dueHasTime"`
	DeadlineAt             string            `json:"deadlineAt"`
	DeadlineHasTime        bool              `json:"deadlineHasTime"`
	DayPart                string            `json:"dayPart"`
	PlanState              string            `json:"planState"`
	IsPinned               bool              `json:"isPinned"`
	PinnedAt               string            `json:"pinnedAt"`
	IsPrivate              bool              `json:"isPrivate"`
	IsComplex              bool              `json:"isComplex"`
	RecurrenceRule         string            `json:"recurrenceRule"`
	CompletedAt            string            `json:"completedAt"`
	TroikiCategory         string            `json:"troikiCategory"`
	RecurrenceCompletionOf string            `json:"recurrenceCompletionOf"`
	LabelKeys              []string          `json:"labelKeys"`
	CreatedAt              string            `json:"createdAt"`
	Notes                  string            `json:"notes"`
}

// priority returns the fixture priority with the schema default applied, so an
// omitted field means the same thing on both sides of the contract.
func (c contractTask) priority() model.Priority {
	if c.Priority == "" {
		return model.PriorityNone
	}
	return model.Priority(c.Priority)
}

func (c contractTask) status() model.TaskStatus {
	if c.Status == "" {
		return model.TaskStatusOpen
	}
	return model.TaskStatus(c.Status)
}

func (c contractTask) dayPart() model.DayPart {
	if c.DayPart == "" {
		return model.DayPartNone
	}
	return model.DayPart(c.DayPart)
}

func (c contractTask) planState() model.PlanState {
	if c.PlanState == "" {
		return model.PlanStateNone
	}
	return model.PlanState(c.PlanState)
}

type contractRelation struct {
	SourceKey string `json:"sourceKey"`
	TargetKey string `json:"targetKey"`
	Type      string `json:"type"`
}

type contractView struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
}

// --- golden shape --------------------------------------------------------

type viewGolden struct {
	View     string   `json:"view"`
	Total    int      `json:"total"`
	TaskKeys []string `json:"taskKeys"`
}

type boardGolden struct {
	View   string       `json:"view"`
	Groups []boardGroup `json:"groups"`
}

type boardGroup struct {
	ProjectKey string   `json:"projectKey"`
	TaskKeys   []string `json:"taskKeys"`
}

// --- loading -------------------------------------------------------------

// contractWorld is the fixture materialised in a fresh database, plus the
// key↔id maps that let the goldens speak in stable fixture keys instead of
// database ids (which differ on every side of the contract).
type contractWorld struct {
	fixture    *contractFixture
	tasks      *TaskRepo
	contextIDs map[string]int64
	projectIDs map[string]int64
	sectionIDs map[string]int64
	labelIDs   map[string]int64
	taskIDs    map[string]int64
	taskKeys   map[int64]string
}

func readContractFixture(t *testing.T) *contractFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(viewContractDir, "fixture.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f contractFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &f
}

func mustParseContractTime(t *testing.T, field, value string) time.Time {
	t.Helper()
	ts, err := model.ParseUTC(value)
	if err != nil {
		t.Fatalf("%s: parse %q: %v", field, value, err)
	}
	return ts
}

func optionalContractTime(t *testing.T, field, value string) *time.Time {
	t.Helper()
	if value == "" {
		return nil
	}
	ts := mustParseContractTime(t, field, value)
	return &ts
}

// loadContractWorld replays the fixture through the repositories rather than
// inserting rows directly, so every schema default and every repository-level
// invariant applies exactly as it would in production. The clock-derived
// columns are the one exception: created_at, updated_at and pinned_at are
// stamped from the wall clock at write time, which would make the orderings
// depend on the machine running the test, so they are pinned to the fixture's
// fixed instants once the rows exist.
func loadContractWorld(t *testing.T) *contractWorld {
	t.Helper()
	f := readContractFixture(t)
	d := setupTestDB(t)
	ctx := context.Background()

	contexts := NewContextRepo(d)
	projectLabels := NewProjectLabelsRepo(d)
	projects := NewProjectRepo(d, projectLabels)
	sections := NewProjectSectionRepo(d)
	labels := NewLabelRepo(d)
	taskLabels := NewTaskLabelsRepo(d)
	relations := NewTaskRelationsRepo(d)
	tasks := NewTaskRepo(d, taskLabels, relations)

	w := &contractWorld{
		fixture:    f,
		tasks:      tasks,
		contextIDs: map[string]int64{},
		projectIDs: map[string]int64{},
		sectionIDs: map[string]int64{},
		labelIDs:   map[string]int64{},
		taskIDs:    map[string]int64{},
		taskKeys:   map[int64]string{},
	}

	for _, c := range f.Contexts {
		row, err := contexts.Create(ctx, c.Name, c.Color, c.IsFavourite)
		if err != nil {
			t.Fatalf("create context %s: %v", c.Key, err)
		}
		w.contextIDs[c.Key] = row.ID
	}

	for _, p := range f.Projects {
		ctxID, ok := w.contextIDs[p.ContextKey]
		if !ok {
			t.Fatalf("project %s: unknown contextKey %q", p.Key, p.ContextKey)
		}
		row, err := projects.Create(ctx, CreateProject{
			ContextID: ctxID,
			Title:     p.Title,
			Color:     p.Color,
			Type:      model.ProjectType(p.Type),
		})
		if err != nil {
			t.Fatalf("create project %s: %v", p.Key, err)
		}
		w.projectIDs[p.Key] = row.ID
		if p.TroikiCategory != "" {
			cat := model.TroikiCategory(p.TroikiCategory)
			if _, err := projects.Update(ctx, row.ID, ProjectUpdate{TroikiCategory: &cat}); err != nil {
				t.Fatalf("set troiki category on project %s: %v", p.Key, err)
			}
		}
		if p.Status != "" && p.Status != string(model.ProjectStatusOpen) {
			if err := projects.UpdateStatus(ctx, row.ID, model.ProjectStatus(p.Status)); err != nil {
				t.Fatalf("set status on project %s: %v", p.Key, err)
			}
		}
	}

	for _, s := range f.Sections {
		projectID, ok := w.projectIDs[s.ProjectKey]
		if !ok {
			t.Fatalf("section %s: unknown projectKey %q", s.Key, s.ProjectKey)
		}
		row, err := sections.Create(ctx, projectID, s.Title)
		if err != nil {
			t.Fatalf("create section %s: %v", s.Key, err)
		}
		w.sectionIDs[s.Key] = row.ID
	}

	for _, l := range f.Labels {
		row, err := labels.Create(ctx, l.Name, l.Color, false)
		if err != nil {
			t.Fatalf("create label %s: %v", l.Key, err)
		}
		w.labelIDs[l.Key] = row.ID
	}

	for _, task := range f.Tasks {
		w.createContractTask(t, ctx, tasks, taskLabels, task)
	}

	for _, rel := range f.Relations {
		source, ok := w.taskIDs[rel.SourceKey]
		if !ok {
			t.Fatalf("relation: unknown sourceKey %q", rel.SourceKey)
		}
		target, ok := w.taskIDs[rel.TargetKey]
		if !ok {
			t.Fatalf("relation: unknown targetKey %q", rel.TargetKey)
		}
		if _, err := relations.Create(ctx, source, target, model.RelationType(rel.Type)); err != nil {
			t.Fatalf("create relation %s -> %s: %v", rel.SourceKey, rel.TargetKey, err)
		}
	}

	for _, task := range f.Tasks {
		created := mustParseContractTime(t, task.Key+".createdAt", task.CreatedAt)
		var pinnedAt any
		if task.IsPinned {
			pinnedAt = model.FormatUTC(mustParseContractTime(t, task.Key+".pinnedAt", task.PinnedAt))
		}
		if _, err := d.ExecContext(ctx,
			`UPDATE tasks SET created_at = ?, updated_at = ?, pinned_at = ? WHERE id = ?`,
			model.FormatUTC(created), model.FormatUTC(created), pinnedAt, w.taskIDs[task.Key],
		); err != nil {
			t.Fatalf("pin clock columns for %s: %v", task.Key, err)
		}
	}

	return w
}

func (w *contractWorld) placementFor(t *testing.T, task contractTask) Placement {
	t.Helper()
	if task.Placement.Inbox {
		// The inbox is a single seeded row; a task either lives in it or in a context.
		return Placement{InboxID: ptr(int64(1))}
	}
	ctxID, ok := w.contextIDs[task.Placement.ContextKey]
	if !ok {
		t.Fatalf("task %s: unknown contextKey %q", task.Key, task.Placement.ContextKey)
	}
	p := Placement{ContextID: &ctxID}
	if key := task.Placement.ProjectKey; key != "" {
		id, ok := w.projectIDs[key]
		if !ok {
			t.Fatalf("task %s: unknown projectKey %q", task.Key, key)
		}
		p.ProjectID = &id
	}
	if key := task.Placement.SectionKey; key != "" {
		id, ok := w.sectionIDs[key]
		if !ok {
			t.Fatalf("task %s: unknown sectionKey %q", task.Key, key)
		}
		p.SectionID = &id
	}
	if key := task.Placement.ParentKey; key != "" {
		id, ok := w.taskIDs[key]
		if !ok {
			t.Fatalf("task %s: parentKey %q must appear earlier in the fixture", task.Key, key)
		}
		p.ParentID = &id
	}
	return p
}

func (w *contractWorld) createContractTask(t *testing.T, ctx context.Context, tasks *TaskRepo, taskLabels *TaskLabelsRepo, task contractTask) {
	t.Helper()
	var row *model.Task
	if source := task.RecurrenceCompletionOf; source != "" {
		// A completion snapshot is only ever produced by the recurrence path, so
		// the fixture goes through it instead of forging an equivalent row.
		sourceID, ok := w.taskIDs[source]
		if !ok {
			t.Fatalf("task %s: unknown recurrenceCompletionOf %q", task.Key, source)
		}
		base, err := tasks.Get(ctx, sourceID)
		if err != nil {
			t.Fatalf("task %s: load recurrence source: %v", task.Key, err)
		}
		completedAt := mustParseContractTime(t, task.Key+".completedAt", task.CompletedAt)
		row, err = tasks.CreateRecurrenceCompletion(ctx, base, completedAt)
		if err != nil {
			t.Fatalf("create recurrence completion %s: %v", task.Key, err)
		}
	} else {
		var err error
		row, err = tasks.Create(ctx, CreateTask{
			Placement:       w.placementFor(t, task),
			Title:           task.Title,
			Priority:        task.priority(),
			DueAt:           optionalContractTime(t, task.Key+".dueAt", task.DueAt),
			DueHasTime:      task.DueHasTime,
			DeadlineAt:      optionalContractTime(t, task.Key+".deadlineAt", task.DeadlineAt),
			DeadlineHasTime: task.DeadlineHasTime,
			DayPart:         task.dayPart(),
			PlanState:       task.planState(),
			RecurrenceRule:  optionalContractString(task.RecurrenceRule),
			IsComplex:       task.IsComplex,
		})
		if err != nil {
			t.Fatalf("create task %s: %v", task.Key, err)
		}
	}
	w.taskIDs[task.Key] = row.ID
	w.taskKeys[row.ID] = task.Key

	update := TaskUpdate{}
	dirty := false
	if st := task.status(); st != model.TaskStatusOpen {
		update.Status = &st
		update.CompletedAt = optionalContractTime(t, task.Key+".completedAt", task.CompletedAt)
		dirty = true
	}
	if task.IsPrivate {
		update.IsPrivate = ptr(true)
		dirty = true
	}
	if task.TroikiCategory != "" {
		cat := model.TroikiCategory(task.TroikiCategory)
		update.TroikiCategory = &cat
		dirty = true
	}
	if dirty {
		if _, err := tasks.Update(ctx, row.ID, update); err != nil {
			t.Fatalf("update task %s: %v", task.Key, err)
		}
	}
	if task.IsPinned {
		if err := tasks.SetPinned(ctx, row.ID, true); err != nil {
			t.Fatalf("pin task %s: %v", task.Key, err)
		}
	}
	if len(task.LabelKeys) > 0 {
		ids := make([]int64, 0, len(task.LabelKeys))
		for _, key := range task.LabelKeys {
			id, ok := w.labelIDs[key]
			if !ok {
				t.Fatalf("task %s: unknown labelKey %q", task.Key, key)
			}
			ids = append(ids, id)
		}
		if err := taskLabels.SetForTask(ctx, row.ID, ids); err != nil {
			t.Fatalf("set labels on task %s: %v", task.Key, err)
		}
	}
}

func optionalContractString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (w *contractWorld) keysOf(t *testing.T, tasks []model.Task) []string {
	t.Helper()
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		key, ok := w.taskKeys[task.ID]
		if !ok {
			t.Fatalf("task id %d is not part of the fixture", task.ID)
		}
		out = append(out, key)
	}
	return out
}

// --- rendering -----------------------------------------------------------

type contractRenderer struct {
	name string
	run  func(t *testing.T, w *contractWorld) viewGolden
}

func contractRenderers(t *testing.T, c contractClock) []contractRenderer {
	t.Helper()
	todayStart := mustParseContractTime(t, "clock.todayStart", c.TodayStart)
	weekStart := mustParseContractTime(t, "clock.weekStart", c.WeekStart)
	weekEnd := mustParseContractTime(t, "clock.weekEnd", c.WeekEnd)
	completedFrom := mustParseContractTime(t, "clock.completedWindowStart", c.CompletedWindowStart)
	completedTo := mustParseContractTime(t, "clock.completedWindowEnd", c.CompletedWindowEnd)

	// The goldens pin the full ordering of every view, so each paginated view is
	// rendered with the largest page the repositories accept.
	page := Page{Limit: 200}

	simple := func(name string, fn func(w *contractWorld) ([]model.Task, int, error)) contractRenderer {
		return contractRenderer{name: name, run: func(t *testing.T, w *contractWorld) viewGolden {
			t.Helper()
			items, total, err := fn(w)
			if err != nil {
				t.Fatalf("render %s: %v", name, err)
			}
			return viewGolden{View: name, Total: total, TaskKeys: w.keysOf(t, items)}
		}}
	}

	ctx := context.Background()
	high := model.PriorityHigh

	return []contractRenderer{
		simple("inbox", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListInbox(ctx, TaskFilter{}, page)
		}),
		simple("today", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListToday(ctx, todayStart, TaskFilter{}, page)
		}),
		simple("today-high-priority", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListToday(ctx, todayStart, TaskFilter{Priority: &high}, page)
		}),
		simple("tomorrow", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListTomorrow(ctx, todayStart, TaskFilter{}, page)
		}),
		simple("overdue", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListOverdue(ctx, todayStart, TaskFilter{}, page)
		}),
		simple("week", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListWeek(ctx, weekStart, weekEnd, TaskFilter{})
		}),
		simple("backlog", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListBacklog(ctx, TaskFilter{})
		}),
		simple("pinned", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListPinned(ctx, TaskFilter{})
		}),
		simple("completed", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListCompletedInRange(ctx, completedFrom, completedTo, TaskFilter{}, page)
		}),
		simple("troiki-important", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListByTroikiCategory(ctx, model.TroikiCategoryImportant)
		}),
		simple("troiki-medium", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListByTroikiCategory(ctx, model.TroikiCategoryMedium)
		}),
		simple("troiki-rest", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListByTroikiCategory(ctx, model.TroikiCategoryRest)
		}),
		simple("project-alpha", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListByProject(ctx, w.projectIDs["prj-alpha"], TaskFilter{}, page)
		}),
		simple("section-alpha-doing", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListBySection(ctx, w.sectionIDs["sec-alpha-doing"], TaskFilter{}, page)
		}),
		simple("label-urgent", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListByLabel(ctx, w.labelIDs["lbl-urgent"], TaskFilter{}, page)
		}),
		simple("context-work-direct", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListByContext(ctx, w.contextIDs["ctx-work"], false, TaskFilter{}, page)
		}),
		simple("context-work-all", func(w *contractWorld) ([]model.Task, int, error) {
			return w.tasks.ListByContext(ctx, w.contextIDs["ctx-work"], true, TaskFilter{}, page)
		}),
		simple("subtasks-direct", func(w *contractWorld) ([]model.Task, int, error) {
			items, err := w.tasks.ListSubtasks(ctx, w.taskIDs["t-week-planned-high"])
			return items, len(items), err
		}),
		simple("subtasks-recursive", func(w *contractWorld) ([]model.Task, int, error) {
			items, err := w.tasks.ListSubtasksRecursive(ctx, w.taskIDs["t-week-planned-high"])
			return items, len(items), err
		}),
	}
}

const contractBoardView = "troiki-board"

func renderContractBoard(t *testing.T, w *contractWorld) boardGolden {
	t.Helper()
	projectKeys := []string{"prj-alpha", "prj-beta", "prj-home"}
	ids := make([]int64, 0, len(projectKeys))
	for _, key := range projectKeys {
		ids = append(ids, w.projectIDs[key])
	}
	grouped, err := w.tasks.ListByProjectIDs(context.Background(), ids)
	if err != nil {
		t.Fatalf("render %s: %v", contractBoardView, err)
	}
	out := boardGolden{View: contractBoardView, Groups: make([]boardGroup, 0, len(projectKeys))}
	for _, key := range projectKeys {
		out.Groups = append(out.Groups, boardGroup{
			ProjectKey: key,
			TaskKeys:   w.keysOf(t, grouped[w.projectIDs[key]]),
		})
	}
	return out
}

// --- golden IO -----------------------------------------------------------

func contractGoldenPath(view string) string {
	return filepath.Join(viewContractDir, "golden", view+".json")
}

func compareContractGolden(t *testing.T, view string, got any, want any) {
	t.Helper()
	path := contractGoldenPath(view)
	if *updateViewContractGoldens {
		encoded, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatalf("encode golden %s: %v", view, err)
		}
		if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
			t.Fatalf("write golden %s: %v", view, err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (rerun with -update to create it): %v", view, err)
	}
	if err := json.Unmarshal(raw, want); err != nil {
		t.Fatalf("parse golden %s: %v", view, err)
	}
	wantValue := reflect.ValueOf(want).Elem().Interface()
	if !reflect.DeepEqual(got, wantValue) {
		t.Errorf("view %s: got %s, want %s", view, compactContractJSON(t, got), compactContractJSON(t, wantValue))
	}
}

func compactContractJSON(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(encoded)
}

// --- tests ---------------------------------------------------------------

func TestViewContract_Goldens(t *testing.T) {
	w := loadContractWorld(t)
	for _, r := range contractRenderers(t, w.fixture.Clock) {
		got := r.run(t, w)
		compareContractGolden(t, r.name, got, &viewGolden{})
	}
	compareContractGolden(t, contractBoardView, renderContractBoard(t, w), &boardGolden{})
}

// TestViewContract_FixtureDeclaresEveryRenderedView keeps the fixture the single
// catalogue of the contract: a view rendered here but absent from the fixture
// would be invisible to the client that ports these queries, and a view listed
// there but never rendered would advertise a golden that does not exist.
func TestViewContract_FixtureDeclaresEveryRenderedView(t *testing.T) {
	f := readContractFixture(t)
	declared := make([]string, 0, len(f.Views))
	for _, v := range f.Views {
		if v.Description == "" {
			t.Errorf("view %s: description is required", v.Name)
		}
		declared = append(declared, v.Name)
	}
	rendered := []string{contractBoardView}
	for _, r := range contractRenderers(t, f.Clock) {
		rendered = append(rendered, r.name)
	}
	sort.Strings(declared)
	sort.Strings(rendered)
	if !reflect.DeepEqual(declared, rendered) {
		t.Errorf("fixture views: got %v, want %v", declared, rendered)
	}
}

// TestViewContract_FixtureIsTotallyOrdered guards the goldens against
// flakiness. The shared sort falls back to created_at and the completed history
// sorts by completed_at, so two rows sharing either instant would leave their
// relative order up to the database rather than to the contract.
func TestViewContract_FixtureIsTotallyOrdered(t *testing.T) {
	f := readContractFixture(t)
	keys := map[string]bool{}
	created := map[string]string{}
	completed := map[string]string{}
	for _, task := range f.Tasks {
		if task.Key == "" {
			t.Fatalf("every fixture task needs a key")
		}
		if keys[task.Key] {
			t.Errorf("duplicate task key %q", task.Key)
		}
		keys[task.Key] = true
		if task.Notes == "" {
			t.Errorf("task %s: notes are required — every case must say which invariant it exercises", task.Key)
		}
		if task.CreatedAt == "" {
			t.Errorf("task %s: createdAt is required", task.Key)
		}
		if other, clash := created[task.CreatedAt]; clash {
			t.Errorf("tasks %s and %s share createdAt %s, which leaves their order undefined", other, task.Key, task.CreatedAt)
		}
		created[task.CreatedAt] = task.Key
		if task.status() != model.TaskStatusCompleted {
			continue
		}
		if task.CompletedAt == "" {
			t.Errorf("task %s: a completed task needs completedAt", task.Key)
			continue
		}
		if other, clash := completed[task.CompletedAt]; clash {
			t.Errorf("tasks %s and %s share completedAt %s, which leaves their order in the completed history undefined", other, task.Key, task.CompletedAt)
		}
		completed[task.CompletedAt] = task.Key
	}
}

// TestViewContract_FixtureMatchesStoredRows asserts the fixture describes the
// rows as they end up in storage. The client that ports these views inserts the
// fixture straight into its own tables instead of replaying it through this
// codebase's write paths, so any field the fixture understates — a default the
// repositories fill in, a value the recurrence path derives — would silently
// give the two sides different data to order.
func TestViewContract_FixtureMatchesStoredRows(t *testing.T) {
	w := loadContractWorld(t)
	ctx := context.Background()
	for _, task := range w.fixture.Tasks {
		row, err := w.tasks.Get(ctx, w.taskIDs[task.Key])
		if err != nil {
			t.Fatalf("load task %s: %v", task.Key, err)
		}
		checkContractRow(t, w, task, row)
	}
}

func checkContractRow(t *testing.T, w *contractWorld, task contractTask, row *model.Task) {
	t.Helper()
	if row.Title != task.Title {
		t.Errorf("task %s title: got %q, want %q", task.Key, row.Title, task.Title)
	}
	if row.Priority != task.priority() {
		t.Errorf("task %s priority: got %s, want %s", task.Key, row.Priority, task.priority())
	}
	if row.Status != task.status() {
		t.Errorf("task %s status: got %s, want %s", task.Key, row.Status, task.status())
	}
	if row.DayPart != task.dayPart() {
		t.Errorf("task %s dayPart: got %s, want %s", task.Key, row.DayPart, task.dayPart())
	}
	if row.PlanState != task.planState() {
		t.Errorf("task %s planState: got %s, want %s", task.Key, row.PlanState, task.planState())
	}
	if row.IsPinned != task.IsPinned {
		t.Errorf("task %s isPinned: got %v, want %v", task.Key, row.IsPinned, task.IsPinned)
	}
	if row.IsPrivate != task.IsPrivate {
		t.Errorf("task %s isPrivate: got %v, want %v", task.Key, row.IsPrivate, task.IsPrivate)
	}
	if row.IsComplex != task.IsComplex {
		t.Errorf("task %s isComplex: got %v, want %v", task.Key, row.IsComplex, task.IsComplex)
	}
	if row.DueHasTime != task.DueHasTime {
		t.Errorf("task %s dueHasTime: got %v, want %v", task.Key, row.DueHasTime, task.DueHasTime)
	}
	if row.DeadlineHasTime != task.DeadlineHasTime {
		t.Errorf("task %s deadlineHasTime: got %v, want %v", task.Key, row.DeadlineHasTime, task.DeadlineHasTime)
	}
	checkContractTime(t, task.Key, "dueAt", row.DueAt, task.DueAt)
	checkContractTime(t, task.Key, "deadlineAt", row.DeadlineAt, task.DeadlineAt)
	checkContractTime(t, task.Key, "pinnedAt", row.PinnedAt, task.PinnedAt)
	checkContractTime(t, task.Key, "completedAt", row.CompletedAt, task.CompletedAt)
	checkContractTime(t, task.Key, "createdAt", &row.CreatedAt, task.CreatedAt)
	checkContractString(t, task.Key, "recurrenceRule", row.RecurrenceRule, task.RecurrenceRule)
	if got := troikiCategoryString(row.TroikiCategory); got != task.TroikiCategory {
		t.Errorf("task %s troikiCategory: got %q, want %q", task.Key, got, task.TroikiCategory)
	}
	checkContractSourceTask(t, task.Key, w.taskKeys, row, task.RecurrenceCompletionOf)
	checkContractPlacement(t, w, task, row)
	checkContractLabels(t, w, task, row)
}

func troikiCategoryString(c *model.TroikiCategory) string {
	if c == nil {
		return ""
	}
	return string(*c)
}

func checkContractTime(t *testing.T, key, field string, got *time.Time, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Errorf("task %s %s: got %s, want none", key, field, model.FormatUTC(*got))
		}
		return
	}
	if got == nil {
		t.Errorf("task %s %s: got none, want %s", key, field, want)
		return
	}
	if formatted := model.FormatUTC(*got); formatted != want {
		t.Errorf("task %s %s: got %s, want %s", key, field, formatted, want)
	}
}

func checkContractString(t *testing.T, key, field string, got *string, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Errorf("task %s %s: got %q, want none", key, field, *got)
		}
		return
	}
	if got == nil || *got != want {
		t.Errorf("task %s %s: got %v, want %q", key, field, got, want)
	}
}

// checkContractSourceTask verifies the recurrence back-pointer, which only the
// recurrence path can produce: a completion snapshot points at the task it was
// cut from, every other row points at nothing.
func checkContractSourceTask(t *testing.T, key string, taskKeys map[int64]string, row *model.Task, want string) {
	t.Helper()
	got := ""
	if row.SourceTaskID != nil {
		got = taskKeys[*row.SourceTaskID]
	}
	if got != want {
		t.Errorf("task %s recurrenceCompletionOf: got %q, want %q", key, got, want)
	}
}

func checkContractPlacement(t *testing.T, w *contractWorld, task contractTask, row *model.Task) {
	t.Helper()
	if task.Placement.Inbox {
		if row.InboxID == nil {
			t.Errorf("task %s: got no inbox placement, want inbox", task.Key)
		}
		if row.ContextID != nil {
			t.Errorf("task %s: an inbox task must have no context", task.Key)
		}
		return
	}
	checkContractID(t, task.Key, "contextKey", row.ContextID, w.contextIDs, task.Placement.ContextKey)
	checkContractID(t, task.Key, "projectKey", row.ProjectID, w.projectIDs, task.Placement.ProjectKey)
	checkContractID(t, task.Key, "sectionKey", row.SectionID, w.sectionIDs, task.Placement.SectionKey)
	// A recurrence snapshot is deliberately detached from its parent, so the
	// fixture declares no parent for it and the row must not have one either.
	checkContractID(t, task.Key, "parentKey", row.ParentID, w.taskIDs, task.Placement.ParentKey)
}

func checkContractID(t *testing.T, key, field string, got *int64, ids map[string]int64, wantKey string) {
	t.Helper()
	if wantKey == "" {
		if got != nil {
			t.Errorf("task %s %s: got id %d, want none", key, field, *got)
		}
		return
	}
	want, ok := ids[wantKey]
	if !ok {
		t.Fatalf("task %s %s: unknown key %q", key, field, wantKey)
	}
	if got == nil || *got != want {
		t.Errorf("task %s %s: got %v, want %q", key, field, got, wantKey)
	}
}

func checkContractLabels(t *testing.T, w *contractWorld, task contractTask, row *model.Task) {
	t.Helper()
	got := make([]string, 0, len(row.Labels))
	for _, l := range row.Labels {
		for key, id := range w.labelIDs {
			if id == l.ID {
				got = append(got, key)
			}
		}
	}
	want := append([]string{}, task.LabelKeys...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("task %s labels: got %v, want %v", task.Key, got, want)
	}
}

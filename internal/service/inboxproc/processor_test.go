package inboxproc

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	"github.com/lebe-dev/turboist/internal/service/events"
)

// fakeClassifier answers from a function and records every call.
type fakeClassifier struct {
	mu      sync.Mutex
	calls   []string
	respond func(ctx context.Context, user string) (Completion, error)
}

func (f *fakeClassifier) Classify(ctx context.Context, _ string, user string) (Completion, error) {
	f.mu.Lock()
	f.calls = append(f.calls, user)
	respond := f.respond
	f.mu.Unlock()
	return respond(ctx, user)
}

func (f *fakeClassifier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func answer(content string) func(context.Context, string) (Completion, error) {
	return func(context.Context, string) (Completion, error) {
		return Completion{Content: content, Model: "fake/model", PromptTokens: 10, CompletionTokens: 5}, nil
	}
}

type procFixture struct {
	proc      *Processor
	llm       *fakeClassifier
	hub       *events.Hub
	tasks     *repo.TaskRepo
	tlabels   *repo.TaskLabelsRepo
	projects  *repo.ProjectRepo
	labels    *repo.LabelRepo
	state     *repo.InboxProcessingRepo
	settings  *repo.AppSettingsRepo
	contextID int64
	garden    *model.Project
	outdoor   *model.Label
	now       time.Time
	loc       *time.Location
}

const testInterval = time.Minute

func newProcFixture(t *testing.T) *procFixture {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	users := repo.NewUserRepo(d)
	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	contexts := repo.NewContextRepo(d)
	labels := repo.NewLabelRepo(d)
	state := repo.NewInboxProcessingRepo(d, tlabels)
	settings := repo.NewAppSettingsRepo(d)
	hub := events.NewHub(slog.Default())
	t.Cleanup(hub.Close)

	home, err := contexts.Create(ctx, "home", "green", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	garden, err := projects.Create(ctx, repo.CreateProject{ContextID: home.ID, Title: "Garden", Color: "green"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	outdoor, err := labels.Create(ctx, "outdoor", "green", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}

	loc, _ := time.LoadLocation("Europe/Moscow")
	f := &procFixture{
		llm: &fakeClassifier{respond: answer(`{"action":"keep","confidence":1,"reason":"unsure"}`)},
		hub: hub, tasks: tasks, tlabels: tlabels, projects: projects, labels: labels, state: state, settings: settings,
		contextID: home.ID, garden: garden, outdoor: outdoor,
		now: time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC),
		loc: loc,
	}
	f.proc = NewProcessor(Config{
		Enabled: true, Interval: testInterval, Model: "fake/model", BatchLimit: 10, Timeout: time.Second,
	}, f.llm, Deps{
		Tasks: tasks, TaskLabels: tlabels, State: state, Contexts: contexts, Projects: projects, Labels: labels,
		AppSettings: settings, Users: users, Move: service.NewMoveService(tasks, projects), Hub: hub,
		Location: loc, Log: slog.Default(),
	})
	f.proc.now = func() time.Time { return f.now }
	return f
}

func (f *procFixture) inboxTask(t *testing.T, title string) *model.Task {
	t.Helper()
	inboxID := int64(1)
	task, err := f.tasks.Create(context.Background(), repo.CreateTask{Placement: repo.Placement{InboxID: &inboxID}, Title: title})
	if err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	return task
}

func (f *procFixture) sortAnswer() string {
	return `{"action":"sort","projectId":` + itoa(f.garden.ID) + `,"labelIds":[` + itoa(f.outdoor.ID) +
		`],"priority":"high","dueDate":"2026-09-20","confidence":0.9,"reason":"garden work"}`
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func TestProcessor_SortsTask(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	existing, err := f.labels.Create(ctx, "captured", "blue", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	task := f.inboxTask(t, "Plant tulips")
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{existing.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}
	f.llm.respond = answer(f.sortAnswer())
	events, cancel := f.hub.Subscribe(1)
	defer cancel()

	summary, ran := f.proc.RunOnce(ctx)
	if !ran || summary.Sorted != 1 || summary.Kept != 0 || summary.Failed != 0 {
		t.Fatalf("summary: got %+v ran=%v, want one sorted", summary, ran)
	}

	got, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.InboxID != nil || got.ProjectID == nil || *got.ProjectID != f.garden.ID || *got.ContextID != f.contextID {
		t.Errorf("placement: got inbox=%v ctx=%v project=%v", got.InboxID, got.ContextID, got.ProjectID)
	}
	if ids := labelIDsOf(got.Labels); len(ids) != 2 || !containsID(ids, existing.ID) || !containsID(ids, f.outdoor.ID) {
		t.Errorf("labels: got %v, want the union of captured and outdoor", ids)
	}
	if got.Priority != model.PriorityHigh {
		t.Errorf("priority: got %s, want high", got.Priority)
	}
	wantDue := time.Date(2026, 9, 20, 0, 0, 0, 0, f.loc)
	if got.DueAt == nil || !got.DueAt.Equal(wantDue) || got.DueHasTime {
		t.Errorf("due: got %v hasTime=%v, want %v date-only", got.DueAt, got.DueHasTime, wantDue)
	}
	if got.AutoSortedAt == nil || !got.AutoSortedAt.Equal(f.now) {
		t.Errorf("marker: got %v, want %v", got.AutoSortedAt, f.now)
	}

	entries, _, err := f.state.ListLog(ctx, repo.Page{})
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	if len(entries) != 1 || entries[0].Outcome != model.InboxOutcomeSorted || entries[0].After == nil ||
		entries[0].After.ProjectID != f.garden.ID || entries[0].Reason != "garden work" ||
		len(entries[0].Before.LabelIDs) != 1 || entries[0].PromptTokens == nil {
		t.Errorf("journal: got %+v", entries)
	}
	if _, err := f.state.GetState(ctx, task.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("state after sort: got %v, want no row", err)
	}

	seen := map[string]bool{}
	for len(seen) < 3 {
		select {
		case ev := <-events:
			seen[string(ev.Scope)] = true
		case <-time.After(time.Second):
			t.Fatalf("published scopes: got %v, want tasks, inbox and plan", seen)
		}
	}
	for _, s := range []string{"tasks", "inbox", "plan"} {
		if !seen[s] {
			t.Errorf("scope %s was not published: %v", s, seen)
		}
	}
}

func TestProcessor_DoesNotOverrideUserPriorityOrDue(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "Plant tulips")
	userDue := time.Date(2026, 10, 1, 0, 0, 0, 0, f.loc)
	low := model.PriorityLow
	if _, err := f.tasks.Update(ctx, task.ID, repo.TaskUpdate{Priority: &low, DueAt: &userDue}); err != nil {
		t.Fatalf("update: %v", err)
	}
	f.llm.respond = answer(f.sortAnswer())

	if summary, _ := f.proc.RunOnce(ctx); summary.Sorted != 1 {
		t.Fatalf("summary: got %+v, want one sorted", summary)
	}
	got, _ := f.tasks.Get(ctx, task.ID)
	if got.Priority != model.PriorityLow {
		t.Errorf("priority: got %s, want the user's low", got.Priority)
	}
	if got.DueAt == nil || !got.DueAt.Equal(userDue) {
		t.Errorf("due: got %v, want the user's %v", got.DueAt, userDue)
	}
}

func TestProcessor_KeepIsNotAskedAgainUntilEdited(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "hmm")

	if summary, _ := f.proc.RunOnce(ctx); summary.Kept != 1 {
		t.Fatalf("first run: got %+v, want one kept", summary)
	}
	st, err := f.state.GetState(ctx, task.ID)
	if err != nil || st.Status != model.InboxStateKept {
		t.Fatalf("state: got %+v err=%v, want kept", st, err)
	}
	got, _ := f.tasks.Get(ctx, task.ID)
	if got.InboxID == nil {
		t.Fatal("a kept task must stay in the Inbox")
	}

	f.proc.RunOnce(ctx)
	if n := f.llm.callCount(); n != 1 {
		t.Fatalf("calls after second run: got %d, want 1", n)
	}

	title := "hmm, buy seeds for the garden"
	if _, err := f.tasks.Update(ctx, task.ID, repo.TaskUpdate{Title: &title}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	f.proc.RunOnce(ctx)
	if n := f.llm.callCount(); n != 2 {
		t.Errorf("calls after edit: got %d, want 2", n)
	}
}

func TestProcessor_LowConfidenceIsKept(t *testing.T) {
	f := newProcFixture(t)
	f.inboxTask(t, "something")
	f.llm.respond = answer(`{"action":"sort","projectId":` + itoa(f.garden.ID) + `,"confidence":0.2,"reason":"maybe"}`)
	if summary, _ := f.proc.RunOnce(context.Background()); summary.Kept != 1 || summary.Sorted != 0 {
		t.Errorf("summary: got %+v, want one kept", summary)
	}
}

func TestProcessor_FailureBacksOffAndSleeps(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "garbled")
	f.llm.respond = answer("not json at all")

	wantNext := []time.Duration{testInterval, 2 * testInterval, 4 * testInterval, 8 * testInterval}
	for attempt := 1; attempt <= 5; attempt++ {
		summary, _ := f.proc.RunOnce(ctx)
		if summary.Failed != 1 {
			t.Fatalf("attempt %d: got %+v, want one failed", attempt, summary)
		}
		st, err := f.state.GetState(ctx, task.ID)
		if err != nil {
			t.Fatalf("attempt %d: state: %v", attempt, err)
		}
		if st.Status != model.InboxStateFailed || st.Attempts != attempt || st.LastError == nil {
			t.Fatalf("attempt %d: state %+v", attempt, st)
		}
		if attempt < 5 {
			if st.NextAttemptAt == nil || !st.NextAttemptAt.Equal(f.now.Add(wantNext[attempt-1])) {
				t.Fatalf("attempt %d: next attempt %v, want now+%v", attempt, st.NextAttemptAt, wantNext[attempt-1])
			}
			// Not due yet: a run at the same instant must not ask again.
			f.proc.RunOnce(ctx)
			if n := f.llm.callCount(); n != attempt {
				t.Fatalf("attempt %d: calls %d before the retry time", attempt, n)
			}
			f.now = *st.NextAttemptAt
			continue
		}
		if st.NextAttemptAt != nil {
			t.Errorf("after 5 attempts: next attempt %v, want nil (sleep until edited)", st.NextAttemptAt)
		}
	}
	f.now = f.now.Add(24 * time.Hour)
	f.proc.RunOnce(ctx)
	if n := f.llm.callCount(); n != 5 {
		t.Errorf("calls while asleep: got %d, want 5", n)
	}
}

func TestProcessor_UnknownProjectFails(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "x")
	f.llm.respond = answer(`{"action":"sort","projectId":9999,"confidence":0.9}`)
	if summary, _ := f.proc.RunOnce(ctx); summary.Failed != 1 {
		t.Fatalf("summary: got %+v, want one failed", summary)
	}
	got, _ := f.tasks.Get(ctx, task.ID)
	if got.InboxID == nil {
		t.Error("a failed task must stay in the Inbox")
	}
	entries, _, _ := f.state.ListLog(ctx, repo.Page{})
	if len(entries) != 1 || entries[0].Outcome != model.InboxOutcomeFailed || entries[0].Error == nil {
		t.Errorf("journal: got %+v, want one failed row with an error", entries)
	}
}

func TestProcessor_ProviderErrorAbortsRun(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	a := f.inboxTask(t, "a")
	f.inboxTask(t, "b")
	f.llm.respond = func(context.Context, string) (Completion, error) {
		return Completion{}, ErrRateLimited
	}

	summary, _ := f.proc.RunOnce(ctx)
	if summary != (RunSummary{}) {
		t.Errorf("summary: got %+v, want nothing counted", summary)
	}
	if n := f.llm.callCount(); n != 1 {
		t.Fatalf("calls: got %d, want the run to stop after the first", n)
	}
	if _, err := f.state.GetState(ctx, a.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("state: got %v, want no attempt charged", err)
	}
	status := f.proc.Status()
	if status.LastError == nil || status.BackoffUntil == nil || !status.BackoffUntil.After(f.now) {
		t.Errorf("status: got %+v, want lastError and a backoff", status)
	}

	f.now = f.now.Add(testInterval)
	f.proc.RunOnce(ctx)
	if n := f.llm.callCount(); n != 1 {
		t.Errorf("calls during backoff: got %d, want 1", n)
	}

	// A manual run is the user's explicit request and ignores the backoff.
	f.llm.respond = answer(`{"action":"keep","confidence":1}`)
	if summary, _ := f.proc.runOnce(ctx, true); summary.Kept != 2 {
		t.Errorf("manual run: got %+v, want both kept", summary)
	}
	if status := f.proc.Status(); status.LastError != nil || status.BackoffUntil != nil {
		t.Errorf("status after a clean run: got %+v, want the error cleared", status)
	}
}

func TestProcessor_TaskEditedDuringRequestIsSkipped(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "Plant tulips")
	content := f.sortAnswer()
	f.llm.respond = func(context.Context, string) (Completion, error) {
		title := "Plant tulips and roses"
		if _, err := f.tasks.Update(ctx, task.ID, repo.TaskUpdate{Title: &title}); err != nil {
			t.Errorf("edit during request: %v", err)
		}
		return Completion{Content: content, Model: "fake/model"}, nil
	}

	summary, _ := f.proc.RunOnce(ctx)
	if summary != (RunSummary{}) {
		t.Errorf("summary: got %+v, want the task skipped", summary)
	}
	got, _ := f.tasks.Get(ctx, task.ID)
	if got.InboxID == nil || got.AutoSortedAt != nil {
		t.Errorf("task: got inbox=%v marker=%v, want untouched", got.InboxID, got.AutoSortedAt)
	}
	entries, total, _ := f.state.ListLog(ctx, repo.Page{})
	if total != 0 {
		t.Errorf("journal: got %+v, want nothing written", entries)
	}
}

func TestProcessor_TroikiProjectKeepsBucketPriority(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	important := model.TroikiCategoryImportant
	if _, err := f.projects.Update(ctx, f.garden.ID, repo.ProjectUpdate{TroikiCategory: &important}); err != nil {
		t.Fatalf("set category: %v", err)
	}
	task := f.inboxTask(t, "Plant tulips")
	f.llm.respond = answer(`{"action":"sort","projectId":` + itoa(f.garden.ID) + `,"priority":"low","confidence":0.9}`)

	if summary, _ := f.proc.RunOnce(ctx); summary.Sorted != 1 {
		t.Fatalf("summary: got %+v, want one sorted", summary)
	}
	got, _ := f.tasks.Get(ctx, task.ID)
	if got.Priority != model.PriorityHigh {
		t.Errorf("priority: got %s, want high from the important bucket", got.Priority)
	}
}

func TestProcessor_EmptyInboxMakesNoCalls(t *testing.T) {
	f := newProcFixture(t)
	summary, ran := f.proc.RunOnce(context.Background())
	if !ran || summary != (RunSummary{}) {
		t.Errorf("summary: got %+v ran=%v", summary, ran)
	}
	if n := f.llm.callCount(); n != 0 {
		t.Errorf("calls: got %d, want 0", n)
	}
	if status := f.proc.Status(); status.LastRunAt == nil {
		t.Error("status: an empty run still records when it ran")
	}
}

func TestProcessor_RunsNeverOverlap(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	f.inboxTask(t, "slow")
	entered := make(chan struct{})
	release := make(chan struct{})
	f.llm.respond = func(context.Context, string) (Completion, error) {
		close(entered)
		<-release
		return Completion{Content: `{"action":"keep","confidence":1}`, Model: "fake/model"}, nil
	}

	done := make(chan RunSummary)
	go func() {
		s, _ := f.proc.RunOnce(ctx)
		done <- s
	}()
	<-entered
	if !f.proc.Status().Running {
		t.Error("status: got running=false during a run")
	}
	if _, ran := f.proc.RunOnce(ctx); ran {
		t.Error("second run: got ran=true while the first is in flight")
	}
	close(release)
	if s := <-done; s.Kept != 1 {
		t.Errorf("first run: got %+v, want one kept", s)
	}
}

func TestProcessor_Revert(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	captured, err := f.labels.Create(ctx, "captured", "blue", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	task := f.inboxTask(t, "Plant tulips")
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{captured.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}
	f.llm.respond = answer(f.sortAnswer())
	if summary, _ := f.proc.RunOnce(ctx); summary.Sorted != 1 {
		t.Fatalf("summary: got %+v, want one sorted", summary)
	}
	entries, _, _ := f.state.ListLog(ctx, repo.Page{})
	logID := entries[0].ID

	f.now = f.now.Add(time.Hour)
	got, err := f.proc.Revert(ctx, logID)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}
	if got.InboxID == nil || got.ProjectID != nil || got.ContextID != nil {
		t.Errorf("placement: got inbox=%v ctx=%v project=%v, want the Inbox", got.InboxID, got.ContextID, got.ProjectID)
	}
	if ids := labelIDsOf(got.Labels); len(ids) != 1 || ids[0] != captured.ID {
		t.Errorf("labels: got %v, want only the captured label back", ids)
	}
	if got.Priority != model.PriorityNone || got.DueAt != nil {
		t.Errorf("priority/due: got %s %v, want the originals", got.Priority, got.DueAt)
	}
	if got.AutoSortedAt != nil {
		t.Errorf("marker: got %v, want nil", got.AutoSortedAt)
	}
	st, err := f.state.GetState(ctx, task.ID)
	if err != nil || st.Status != model.InboxStateReverted {
		t.Errorf("state: got %+v err=%v, want reverted", st, err)
	}
	entry, _ := f.state.GetLog(ctx, logID)
	if entry.RevertedAt == nil || !entry.RevertedAt.Equal(f.now) {
		t.Errorf("reverted at: got %v, want %v", entry.RevertedAt, f.now)
	}

	f.proc.RunOnce(ctx)
	if n := f.llm.callCount(); n != 1 {
		t.Errorf("calls after revert: got %d, want the task not filed again", n)
	}
	if _, err := f.proc.Revert(ctx, logID); !errors.Is(err, repo.ErrConflict) {
		t.Errorf("second revert: got %v, want ErrConflict", err)
	}
}

func TestProcessor_RevertKeepsLaterUserEdits(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "Plant tulips")
	f.llm.respond = answer(f.sortAnswer())
	f.proc.RunOnce(ctx)
	entries, _, _ := f.state.ListLog(ctx, repo.Page{})

	extra, err := f.labels.Create(ctx, "weekend", "blue", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{f.outdoor.ID, extra.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}
	medium := model.PriorityMedium
	if _, err := f.tasks.Update(ctx, task.ID, repo.TaskUpdate{Priority: &medium}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := f.proc.Revert(ctx, entries[0].ID)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}
	if ids := labelIDsOf(got.Labels); len(ids) != 1 || ids[0] != extra.ID {
		t.Errorf("labels: got %v, want the user's weekend label kept", ids)
	}
	if got.Priority != model.PriorityMedium {
		t.Errorf("priority: got %s, want the user's medium kept", got.Priority)
	}
}

func TestProcessor_RevertRefusals(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	if _, err := f.proc.Revert(ctx, 12345); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("missing row: got %v, want ErrNotFound", err)
	}
	f.inboxTask(t, "hmm")
	f.proc.RunOnce(ctx)
	entries, _, _ := f.state.ListLog(ctx, repo.Page{})
	if _, err := f.proc.Revert(ctx, entries[0].ID); !errors.Is(err, repo.ErrConflict) {
		t.Errorf("kept row: got %v, want ErrConflict", err)
	}
}

func TestProcessor_RevertOfSubtaskIsRefused(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "Plant tulips")
	f.llm.respond = answer(f.sortAnswer())
	f.proc.RunOnce(ctx)
	entries, _, _ := f.state.ListLog(ctx, repo.Page{})

	parent, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &f.contextID, ProjectID: &f.garden.ID}, Title: "Spring",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := f.tasks.Move(ctx, task.ID, repo.Placement{ContextID: &f.contextID, ProjectID: &f.garden.ID, ParentID: &parent.ID}); err != nil {
		t.Fatalf("make subtask: %v", err)
	}
	if _, err := f.proc.Revert(ctx, entries[0].ID); !errors.Is(err, repo.ErrInvalidPlacement) {
		t.Errorf("subtask revert: got %v, want ErrInvalidPlacement", err)
	}
}

func TestProcessor_PreviewAndValidate(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()

	rendered, err := f.proc.Preview(ctx, ptr("Projects: {{len .Projects}}; task: {{.Task.Title}}"))
	if err != nil {
		t.Fatalf("preview on empty inbox: %v", err)
	}
	if want := "Projects: 1; task: Fix login redirect on Safari\n\n" + outputContract; rendered != want {
		t.Errorf("preview: got %q, want %q", rendered, want)
	}

	f.inboxTask(t, "Oldest")
	f.inboxTask(t, "Newest")
	rendered, err = f.proc.Preview(ctx, ptr("{{.Task.Title}}"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if want := "Oldest\n\n" + outputContract; rendered != want {
		t.Errorf("preview: got %q, want the oldest Inbox task", rendered)
	}

	if err := f.settings.Set(ctx, &model.AppSettings{InboxProcessing: model.InboxProcessingSettings{Prompt: "saved {{.Locale}}"}}); err != nil {
		t.Fatalf("save prompt: %v", err)
	}
	rendered, err = f.proc.Preview(ctx, nil)
	if err != nil {
		t.Fatalf("preview saved: %v", err)
	}
	if want := "saved en\n\n" + outputContract; rendered != want {
		t.Errorf("preview of the saved prompt: got %q, want %q", rendered, want)
	}

	rendered, err = f.proc.Preview(ctx, ptr(""))
	if err != nil {
		t.Fatalf("preview default: %v", err)
	}
	if rendered == "saved en\n\n"+outputContract {
		t.Error("an empty prompt must preview the built-in default, not the saved one")
	}
	if _, err := f.proc.Preview(ctx, ptr("{{.Nope}}")); !errors.Is(err, ErrTemplate) {
		t.Errorf("broken template: got %v, want ErrTemplate", err)
	}
	if err := f.proc.ValidatePrompt(ctx, "{{range .Projects}}"); !errors.Is(err, ErrTemplate) {
		t.Errorf("validate broken: got %v, want ErrTemplate", err)
	}
	if err := f.proc.ValidatePrompt(ctx, ""); err != nil {
		t.Errorf("validate empty: got %v, want the default to be valid", err)
	}
}

func TestProcessor_TriggerNowWhenDisabled(t *testing.T) {
	f := newProcFixture(t)
	f.proc.cfg.Enabled = false
	if f.proc.TriggerNow() {
		t.Error("trigger: got true for a disabled processor")
	}
}

func TestProcessor_RunLoopAnswersTrigger(t *testing.T) {
	f := newProcFixture(t)
	f.proc.cfg.Interval = time.Hour
	task := f.inboxTask(t, "loop")
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		f.proc.Run(ctx)
		close(stopped)
	}()

	waitFor(t, func() bool { return f.llm.callCount() == 1 })
	title := "loop edited"
	if _, err := f.tasks.Update(context.Background(), task.ID, repo.TaskUpdate{Title: &title}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !f.proc.TriggerNow() {
		t.Fatal("trigger: got false for an enabled processor")
	}
	waitFor(t, func() bool { return f.llm.callCount() == 2 })
	waitFor(t, func() bool { return !f.proc.Status().Running })

	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("run loop did not stop on cancel")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func labelIDsOf(labels []model.Label) []int64 {
	out := make([]int64, 0, len(labels))
	for _, l := range labels {
		out = append(out, l.ID)
	}
	return out
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func TestProcessor_PausedSkipsScheduledRunsButNotManualOnes(t *testing.T) {
	f := newProcFixture(t)
	ctx := context.Background()
	f.inboxTask(t, "waiting")
	if err := f.settings.Set(ctx, &model.AppSettings{InboxProcessing: model.InboxProcessingSettings{Paused: true}}); err != nil {
		t.Fatalf("pause: %v", err)
	}

	summary, ran := f.proc.RunOnce(ctx)
	if !ran || summary != (RunSummary{}) {
		t.Errorf("paused scheduled run: got %+v ran=%v, want nothing done", summary, ran)
	}
	if n := f.llm.callCount(); n != 0 {
		t.Fatalf("calls while paused: got %d, want 0", n)
	}
	if status := f.proc.Status(); status.LastRunAt != nil {
		t.Errorf("status: a paused tick must not count as a run, got %v", status.LastRunAt)
	}

	if summary, _ := f.proc.runOnce(ctx, true); summary.Kept != 1 {
		t.Errorf("manual run while paused: got %+v, want the task decided", summary)
	}

	if err := f.settings.Set(ctx, &model.AppSettings{}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	f.inboxTask(t, "after resume")
	if summary, _ := f.proc.RunOnce(ctx); summary.Kept != 1 {
		t.Errorf("scheduled run after resume: got %+v, want one kept", summary)
	}
}

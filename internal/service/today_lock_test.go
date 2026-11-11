package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// The fixture clock: 14:00 in Moscow on 2026-11-11, so the lock ends at the
// next Moscow midnight, 2026-11-11T21:00Z.
var (
	lockLoc      = mustLoadLocation("Europe/Moscow")
	lockNow      = time.Date(2026, 11, 11, 14, 0, 0, 0, lockLoc)
	lockEnd      = time.Date(2026, 11, 11, 21, 0, 0, 0, time.UTC)
	dueToday     = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	dueYesterday = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	dueTomorrow  = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
)

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

type lockFixture struct {
	lock         *service.TodayLockService
	taskSvc      *service.TaskService
	moveSvc      *service.MoveService
	complete     *service.CompleteService
	tasks        *repo.TaskRepo
	users        *repo.UserRepo
	ctxID        int64
	now          *time.Time
	projectsRepo *repo.ProjectRepo
}

func setupTodayLock(t *testing.T) *lockFixture {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	trelations := repo.NewTaskRelationsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, trelations)
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	ctxs := repo.NewContextRepo(d)
	c, err := ctxs.Create(context.Background(), "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	now := lockNow
	lock := service.NewTodayLockService(users, tasks, lockLoc).WithClock(func() time.Time { return now })
	auto := service.NewAutoLabelsService(repo.NewLabelRepo(d), repo.NewAppSettingsRepo(d))
	f := &lockFixture{
		lock:     lock,
		taskSvc:  service.NewTaskService(tasks, projects, tlabels, auto).WithTodayLock(lock),
		moveSvc:  service.NewMoveService(tasks, projects).WithTodayLock(lock),
		complete: service.NewCompleteService(tasks, projects, users, trelations).WithTodayLock(lock),
		tasks:    tasks,
		users:    users,
		ctxID:    c.ID,
		now:      &now,
	}
	f.projectsRepo = projects
	return f
}

func (f *lockFixture) project(t *testing.T, title string) int64 {
	t.Helper()
	p, err := f.projectsRepo.Create(context.Background(), repo.CreateProject{ContextID: f.ctxID, Title: title, Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p.ID
}

func (f *lockFixture) taskIn(t *testing.T, projectID int64, due *time.Time) *model.Task {
	t.Helper()
	cid := f.ctxID
	task, err := f.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ProjectID: &projectID},
		Title:     "t",
		DueAt:     due,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task
}

func (f *lockFixture) inboxTask(t *testing.T, due *time.Time) *model.Task {
	t.Helper()
	inbox := int64(1)
	task, err := f.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{InboxID: &inbox},
		Title:     "inbox",
		DueAt:     due,
	})
	if err != nil {
		t.Fatalf("create inbox task: %v", err)
	}
	return task
}

func (f *lockFixture) setStatus(t *testing.T, id int64, status model.TaskStatus) {
	t.Helper()
	if _, err := f.tasks.Update(context.Background(), id, repo.TaskUpdate{Status: &status}); err != nil {
		t.Fatalf("set status: %v", err)
	}
}

func (f *lockFixture) turnOn(t *testing.T) {
	t.Helper()
	if _, err := f.lock.Lock(context.Background()); err != nil {
		t.Fatalf("lock: %v", err)
	}
}

func (f *lockFixture) createIn(projectID int64, due *time.Time) error {
	cid := f.ctxID
	_, err := f.taskSvc.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ProjectID: &projectID},
		Title:     "new",
		DueAt:     due,
	}, nil, nil)
	return err
}

func assertTodayLocked(t *testing.T, err error, wantProject *int64) {
	t.Helper()
	var locked *service.TodayLockedError
	if !errors.As(err, &locked) {
		t.Fatalf("err: got %v, want TodayLockedError", err)
	}
	if !locked.Until.Equal(lockEnd) {
		t.Errorf("until: got %v, want %v", locked.Until, lockEnd)
	}
	if (locked.ProjectID == nil) != (wantProject == nil) ||
		(wantProject != nil && *locked.ProjectID != *wantProject) {
		t.Errorf("project: got %v, want %v", locked.ProjectID, wantProject)
	}
}

func ptrTime(v time.Time) *time.Time { return &v }

// --- the lock itself ---------------------------------------------------------

func TestTodayLock_LockEndsAtNextMidnightInServerZone(t *testing.T) {
	f := setupTodayLock(t)
	until, err := f.lock.Lock(context.Background())
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if !until.Equal(lockEnd) {
		t.Errorf("until: got %v, want %v", until, lockEnd)
	}
	active, ok, err := f.lock.ActiveUntil(context.Background())
	if err != nil || !ok {
		t.Fatalf("active: got %v %v, want running", ok, err)
	}
	if !active.Equal(lockEnd) {
		t.Errorf("active until: got %v, want %v", active, lockEnd)
	}
	s, _ := f.users.GetSettings(context.Background(), service.SingleUserID)
	if s.TodayLockUntil != "2026-11-11T21:00:00.000Z" {
		t.Errorf("stored: got %q, want %q", s.TodayLockUntil, "2026-11-11T21:00:00.000Z")
	}
}

func TestTodayLock_NotActiveBeforeFirstLock(t *testing.T) {
	f := setupTodayLock(t)
	if _, ok, err := f.lock.ActiveUntil(context.Background()); err != nil || ok {
		t.Errorf("active: got %v %v, want none", ok, err)
	}
}

func TestTodayLock_LockingAgainKeepsTheRunningLock(t *testing.T) {
	f := setupTodayLock(t)
	// A lock that runs past the end a fresh one would get: re-locking must not
	// pull it in.
	later := lockEnd.Add(3 * time.Hour)
	if _, err := f.users.LockTodayUntil(context.Background(), service.SingleUserID, later, lockNow); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	until, err := f.lock.Lock(context.Background())
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if !until.Equal(later) {
		t.Errorf("until: got %v, want the running %v", until, later)
	}
}

func TestTodayLock_ExpiresAtMidnightAndCanBeTurnedOnAgain(t *testing.T) {
	f := setupTodayLock(t)
	f.turnOn(t)
	*f.now = lockEnd
	if _, ok, _ := f.lock.ActiveUntil(context.Background()); ok {
		t.Fatal("lock still running at midnight")
	}
	*f.now = lockNow.Add(24 * time.Hour)
	until, err := f.lock.Lock(context.Background())
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if want := lockEnd.Add(24 * time.Hour); !until.Equal(want) {
		t.Errorf("until: got %v, want %v", until, want)
	}
}

func TestTodayLock_KeepsOtherSettingsAndSurvivesSettingsWrite(t *testing.T) {
	f := setupTodayLock(t)
	ctx := context.Background()
	s, _ := f.users.GetSettings(ctx, service.SingleUserID)
	s.BannerText = "focus"
	if err := f.users.SetSettings(ctx, service.SingleUserID, s); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	f.turnOn(t)
	got, _ := f.users.GetSettings(ctx, service.SingleUserID)
	if got.BannerText != "focus" {
		t.Errorf("banner: got %q, want %q", got.BannerText, "focus")
	}
	// The settings PATCH path reads, changes a field and writes the whole blob:
	// the lock must ride along untouched.
	got.Locale = "ru"
	if err := f.users.SetSettings(ctx, service.SingleUserID, got); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	if _, ok, _ := f.lock.ActiveUntil(ctx); !ok {
		t.Error("lock lost after a settings write")
	}
}

func TestTodayLock_UnreadableValueIsIgnored(t *testing.T) {
	f := setupTodayLock(t)
	ctx := context.Background()
	s, _ := f.users.GetSettings(ctx, service.SingleUserID)
	s.TodayLockUntil = "not a time"
	if err := f.users.SetSettings(ctx, service.SingleUserID, s); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	if _, ok, err := f.lock.ActiveUntil(ctx); err != nil || ok {
		t.Errorf("active: got %v %v, want none", ok, err)
	}
	// And locking replaces it with a real end.
	until, err := f.lock.Lock(ctx)
	if err != nil || !until.Equal(lockEnd) {
		t.Errorf("lock: got %v %v, want %v", until, err, lockEnd)
	}
}

// --- creating ----------------------------------------------------------------

func TestTodayLock_CreateWithoutLockIsFree(t *testing.T) {
	f := setupTodayLock(t)
	other := f.project(t, "other")
	if err := f.createIn(other, ptrTime(dueToday)); err != nil {
		t.Errorf("create: got %v, want nil", err)
	}
}

func TestTodayLock_CreateTodayInTodaysProjectIsAllowed(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	f.taskIn(t, focus, ptrTime(dueToday))
	f.turnOn(t)
	if err := f.createIn(focus, ptrTime(dueToday)); err != nil {
		t.Errorf("create: got %v, want nil", err)
	}
}

func TestTodayLock_CreateTodayInOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	other := f.project(t, "other")
	f.taskIn(t, focus, ptrTime(dueToday))
	f.turnOn(t)
	assertTodayLocked(t, f.createIn(other, ptrTime(dueToday)), &other)
	// Nothing was written.
	ids, _ := f.tasks.TodayProjectIDs(context.Background(), lockEnd, 0)
	if len(ids) != 1 || ids[0] != focus {
		t.Errorf("today projects: got %v, want [%d]", ids, focus)
	}
}

func TestTodayLock_CreateOverdueInOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLock(t)
	other := f.project(t, "other")
	f.turnOn(t)
	assertTodayLocked(t, f.createIn(other, ptrTime(dueYesterday)), &other)
}

func TestTodayLock_CreateForAnotherDayOrUndatedIsAllowed(t *testing.T) {
	f := setupTodayLock(t)
	other := f.project(t, "other")
	f.turnOn(t)
	if err := f.createIn(other, ptrTime(dueTomorrow)); err != nil {
		t.Errorf("create tomorrow: got %v, want nil", err)
	}
	if err := f.createIn(other, nil); err != nil {
		t.Errorf("create undated: got %v, want nil", err)
	}
	// Exactly at midnight is already tomorrow.
	if err := f.createIn(other, ptrTime(lockEnd)); err != nil {
		t.Errorf("create at midnight: got %v, want nil", err)
	}
}

func TestTodayLock_CreateTodayInInboxIsRefused(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	f.taskIn(t, focus, ptrTime(dueToday))
	f.inboxTask(t, ptrTime(dueToday)) // an inbox task on Today does not open the inbox
	f.turnOn(t)
	inbox := int64(1)
	_, err := f.taskSvc.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{InboxID: &inbox},
		Title:     "capture",
		DueAt:     ptrTime(dueToday),
	}, nil, nil)
	assertTodayLocked(t, err, nil)
	// Capturing into the inbox without a date stays possible.
	if _, err := f.taskSvc.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{InboxID: &inbox},
		Title:     "capture",
	}, nil, nil); err != nil {
		t.Errorf("undated inbox create: got %v, want nil", err)
	}
}

func TestTodayLock_ProjectWithOnlyOverdueWorkIsOnToday(t *testing.T) {
	f := setupTodayLock(t)
	late := f.project(t, "late")
	f.taskIn(t, late, ptrTime(dueYesterday))
	f.turnOn(t)
	if err := f.createIn(late, ptrTime(dueToday)); err != nil {
		t.Errorf("create: got %v, want nil", err)
	}
}

func TestTodayLock_ProjectDropsOutOnceItsTodayWorkIsDone(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	only := f.taskIn(t, focus, ptrTime(dueToday))
	f.turnOn(t)
	f.setStatus(t, only.ID, model.TaskStatusCompleted)
	assertTodayLocked(t, f.createIn(focus, ptrTime(dueToday)), &focus)
}

func TestTodayLock_CancelledWorkDoesNotKeepAProjectOpen(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	gone := f.taskIn(t, focus, ptrTime(dueToday))
	f.setStatus(t, gone.ID, model.TaskStatusCancelled)
	f.turnOn(t)
	assertTodayLocked(t, f.createIn(focus, ptrTime(dueToday)), &focus)
}

func TestTodayLock_ProjectWithTodayWorkAddedLaterIsNotOpened(t *testing.T) {
	f := setupTodayLock(t)
	other := f.project(t, "other")
	f.turnOn(t)
	// Work for tomorrow is fine, and it does not open the project for today.
	if err := f.createIn(other, ptrTime(dueTomorrow)); err != nil {
		t.Fatalf("create tomorrow: %v", err)
	}
	assertTodayLocked(t, f.createIn(other, ptrTime(dueToday)), &other)
}

func TestTodayLock_ExpiredLockRefusesNothing(t *testing.T) {
	f := setupTodayLock(t)
	other := f.project(t, "other")
	f.turnOn(t)
	*f.now = lockEnd.Add(time.Minute)
	if err := f.createIn(other, ptrTime(lockEnd.Add(time.Hour))); err != nil {
		t.Errorf("create after expiry: got %v, want nil", err)
	}
}

// --- editing -----------------------------------------------------------------

func TestTodayLock_CheckEditingATaskAlreadyOnTodayPasses(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	task := f.taskIn(t, focus, ptrTime(dueToday))
	f.turnOn(t)
	after := service.PlacementOf(task)
	after.DueAt = ptrTime(dueToday.Add(5 * time.Hour))
	if err := f.taskSvc.CheckTodayLock(context.Background(), task, after); err != nil {
		t.Errorf("reschedule within today: got %v, want nil", err)
	}
}

func TestTodayLock_CheckEditingAnInboxTaskAlreadyOnTodayPasses(t *testing.T) {
	f := setupTodayLock(t)
	task := f.inboxTask(t, ptrTime(dueToday))
	f.turnOn(t)
	if err := f.taskSvc.CheckTodayLock(context.Background(), task, service.PlacementOf(task)); err != nil {
		t.Errorf("edit: got %v, want nil", err)
	}
}

func TestTodayLock_CheckSchedulingForTodayFromAnotherProjectIsRefused(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	other := f.project(t, "other")
	f.taskIn(t, focus, ptrTime(dueToday))
	task := f.taskIn(t, other, ptrTime(dueTomorrow))
	f.turnOn(t)
	after := service.PlacementOf(task)
	after.DueAt = ptrTime(dueToday)
	assertTodayLocked(t, f.taskSvc.CheckTodayLock(context.Background(), task, after), &other)

	undated := f.taskIn(t, other, nil)
	after = service.PlacementOf(undated)
	after.DueAt = ptrTime(dueToday)
	assertTodayLocked(t, f.taskSvc.CheckTodayLock(context.Background(), undated, after), &other)
}

func TestTodayLock_CheckSchedulingForTodayInTodaysProjectPasses(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	f.taskIn(t, focus, ptrTime(dueToday))
	task := f.taskIn(t, focus, nil)
	f.turnOn(t)
	after := service.PlacementOf(task)
	after.DueAt = ptrTime(dueToday)
	if err := f.taskSvc.CheckTodayLock(context.Background(), task, after); err != nil {
		t.Errorf("schedule: got %v, want nil", err)
	}
}

func TestTodayLock_CheckMovingWorkOffTodayPasses(t *testing.T) {
	f := setupTodayLock(t)
	other := f.project(t, "other")
	f.turnOn(t)
	task := f.taskIn(t, other, nil)
	after := service.PlacementOf(task)
	after.DueAt = ptrTime(dueTomorrow)
	if err := f.taskSvc.CheckTodayLock(context.Background(), task, after); err != nil {
		t.Errorf("schedule tomorrow: got %v, want nil", err)
	}
}

func TestTodayLock_NilServiceChecksNothing(t *testing.T) {
	var lock *service.TodayLockService
	err := lock.Check(context.Background(), nil, service.TodayPlacement{DueAt: ptrTime(dueToday), Status: model.TaskStatusOpen})
	if err != nil {
		t.Errorf("check: got %v, want nil", err)
	}
}

// --- moving ------------------------------------------------------------------

func TestTodayLock_MoveTodayTaskIntoOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	other := f.project(t, "other")
	task := f.taskIn(t, focus, ptrTime(dueToday))
	f.turnOn(t)
	cid := f.ctxID
	_, err := f.moveSvc.Move(context.Background(), task.ID, repo.Placement{ContextID: &cid, ProjectID: &other})
	assertTodayLocked(t, err, &other)
	got, _ := f.tasks.Get(context.Background(), task.ID)
	if got.ProjectID == nil || *got.ProjectID != focus {
		t.Errorf("project after refused move: got %v, want %d", got.ProjectID, focus)
	}
}

func TestTodayLock_MoveTodayTaskBetweenTodaysProjectsIsAllowed(t *testing.T) {
	f := setupTodayLock(t)
	a := f.project(t, "a")
	b := f.project(t, "b")
	task := f.taskIn(t, a, ptrTime(dueToday))
	f.taskIn(t, b, ptrTime(dueToday))
	f.turnOn(t)
	cid := f.ctxID
	if _, err := f.moveSvc.Move(context.Background(), task.ID, repo.Placement{ContextID: &cid, ProjectID: &b}); err != nil {
		t.Errorf("move: got %v, want nil", err)
	}
}

func TestTodayLock_MoveTodayInboxTaskIntoOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLock(t)
	other := f.project(t, "other")
	task := f.inboxTask(t, ptrTime(dueToday))
	f.turnOn(t)
	cid := f.ctxID
	_, err := f.moveSvc.Move(context.Background(), task.ID, repo.Placement{ContextID: &cid, ProjectID: &other})
	assertTodayLocked(t, err, &other)
}

func TestTodayLock_MoveUndatedTaskAnywhereIsAllowed(t *testing.T) {
	f := setupTodayLock(t)
	a := f.project(t, "a")
	other := f.project(t, "other")
	task := f.taskIn(t, a, nil)
	f.turnOn(t)
	cid := f.ctxID
	if _, err := f.moveSvc.Move(context.Background(), task.ID, repo.Placement{ContextID: &cid, ProjectID: &other}); err != nil {
		t.Errorf("move: got %v, want nil", err)
	}
	inbox := int64(1)
	if _, err := f.moveSvc.Move(context.Background(), task.ID, repo.Placement{InboxID: &inbox}); err != nil {
		t.Errorf("move to inbox: got %v, want nil", err)
	}
}

// --- reopening ---------------------------------------------------------------

func TestTodayLock_UncompleteTodayTaskOfOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLock(t)
	other := f.project(t, "other")
	task := f.taskIn(t, other, ptrTime(dueToday))
	f.setStatus(t, task.ID, model.TaskStatusCompleted)
	f.turnOn(t)
	_, err := f.complete.Uncomplete(context.Background(), task.ID)
	assertTodayLocked(t, err, &other)
	got, _ := f.tasks.Get(context.Background(), task.ID)
	if got.Status != model.TaskStatusCompleted {
		t.Errorf("status after refused reopen: got %s, want completed", got.Status)
	}
}

func TestTodayLock_UncompleteInTodaysProjectIsAllowed(t *testing.T) {
	f := setupTodayLock(t)
	focus := f.project(t, "focus")
	f.taskIn(t, focus, ptrTime(dueToday))
	task := f.taskIn(t, focus, ptrTime(dueToday))
	f.setStatus(t, task.ID, model.TaskStatusCompleted)
	f.turnOn(t)
	if _, err := f.complete.Uncomplete(context.Background(), task.ID); err != nil {
		t.Errorf("uncomplete: got %v, want nil", err)
	}
}

func TestTodayLock_UncompleteTaskDueAnotherDayIsAllowed(t *testing.T) {
	f := setupTodayLock(t)
	other := f.project(t, "other")
	task := f.taskIn(t, other, ptrTime(dueTomorrow))
	f.setStatus(t, task.ID, model.TaskStatusCompleted)
	f.turnOn(t)
	if _, err := f.complete.Uncomplete(context.Background(), task.ID); err != nil {
		t.Errorf("uncomplete: got %v, want nil", err)
	}
}

// --- the repository query ----------------------------------------------------

func TestTaskRepo_TodayProjectIDs_ExcludesTheNamedTask(t *testing.T) {
	f := setupTodayLock(t)
	a := f.project(t, "a")
	b := f.project(t, "b")
	onlyA := f.taskIn(t, a, ptrTime(dueToday))
	f.taskIn(t, b, ptrTime(dueYesterday))
	f.taskIn(t, b, ptrTime(dueTomorrow))
	ids, err := f.tasks.TodayProjectIDs(context.Background(), lockEnd, 0)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(ids) != 2 || ids[0] != a || ids[1] != b {
		t.Errorf("all: got %v, want [%d %d]", ids, a, b)
	}
	ids, _ = f.tasks.TodayProjectIDs(context.Background(), lockEnd, onlyA.ID)
	if len(ids) != 1 || ids[0] != b {
		t.Errorf("without task: got %v, want [%d]", ids, b)
	}
}

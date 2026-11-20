package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TodayLockedError reports a write the Today focus lock refused: it would have
// put an open task on Today in a project that has no open task there already, or
// finished work of a project that is not part of the day at all. ProjectID is
// the project the task belongs to; nil means the inbox or a bare context, which
// the lock never lets in.
type TodayLockedError struct {
	Until     time.Time
	ProjectID *int64
}

func (e *TodayLockedError) Error() string {
	return "today_locked: Today is locked to its projects until " + model.FormatUTC(e.Until)
}

// TodayPlacement is where a task stands after a write, as far as the focus lock
// cares. TaskID is 0 for a task that does not exist yet.
type TodayPlacement struct {
	TaskID    int64
	ProjectID *int64
	DueAt     *time.Time
	Status    model.TaskStatus
}

// PlacementOf reads the lock-relevant fields off an existing task.
func PlacementOf(t *model.Task) TodayPlacement {
	return TodayPlacement{TaskID: t.ID, ProjectID: t.ProjectID, DueAt: t.DueAt, Status: t.Status}
}

// TodayLockService owns the Today focus lock.
//
// Turned on from the Today screen, the lock lasts until the next midnight in the
// server timezone and cannot be lifted early. While it runs, only the projects
// that have an open task on Today (due today or overdue) may have work taken
// into the day. The set is read live on every check, so finishing the last task
// of a project for today closes that project for the rest of the day. Adding work
// to any project for another day, or with no date, stays allowed.
//
// The lock also decides what may be finished: only the work of those projects can
// be ticked off, since a task finished elsewhere is work done outside the day the
// user planned. A task already on Today always qualifies by itself — that is what
// lets the last task of a project be completed. Cancelling, deleting and taking
// work off Today stay allowed.
//
// The week is the one exception for placement: an open task already on the Week
// screen — planned for the week, or dated another day of the current week — may be
// taken into today whatever its project. It is the task that qualifies, not its
// project: the project does not open for other work, and the task has to be taken
// into the day before it can be finished.
//
// Every write that can put a task on Today asks Check before it commits:
// TaskService.Create (and everything built on it: subtasks, duplicates, splits,
// groups, templates), MoveService.Move, CompleteService.Uncomplete, the task
// PATCH handler and the inbox processor. Every completion asks CheckCompletion
// first. The advance a repeating task makes to its next run is the schedule the
// user already chose and is not checked; ticking the task off still is.
type TodayLockService struct {
	users *repo.UserRepo
	tasks *repo.TaskRepo
	loc   *time.Location
	now   func() time.Time
}

// NewTodayLockService constructs the service; loc is the server timezone that
// decides where the day ends.
func NewTodayLockService(users *repo.UserRepo, tasks *repo.TaskRepo, loc *time.Location) *TodayLockService {
	if loc == nil {
		loc = time.UTC
	}
	return &TodayLockService{users: users, tasks: tasks, loc: loc, now: time.Now}
}

// WithClock replaces the clock, for tests. Returns the service for chaining.
func (s *TodayLockService) WithClock(now func() time.Time) *TodayLockService {
	s.now = now
	return s
}

// Lock starts the lock until the next midnight. Locking again while a lock runs
// keeps the running one. Returns the moment the lock ends.
func (s *TodayLockService) Lock(ctx context.Context) (time.Time, error) {
	const op = "service.TodayLockService.Lock"
	log := logging.FromContext(ctx)
	now := s.now()
	_, end := dayBounds(now, s.loc)
	until, err := s.users.LockTodayUntil(ctx, SingleUserID, end.UTC(), now)
	if err != nil {
		logRepoErr(ctx, op+": lock", err)
		return time.Time{}, err
	}
	log.InfoContext(ctx, "today lock turned on", slog.String("op", op), slog.String("until", model.FormatUTC(until)))
	return until, nil
}

// ActiveUntil returns the end of the running lock, and false when none runs.
func (s *TodayLockService) ActiveUntil(ctx context.Context) (time.Time, bool, error) {
	settings, err := s.users.GetSettings(ctx, SingleUserID)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("today lock: load settings: %w", err)
	}
	if settings.TodayLockUntil == "" {
		return time.Time{}, false, nil
	}
	until, err := model.ParseUTC(settings.TodayLockUntil)
	if err != nil {
		// An unreadable value cannot say when it ends, and a lock that cannot
		// end is worse than none.
		logging.FromContext(ctx).WarnContext(ctx, "today lock value is unreadable and is ignored",
			slog.String("op", "service.TodayLockService.ActiveUntil"), slog.String("value", settings.TodayLockUntil))
		return time.Time{}, false, nil
	}
	if !s.now().Before(until) {
		return time.Time{}, false, nil
	}
	return until, true, nil
}

// AllowedProjects returns the projects the running lock lets work for today into:
// those with an open task due today or overdue plus the ones marked as always
// allowed. active is false, with no projects, when no lock runs. It is the set
// Check reads, so a client can refuse a draft before sending it.
func (s *TodayLockService) AllowedProjects(ctx context.Context) (until time.Time, active bool, ids []int64, err error) {
	until, active, err = s.ActiveUntil(ctx)
	if err != nil || !active {
		return time.Time{}, false, []int64{}, err
	}
	ids, err = s.tasks.TodayProjectIDs(ctx, until, 0)
	if err != nil {
		return time.Time{}, false, []int64{}, err
	}
	return until, true, ids, nil
}

// Check refuses a write that would put a task on Today in a project the lock
// keeps out. before is the task as it was (nil for a new one); after is where
// the write leaves it. A task that already stood on Today in the same project
// passes unchanged — editing it, or moving it to another hour, takes nothing new
// into the day. A nil service checks nothing.
func (s *TodayLockService) Check(ctx context.Context, before *model.Task, after TodayPlacement) error {
	const (
		op      = "service.TodayLockService.Check"
		refusal = "today lock refused to take a task into today"
	)
	if s == nil || after.Status != model.TaskStatusOpen || after.DueAt == nil {
		return nil
	}
	until, active, err := s.ActiveUntil(ctx)
	if err != nil || !active {
		return err
	}
	if !after.DueAt.Before(until) {
		return nil
	}
	if before != nil && onToday(before.Status, before.DueAt, until) && sameProject(before.ProjectID, after.ProjectID) {
		return nil
	}
	if before != nil && s.onWeek(before, until) {
		return nil
	}
	refused := &TodayLockedError{Until: until, ProjectID: after.ProjectID}
	if after.ProjectID == nil {
		s.logRefusal(ctx, op, refusal, after.TaskID, refused)
		return refused
	}
	allowed, err := s.tasks.TodayProjectIDs(ctx, until, after.TaskID)
	if err != nil {
		return err
	}
	if slices.Contains(allowed, *after.ProjectID) {
		return nil
	}
	s.logRefusal(ctx, op, refusal, after.TaskID, refused)
	return refused
}

// CheckCompletion refuses finishing a task the lock keeps out of the day: while
// the lock runs, the day belongs to the projects already part of it, and ticking
// off the work of another project is doing work the day was meant to exclude.
//
// The task itself is part of Today when it is open and due today or overdue, and
// that alone opens its project — which is why the last task of a project can
// still be completed, after which the project is closed for the rest of the day.
// A task with no project (the inbox, a bare context) is never part of the day.
//
// A task that is already closed is not checked: completing twice is idempotent,
// and the repeat must not fail on a project the first completion emptied.
func (s *TodayLockService) CheckCompletion(ctx context.Context, t *model.Task) error {
	const op = "service.TodayLockService.CheckCompletion"
	if s == nil || t == nil || t.Status != model.TaskStatusOpen {
		return nil
	}
	until, active, err := s.ActiveUntil(ctx)
	if err != nil || !active {
		return err
	}
	if t.ProjectID != nil {
		allowed, err := s.tasks.TodayProjectIDs(ctx, until, 0)
		if err != nil {
			return err
		}
		if slices.Contains(allowed, *t.ProjectID) {
			return nil
		}
	}
	refused := &TodayLockedError{Until: until, ProjectID: t.ProjectID}
	s.logRefusal(ctx, op, "today lock refused to complete work outside today's projects", t.ID, refused)
	return refused
}

func (s *TodayLockService) logRefusal(ctx context.Context, op, msg string, taskID int64, e *TodayLockedError) {
	attrs := []any{
		slog.String("op", op),
		slog.Int64("task_id", taskID),
		slog.String("until", model.FormatUTC(e.Until)),
	}
	if e.ProjectID != nil {
		attrs = append(attrs, slog.Int64("project_id", *e.ProjectID))
	}
	logging.FromContext(ctx).WarnContext(ctx, msg, attrs...)
}

// onWeek reports whether a task waits on the Week screen: open and either
// planned for the week or dated a later day of the current Monday-to-Monday
// week of the server timezone. until is the end of today; a task dated today or
// earlier is already on Today and is judged by its project.
func (s *TodayLockService) onWeek(t *model.Task, until time.Time) bool {
	if t.Status != model.TaskStatusOpen {
		return false
	}
	if t.PlanState == model.PlanStateWeek {
		return true
	}
	if t.DueAt == nil {
		return false
	}
	_, end := weekBounds(s.now(), s.loc)
	return !t.DueAt.Before(until) && t.DueAt.Before(end)
}

// weekBounds returns [Monday 00:00, next Monday 00:00) in loc for the week
// containing t.
func weekBounds(t time.Time, loc *time.Location) (time.Time, time.Time) {
	dayStart, _ := dayBounds(t, loc)
	daysFromMonday := (int(dayStart.Weekday()) + 6) % 7
	start := time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day()-daysFromMonday, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 0, 7)
}

func onToday(status model.TaskStatus, due *time.Time, until time.Time) bool {
	return status == model.TaskStatusOpen && due != nil && due.Before(until)
}

func sameProject(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

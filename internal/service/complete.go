package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	rrule "github.com/teambition/rrule-go"
)

// dayBounds returns midnight in `loc` for the calendar day containing `t`,
// plus the midnight 24h later. Used to scope "snapshot already exists today"
// idempotency checks to the user's clock.
func dayBounds(t time.Time, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.UTC
	}
	local := t.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return start, start.Add(24 * time.Hour)
}

// RecurrenceError wraps RRULE parse or compute failures.
type RecurrenceError struct{ Err error }

func (e *RecurrenceError) Error() string { return "recurrence_invalid: " + e.Err.Error() }
func (e *RecurrenceError) Unwrap() error { return e.Err }

// CompleteService handles task completion, including recurring task advancement.
// Troiki capacity grants are derived from the parent project's category, so the
// service depends on ProjectRepo to look up the project on each completion.
type CompleteService struct {
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
	users    *repo.UserRepo
	loc      *time.Location
}

func NewCompleteService(tasks *repo.TaskRepo, projects *repo.ProjectRepo, users *repo.UserRepo) *CompleteService {
	return &CompleteService{tasks: tasks, projects: projects, users: users, loc: time.UTC}
}

// NewCompleteServiceWithLoc constructs a CompleteService anchored to a specific
// timezone for RRULE evaluation (so e.g. daily 9 AM rules align with the user's clock).
func NewCompleteServiceWithLoc(tasks *repo.TaskRepo, projects *repo.ProjectRepo, users *repo.UserRepo, loc *time.Location) *CompleteService {
	if loc == nil {
		loc = time.UTC
	}
	return &CompleteService{tasks: tasks, projects: projects, users: users, loc: loc}
}

func (s *CompleteService) Complete(ctx context.Context, taskID int64) (*model.Task, error) {
	return s.completeAt(ctx, taskID, nil)
}

// CompleteAt marks a task completed with an explicit completion timestamp.
// Recurring tasks ignore the override (recurrence advance is always anchored to
// the current moment) — pass a nil time on those.
func (s *CompleteService) CompleteAt(ctx context.Context, taskID int64, completedAt time.Time) (*model.Task, error) {
	return s.completeAt(ctx, taskID, &completedAt)
}

func (s *CompleteService) completeAt(ctx context.Context, taskID int64, completedAt *time.Time) (*model.Task, error) {
	const op = "service.CompleteService.Complete"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		logRepoErr(ctx, op+": get task", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	if t.RecurrenceRule != nil {
		return s.advanceRecurring(ctx, t, completedAt)
	}
	// Always attempt the capacity bump even when the task is already completed:
	// if a previous Complete crashed between Update and bumpTroikiCapacity, the
	// task sits completed with an unset grant flag, and only a retry can recover
	// the lost grant. The flag-flip inside bumpTroikiCapacity is idempotent, so
	// it's a no-op when capacity was already granted.
	if t.Status != model.TaskStatusCompleted {
		status := model.TaskStatusCompleted
		updated, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status, CompletedAt: completedAt})
		if err != nil {
			logRepoErr(ctx, op+": update status", err, slog.Int64("task_id", taskID))
			return nil, err
		}
		t = updated
	}
	if err := s.bumpTroikiCapacity(ctx, t); err != nil {
		log.ErrorContext(ctx, op+": bump troiki capacity", slog.Int64("task_id", taskID), slog.String("err", err.Error()))
		return nil, err
	}
	log.InfoContext(ctx, "task completed", slog.String("op", op), slog.Int64("task_id", taskID))
	return t, nil
}

func (s *CompleteService) advanceRecurring(ctx context.Context, t *model.Task, completedAt *time.Time) (*model.Task, error) {
	const op = "service.CompleteService.advanceRecurring"
	log := logging.FromContext(ctx)
<<<<<<< HEAD

	// Idempotency: if we already snapped a completion for this recurring task
	// today (in the configured location), a second Complete call is a no-op —
	// otherwise rapid double-clicks, retries, or any duplicate request would
	// keep advancing the parent and snapping new rows, surfacing as duplicate
	// entries on the "completed yesterday" view.
	completionTS := time.Now()
	if completedAt != nil {
		completionTS = *completedAt
	}
	dayStart, dayEnd := dayBounds(completionTS, s.loc)
	if has, err := s.tasks.HasRecurrenceCompletionOnDay(ctx, t.ID, dayStart, dayEnd); err != nil {
		logRepoErr(ctx, op+": check recurrence completion", err, slog.Int64("task_id", t.ID))
		return nil, err
	} else if has {
		log.DebugContext(ctx, op+": skip duplicate", slog.Int64("task_id", t.ID))
		return t, nil
	}

=======
>>>>>>> 049dc85 (v1.6.0 (#32))
	r, err := rrule.StrToRRule(*t.RecurrenceRule)
	if err != nil {
		log.WarnContext(ctx, op+": invalid RRULE", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
		return nil, &RecurrenceError{Err: err}
	}

	// Base: current due_at if in the future, otherwise now. Anchor to the
	// configured location so RRULE BYHOUR/BYDAY semantics follow the user's clock.
	base := time.Now().In(s.loc)
	if t.DueAt != nil && t.DueAt.After(base) {
		base = t.DueAt.In(s.loc)
	}
	r.DTStart(base)

	next := r.After(base, false)

	planNone := model.PlanStateNone
	upd := repo.TaskUpdate{PlanState: &planNone}
	terminal := next.IsZero()
	if terminal {
		status := model.TaskStatusCompleted
		upd.Status = &status
		upd.CompletedAt = completedAt
	} else {
		upd.DueAt = &next
		upd.ResetPostponeCount = true
	}
	updated, err := s.tasks.Update(ctx, t.ID, upd)
	if err != nil {
		logRepoErr(ctx, op+": update task", err, slog.Int64("task_id", t.ID))
		return nil, err
	}
	// For non-terminal completions the parent task stays open (advanced to the
	// next occurrence), so the completed view would never show this run. Snap
	// off a completed history row so the user can see what they got done.
	if !terminal {
<<<<<<< HEAD
		if _, err := s.tasks.CreateRecurrenceCompletion(ctx, t, completionTS); err != nil {
=======
		ts := time.Now()
		if completedAt != nil {
			ts = *completedAt
		}
		if _, err := s.tasks.CreateRecurrenceCompletion(ctx, t, ts); err != nil {
>>>>>>> 049dc85 (v1.6.0 (#32))
			log.ErrorContext(ctx, op+": create recurrence completion", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
			return nil, err
		}
	}
	if terminal {
		if err := s.bumpTroikiCapacity(ctx, t); err != nil {
			log.ErrorContext(ctx, op+": bump troiki capacity", slog.Int64("task_id", t.ID), slog.String("err", err.Error()))
			return nil, err
		}
	}
	log.InfoContext(ctx, "recurring task advanced", slog.String("op", op), slog.Int64("task_id", t.ID), slog.Bool("terminal", terminal))
	return updated, nil
}

// bumpTroikiCapacity grants +1 capacity to the next-tier slot when a task is
// completed inside a categorised project: important → +medium, medium → +rest.
// Rest and uncategorised projects (or tasks outside any project) have no
// effect. The grant flag on the task makes the operation idempotent across
// uncomplete/recomplete cycles — capacity is granted only once per
// (task, project-category-assignment) until the project's category is
// cleared or changed.
func (s *CompleteService) bumpTroikiCapacity(ctx context.Context, t *model.Task) error {
	if t == nil || t.ProjectID == nil || s.projects == nil || s.users == nil {
		return nil
	}
	p, err := s.projects.Get(ctx, *t.ProjectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil
		}
		return err
	}
	if p.TroikiCategory == nil {
		return nil
	}
	var col string
	switch *p.TroikiCategory {
	case model.TroikiCategoryImportant:
		col = "troiki_medium_capacity"
	case model.TroikiCategoryMedium:
		col = "troiki_rest_capacity"
	default:
		return nil
	}
	// Single transaction: flag flip + counter bump must succeed or both roll back.
	// Otherwise a failure between them strands the grant — the flag blocks retries.
	_, err = s.tasks.GrantAndBumpTroikiCapacity(ctx, t.ID, SingleUserID, col)
	return err
}

// Uncomplete reopens a completed/cancelled task. Project-level Troiki
// categorisation means reopening a task does not affect any slot capacity, so
// no slot guard is needed here. If the parent project carries a category, the
// task's priority is re-pinned to the category-derived priority — without this,
// a task completed before the category was assigned (or moved into a
// categorised project while completed) would come back open with a stale
// priority that the frontend then locks against edits.
//
// The status transition and priority pin are performed in a single SQL
// statement that reads projects.troiki_category atomically with the UPDATE,
// eliminating a race with a concurrent SetCategory that would otherwise let
// the task come back open with a priority derived from the project's previous
// category.
func (s *CompleteService) Uncomplete(ctx context.Context, taskID int64) (*model.Task, error) {
	const op = "service.CompleteService.Uncomplete"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		logRepoErr(ctx, op+": get task", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	if t.Status == model.TaskStatusOpen {
		return t, nil
	}
	reopened, err := s.tasks.ReopenAndPinProjectPriority(ctx, taskID)
	if err != nil {
		logRepoErr(ctx, op+": reopen", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	log.InfoContext(ctx, "task reopened", slog.String("op", op), slog.Int64("task_id", taskID))
	return reopened, nil
}

// Cancel marks a task cancelled. With project-owned Troiki categories, cancelling
// a single task does not release any slot — the project keeps its category until
// the user explicitly clears it.
func (s *CompleteService) Cancel(ctx context.Context, taskID int64) (*model.Task, error) {
	const op = "service.CompleteService.Cancel"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))
	status := model.TaskStatusCancelled
	updated, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
	if err != nil {
		logRepoErr(ctx, op+": update", err, slog.Int64("task_id", taskID))
		return nil, err
	}
	log.InfoContext(ctx, "task cancelled", slog.String("op", op), slog.Int64("task_id", taskID))
	return updated, nil
}

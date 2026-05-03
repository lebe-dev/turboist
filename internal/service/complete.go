package service

import (
	"context"
	"errors"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	rrule "github.com/teambition/rrule-go"
)

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
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if t.RecurrenceRule != nil {
		return s.advanceRecurring(ctx, t)
	}
	// Always attempt the capacity bump even when the task is already completed:
	// if a previous Complete crashed between Update and bumpTroikiCapacity, the
	// task sits completed with an unset grant flag, and only a retry can recover
	// the lost grant. The flag-flip inside bumpTroikiCapacity is idempotent, so
	// it's a no-op when capacity was already granted.
	if t.Status != model.TaskStatusCompleted {
		status := model.TaskStatusCompleted
		updated, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
		if err != nil {
			return nil, err
		}
		t = updated
	}
	if err := s.bumpTroikiCapacity(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *CompleteService) advanceRecurring(ctx context.Context, t *model.Task) (*model.Task, error) {
	r, err := rrule.StrToRRule(*t.RecurrenceRule)
	if err != nil {
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
	dayNone := model.DayPartNone
	upd := repo.TaskUpdate{PlanState: &planNone, DayPart: &dayNone}
	terminal := next.IsZero()
	if terminal {
		status := model.TaskStatusCompleted
		upd.Status = &status
	} else {
		upd.DueAt = &next
	}
	updated, err := s.tasks.Update(ctx, t.ID, upd)
	if err != nil {
		return nil, err
	}
	if terminal {
		if err := s.bumpTroikiCapacity(ctx, t); err != nil {
			return nil, err
		}
	}
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
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if t.Status == model.TaskStatusOpen {
		return t, nil
	}
	return s.tasks.ReopenAndPinProjectPriority(ctx, taskID)
}

// Cancel marks a task cancelled. With project-owned Troiki categories, cancelling
// a single task does not release any slot — the project keeps its category until
// the user explicitly clears it.
func (s *CompleteService) Cancel(ctx context.Context, taskID int64) (*model.Task, error) {
	status := model.TaskStatusCancelled
	return s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
}

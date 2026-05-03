package service

import (
	"context"
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
type CompleteService struct {
	tasks *repo.TaskRepo
	users *repo.UserRepo
	loc   *time.Location
}

func NewCompleteService(tasks *repo.TaskRepo, users *repo.UserRepo) *CompleteService {
	return &CompleteService{tasks: tasks, users: users, loc: time.UTC}
}

// NewCompleteServiceWithLoc constructs a CompleteService anchored to a specific
// timezone for RRULE evaluation (so e.g. daily 9 AM rules align with the user's clock).
func NewCompleteServiceWithLoc(tasks *repo.TaskRepo, users *repo.UserRepo, loc *time.Location) *CompleteService {
	if loc == nil {
		loc = time.UTC
	}
	return &CompleteService{tasks: tasks, users: users, loc: loc}
}

func (s *CompleteService) Complete(ctx context.Context, taskID int64) (*model.Task, error) {
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	// Already-completed non-recurring tasks are a no-op — re-completing must not
	// re-grant Troiki capacity.
	if t.RecurrenceRule == nil && t.Status == model.TaskStatusCompleted {
		return t, nil
	}
	if t.RecurrenceRule == nil {
		status := model.TaskStatusCompleted
		updated, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
		if err != nil {
			return nil, err
		}
		if err := s.bumpTroikiCapacity(ctx, t.TroikiCategory); err != nil {
			return nil, err
		}
		return updated, nil
	}
	return s.advanceRecurring(ctx, t)
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
		if err := s.bumpTroikiCapacity(ctx, t.TroikiCategory); err != nil {
			return nil, err
		}
	}
	return updated, nil
}

// bumpTroikiCapacity grants +1 capacity to the next-tier slot when a categorised
// task is completed: important → +medium, medium → +rest. Rest and uncategorised
// completions have no effect.
func (s *CompleteService) bumpTroikiCapacity(ctx context.Context, cat *model.TroikiCategory) error {
	if cat == nil || s.users == nil {
		return nil
	}
	switch *cat {
	case model.TroikiCategoryImportant:
		return s.users.IncTroikiCapacity(ctx, SingleUserID, model.TroikiCategoryMedium)
	case model.TroikiCategoryMedium:
		return s.users.IncTroikiCapacity(ctx, SingleUserID, model.TroikiCategoryRest)
	}
	return nil
}

func (s *CompleteService) Uncomplete(ctx context.Context, taskID int64) (*model.Task, error) {
	status := model.TaskStatusOpen
	return s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
}

func (s *CompleteService) Cancel(ctx context.Context, taskID int64) (*model.Task, error) {
	status := model.TaskStatusCancelled
	return s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
}

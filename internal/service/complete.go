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
	loc   *time.Location
}

func NewCompleteService(tasks *repo.TaskRepo) *CompleteService {
	return &CompleteService{tasks: tasks, loc: time.UTC}
}

// NewCompleteServiceWithLoc constructs a CompleteService anchored to a specific
// timezone for RRULE evaluation (so e.g. daily 9 AM rules align with the user's clock).
func NewCompleteServiceWithLoc(tasks *repo.TaskRepo, loc *time.Location) *CompleteService {
	if loc == nil {
		loc = time.UTC
	}
	return &CompleteService{tasks: tasks, loc: loc}
}

func (s *CompleteService) Complete(ctx context.Context, taskID int64) (*model.Task, error) {
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if t.RecurrenceRule == nil {
		status := model.TaskStatusCompleted
		return s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
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

	upd := repo.TaskUpdate{}
	if next.IsZero() {
		status := model.TaskStatusCompleted
		upd.Status = &status
	} else {
		upd.DueAt = &next
	}
	return s.tasks.Update(ctx, t.ID, upd)
}

func (s *CompleteService) Uncomplete(ctx context.Context, taskID int64) (*model.Task, error) {
	status := model.TaskStatusOpen
	return s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
}

func (s *CompleteService) Cancel(ctx context.Context, taskID int64) (*model.Task, error) {
	status := model.TaskStatusCancelled
	return s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
}

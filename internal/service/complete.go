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
	if t.RecurrenceRule != nil {
		return s.advanceRecurring(ctx, t)
	}
	// Always attempt the capacity bump even when the task is already completed:
	// if a previous Complete crashed between Update and bumpTroikiCapacity, the
	// task sits completed with an unset grant flag, and only a retry can recover
	// the lost grant. The flag-flip inside bumpTroikiCapacity is idempotent, so a
	// no-op when capacity was already granted.
	if t.Status != model.TaskStatusCompleted {
		status := model.TaskStatusCompleted
		updated, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
		if err != nil {
			return nil, err
		}
		t = updated
	}
	if err := s.bumpTroikiCapacity(ctx, taskID, t.TroikiCategory); err != nil {
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
		if err := s.bumpTroikiCapacity(ctx, t.ID, t.TroikiCategory); err != nil {
			return nil, err
		}
	}
	return updated, nil
}

// bumpTroikiCapacity grants +1 capacity to the next-tier slot when a categorised
// task is completed: important → +medium, medium → +rest. Rest and uncategorised
// completions have no effect. The grant flag on the task makes the operation
// idempotent across uncomplete/recomplete cycles — capacity is granted only
// once per (task, category-assignment) until the category is cleared or changed.
func (s *CompleteService) bumpTroikiCapacity(ctx context.Context, taskID int64, cat *model.TroikiCategory) error {
	if cat == nil || s.users == nil {
		return nil
	}
	var col string
	switch *cat {
	case model.TroikiCategoryImportant:
		col = "troiki_medium_capacity"
	case model.TroikiCategoryMedium:
		col = "troiki_rest_capacity"
	default:
		return nil
	}
	// Single transaction: flag flip + counter bump must succeed or both roll back.
	// Otherwise a failure between them strands the grant — the flag blocks retries.
	_, err := s.tasks.GrantAndBumpTroikiCapacity(ctx, taskID, SingleUserID, col)
	return err
}

func (s *CompleteService) Uncomplete(ctx context.Context, taskID int64) (*model.Task, error) {
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	// Already open: nothing to do. Without this guard a duplicate /uncomplete
	// on a categorised task would hit ReopenIfTroikiRoom's status != 'open'
	// filter, get (false, nil), and surface as a spurious ErrTroikiSlotFull.
	if t.Status == model.TaskStatusOpen {
		return t, nil
	}
	// Reopening a categorised task must respect the slot cap — its slot may have
	// been refilled while the task sat in completed/cancelled state. The cap
	// check has to be atomic with the status flip: a separate read+write lets
	// two concurrent uncompletes (or an uncomplete racing a SetCategory) both
	// observe room and both commit, leaving the slot over capacity.
	if t.TroikiCategory != nil {
		capacity, err := s.troikiCapacityFor(ctx, *t.TroikiCategory)
		if err != nil {
			return nil, err
		}
		ok, err := s.tasks.ReopenIfTroikiRoom(ctx, taskID, *t.TroikiCategory, capacity)
		if err != nil {
			return nil, err
		}
		if !ok {
			// Disambiguate: a concurrent /uncomplete may have already reopened
			// the task between our Get and the atomic UPDATE — that's success,
			// not a slot-full conflict.
			cur, err := s.tasks.Get(ctx, taskID)
			if err != nil {
				return nil, err
			}
			if cur.Status == model.TaskStatusOpen {
				return cur, nil
			}
			// Category may have changed concurrently (e.g., a racing Cancel
			// cleared it). The slot-full conclusion was based on a stale
			// category — retry against the current state instead of surfacing
			// a spurious 409. Recursion is bounded: each retry observes the
			// committed category, and only a fresh concurrent change would
			// trigger another retry.
			if !sameTroikiCategory(cur.TroikiCategory, t.TroikiCategory) {
				return s.Uncomplete(ctx, taskID)
			}
			return nil, ErrTroikiSlotFull
		}
		return s.tasks.Get(ctx, taskID)
	}
	status := model.TaskStatusOpen
	return s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status})
}

// Cancel marks a task cancelled and frees its Troiki slot, if any. Cancellation
// is a terminal user action — the slot is released definitively, and a later
// Uncomplete reopens the task without a category (the user must re-categorise
// explicitly if they want it back in a slot).
func (s *CompleteService) Cancel(ctx context.Context, taskID int64) (*model.Task, error) {
	status := model.TaskStatusCancelled
	return s.tasks.Update(ctx, taskID, repo.TaskUpdate{Status: &status, TroikiCategoryClear: true})
}

func sameTroikiCategory(a, b *model.TroikiCategory) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *CompleteService) troikiCapacityFor(ctx context.Context, cat model.TroikiCategory) (int, error) {
	if cat == model.TroikiCategoryImportant {
		return TroikiImportantCap, nil
	}
	if s.users == nil {
		return 0, nil
	}
	c, err := s.users.GetTroikiCapacity(ctx, SingleUserID)
	if err != nil {
		return 0, err
	}
	switch cat {
	case model.TroikiCategoryMedium:
		return c.Medium, nil
	case model.TroikiCategoryRest:
		return c.Rest, nil
	}
	return 0, nil
}

package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TroikiImportantCap is the fixed capacity of the Important slot.
// Medium and Rest capacities accumulate per user (see User.TroikiMediumCapacity / TroikiRestCapacity).
const TroikiImportantCap = 3

// PriorityForCategory returns the task priority that a Troiki category enforces.
// Tasks with a category are pinned to this priority; direct priority edits are
// rejected by the task update handler.
func PriorityForCategory(cat model.TroikiCategory) model.Priority {
	switch cat {
	case model.TroikiCategoryImportant:
		return model.PriorityHigh
	case model.TroikiCategoryMedium:
		return model.PriorityMedium
	case model.TroikiCategoryRest:
		return model.PriorityLow
	}
	return model.PriorityNone
}

// SingleUserID is the id of the only user (single-user app, see migration 002).
const SingleUserID int64 = 1

var (
	ErrTroikiSlotFull    = errors.New("service: troiki slot full")
	ErrTroikiNotRootTask = errors.New("service: troiki category requires a root open task")
)

// TroikiSlot is one of the three Troiki sections returned by View.
type TroikiSlot struct {
	Capacity int
	Tasks    []model.Task
}

// TroikiView is the aggregate snapshot of all three Troiki slots.
//
// Started signals whether the user has confirmed the start of a Troiki cycle.
// Until Started is true, Medium and Rest accept tasks without capacity checks
// (initial fill mode); once started, the methodology rules apply (capacity
// grows only by completing tasks in the previous category).
type TroikiView struct {
	Important TroikiSlot
	Medium    TroikiSlot
	Rest      TroikiSlot
	Started   bool
}

// TroikiService manages Troiki category assignment and view aggregation.
type TroikiService struct {
	tasks *repo.TaskRepo
	users *repo.UserRepo
}

func NewTroikiService(tasks *repo.TaskRepo, users *repo.UserRepo) *TroikiService {
	return &TroikiService{tasks: tasks, users: users}
}

// SetCategory assigns or clears the Troiki category for a root open task.
// Passing nil clears the category. Re-assigning the same category is a no-op.
func (s *TroikiService) SetCategory(ctx context.Context, taskID int64, cat *model.TroikiCategory) (*model.Task, error) {
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if t.ParentID != nil {
		return nil, ErrTroikiNotRootTask
	}
	if t.Status != model.TaskStatusOpen {
		return nil, ErrTroikiNotRootTask
	}

	if cat == nil {
		return s.tasks.Update(ctx, taskID, repo.TaskUpdate{TroikiCategoryClear: true})
	}
	if !cat.IsValid() {
		return nil, fmt.Errorf("troiki: invalid category %q", *cat)
	}
	if t.TroikiCategory != nil && *t.TroikiCategory == *cat {
		return t, nil
	}

	cap, err := s.users.GetTroikiCapacity(ctx, SingleUserID)
	if err != nil {
		return nil, err
	}
	// Initial-fill mode: before the user presses "Start the system", Medium and
	// Rest accept tasks without capacity checks. Important always honors its
	// fixed cap of 3 — that's a core methodology rule, not a soft limit.
	derivedPriority := PriorityForCategory(*cat)
	if !cap.Started && (*cat == model.TroikiCategoryMedium || *cat == model.TroikiCategoryRest) {
		updated, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{
			TroikiCategory: cat,
			Priority:       &derivedPriority,
		})
		if err != nil {
			return nil, err
		}
		return updated, nil
	}
	capacity, err := s.capacityForWith(*cat, cap)
	if err != nil {
		return nil, err
	}
	// Atomic capacity-checked assignment — a separate read+write would race with
	// a concurrent SetCategory and let both requests exceed the slot cap.
	ok, err := s.tasks.SetTroikiCategoryIfRoom(ctx, taskID, *cat, capacity)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Disambiguate: SetTroikiCategoryIfRoom returns false when the slot is
		// full, the task stopped being root+open between our Get and the atomic
		// UPDATE (concurrent move/complete), or a concurrent request already
		// assigned the same category (the WHERE-clause COUNT then sees the task
		// itself in the slot and rejects the redundant write). Re-read to
		// surface the actual cause.
		cur, err := s.tasks.Get(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if cur.ParentID != nil || cur.Status != model.TaskStatusOpen {
			return nil, ErrTroikiNotRootTask
		}
		if cur.TroikiCategory != nil && *cur.TroikiCategory == *cat {
			return cur, nil
		}
		return nil, ErrTroikiSlotFull
	}
	// Pin priority to the category-derived value. The atomic UPDATE above only
	// touches troiki_category; a separate UPDATE keeps SetTroikiCategoryIfRoom's
	// capacity-checking SQL focused.
	if _, err := s.tasks.Update(ctx, taskID, repo.TaskUpdate{Priority: &derivedPriority}); err != nil {
		return nil, err
	}
	return s.tasks.Get(ctx, taskID)
}

// View returns capacities and open tasks for all three Troiki slots.
func (s *TroikiService) View(ctx context.Context) (TroikiView, error) {
	cap, err := s.users.GetTroikiCapacity(ctx, SingleUserID)
	if err != nil {
		return TroikiView{}, err
	}
	important, _, err := s.tasks.ListByTroikiCategory(ctx, model.TroikiCategoryImportant)
	if err != nil {
		return TroikiView{}, err
	}
	medium, _, err := s.tasks.ListByTroikiCategory(ctx, model.TroikiCategoryMedium)
	if err != nil {
		return TroikiView{}, err
	}
	rest, _, err := s.tasks.ListByTroikiCategory(ctx, model.TroikiCategoryRest)
	if err != nil {
		return TroikiView{}, err
	}
	mediumCap, restCap := cap.Medium, cap.Rest
	// Before start, Medium/Rest capacity reported as the current task count so
	// the UI doesn't render bogus empty-slot placeholders against a zero cap.
	if !cap.Started {
		mediumCap = len(medium)
		restCap = len(rest)
	}
	return TroikiView{
		Important: TroikiSlot{Capacity: TroikiImportantCap, Tasks: important},
		Medium:    TroikiSlot{Capacity: mediumCap, Tasks: medium},
		Rest:      TroikiSlot{Capacity: restCap, Tasks: rest},
		Started:   cap.Started,
	}, nil
}

// Start confirms the start of a Troiki cycle: snapshots current Medium/Rest
// task counts as capacity and flips troiki_started=1. Idempotent — calling on
// an already-started user is a no-op.
func (s *TroikiService) Start(ctx context.Context) error {
	cap, err := s.users.GetTroikiCapacity(ctx, SingleUserID)
	if err != nil {
		return err
	}
	if cap.Started {
		return nil
	}
	medium, _, err := s.tasks.ListByTroikiCategory(ctx, model.TroikiCategoryMedium)
	if err != nil {
		return err
	}
	rest, _, err := s.tasks.ListByTroikiCategory(ctx, model.TroikiCategoryRest)
	if err != nil {
		return err
	}
	return s.users.StartTroiki(ctx, SingleUserID, len(medium), len(rest))
}

func (s *TroikiService) capacityForWith(cat model.TroikiCategory, cap repo.TroikiCapacity) (int, error) {
	switch cat {
	case model.TroikiCategoryImportant:
		return TroikiImportantCap, nil
	case model.TroikiCategoryMedium:
		return cap.Medium, nil
	case model.TroikiCategoryRest:
		return cap.Rest, nil
	}
	return 0, fmt.Errorf("troiki: unsupported category %q", cat)
}

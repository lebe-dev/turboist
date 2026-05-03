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
type TroikiView struct {
	Important TroikiSlot
	Medium    TroikiSlot
	Rest      TroikiSlot
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

	capacity, err := s.capacityFor(ctx, *cat)
	if err != nil {
		return nil, err
	}
	count, err := s.tasks.CountOpenByTroikiCategory(ctx, *cat)
	if err != nil {
		return nil, err
	}
	if count >= capacity {
		return nil, ErrTroikiSlotFull
	}
	return s.tasks.Update(ctx, taskID, repo.TaskUpdate{TroikiCategory: cat})
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
	return TroikiView{
		Important: TroikiSlot{Capacity: TroikiImportantCap, Tasks: important},
		Medium:    TroikiSlot{Capacity: cap.Medium, Tasks: medium},
		Rest:      TroikiSlot{Capacity: cap.Rest, Tasks: rest},
	}, nil
}

func (s *TroikiService) capacityFor(ctx context.Context, cat model.TroikiCategory) (int, error) {
	if cat == model.TroikiCategoryImportant {
		return TroikiImportantCap, nil
	}
	cap, err := s.users.GetTroikiCapacity(ctx, SingleUserID)
	if err != nil {
		return 0, err
	}
	switch cat {
	case model.TroikiCategoryMedium:
		return cap.Medium, nil
	case model.TroikiCategoryRest:
		return cap.Rest, nil
	}
	return 0, fmt.Errorf("troiki: unsupported category %q", cat)
}

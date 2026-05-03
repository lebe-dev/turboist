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
// Tasks belonging to a project with a category are pinned to this priority;
// direct priority edits are rejected by the task update handler.
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
	ErrTroikiSlotFull       = errors.New("service: troiki slot full")
	ErrTroikiInvalidProject = errors.New("service: troiki category requires an open non-inbox project")
)

// TroikiSlotProject is one of the three Troiki sections returned by View, scoped
// to projects (the Troiki unit of work). Tasks groups every open task of every
// project in Projects by project_id (root + subtasks), so the UI renders the
// full task tree without further fetches.
type TroikiSlotProject struct {
	Capacity int
	Projects []model.Project
	Tasks    map[int64][]model.Task
}

// TroikiView is the aggregate snapshot of all three Troiki slots.
//
// Started signals whether the user has confirmed the start of a Troiki cycle.
// Until Started is true, Medium and Rest accept projects without capacity
// checks (initial fill mode); once started, the methodology rules apply
// (capacity grows only by completing tasks in the previous category).
type TroikiView struct {
	Important TroikiSlotProject
	Medium    TroikiSlotProject
	Rest      TroikiSlotProject
	Started   bool
}

// TroikiService manages Troiki category assignment (per project) and view
// aggregation.
type TroikiService struct {
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
	users    *repo.UserRepo
}

func NewTroikiService(tasks *repo.TaskRepo, projects *repo.ProjectRepo, users *repo.UserRepo) *TroikiService {
	return &TroikiService{tasks: tasks, projects: projects, users: users}
}

// SetCategory assigns or clears the Troiki category for an open project.
// Passing nil clears the category (open task priorities are left untouched —
// the user explicitly chose to drop the project from a slot, not to rewrite
// every task's priority). Re-assigning the same category is a no-op.
//
// On success, EnforceProjectPriority is invoked so every open task in the
// project (root + subtasks) is pinned to the category-derived priority.
func (s *TroikiService) SetCategory(ctx context.Context, projectID int64, cat *model.TroikiCategory) (*model.Project, error) {
	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if p.Status != model.ProjectStatusOpen {
		return nil, ErrTroikiInvalidProject
	}

	if cat == nil {
		updated, err := s.projects.Update(ctx, projectID, repo.ProjectUpdate{TroikiCategoryClear: true})
		if err != nil {
			return nil, err
		}
		// Reset per-task grant flags so a future re-categorisation can grant
		// capacity again on the same tasks.
		if err := s.tasks.ResetTroikiGrantedByProject(ctx, projectID); err != nil {
			return nil, err
		}
		return updated, nil
	}
	if !cat.IsValid() {
		return nil, fmt.Errorf("troiki: invalid category %q", *cat)
	}
	if p.TroikiCategory != nil && *p.TroikiCategory == *cat {
		return p, nil
	}

	cap, err := s.users.GetTroikiCapacity(ctx, SingleUserID)
	if err != nil {
		return nil, err
	}
	// Initial-fill mode: before the user presses "Start the system", Medium and
	// Rest accept projects without capacity checks. Important always honors its
	// fixed cap of 3 — that's a core methodology rule, not a soft limit.
	if !cap.Started && (*cat == model.TroikiCategoryMedium || *cat == model.TroikiCategoryRest) {
		updated, err := s.projects.Update(ctx, projectID, repo.ProjectUpdate{TroikiCategory: cat})
		if err != nil {
			return nil, err
		}
		if err := s.tasks.ResetTroikiGrantedByProject(ctx, projectID); err != nil {
			return nil, err
		}
		if err := s.EnforceProjectPriority(ctx, projectID, PriorityForCategory(*cat)); err != nil {
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
	ok, err := s.projects.SetTroikiCategoryIfRoom(ctx, projectID, *cat, capacity)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Disambiguate: SetTroikiCategoryIfRoom returns false when the slot is
		// full, the project stopped being open between our Get and the atomic
		// UPDATE, or a concurrent request already assigned the same category
		// (the WHERE-clause COUNT then sees the project itself in the slot and
		// rejects the redundant write). Re-read to surface the actual cause.
		cur, err := s.projects.Get(ctx, projectID)
		if err != nil {
			return nil, err
		}
		if cur.Status != model.ProjectStatusOpen {
			return nil, ErrTroikiInvalidProject
		}
		if cur.TroikiCategory != nil && *cur.TroikiCategory == *cat {
			return cur, nil
		}
		return nil, ErrTroikiSlotFull
	}
	if err := s.tasks.ResetTroikiGrantedByProject(ctx, projectID); err != nil {
		return nil, err
	}
	if err := s.EnforceProjectPriority(ctx, projectID, PriorityForCategory(*cat)); err != nil {
		return nil, err
	}
	return s.projects.Get(ctx, projectID)
}

// EnforceProjectPriority pins all open tasks of the project (root + subtasks)
// to the given priority via a single bulk UPDATE.
func (s *TroikiService) EnforceProjectPriority(ctx context.Context, projectID int64, priority model.Priority) error {
	return s.tasks.UpdatePriorityByProject(ctx, projectID, priority)
}

// View returns capacities, projects, and grouped tasks for all three Troiki slots.
func (s *TroikiService) View(ctx context.Context) (TroikiView, error) {
	cap, err := s.users.GetTroikiCapacity(ctx, SingleUserID)
	if err != nil {
		return TroikiView{}, err
	}
	important, err := s.buildSlot(ctx, model.TroikiCategoryImportant, TroikiImportantCap)
	if err != nil {
		return TroikiView{}, err
	}
	mediumCap, restCap := cap.Medium, cap.Rest
	if !cap.Started {
		// Before start, Medium/Rest capacity reported as the current project
		// count so the UI doesn't render bogus empty-slot placeholders against
		// a zero cap.
		n, err := s.projects.CountOpenByTroikiCategory(ctx, model.TroikiCategoryMedium)
		if err != nil {
			return TroikiView{}, err
		}
		mediumCap = n
		n, err = s.projects.CountOpenByTroikiCategory(ctx, model.TroikiCategoryRest)
		if err != nil {
			return TroikiView{}, err
		}
		restCap = n
	}
	medium, err := s.buildSlot(ctx, model.TroikiCategoryMedium, mediumCap)
	if err != nil {
		return TroikiView{}, err
	}
	rest, err := s.buildSlot(ctx, model.TroikiCategoryRest, restCap)
	if err != nil {
		return TroikiView{}, err
	}
	return TroikiView{
		Important: important,
		Medium:    medium,
		Rest:      rest,
		Started:   cap.Started,
	}, nil
}

func (s *TroikiService) buildSlot(ctx context.Context, cat model.TroikiCategory, capacity int) (TroikiSlotProject, error) {
	projects, _, err := s.projects.ListByTroikiCategory(ctx, cat)
	if err != nil {
		return TroikiSlotProject{}, err
	}
	ids := make([]int64, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}
	tasks, err := s.tasks.ListByProjectIDs(ctx, ids)
	if err != nil {
		return TroikiSlotProject{}, err
	}
	return TroikiSlotProject{Capacity: capacity, Projects: projects, Tasks: tasks}, nil
}

// Start confirms the start of a Troiki cycle: snapshots current Medium/Rest
// project counts as capacity and flips troiki_started=1. Idempotent — calling
// on an already-started user is a no-op.
func (s *TroikiService) Start(ctx context.Context) error {
	cap, err := s.users.GetTroikiCapacity(ctx, SingleUserID)
	if err != nil {
		return err
	}
	if cap.Started {
		return nil
	}
	medium, err := s.projects.CountOpenByTroikiCategory(ctx, model.TroikiCategoryMedium)
	if err != nil {
		return err
	}
	rest, err := s.projects.CountOpenByTroikiCategory(ctx, model.TroikiCategoryRest)
	if err != nil {
		return err
	}
	return s.users.StartTroiki(ctx, SingleUserID, medium, rest)
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

package service

import (
	"context"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// MoveService wraps repo.TaskRepo.Move and re-fetches the updated task.
// When the destination project carries a Troiki category, every task of that
// project (including the moved subtree) is re-pinned to the derived priority
// to keep the slot invariant: "all tasks of a categorised project share the
// category-derived priority".
type MoveService struct {
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
}

func NewMoveService(tasks *repo.TaskRepo, projects *repo.ProjectRepo) *MoveService {
	return &MoveService{tasks: tasks, projects: projects}
}

func (s *MoveService) Move(ctx context.Context, taskID int64, target repo.Placement) (*model.Task, error) {
	if err := s.tasks.Move(ctx, taskID, target); err != nil {
		return nil, err
	}
	if target.ProjectID != nil && s.projects != nil {
		p, err := s.projects.Get(ctx, *target.ProjectID)
		if err != nil {
			return nil, err
		}
		if p.TroikiCategory != nil {
			if err := s.tasks.UpdatePriorityByProject(ctx, *target.ProjectID, PriorityForCategory(*p.TroikiCategory)); err != nil {
				return nil, err
			}
		}
	}
	return s.tasks.Get(ctx, taskID)
}

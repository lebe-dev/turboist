package service

import (
	"context"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// MoveService wraps repo.TaskRepo.Move and re-fetches the updated task.
type MoveService struct {
	tasks *repo.TaskRepo
}

func NewMoveService(tasks *repo.TaskRepo) *MoveService {
	return &MoveService{tasks: tasks}
}

func (s *MoveService) Move(ctx context.Context, taskID int64, target repo.Placement) (*model.Task, error) {
	if err := s.tasks.Move(ctx, taskID, target); err != nil {
		return nil, err
	}
	return s.tasks.Get(ctx, taskID)
}

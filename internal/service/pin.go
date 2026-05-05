package service

import (
	"context"
	"errors"

	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrPinLimitExceeded is returned when the max-pinned limit would be exceeded.
var ErrPinLimitExceeded = errors.New("service: pin limit exceeded")

// PinService enforces the max-pinned constraint for projects and tasks separately.
type PinService struct {
	tasks     *repo.TaskRepo
	projects  *repo.ProjectRepo
	maxPinned int
}

func NewPinService(tasks *repo.TaskRepo, projects *repo.ProjectRepo, maxPinned int) *PinService {
	return &PinService{tasks: tasks, projects: projects, maxPinned: maxPinned}
}

func (s *PinService) PinProject(ctx context.Context, projectID int64) error {
	count, err := s.tasks.CountPinnedProjects(ctx)
	if err != nil {
		return err
	}
	if count >= s.maxPinned {
		return ErrPinLimitExceeded
	}
	return s.projects.SetPinned(ctx, projectID, true)
}

func (s *PinService) UnpinProject(ctx context.Context, projectID int64) error {
	return s.projects.SetPinned(ctx, projectID, false)
}

func (s *PinService) PinTask(ctx context.Context, taskID int64) error {
	count, err := s.tasks.CountPinnedTasks(ctx)
	if err != nil {
		return err
	}
	if count >= s.maxPinned {
		return ErrPinLimitExceeded
	}
	return s.tasks.SetPinned(ctx, taskID, true)
}

func (s *PinService) UnpinTask(ctx context.Context, taskID int64) error {
	return s.tasks.SetPinned(ctx, taskID, false)
}

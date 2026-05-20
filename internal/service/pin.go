package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
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
	const op = "service.PinService.PinProject"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("project_id", projectID))
	count, err := s.tasks.CountPinnedProjects(ctx)
	if err != nil {
		log.ErrorContext(ctx, op+": count pinned projects", slog.String("err", err.Error()))
		return err
	}
	if count >= s.maxPinned {
		log.WarnContext(ctx, op+": pin limit exceeded", slog.Int64("project_id", projectID), slog.Int("count", count), slog.Int("limit", s.maxPinned))
		return ErrPinLimitExceeded
	}
	if err := s.projects.SetPinned(ctx, projectID, true); err != nil {
		log.ErrorContext(ctx, op+": set pinned", slog.Int64("project_id", projectID), slog.String("err", err.Error()))
		return err
	}
	log.InfoContext(ctx, "project pinned", slog.String("op", op), slog.Int64("project_id", projectID))
	return nil
}

func (s *PinService) UnpinProject(ctx context.Context, projectID int64) error {
	const op = "service.PinService.UnpinProject"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("project_id", projectID))
	if err := s.projects.SetPinned(ctx, projectID, false); err != nil {
		log.ErrorContext(ctx, op+": set unpinned", slog.Int64("project_id", projectID), slog.String("err", err.Error()))
		return err
	}
	log.InfoContext(ctx, "project unpinned", slog.String("op", op), slog.Int64("project_id", projectID))
	return nil
}

func (s *PinService) PinTask(ctx context.Context, taskID int64) error {
	const op = "service.PinService.PinTask"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))
	count, err := s.tasks.CountPinnedTasks(ctx)
	if err != nil {
		log.ErrorContext(ctx, op+": count pinned tasks", slog.String("err", err.Error()))
		return err
	}
	if count >= s.maxPinned {
		log.WarnContext(ctx, op+": pin limit exceeded", slog.Int64("task_id", taskID), slog.Int("count", count), slog.Int("limit", s.maxPinned))
		return ErrPinLimitExceeded
	}
	if err := s.tasks.SetPinned(ctx, taskID, true); err != nil {
		log.ErrorContext(ctx, op+": set pinned", slog.Int64("task_id", taskID), slog.String("err", err.Error()))
		return err
	}
	log.InfoContext(ctx, "task pinned", slog.String("op", op), slog.Int64("task_id", taskID))
	return nil
}

func (s *PinService) UnpinTask(ctx context.Context, taskID int64) error {
	const op = "service.PinService.UnpinTask"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))
	if err := s.tasks.SetPinned(ctx, taskID, false); err != nil {
		log.ErrorContext(ctx, op+": set unpinned", slog.Int64("task_id", taskID), slog.String("err", err.Error()))
		return err
	}
	log.InfoContext(ctx, "task unpinned", slog.String("op", op), slog.Int64("task_id", taskID))
	return nil
}

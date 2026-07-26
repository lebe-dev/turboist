package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrPinLimitExceeded is returned when the pinned cap would be exceeded.
var ErrPinLimitExceeded = errors.New("service: pin limit exceeded")

// PinService enforces the pinned caps for projects and tasks separately. Both
// caps are per-user preferences (users.settings.maxPinnedTasks /
// maxPinnedProjects) read on every pin, so a change in Settings takes effect
// immediately without a restart.
type PinService struct {
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
	users    *repo.UserRepo
}

func NewPinService(tasks *repo.TaskRepo, projects *repo.ProjectRepo, users *repo.UserRepo) *PinService {
	return &PinService{tasks: tasks, projects: projects, users: users}
}

// limits loads the two caps for the single user. GetSettings already normalizes
// out-of-range and pre-migration-048 values to model.DefaultMaxPinned.
func (s *PinService) limits(ctx context.Context) (tasks int, projects int, err error) {
	settings, err := s.users.GetSettings(ctx, SingleUserID)
	if err != nil {
		return 0, 0, err
	}
	return settings.MaxPinnedTasks, settings.MaxPinnedProjects, nil
}

func (s *PinService) PinProject(ctx context.Context, projectID int64) error {
	const op = "service.PinService.PinProject"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("project_id", projectID))
	_, limit, err := s.limits(ctx)
	if err != nil {
		log.ErrorContext(ctx, op+": load settings", slog.String("err", err.Error()))
		return err
	}
	count, err := s.tasks.CountPinnedProjects(ctx)
	if err != nil {
		log.ErrorContext(ctx, op+": count pinned projects", slog.String("err", err.Error()))
		return err
	}
	if count >= limit {
		log.WarnContext(ctx, op+": pin limit exceeded", slog.Int64("project_id", projectID), slog.Int("count", count), slog.Int("limit", limit))
		return ErrPinLimitExceeded
	}
	if err := s.projects.SetPinned(ctx, projectID, true); err != nil {
		logRepoErr(ctx, op+": set pinned", err, slog.Int64("project_id", projectID))
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
		logRepoErr(ctx, op+": set unpinned", err, slog.Int64("project_id", projectID))
		return err
	}
	log.InfoContext(ctx, "project unpinned", slog.String("op", op), slog.Int64("project_id", projectID))
	return nil
}

func (s *PinService) PinTask(ctx context.Context, taskID int64) error {
	const op = "service.PinService.PinTask"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID))
	limit, _, err := s.limits(ctx)
	if err != nil {
		log.ErrorContext(ctx, op+": load settings", slog.String("err", err.Error()))
		return err
	}
	count, err := s.tasks.CountPinnedTasks(ctx)
	if err != nil {
		log.ErrorContext(ctx, op+": count pinned tasks", slog.String("err", err.Error()))
		return err
	}
	if count >= limit {
		log.WarnContext(ctx, op+": pin limit exceeded", slog.Int64("task_id", taskID), slog.Int("count", count), slog.Int("limit", limit))
		return ErrPinLimitExceeded
	}
	if err := s.tasks.SetPinned(ctx, taskID, true); err != nil {
		logRepoErr(ctx, op+": set pinned", err, slog.Int64("task_id", taskID))
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
		logRepoErr(ctx, op+": set unpinned", err, slog.Int64("task_id", taskID))
		return err
	}
	log.InfoContext(ctx, "task unpinned", slog.String("op", op), slog.Int64("task_id", taskID))
	return nil
}

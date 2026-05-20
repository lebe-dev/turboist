package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

var (
	ErrPlanLimitExceeded = errors.New("service: plan limit exceeded")
	ErrNoContextForInbox = errors.New("service: cannot plan inbox task — create a context first")
)

// PlanService enforces weekly/backlog limits before changing plan_state.
//
// Planning an Inbox task into week/backlog also moves it out of Inbox into the
// first context (ordered by favourite, name). The schema requires every task
// to have either inbox_id or context_id, so we cannot just unset inbox_id.
type PlanService struct {
	tasks        *repo.TaskRepo
	contexts     *repo.ContextRepo
	weeklyLimit  int
	backlogLimit int
}

func NewPlanService(tasks *repo.TaskRepo, contexts *repo.ContextRepo, weeklyLimit, backlogLimit int) *PlanService {
	return &PlanService{tasks: tasks, contexts: contexts, weeklyLimit: weeklyLimit, backlogLimit: backlogLimit}
}

func (s *PlanService) SetPlanState(ctx context.Context, taskID int64, state model.PlanState) (*model.Task, error) {
	const op = "service.PlanService.SetPlanState"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", taskID), slog.String("state", string(state)))
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		log.ErrorContext(ctx, op+": get task", slog.Int64("task_id", taskID), slog.String("err", err.Error()))
		return nil, err
	}
	if t.PlanState == state {
		return t, nil
	}
	switch state {
	case model.PlanStateWeek:
		count, err := s.tasks.CountWeek(ctx)
		if err != nil {
			log.ErrorContext(ctx, op+": count week", slog.String("err", err.Error()))
			return nil, err
		}
		if count >= s.weeklyLimit {
			log.WarnContext(ctx, op+": weekly limit exceeded", slog.Int64("task_id", taskID), slog.Int("count", count), slog.Int("limit", s.weeklyLimit))
			return nil, ErrPlanLimitExceeded
		}
	case model.PlanStateBacklog:
		count, err := s.tasks.CountBacklog(ctx)
		if err != nil {
			log.ErrorContext(ctx, op+": count backlog", slog.String("err", err.Error()))
			return nil, err
		}
		if count >= s.backlogLimit {
			log.WarnContext(ctx, op+": backlog limit exceeded", slog.Int64("task_id", taskID), slog.Int("count", count), slog.Int("limit", s.backlogLimit))
			return nil, ErrPlanLimitExceeded
		}
	}
	if t.InboxID != nil && (state == model.PlanStateWeek || state == model.PlanStateBacklog) {
		ctxs, _, err := s.contexts.List(ctx, repo.Page{Limit: 1})
		if err != nil {
			log.ErrorContext(ctx, op+": list contexts", slog.String("err", err.Error()))
			return nil, err
		}
		if len(ctxs) == 0 {
			log.WarnContext(ctx, op+": no context for inbox task", slog.Int64("task_id", taskID))
			return nil, ErrNoContextForInbox
		}
		ctxID := ctxs[0].ID
		if err := s.tasks.Move(ctx, taskID, repo.Placement{ContextID: &ctxID}); err != nil {
			log.ErrorContext(ctx, op+": move inbox task", slog.Int64("task_id", taskID), slog.String("err", err.Error()))
			return nil, err
		}
	}
	update := repo.TaskUpdate{PlanState: &state}
	if state == model.PlanStateWeek || state == model.PlanStateBacklog {
		update.DueAtClear = true
	}
	updated, err := s.tasks.Update(ctx, taskID, update)
	if err != nil {
		log.ErrorContext(ctx, op+": update task", slog.Int64("task_id", taskID), slog.String("err", err.Error()))
		return nil, err
	}
	log.InfoContext(ctx, "plan state changed", slog.String("op", op), slog.Int64("task_id", taskID), slog.String("state", string(state)))
	return updated, nil
}

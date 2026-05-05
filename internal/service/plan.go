package service

import (
	"context"
	"errors"

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
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if t.PlanState == state {
		return t, nil
	}
	switch state {
	case model.PlanStateWeek:
		count, err := s.tasks.CountWeek(ctx)
		if err != nil {
			return nil, err
		}
		if count >= s.weeklyLimit {
			return nil, ErrPlanLimitExceeded
		}
	case model.PlanStateBacklog:
		count, err := s.tasks.CountBacklog(ctx)
		if err != nil {
			return nil, err
		}
		if count >= s.backlogLimit {
			return nil, ErrPlanLimitExceeded
		}
	}
	if t.InboxID != nil && (state == model.PlanStateWeek || state == model.PlanStateBacklog) {
		ctxs, _, err := s.contexts.List(ctx, repo.Page{Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(ctxs) == 0 {
			return nil, ErrNoContextForInbox
		}
		ctxID := ctxs[0].ID
		if err := s.tasks.Move(ctx, taskID, repo.Placement{ContextID: &ctxID}); err != nil {
			return nil, err
		}
	}
	update := repo.TaskUpdate{PlanState: &state}
	if state == model.PlanStateWeek || state == model.PlanStateBacklog {
		update.DueAtClear = true
	}
	return s.tasks.Update(ctx, taskID, update)
}

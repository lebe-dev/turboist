package service

import (
	"context"
	"errors"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

var ErrPlanLimitExceeded = errors.New("service: plan limit exceeded")

// PlanService enforces weekly/backlog limits before changing plan_state.
type PlanService struct {
	tasks        *repo.TaskRepo
	weeklyLimit  int
	backlogLimit int
}

func NewPlanService(tasks *repo.TaskRepo, weeklyLimit, backlogLimit int) *PlanService {
	return &PlanService{tasks: tasks, weeklyLimit: weeklyLimit, backlogLimit: backlogLimit}
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
	return s.tasks.Update(ctx, taskID, repo.TaskUpdate{PlanState: &state})
}

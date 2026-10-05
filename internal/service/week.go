package service

import (
	"context"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
)

// CascadeWeek pulls every open descendant subtask (any depth) into the current
// week alongside its parent. Called from the two paths a task can be planned for
// the week through: PlanService.SetPlanState (the plan endpoint) and a PATCH that
// sets planState directly. Mirrors CascadeBacklog.
func (s *TaskService) CascadeWeek(ctx context.Context, parentID int64) error {
	const op = "service.TaskService.CascadeWeek"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", parentID))
	return s.tasks.CascadeWeekToDescendants(ctx, parentID)
}

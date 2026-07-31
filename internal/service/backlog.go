package service

import (
	"context"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
)

// CascadeBacklog parks every open descendant subtask (any depth) in the backlog
// alongside its parent, clearing their due dates. Called from the two paths a task
// can enter the backlog through: PlanService.SetPlanState (the plan endpoint) and
// a PATCH that sets planState directly.
func (s *TaskService) CascadeBacklog(ctx context.Context, parentID int64) error {
	const op = "service.TaskService.CascadeBacklog"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", parentID))
	return s.tasks.CascadeBacklogToDescendants(ctx, parentID)
}

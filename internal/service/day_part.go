package service

import (
	"context"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// CascadeDayPart propagates a parent task's day-part change to every open
// descendant subtask (any depth), keeping a task's phase-of-day grouping
// consistent with its parent when the parent is moved between day parts.
func (s *TaskService) CascadeDayPart(ctx context.Context, parentID int64, dp model.DayPart) error {
	const op = "service.TaskService.CascadeDayPart"
	log := logging.FromContext(ctx)
	log.DebugContext(ctx, op, slog.Int64("task_id", parentID), slog.String("day_part", string(dp)))
	return s.tasks.CascadeDayPartToDescendants(ctx, parentID, dp)
}

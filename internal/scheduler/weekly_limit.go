package scheduler

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

// TaskReader is the subset of todoist.Cache that WeeklyLimit needs.
type TaskReader interface {
	Tasks() []*todoist.Task
}

// WeeklyLimit logs a warning when the number of tasks with the weekly label exceeds max_tasks.
type WeeklyLimit struct {
	reader TaskReader
	cfg    config.WeeklyConfig
}

// NewWeeklyLimit creates a WeeklyLimit job.
func NewWeeklyLimit(reader TaskReader, cfg config.WeeklyConfig) *WeeklyLimit {
	return &WeeklyLimit{reader: reader, cfg: cfg}
}

// Job implements scheduler.Job. Register it with the Scheduler.
func (wl *WeeklyLimit) Job(_ context.Context) {
	if wl.cfg.MaxTasks <= 0 || wl.cfg.Label == "" {
		return
	}

	count := 0
	for _, task := range wl.reader.Tasks() {
		if taskHasLabel(task, wl.cfg.Label) {
			count++
		}
	}

	if count > wl.cfg.MaxTasks {
		log.Warn("weekly_limit: exceeded",
			"label", wl.cfg.Label,
			"count", count,
			"max", wl.cfg.MaxTasks,
		)
	}
}

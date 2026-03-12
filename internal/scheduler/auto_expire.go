package scheduler

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

// Completer is the subset of todoist.Cache that AutoExpire needs.
type Completer interface {
	Tasks() []*todoist.Task
	CompleteTask(ctx context.Context, id string) error
}

// AutoExpire watches tasks labelled with expire rules and completes them when their TTL elapses.
// first_seen_at is tracked in memory; on restart tasks receive a fresh TTL.
type AutoExpire struct {
	completer Completer
	rules     []config.AutoExpireConfig
	now       func() time.Time

	mu        sync.Mutex
	firstSeen map[string]time.Time // key: taskID+":"+label
}

// NewAutoExpire creates an AutoExpire job.
func NewAutoExpire(completer Completer, rules []config.AutoExpireConfig) *AutoExpire {
	return &AutoExpire{
		completer: completer,
		rules:     rules,
		now:       time.Now,
		firstSeen: make(map[string]time.Time),
	}
}

// Job implements scheduler.Job. Register it with the Scheduler.
func (ae *AutoExpire) Job(ctx context.Context) {
	tasks := ae.completer.Tasks()
	now := ae.now()

	for _, rule := range ae.rules {
		for _, task := range tasks {
			if !taskHasLabel(task, rule.Label) {
				continue
			}

			key := task.ID + ":" + rule.Label

			ae.mu.Lock()
			if _, seen := ae.firstSeen[key]; !seen {
				ae.firstSeen[key] = now
				log.Debug("auto_expire: first seen", "task", task.ID, "label", rule.Label)
			}
			seenAt := ae.firstSeen[key]
			ae.mu.Unlock()

			if now.Sub(seenAt) >= rule.TTL {
				log.Info("auto_expire: completing task",
					"task", task.ID,
					"content", task.Content,
					"label", rule.Label,
					"ttl", rule.TTL,
				)
				if err := ae.completer.CompleteTask(ctx, task.ID); err != nil {
					log.Error("auto_expire: failed to complete task", "task", task.ID, "err", err)
					continue
				}
				ae.mu.Lock()
				delete(ae.firstSeen, key)
				ae.mu.Unlock()
			}
		}
	}
}

func taskHasLabel(task *todoist.Task, label string) bool {
	return slices.Contains(task.Labels, label)
}

package scheduler

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

// AutoRemoveStore is the persistence interface for first-seen timestamps.
type AutoRemoveStore interface {
	GetAutoRemoveFirstSeen() (map[string]time.Time, error)
	UpsertAutoRemoveFirstSeen(taskID, label string, firstSeen time.Time) error
	DeleteAutoRemoveFirstSeen(taskID, label string) error
	CleanupAutoRemoveFirstSeen(activeKeys map[string]bool) error
}

// Deleter is the subset of todoist.Cache that AutoRemove needs.
type Deleter interface {
	Tasks() []*todoist.Task
	DeleteTask(ctx context.Context, id string) error
}

// AutoRemove watches tasks labelled with remove rules and deletes them when their TTL elapses.
// first_seen is persisted in SQLite and cached in memory.
type AutoRemove struct {
	deleter Deleter
	cfg     config.AutoRemoveConfig
	store   AutoRemoveStore
	now     func() time.Time

	mu        sync.Mutex
	firstSeen map[string]time.Time // key: taskID+":"+label

	paused atomic.Bool // set when circuit breaker trips
}

// NewAutoRemove creates an AutoRemove job, loading persisted first-seen timestamps from the store.
func NewAutoRemove(deleter Deleter, cfg config.AutoRemoveConfig, store AutoRemoveStore) *AutoRemove {
	firstSeen := make(map[string]time.Time)
	if store != nil {
		loaded, err := store.GetAutoRemoveFirstSeen()
		if err != nil {
			log.Error("auto_remove: failed to load first_seen from DB, starting fresh", "err", err)
		} else {
			firstSeen = loaded
			log.Info("auto_remove: loaded first_seen from DB", "count", len(firstSeen))
		}
	}

	return &AutoRemove{
		deleter:   deleter,
		cfg:       cfg,
		store:     store,
		now:       time.Now,
		firstSeen: firstSeen,
	}
}

// Paused returns true when the circuit breaker has tripped (too many tasks match for deletion).
func (ar *AutoRemove) Paused() bool {
	return ar.paused.Load()
}

// candidate holds a task ID and the rule that matched it for deletion.
type candidate struct {
	taskID string
	label  string
}

// Job implements scheduler.Job. Register it with the Scheduler.
func (ar *AutoRemove) Job(ctx context.Context) {
	tasks := ar.deleter.Tasks()
	now := ar.now()

	// Phase 1: record first-seen for all matching tasks, build candidate list.
	activeKeys := make(map[string]bool)
	var candidates []candidate

	for _, rule := range ar.cfg.Rules {
		for _, task := range tasks {
			if !slices.Contains(task.Labels, rule.Label) {
				continue
			}

			key := task.ID + ":" + rule.Label
			activeKeys[key] = true

			ar.mu.Lock()
			seenAt, seen := ar.firstSeen[key]
			if !seen {
				seenAt = now
				ar.firstSeen[key] = seenAt
				log.Debug("auto_remove: first seen", "task", task.ID, "label", rule.Label)
				if ar.store != nil {
					if err := ar.store.UpsertAutoRemoveFirstSeen(task.ID, rule.Label, seenAt); err != nil {
						log.Error("auto_remove: failed to persist first_seen", "task", task.ID, "err", err)
					}
				}
			}
			ar.mu.Unlock()

			if now.Sub(seenAt) >= rule.TTL {
				candidates = append(candidates, candidate{taskID: task.ID, label: rule.Label})
			}
		}
	}

	// Phase 2: circuit breaker — if too many tasks would be deleted, pause.
	totalTasks := len(tasks)
	if totalTasks > 0 && ar.cfg.MaxPercent > 0 {
		threshold := totalTasks * ar.cfg.MaxPercent / 100
		if threshold < 1 {
			threshold = 1
		}
		if len(candidates) > threshold {
			ar.paused.Store(true)
			log.Error("auto_remove: circuit breaker tripped",
				"candidates", len(candidates),
				"total_tasks", totalTasks,
				"threshold_percent", ar.cfg.MaxPercent,
				"threshold_count", threshold,
			)
			return
		}
	}
	ar.paused.Store(false)

	// Phase 3: delete up to maxPerTick candidates.
	deleted := 0
	for _, c := range candidates {
		if deleted >= ar.cfg.MaxPerTick {
			log.Debug("auto_remove: per-tick limit reached", "limit", ar.cfg.MaxPerTick)
			break
		}

		log.Info("auto_remove: deleting task",
			"task", c.taskID,
			"label", c.label,
		)
		if err := ar.deleter.DeleteTask(ctx, c.taskID); err != nil {
			log.Error("auto_remove: failed to delete task", "task", c.taskID, "err", err)
			continue
		}

		ar.mu.Lock()
		delete(ar.firstSeen, c.taskID+":"+c.label)
		ar.mu.Unlock()
		if ar.store != nil {
			if err := ar.store.DeleteAutoRemoveFirstSeen(c.taskID, c.label); err != nil {
				log.Error("auto_remove: failed to remove first_seen from DB", "task", c.taskID, "err", err)
			}
		}
		deleted++
	}

	// Phase 4: cleanup stale first-seen entries (tasks removed or labels changed).
	if ar.store != nil {
		if err := ar.store.CleanupAutoRemoveFirstSeen(activeKeys); err != nil {
			log.Error("auto_remove: cleanup failed", "err", err)
		}
	}
	ar.mu.Lock()
	for key := range ar.firstSeen {
		if !activeKeys[key] {
			delete(ar.firstSeen, key)
		}
	}
	ar.mu.Unlock()
}

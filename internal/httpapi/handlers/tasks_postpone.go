package handlers

import (
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

const postponeGracePeriod = 5 * time.Minute

// shouldIncPostpone reports whether a PATCH represents a postpone — moving an
// existing future-bound dueAt strictly later, after the grace period elapsed.
// Setting a dueAt for the first time, clearing it, advancing it, or moving it
// into the past does not count.
func shouldIncPostpone(t *model.Task, u repo.TaskUpdate, now time.Time) bool {
	if u.DueAtClear || u.DueAt == nil {
		return false
	}
	if t.DueAt == nil {
		return false
	}
	if !u.DueAt.After(*t.DueAt) {
		return false
	}
	if !u.DueAt.After(now) {
		return false
	}
	if now.Sub(t.CreatedAt) <= postponeGracePeriod {
		return false
	}
	return true
}

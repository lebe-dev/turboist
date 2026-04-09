package ws

import (
	"encoding/json"
	"hash/fnv"

	"github.com/lebe-dev/turboist/internal/todoist"
)

// TroikiSnapshot stores per-section hashes for delta computation.
type TroikiSnapshot [3]uint64

// TroikiDelta represents changed sections in the troiki channel.
type TroikiDelta struct {
	Sections []any `json:"sections"` // changed SectionState entries
}

// TasksSnapshot maps task ID → hash for delta computation.
type TasksSnapshot map[string]uint64

// PlanningSnapshot holds snapshots for both planning lists.
type PlanningSnapshot struct {
	Backlog TasksSnapshot
	Weekly  TasksSnapshot
}

// TasksDelta represents changes between two task snapshots.
type TasksDelta struct {
	Upserted []*todoist.Task `json:"upserted"`
	Removed  []string        `json:"removed"`
	Meta     any             `json:"meta"`
}

// PlanningDelta represents changes in planning view.
type PlanningDelta struct {
	BacklogUpserted []*todoist.Task `json:"backlog_upserted"`
	BacklogRemoved  []string        `json:"backlog_removed"`
	WeeklyUpserted  []*todoist.Task `json:"weekly_upserted"`
	WeeklyRemoved   []string        `json:"weekly_removed"`
	Meta            any             `json:"meta"`
}

// hashTask computes a fast 64-bit hash of a task's JSON (without children).
func hashTask(t *todoist.Task) uint64 {
	// Clone without children to get stable hash of just this task's data
	clone := *t
	clone.Children = nil
	data, err := json.Marshal(clone)
	if err != nil {
		return 0
	}
	h := fnv.New64a()
	h.Write(data)
	return h.Sum64()
}

// buildSnapshot creates a snapshot from a task tree (flattened).
func buildSnapshot(tasks []*todoist.Task) TasksSnapshot {
	snap := make(TasksSnapshot)
	var walk func([]*todoist.Task)
	walk = func(ts []*todoist.Task) {
		for _, t := range ts {
			snap[t.ID] = hashTask(t)
			walk(t.Children)
		}
	}
	walk(tasks)
	return snap
}

// computeTasksDelta computes upserted/removed tasks between old and new snapshots.
// Returns the full tree entries for upserted (caller provides the new tree).
func computeTasksDelta(oldSnap TasksSnapshot, newTasks []*todoist.Task, meta any) (*TasksDelta, TasksSnapshot) {
	newSnap := buildSnapshot(newTasks)

	var upserted []*todoist.Task
	var removed []string

	// Find upserted: new or changed tasks
	upsertedIDs := make(map[string]struct{})
	for id, newHash := range newSnap {
		oldHash, exists := oldSnap[id]
		if !exists || oldHash != newHash {
			upsertedIDs[id] = struct{}{}
		}
	}

	// Collect root-level tree entries that contain upserted tasks
	if len(upsertedIDs) > 0 {
		upserted = collectTreeEntries(newTasks, upsertedIDs)
	}

	// Find removed: in old but not in new
	for id := range oldSnap {
		if _, exists := newSnap[id]; !exists {
			removed = append(removed, id)
		}
	}

	if len(upserted) == 0 && len(removed) == 0 {
		return nil, newSnap
	}

	return &TasksDelta{
		Upserted: upserted,
		Removed:  removed,
		Meta:     meta,
	}, newSnap
}

// collectTreeEntries returns root-level tree entries that contain any of the given IDs.
func collectTreeEntries(tasks []*todoist.Task, ids map[string]struct{}) []*todoist.Task {
	var result []*todoist.Task
	for _, t := range tasks {
		if treeContainsAny(t, ids) {
			result = append(result, t)
		}
	}
	return result
}

func treeContainsAny(t *todoist.Task, ids map[string]struct{}) bool {
	if _, ok := ids[t.ID]; ok {
		return true
	}
	for _, c := range t.Children {
		if treeContainsAny(c, ids) {
			return true
		}
	}
	return false
}

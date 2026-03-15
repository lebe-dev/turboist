package storage

// PinnedTask represents a pinned task with its ID and content.
type PinnedTask struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// UserState represents the full user state stored in the database.
type UserState struct {
	PinnedTasks      []PinnedTask `json:"pinned_tasks"`
	ActiveContextID  string       `json:"active_context_id"`
	ActiveView       string       `json:"active_view"`
	CollapsedIDs     []string     `json:"collapsed_ids"`
	SidebarCollapsed bool         `json:"sidebar_collapsed"`
	PlanningOpen     bool         `json:"planning_open"`
}

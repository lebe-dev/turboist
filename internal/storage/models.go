package storage

// PinnedTask represents a pinned task with its ID and content.
type PinnedTask struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Priority int    `json:"priority,omitempty"`
}

// AllFiltersState stores the filter state for the "all tasks" view.
type AllFiltersState struct {
	SelectedPriorities []int    `json:"selected_priorities"`
	SelectedLabels     []string `json:"selected_labels"`
	LinksOnly          bool     `json:"links_only"`
	FiltersExpanded    bool     `json:"filters_expanded"`
}

// UserState represents the full user state stored in the database.
type UserState struct {
	PinnedTasks         []PinnedTask      `json:"pinned_tasks"`
	ActiveContextID     string            `json:"active_context_id"`
	ActiveView          string            `json:"active_view"`
	CollapsedIDs        []string          `json:"collapsed_ids"`
	SidebarCollapsed    bool              `json:"sidebar_collapsed"`
	PlanningOpen        bool              `json:"planning_open"`
	DayPartNotes        map[string]string `json:"day_part_notes"`
	Locale              string            `json:"locale"`
	AllFilters          *AllFiltersState  `json:"all_filters"`
	BannerText          string            `json:"banner_text"`
	BannerDismissedText string            `json:"banner_dismissed_text"`
}

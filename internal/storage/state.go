package storage

import (
	"encoding/json"
	"fmt"
)

// GetState reads all user state keys from the database and returns a UserState.
func (s *Store) GetState() (*UserState, error) {
	rows, err := s.db.Query("SELECT key, value FROM user_state")
	if err != nil {
		return nil, fmt.Errorf("query state: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	kv := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan state row: %w", err)
		}
		kv[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate state rows: %w", err)
	}

	state := &UserState{}

	if raw, ok := kv["pinned_tasks"]; ok {
		if err := json.Unmarshal([]byte(raw), &state.PinnedTasks); err != nil {
			return nil, fmt.Errorf("unmarshal pinned_tasks: %w", err)
		}
	}
	if state.PinnedTasks == nil {
		state.PinnedTasks = []PinnedTask{}
	}

	state.ActiveContextID = kv["active_context_id"]
	state.ActiveView = kv["active_view"]
	if state.ActiveView == "" {
		state.ActiveView = "all"
	}

	if raw, ok := kv["collapsed_ids"]; ok {
		if err := json.Unmarshal([]byte(raw), &state.CollapsedIDs); err != nil {
			return nil, fmt.Errorf("unmarshal collapsed_ids: %w", err)
		}
	}
	if state.CollapsedIDs == nil {
		state.CollapsedIDs = []string{}
	}

	state.SidebarCollapsed = kv["sidebar_collapsed"] == "true"
	state.PlanningOpen = kv["planning_open"] == "true"

	if raw, ok := kv["day_part_notes"]; ok {
		if err := json.Unmarshal([]byte(raw), &state.DayPartNotes); err != nil {
			return nil, fmt.Errorf("unmarshal day_part_notes: %w", err)
		}
	}
	if state.DayPartNotes == nil {
		state.DayPartNotes = map[string]string{}
	}

	state.Locale = kv["locale"]

	if raw, ok := kv["all_filters"]; ok {
		var af AllFiltersState
		if err := json.Unmarshal([]byte(raw), &af); err != nil {
			return nil, fmt.Errorf("unmarshal all_filters: %w", err)
		}
		state.AllFilters = &af
	}

	state.BannerText = kv["banner_text"]
	state.BannerDismissedText = kv["banner_dismissed_text"]

	if raw, ok := kv["constraint_pool"]; ok {
		if err := json.Unmarshal([]byte(raw), &state.ConstraintPool); err != nil {
			return nil, fmt.Errorf("unmarshal constraint_pool: %w", err)
		}
	}
	if state.ConstraintPool == nil {
		state.ConstraintPool = []string{}
	}

	return state, nil
}

// SetValue upserts a single key-value pair in the user_state table.
func (s *Store) SetValue(key, value string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO user_state (key, value) VALUES (?, ?)",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set state %q: %w", key, err)
	}
	return nil
}

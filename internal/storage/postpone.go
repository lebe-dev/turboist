package storage

import "fmt"

// GetPostponeCounts returns all postpone counts as a map of taskID → count.
func (s *Store) GetPostponeCounts() (map[string]int, error) {
	rows, err := s.db.Query("SELECT task_id, count FROM postpone_counts")
	if err != nil {
		return nil, fmt.Errorf("query postpone counts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]int)
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("scan postpone count: %w", err)
		}
		result[id] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postpone counts: %w", err)
	}
	return result, nil
}

// ResetPostponeCount removes the postpone count for a task (resets to zero).
func (s *Store) ResetPostponeCount(taskID string) error {
	_, err := s.db.Exec("DELETE FROM postpone_counts WHERE task_id = ?", taskID)
	if err != nil {
		return fmt.Errorf("reset postpone count for %s: %w", taskID, err)
	}
	return nil
}

// IncrementPostponeCount atomically increments the postpone count for a task.
func (s *Store) IncrementPostponeCount(taskID string) error {
	_, err := s.db.Exec(
		"INSERT INTO postpone_counts (task_id, count) VALUES (?, 1) ON CONFLICT(task_id) DO UPDATE SET count = count + 1",
		taskID,
	)
	if err != nil {
		return fmt.Errorf("increment postpone count for %s: %w", taskID, err)
	}
	return nil
}

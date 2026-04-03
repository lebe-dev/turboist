package storage

import (
	"fmt"
	"time"
)

// GetAutoRemoveFirstSeen returns all first-seen timestamps as a map keyed by "taskID:label".
func (s *Store) GetAutoRemoveFirstSeen() (map[string]time.Time, error) {
	rows, err := s.db.Query("SELECT task_id, label, first_seen FROM auto_remove_first_seen")
	if err != nil {
		return nil, fmt.Errorf("query auto_remove first_seen: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]time.Time)
	for rows.Next() {
		var taskID, label, ts string
		if err := rows.Scan(&taskID, &label, &ts); err != nil {
			return nil, fmt.Errorf("scan auto_remove first_seen: %w", err)
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, fmt.Errorf("parse auto_remove first_seen %q: %w", ts, err)
		}
		result[taskID+":"+label] = t
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate auto_remove first_seen: %w", err)
	}
	return result, nil
}

// UpsertAutoRemoveFirstSeen records the first time a task was seen with a label.
// Uses INSERT OR IGNORE to preserve the original timestamp.
func (s *Store) UpsertAutoRemoveFirstSeen(taskID, label string, firstSeen time.Time) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO auto_remove_first_seen (task_id, label, first_seen) VALUES (?, ?, ?)",
		taskID, label, firstSeen.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upsert auto_remove first_seen for %s:%s: %w", taskID, label, err)
	}
	return nil
}

// DeleteAutoRemoveFirstSeen removes a single first-seen entry.
func (s *Store) DeleteAutoRemoveFirstSeen(taskID, label string) error {
	_, err := s.db.Exec(
		"DELETE FROM auto_remove_first_seen WHERE task_id = ? AND label = ?",
		taskID, label,
	)
	if err != nil {
		return fmt.Errorf("delete auto_remove first_seen for %s:%s: %w", taskID, label, err)
	}
	return nil
}

// CleanupAutoRemoveFirstSeen removes entries not in the activeKeys set.
// Keys are formatted as "taskID:label".
func (s *Store) CleanupAutoRemoveFirstSeen(activeKeys map[string]bool) error {
	rows, err := s.db.Query("SELECT task_id, label FROM auto_remove_first_seen")
	if err != nil {
		return fmt.Errorf("query auto_remove first_seen for cleanup: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var toDelete [][2]string
	for rows.Next() {
		var taskID, label string
		if err := rows.Scan(&taskID, &label); err != nil {
			return fmt.Errorf("scan auto_remove first_seen for cleanup: %w", err)
		}
		if !activeKeys[taskID+":"+label] {
			toDelete = append(toDelete, [2]string{taskID, label})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate auto_remove first_seen for cleanup: %w", err)
	}

	for _, entry := range toDelete {
		if err := s.DeleteAutoRemoveFirstSeen(entry[0], entry[1]); err != nil {
			return err
		}
	}
	return nil
}

package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// LabelBlock represents a label block entry from the database.
type LabelBlock struct {
	Label     string
	StartedAt time.Time
}

// GetLabelBlocks returns all label block entries.
func (s *Store) GetLabelBlocks() ([]LabelBlock, error) {
	rows, err := s.db.Query("SELECT label, started_at FROM constraints_label_blocks")
	if err != nil {
		return nil, fmt.Errorf("query label blocks: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var blocks []LabelBlock
	for rows.Next() {
		var label, ts string
		if err := rows.Scan(&label, &ts); err != nil {
			return nil, fmt.Errorf("scan label block: %w", err)
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, fmt.Errorf("parse label block started_at %q: %w", ts, err)
		}
		blocks = append(blocks, LabelBlock{Label: label, StartedAt: t})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate label blocks: %w", err)
	}
	return blocks, nil
}

// UpsertLabelBlock inserts a label block or ignores if it already exists (preserving the original started_at).
func (s *Store) UpsertLabelBlock(label string, startedAt time.Time) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO constraints_label_blocks (label, started_at) VALUES (?, ?)",
		label, startedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upsert label block %q: %w", label, err)
	}
	return nil
}

// DeleteLabelBlock removes a single label block entry.
func (s *Store) DeleteLabelBlock(label string) error {
	_, err := s.db.Exec("DELETE FROM constraints_label_blocks WHERE label = ?", label)
	if err != nil {
		return fmt.Errorf("delete label block %q: %w", label, err)
	}
	return nil
}

// DeleteUnconfiguredLabelBlocks removes entries for labels not in the given set of configured labels.
func (s *Store) DeleteUnconfiguredLabelBlocks(configuredLabels []string) error {
	blocks, err := s.GetLabelBlocks()
	if err != nil {
		return err
	}

	configured := make(map[string]bool, len(configuredLabels))
	for _, l := range configuredLabels {
		configured[l] = true
	}

	for _, b := range blocks {
		if !configured[b.Label] {
			if err := s.DeleteLabelBlock(b.Label); err != nil {
				return err
			}
		}
	}
	return nil
}

// DailyConstraintsState represents the JSON-serialized daily constraints state.
type DailyConstraintsState struct {
	Date        string   `json:"date"`
	Items       []string `json:"items"`
	RerollsUsed int      `json:"rerolls_used"`
	Confirmed   bool     `json:"confirmed"`
}

// GetDailyConstraints reads the daily_constraints key from user_state.
func (s *Store) GetDailyConstraints() (*DailyConstraintsState, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM user_state WHERE key = 'daily_constraints'").Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query daily_constraints: %w", err)
	}
	var state DailyConstraintsState
	if err := json.Unmarshal([]byte(value), &state); err != nil {
		return nil, fmt.Errorf("unmarshal daily_constraints: %w", err)
	}
	return &state, nil
}

// SetDailyConstraints saves the daily constraints state to user_state.
func (s *Store) SetDailyConstraints(state *DailyConstraintsState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal daily_constraints: %w", err)
	}
	return s.SetValue("daily_constraints", string(data))
}

// PostponeBudgetState represents the JSON-serialized postpone budget state.
type PostponeBudgetState struct {
	Date string `json:"date"`
	Used int    `json:"used"`
}

// GetPostponeBudget reads the postpone_budget key from user_state.
func (s *Store) GetPostponeBudget() (*PostponeBudgetState, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM user_state WHERE key = 'postpone_budget'").Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query postpone_budget: %w", err)
	}
	var state PostponeBudgetState
	if err := json.Unmarshal([]byte(value), &state); err != nil {
		return nil, fmt.Errorf("unmarshal postpone_budget: %w", err)
	}
	return &state, nil
}

// SetPostponeBudget saves the postpone budget state to user_state.
func (s *Store) SetPostponeBudget(state *PostponeBudgetState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal postpone_budget: %w", err)
	}
	return s.SetValue("postpone_budget", string(data))
}

// GetConstraintPool reads the constraint_pool key from user_state.
func (s *Store) GetConstraintPool() ([]string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM user_state WHERE key = 'constraint_pool'").Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query constraint_pool: %w", err)
	}
	var pool []string
	if err := json.Unmarshal([]byte(value), &pool); err != nil {
		return nil, fmt.Errorf("unmarshal constraint_pool: %w", err)
	}
	return pool, nil
}

// SetConstraintPool saves the constraint pool to user_state.
func (s *Store) SetConstraintPool(pool []string) error {
	data, err := json.Marshal(pool)
	if err != nil {
		return fmt.Errorf("marshal constraint_pool: %w", err)
	}
	return s.SetValue("constraint_pool", string(data))
}

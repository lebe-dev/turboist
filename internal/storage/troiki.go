package storage

import "fmt"

// GetAllTroikiCapacity returns all troiki capacity values as a map of sectionClass → capacity.
func (s *Store) GetAllTroikiCapacity() (map[string]int, error) {
	rows, err := s.db.Query("SELECT section_class, capacity FROM troiki_capacity")
	if err != nil {
		return nil, fmt.Errorf("query troiki capacity: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]int)
	for rows.Next() {
		var class string
		var capacity int
		if err := rows.Scan(&class, &capacity); err != nil {
			return nil, fmt.Errorf("scan troiki capacity: %w", err)
		}
		result[class] = capacity
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate troiki capacity: %w", err)
	}
	return result, nil
}

// IncrementTroikiCapacity atomically increments the capacity for a section class by 1.
func (s *Store) IncrementTroikiCapacity(sectionClass string) error {
	_, err := s.db.Exec(
		"INSERT INTO troiki_capacity (section_class, capacity) VALUES (?, 1) ON CONFLICT(section_class) DO UPDATE SET capacity = capacity + 1",
		sectionClass,
	)
	if err != nil {
		return fmt.Errorf("increment troiki capacity for %s: %w", sectionClass, err)
	}
	return nil
}

// DecrementTroikiCapacity atomically decrements the capacity for a section class by 1, flooring at 0.
func (s *Store) DecrementTroikiCapacity(sectionClass string) error {
	_, err := s.db.Exec(
		"UPDATE troiki_capacity SET capacity = MAX(0, capacity - 1) WHERE section_class = ?",
		sectionClass,
	)
	if err != nil {
		return fmt.Errorf("decrement troiki capacity for %s: %w", sectionClass, err)
	}
	return nil
}

// EnsureMinTroikiCapacity sets capacity to at least min, never decreasing an existing value.
func (s *Store) EnsureMinTroikiCapacity(sectionClass string, min int) error {
	_, err := s.db.Exec(
		`INSERT INTO troiki_capacity (section_class, capacity) VALUES (?, ?)
		 ON CONFLICT(section_class) DO UPDATE SET capacity = MAX(capacity, excluded.capacity)`,
		sectionClass, min,
	)
	if err != nil {
		return fmt.Errorf("ensure min troiki capacity for %s: %w", sectionClass, err)
	}
	return nil
}

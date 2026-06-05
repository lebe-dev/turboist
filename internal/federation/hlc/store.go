package hlc

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
)

// Store is the SQLite-backed clock state. It owns the single hlc_state row (id=1)
// and advances it on every local event via Now().
//
// Concurrency: the app opens SQLite with SetMaxOpenConns(1), so the pool already
// serialises writers — the read-modify-write of hlc_state inside one tx is
// race-free without a second mutex (§3 concurrency DEVIATE row).
//
// now is injectable so tests can pin/advance the wall clock deterministically;
// production leaves it as time.Now. The physical_ms Now() mints is exactly
// now().UnixMilli(), so it equals the updated_at ms written by the same instant.
type Store struct {
	db     *sql.DB
	nodeID string
	now    func() time.Time
}

// NewStore constructs a clock store bound to db and this instance's stable
// install nodeID (federation_keys.node_id — R10).
func NewStore(database *sql.DB, nodeID string) *Store {
	return &Store{db: database, nodeID: nodeID, now: time.Now}
}

// WithClock overrides the wall-clock source used by Now() so callers (the
// in-process federation integration harness, F7.1) can pin or advance an
// instance's clock deterministically — the clock injection that must reach the
// HLC, the transport timestamp, and the skew checks. A nil clock leaves the
// store on time.Now. Returns the store for chaining.
func (s *Store) WithClock(now func() time.Time) *Store {
	if now == nil {
		return s
	}
	s.now = now
	return s
}

// Now advances the local clock for a local event and returns the new HLC,
// persisting the advance to hlc_state in one transaction (§5.3 local rule). The
// returned physical_ms equals now().UnixMilli() (or the last physical when the
// wall clock has not advanced) so it never drifts from updated_at (R11).
func (s *Store) Now(ctx context.Context) (HLC, error) {
	wallMS := s.now().UnixMilli()
	var out HLC
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		prev, err := s.loadTx(ctx, tx)
		if err != nil {
			return err
		}
		out = LocalTick(prev, wallMS)
		out.NodeID = s.nodeID
		return s.saveTx(ctx, tx, out)
	})
	if err != nil {
		return HLC{}, fmt.Errorf("hlc Now: %w", err)
	}
	return out, nil
}

// loadTx reads the singleton clock row, returning a zero clock (physical 0) when
// it does not yet exist. NodeID on the returned value is this store's node.
func (s *Store) loadTx(ctx context.Context, tx *sql.Tx) (HLC, error) {
	var phys, logical int64
	err := tx.QueryRowContext(ctx,
		`SELECT last_physical_ms, last_logical FROM hlc_state WHERE id = 1`).Scan(&phys, &logical)
	if err == sql.ErrNoRows {
		return HLC{NodeID: s.nodeID}, nil
	}
	if err != nil {
		return HLC{}, fmt.Errorf("load hlc_state: %w", err)
	}
	return HLC{PhysicalMS: phys, Logical: logical, NodeID: s.nodeID}, nil
}

// saveTx upserts the singleton clock row to the new physical/logical, recording
// the install node_id alongside (a stable copy for backup/debugging).
func (s *Store) saveTx(ctx context.Context, tx *sql.Tx, h HLC) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO hlc_state (id, last_physical_ms, last_logical, node_id)
		 VALUES (1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   last_physical_ms = excluded.last_physical_ms,
		   last_logical = excluded.last_logical,
		   node_id = excluded.node_id`,
		h.PhysicalMS, h.Logical, s.nodeID)
	if err != nil {
		return fmt.Errorf("save hlc_state: %w", err)
	}
	return nil
}

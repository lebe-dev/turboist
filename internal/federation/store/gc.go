package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/logging"
)

// Retention GC reads/writes for the federation tombstone + queue cleanup
// (Federation v1 F3.3, US-3.7 AC5). The daily gc goroutine (internal/federation/
// gc) drives these: hard-DELETE tombstones older than the tombstone retention
// window, purge aged outbox/inbox rows once they are no longer needed for
// recovery/dedup. All cutoffs are TEXT ISO-8601 UTC (model.FormatUTC) so the
// comparison is a plain lexical string compare — the same fixed-width ordering
// the wire timestamps use.
//
// OldestRetainedHLC backs the stale-pull 410 emit (US-3.7 AC4 emit half): once
// events have been GC'd, a peer whose since_hlc predates the oldest retained
// event must be told to re-snapshot rather than silently miss the pruned changes.

// tombstoneEntityType maps a federated SQL table to the entity_type used in
// entity_field_hlc, so hard-deleting a tombstone also prunes its per-field HLC
// sidecar (no dangling resurrection guard for a row that no longer exists).
var tombstoneEntityType = map[string]string{
	"tasks":            "task",
	"projects":         "project",
	"project_sections": "section",
	"comments":         "comment",
	"checklist_items":  "checklist_item",
}

// DeleteTombstonesOlderThan hard-DELETEs rows of table whose deleted_at predates
// cutoff (a tombstone past the retention window — US-3.7 AC5) and prunes the
// matching entity_field_hlc rows so no orphan resurrection guard survives the
// row. A live (deleted_at IS NULL) row is never touched. table must be one of the
// federated entity tables; an unknown table is a programming error. Returns the
// number of tombstones removed.
func (s *Store) DeleteTombstonesOlderThan(ctx context.Context, table, cutoff string) (int64, error) {
	const op = "store.DeleteTombstonesOlderThan"
	entityType, ok := tombstoneEntityType[table]
	if !ok {
		return 0, fmt.Errorf("%s: unknown federated table %q", op, table)
	}

	// Prune the per-field HLC sidecar for the entities about to be hard-deleted
	// FIRST (the client_id is still resolvable), then delete the rows. Both run in
	// ONE transaction so a crash between the two statements can never leave a
	// half-pruned state (NFR-2 crash-safety) — either both commit or neither does.
	var n int64
	if err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM entity_field_hlc
				          WHERE entity_type = ?
				            AND entity_id IN (SELECT client_id FROM %s WHERE deleted_at IS NOT NULL AND deleted_at < ? AND client_id IS NOT NULL)`, table),
			entityType, cutoff); err != nil {
			return fmt.Errorf("prune field_hlc: %w", err)
		}
		res, err := tx.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE deleted_at IS NOT NULL AND deleted_at < ?`, table), cutoff)
		if err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		n, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows: %w", err)
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return n, nil
}

// PurgeOutboxOlderThan hard-DELETEs federation_outbox rows created before cutoff.
// The outbox is the recovery + pull-replay buffer; once a row ages past the
// retention window it is dropped (US-3.7 AC5 / §16.3 outbox hardcap 30d). Returns
// the number of rows purged.
//
// Prefer PurgeOutboxOlderThanAdvancingFloor on the GC path: this raw purge does
// NOT record the pruned-floor high-water mark, so on its own it can leave the
// stale-pull 410 gate without a durable record of what aged out. It is retained
// for callers (and tests) that only need the row cleanup.
func (s *Store) PurgeOutboxOlderThan(ctx context.Context, cutoff string) (int64, error) {
	const op = "store.PurgeOutboxOlderThan"
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM federation_outbox WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s rows: %w", op, err)
	}
	return n, nil
}

// PurgeOutboxOlderThanAdvancingFloor purges aged outbox rows AND, per project,
// advances a durable pruned-floor HLC high-water mark to the max event HLC of the
// rows it removed (Federation v1 F3.3, US-3.7 AC4 review fix). The stale-pull 410
// gate then keys off that persisted floor instead of the transient presence of
// outbox rows, so a long-quiet federated project whose outbox has been GC'd
// entirely still returns 410 (re-snapshot) to a peer whose cursor predates the
// pruned events — rather than falsely reporting "caught up" with a 200 + empty
// body.
//
// The floor advance and the row delete run in ONE transaction PER PROJECT so a
// crash never leaves the floor advanced past rows that were not actually removed
// (NFR-2 crash-safety). The floor is monotonic: it never moves backwards. Returns
// the total number of rows purged across all projects.
func (s *Store) PurgeOutboxOlderThanAdvancingFloor(ctx context.Context, cutoff, now string) (int64, error) {
	const op = "store.PurgeOutboxOlderThanAdvancingFloor"

	// Group the to-be-purged rows by project up front (read-only), computing the
	// max event HLC per project. We then delete + advance the floor per project in
	// its own short tx (the pool serialises writers on SetMaxOpenConns(1)).
	maxByProject, err := s.purgeCandidatesMaxHLC(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("%s candidates: %w", op, err)
	}
	if len(maxByProject) == 0 {
		return 0, nil
	}

	var total int64
	for projectID, maxHLC := range maxByProject {
		if err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
			if maxHLC != "" {
				if _, err := advancePrunedFloorTx(ctx, tx, projectID, maxHLC, now); err != nil {
					return err
				}
			}
			res, err := tx.ExecContext(ctx,
				`DELETE FROM federation_outbox WHERE local_project_id = ? AND created_at < ?`,
				projectID, cutoff)
			if err != nil {
				return fmt.Errorf("delete: %w", err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("rows: %w", err)
			}
			total += n
			return nil
		}); err != nil {
			return total, fmt.Errorf("%s project %d: %w", op, projectID, err)
		}
	}
	return total, nil
}

// purgeCandidatesMaxHLC scans the outbox rows that PurgeOutboxOlderThanAdvancingFloor
// is about to delete (created_at < cutoff) and returns, per project, the lexically
// greatest event HLC among them — the value the project's pruned floor advances to.
func (s *Store) purgeCandidatesMaxHLC(ctx context.Context, cutoff string) (map[int64]string, error) {
	const op = "store.purgeCandidatesMaxHLC"
	rows, err := s.db.QueryContext(ctx,
		`SELECT local_project_id, payload FROM federation_outbox WHERE created_at < ?`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := map[int64]string{}
	for rows.Next() {
		var projectID int64
		var payload string
		if err := rows.Scan(&projectID, &payload); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		maxHLC := payloadMaxHLC(payload)
		if _, seen := out[projectID]; !seen {
			out[projectID] = maxHLC
			continue
		}
		if hlc.CompareString(maxHLC, out[projectID]) > 0 {
			out[projectID] = maxHLC
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", op, err)
	}
	return out, nil
}

// PrunedFloorHLC returns the project's durable pruned-floor high-water mark — the
// greatest event HLC the retention GC has ever purged from its outbox — or the
// empty string when nothing has been pruned. A pull whose since_hlc is strictly
// LESS than this floor has missed GC'd events and must re-snapshot (US-3.7 AC4).
func (s *Store) PrunedFloorHLC(ctx context.Context, localProjectID int64) (string, error) {
	const op = "store.PrunedFloorHLC"
	var floor string
	err := s.db.QueryRowContext(ctx,
		`SELECT floor_hlc FROM federation_pruned_floor WHERE local_project_id = ?`, localProjectID).
		Scan(&floor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	return floor, nil
}

// AdvancePrunedFloor sets the project's pruned-floor high-water mark to floorHLC
// when that is strictly greater than the current floor (monotonic; never moves
// backwards). It is exported for tests / future callers; the GC advances the floor
// inline as part of PurgeOutboxOlderThanAdvancingFloor's per-project tx.
func (s *Store) AdvancePrunedFloor(ctx context.Context, localProjectID int64, floorHLC, now string) (bool, error) {
	const op = "store.AdvancePrunedFloor"
	var advanced bool
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var aerr error
		advanced, aerr = advancePrunedFloorTx(ctx, tx, localProjectID, floorHLC, now)
		return aerr
	})
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	return advanced, nil
}

// advancePrunedFloorTx upserts the per-project pruned floor inside a caller tx,
// advancing it only when floorHLC is strictly greater than the stored value
// (lexical compare of the zero-padded HLC). Returns whether the floor moved.
func advancePrunedFloorTx(ctx context.Context, tx *sql.Tx, localProjectID int64, floorHLC, now string) (bool, error) {
	var prior string
	err := tx.QueryRowContext(ctx,
		`SELECT floor_hlc FROM federation_pruned_floor WHERE local_project_id = ?`, localProjectID).
		Scan(&prior)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("read pruned floor %d: %w", localProjectID, err)
	}
	if hlc.CompareString(floorHLC, prior) <= 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO federation_pruned_floor (local_project_id, floor_hlc, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(local_project_id) DO UPDATE SET floor_hlc = excluded.floor_hlc, updated_at = excluded.updated_at`,
		localProjectID, floorHLC, now); err != nil {
		return false, fmt.Errorf("advance pruned floor %d: %w", localProjectID, err)
	}
	return true, nil
}

// PurgeAppliedInboxOlderThan hard-DELETEs federation_inbox rows that are BOTH
// applied (applied_at IS NOT NULL — terminal) AND received before cutoff. An
// un-applied (applied_at NULL) row is NEVER purged: the inbox queue still
// re-drives it (NFR-2 at-least-once), so dropping it would strand the event. The
// inbox is only a dedup window for applied events; once aged it is safe to drop
// (a redelivery of an aged event_id is harmless — per-field LWW is idempotent).
// Returns the number of rows purged.
func (s *Store) PurgeAppliedInboxOlderThan(ctx context.Context, cutoff string) (int64, error) {
	const op = "store.PurgeAppliedInboxOlderThan"
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM federation_inbox WHERE applied_at IS NOT NULL AND received_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s rows: %w", op, err)
	}
	return n, nil
}

// OldestRetainedHLC returns the SMALLEST per-event max-field HLC across a
// project's retained outbox events — the lower boundary of what a pull can still
// replay (US-3.7 AC4 emit half). A pull whose since_hlc is strictly LESS than
// this boundary has missed events that the retention GC already pruned, so the
// handler answers 410 (re-snapshot). With no retained events it returns the empty
// string, which the handler treats as "nothing pruned" (no false 410).
func (s *Store) OldestRetainedHLC(ctx context.Context, localProjectID int64) (string, error) {
	oldest, _, err := s.retainedHLCBounds(ctx, localProjectID)
	return oldest, err
}

// HeadRetainedHLC returns the GREATEST per-event max-field HLC across a project's
// retained outbox events — the as_of_hlc reported in a stale-pull 410 body so the
// re-snapshotting peer knows the cutoff it is catching up to (US-3.7 AC4). Empty
// when no events are retained.
func (s *Store) HeadRetainedHLC(ctx context.Context, localProjectID int64) (string, error) {
	_, head, err := s.retainedHLCBounds(ctx, localProjectID)
	return head, err
}

// retainedHLCBounds returns the (min, max) per-event max-field HLC across a
// project's retained outbox events in a single scan.
func (s *Store) retainedHLCBounds(ctx context.Context, localProjectID int64) (oldest, head string, err error) {
	const op = "store.retainedHLCBounds"
	rows, err := s.db.QueryContext(ctx,
		`SELECT event_id, payload FROM federation_outbox WHERE local_project_id = ?`, localProjectID)
	if err != nil {
		return "", "", fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	for rows.Next() {
		var eventID, payload string
		if err := rows.Scan(&eventID, &payload); err != nil {
			return "", "", fmt.Errorf("%s scan: %w", op, err)
		}
		maxHLC, decodeErr := payloadMaxHLCErr(payload)
		if decodeErr != nil {
			// An undecodable stored payload is skipped from the cursor bounds rather
			// than failing the whole stale-pull gate (forward-compat / corruption
			// resilience), but surface it at WARN so a silently-poisoned outbox row is
			// diagnosable (US-3.7 AC4 follow-up).
			logging.FromContext(ctx).WarnContext(ctx, "federation: skip undecodable outbox payload in retained-hlc bounds",
				slog.String("op", op),
				slog.String("event_id", eventID),
				slog.Int64("local_project_id", localProjectID),
				slog.String("err", decodeErr.Error()),
			)
			continue
		}
		if maxHLC == "" {
			continue
		}
		if oldest == "" || hlc.CompareString(maxHLC, oldest) < 0 {
			oldest = maxHLC
		}
		if hlc.CompareString(maxHLC, head) > 0 {
			head = maxHLC
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("%s rows: %w", op, err)
	}
	return oldest, head, nil
}

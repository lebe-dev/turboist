// Package store holds the SQLite queries for the federation sync core
// (Federation v1 F3.1): the per-field HLC compare-and-set that drives per-field
// Last-Writer-Wins, plus the outbox/inbox row writers.
//
// The per-field LWW ordering is a plain LEXICAL compare of the zero-padded HLC
// strings (hlc.CompareString) — the same order the snapshot apply and the pull
// cursor use. On SetMaxOpenConns(1) the pool already serialises writers, so the
// read-modify-write in CASFieldHLC is race-free without a second mutex (§3
// concurrency DEVIATE row).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lebe-dev/turboist/internal/federation/hlc"
)

// ErrMalformedHLC is returned by the CAS when an incoming HLC string does not
// parse as a canonical zero-padded HLC. It is a PERMANENT (do-not-retry) reject:
// the per-event skew validator (internal/federation/inbox, F3.2a) is the intended
// single gate for HLC well-formedness, so a malformed HLC reaching the store is
// defense-in-depth — never store it. Persisting unparseable garbage as a field's
// authoritative HLC would permanently poison per-field LWW for that field (no
// well-formed future write could ever win the lexical CAS again). The inbox
// apply path classifies this as a poison error so the event is dropped rather
// than retried forever.
var ErrMalformedHLC = errors.New("store: malformed incoming HLC")

// Querier is the subset of *sql.DB / *sql.Tx the store needs, so every method
// can run either on its own connection or inside a caller's transaction (the
// transactional-emit path runs CAS inside the same tx as the domain write).
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store is the SQLite-backed federation sync store.
type Store struct {
	db *sql.DB
}

// New constructs a Store bound to db.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// CASFieldHLC compare-and-sets the HLC of one field of a federated entity. It
// returns applied=true when incoming is strictly greater than the stored HLC
// (lexical compare of the zero-padded canonical strings, which equals the HLC
// total order physical→logical→node_id), in which case the row is advanced to
// incoming. A lower-or-equal incoming HLC is a no-op (the stale field is ignored,
// US-3.3 AC2) and applied=false. A field with no prior row is always newer.
//
// It is implemented as a serialized read-modify-write — a SELECT of the prior
// HLC followed by a conditional INSERT ... ON CONFLICT — NOT a single guarded SQL
// statement. The two steps are race-free because SetMaxOpenConns(1) serializes
// all writers (§3 concurrency DEVIATE row): no other writer can interleave
// between the read and the write. The read-then-decide shape is deliberate — a
// single UPSERT cannot distinguish a strictly-greater incoming HLC (apply) from
// an equal one (idempotent redelivery / same-write tie, which must NOT re-apply,
// US-3.3 AC2), so the strict-greater decision is made in Go against the prior.
func (s *Store) CASFieldHLC(ctx context.Context, entityType, entityID, field, incoming string) (bool, error) {
	return casFieldHLC(ctx, s.db, entityType, entityID, field, incoming)
}

// CASFieldHLCTx is CASFieldHLC inside a caller transaction.
func (s *Store) CASFieldHLCTx(ctx context.Context, tx Querier, entityType, entityID, field, incoming string) (bool, error) {
	return casFieldHLC(ctx, tx, entityType, entityID, field, incoming)
}

func casFieldHLC(ctx context.Context, q Querier, entityType, entityID, field, incoming string) (bool, error) {
	// Defense-in-depth: never store an unparseable HLC. The F3.2a skew validator
	// is the intended gate for HLC well-formedness, but a future bypass must not be
	// able to silently poison the field-HLC table — garbage that sorts above all
	// well-formed HLCs would win this lexical CAS and lock the field forever. Reject
	// it here as a permanent (do-not-retry) error rather than persisting it.
	if _, err := hlc.Parse(incoming); err != nil {
		return false, fmt.Errorf("%w: %s/%s/%s %q: %v", ErrMalformedHLC, entityType, entityID, field, incoming, err)
	}

	// Read the prior HLC, then conditionally advance. On SetMaxOpenConns(1) the
	// pool serialises writers, so this read-modify-write is race-free. The applied
	// decision is "incoming is STRICTLY greater than prior" — an equal HLC (an
	// idempotent redelivery, or a same-write tie) must NOT re-apply (US-3.3 AC2),
	// which a single UPSERT+RETURNING cannot distinguish from a genuine advance.
	var prior string
	err := q.QueryRowContext(ctx,
		`SELECT hlc FROM entity_field_hlc WHERE entity_type = ? AND entity_id = ? AND field_name = ?`,
		entityType, entityID, field).Scan(&prior)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("read field_hlc %s/%s/%s: %w", entityType, entityID, field, err)
	}
	if incoming <= prior {
		// Lower-or-equal: stale (or idempotent). Leave the stored hlc untouched.
		return false, nil
	}
	if _, err := q.ExecContext(ctx,
		`INSERT INTO entity_field_hlc (entity_type, entity_id, field_name, hlc)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(entity_type, entity_id, field_name) DO UPDATE SET hlc = excluded.hlc`,
		entityType, entityID, field, incoming); err != nil {
		return false, fmt.Errorf("cas field_hlc %s/%s/%s: %w", entityType, entityID, field, err)
	}
	return true, nil
}

// GetFieldHLC returns the stored HLC for a field, or the empty string when no
// row exists yet (the create-on-missing path treats empty as "always older").
func (s *Store) GetFieldHLC(ctx context.Context, entityType, entityID, field string) (string, error) {
	var hlc string
	err := s.db.QueryRowContext(ctx,
		`SELECT hlc FROM entity_field_hlc WHERE entity_type = ? AND entity_id = ? AND field_name = ?`,
		entityType, entityID, field).Scan(&hlc)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get field_hlc %s/%s/%s: %w", entityType, entityID, field, err)
	}
	return hlc, nil
}

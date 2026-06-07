package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/logging"
)

// Pull / recovery cursor reads over federated_projects (Federation v1 F4.1,
// US-4.1). The recovery loop (internal/federation/recovery) enumerates the
// JOINED peers it should pull from (ListPullTargets), issues a signed GET from
// each peer's last_received_hlc cursor, applies the returned events through the
// same inbox path push uses, and advances the cursor (AdvanceLastReceivedHLC)
// only after the batch is durably recorded. Both reads run on the store's own
// connection so the loop can release it before any network I/O (R1 — never hold
// the lone connection across a peer GET).

// PullTarget is one (joined peer, project) the recovery loop pulls catch-up
// events from: the local project id, the peer's federation URL, the peer's
// remote project id (the :id segment in the peer's pull route), and the
// last_received_hlc cursor the loop resumes the pull from.
type PullTarget struct {
	LocalProjectID  int64
	PeerInstanceURL string
	RemoteProjectID string
	LastReceivedHLC string
}

// ListPullTargets returns every JOINED (is_owner=0), non-revoked, non-paused,
// non-lost federated_projects row whose parent project is live (not soft-deleted),
// each carrying its remote_project_id + last_received_hlc cursor. It is the
// recovery loop's pull-scope read (US-4.1): the owner self-row (is_owner=1) is
// never a target (the owner does not pull from itself); a revoked peer (trust
// terminated) and a paused peer (events accumulate, no pull) are both excluded —
// mirroring the publisher's PeersForProject fan-out filter on the outbound side. A
// LOST copy (lost=1 — the joiner voluntarily LEFT per F5.5/US-6.3, or was revoked,
// or its owner died) is excluded too: its trust link is severed and it is now a
// plain local project, so pulling from it would keep catching it up and, on a 410
// stale pull, re-bootstrap it — silently resurrecting the federation the user
// removed. A tombstoned parent project is excluded so a soft-deleted project is
// never re-bootstrapped from its peer.
func (s *Store) ListPullTargets(ctx context.Context) ([]PullTarget, error) {
	const op = "store.ListPullTargets"
	rows, err := s.db.QueryContext(ctx,
		`SELECT fp.local_project_id, fp.peer_instance_url, fp.remote_project_id, fp.last_received_hlc
		   FROM federated_projects fp
		   JOIN projects p ON p.id = fp.local_project_id AND p.deleted_at IS NULL
		  WHERE fp.is_owner = 0 AND fp.revoked = 0 AND fp.paused = 0 AND fp.lost = 0
		  ORDER BY fp.local_project_id ASC, fp.peer_instance_url ASC`)
	if err != nil {
		return nil, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]PullTarget, 0)
	for rows.Next() {
		var tgt PullTarget
		var lastRecv sql.NullString
		if err := rows.Scan(&tgt.LocalProjectID, &tgt.PeerInstanceURL, &tgt.RemoteProjectID, &lastRecv); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		tgt.LastReceivedHLC = lastRecv.String
		out = append(out, tgt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", op, err)
	}
	return out, nil
}

// AdvanceLastReceivedHLC moves a (project, peer) row's last_received_hlc cursor
// forward to toHLC, MONOTONICALLY: a lower-or-equal HLC is a no-op so the cursor
// never rewinds (US-4.1 — cursor monotonic). The recovery loop calls this only
// AFTER a pulled batch is durably recorded (federation_inbox), so a partial /
// failed apply leaves the cursor where it was and the same range is re-pulled
// next pass (the F4.1 partial-apply-must-not-advance risk). The compare is the
// same lexical HLC total order the per-field CAS and the pull read use, with the
// empty (NULL) cursor sorting first so a fresh peer always advances.
func (s *Store) AdvanceLastReceivedHLC(ctx context.Context, localProjectID int64, peerInstanceURL, toHLC string) error {
	const op = "store.AdvanceLastReceivedHLC"
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s begin: %w", op, err)
	}
	defer func() { _ = tx.Rollback() }()

	var cur sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT last_received_hlc FROM federated_projects WHERE local_project_id = ? AND peer_instance_url = ?`,
		localProjectID, peerInstanceURL).Scan(&cur)
	if err == sql.ErrNoRows {
		return nil // the row vanished (revoke/leave) — nothing to advance.
	}
	if err != nil {
		return fmt.Errorf("%s read cursor: %w", op, err)
	}
	if hlc.CompareString(toHLC, cur.String) <= 0 {
		return nil // not strictly greater — keep the cursor (monotonic).
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE federated_projects SET last_received_hlc = ? WHERE local_project_id = ? AND peer_instance_url = ?`,
		toHLC, localProjectID, peerInstanceURL); err != nil {
		return fmt.Errorf("%s update cursor: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s commit: %w", op, err)
	}
	return nil
}

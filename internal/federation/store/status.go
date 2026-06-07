package store

import (
	"context"
	"fmt"

	"github.com/lebe-dev/turboist/internal/logging"
)

// OverduePendingCount counts the federation_outbox EVENTS (changes) of a project
// that are OVERDUE — created BEFORE cutoff (the >5min "pending" window, Federation
// v1 F4.3, US-4.3 AC2) and still undelivered to AT LEAST ONE of the given active
// peers. It counts EVENTS, not peers: an event owed to two peers is one pending
// change (counted once), matching the "N changes pending" badge / DTO / API.md /
// i18n wording. A peer that has received every overdue event contributes nothing;
// an event delivered to every active peer is not counted.
//
// It reads the project's outbox rows on the store's own connection (no network I/O,
// R1). The cutoff is a TEXT ISO-8601 UTC timestamp (created_at uses the same
// format), so the comparison is a plain lexical compare — valid because
// model.FormatUTC is fixed-width and zero-padded. An empty peers slice
// short-circuits to 0 (nothing is owed when there is no active delivery target).
func (s *Store) OverduePendingCount(ctx context.Context, localProjectID int64, cutoff string, peers []string) (int, error) {
	const op = "store.OverduePendingCount"
	if len(peers) == 0 {
		return 0, nil
	}

	// Only rows OLDER than the cutoff are overdue; a fresh event still inside the
	// push budget must not flip the badge yellow (US-4.3 AC2).
	rows, err := s.db.QueryContext(ctx,
		`SELECT delivered_to FROM federation_outbox WHERE local_project_id = ? AND created_at < ?`,
		localProjectID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	count := 0
	for rows.Next() {
		var deliveredTo string
		if err := rows.Scan(&deliveredTo); err != nil {
			return 0, fmt.Errorf("%s scan: %w", op, err)
		}
		for _, peer := range peers {
			if !deliveredToHas(deliveredTo, peer) {
				count++ // owed to at least one active peer → one overdue change.
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("%s rows: %w", op, err)
	}
	return count, nil
}

// OutboxDepth counts federation_outbox rows that are still pending delivery to at
// least one ACTIVE (non-revoked, non-owner) peer of their project (Federation v1
// F6.5, US-8.2 AC1 — the federation_outbox_depth gauge + /health source). A row
// fully delivered to every active peer of its project is NOT counted; a row whose
// only undelivered targets are the owner self-row or revoked peers is NOT counted
// either (they are never delivery targets). A project with NO active peer never
// contributes (nothing is owed).
//
// It runs entirely on the store's own connection (no network I/O, R1). The active
// peer set is read once per project and cached, then each outbox row is checked
// against its project's active peers using the same delivered_to membership test
// the publisher uses, so the depth matches the publisher's pending definition.
func (s *Store) OutboxDepth(ctx context.Context) (int, error) {
	const op = "store.OutboxDepth"

	// Active (non-owner, non-revoked) peer urls per project.
	peerRows, err := s.db.QueryContext(ctx,
		`SELECT local_project_id, peer_instance_url FROM federated_projects
		  WHERE is_owner = 0 AND revoked = 0`)
	if err != nil {
		return 0, fmt.Errorf("%s peers query: %w", op, err)
	}
	activePeers := make(map[int64][]string)
	for peerRows.Next() {
		var pid int64
		var url string
		if err := peerRows.Scan(&pid, &url); err != nil {
			_ = peerRows.Close()
			return 0, fmt.Errorf("%s peers scan: %w", op, err)
		}
		activePeers[pid] = append(activePeers[pid], url)
	}
	if err := peerRows.Err(); err != nil {
		_ = peerRows.Close()
		return 0, fmt.Errorf("%s peers rows: %w", op, err)
	}
	if err := peerRows.Close(); err != nil {
		return 0, fmt.Errorf("%s peers close: %w", op, err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT local_project_id, delivered_to FROM federation_outbox`)
	if err != nil {
		return 0, fmt.Errorf("%s outbox query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	depth := 0
	for rows.Next() {
		var pid int64
		var deliveredTo string
		if err := rows.Scan(&pid, &deliveredTo); err != nil {
			return 0, fmt.Errorf("%s outbox scan: %w", op, err)
		}
		for _, peer := range activePeers[pid] {
			if !deliveredToHas(deliveredTo, peer) {
				depth++
				break // one undelivered active peer is enough to count the row once.
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("%s outbox rows: %w", op, err)
	}
	return depth, nil
}

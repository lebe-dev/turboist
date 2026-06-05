package federation

import (
	"bytes"
	"context"
	"fmt"

	"github.com/lebe-dev/turboist/internal/federation/snapshot"
)

// ReBootstrapResult is the joiner-side outcome of consuming a 410 stale-pull
// (Federation v1 F4.2, US-4.2). CutoffHLC is the snapshot's as_of_hlc (the causal
// cutoff X); RebootstrappedAt is the wall-clock the overwrite committed at — the
// human-readable X the re-sync banner surfaces (US-4.2 AC4).
type ReBootstrapResult struct {
	LocalProjectID   int64
	CutoffHLC        string
	RebootstrappedAt string
}

// ReBootstrap CONSUMES a 410 stale_pull (the consumer half of US-3.7 AC4 /
// US-4.2): when the recovery loop's pull is answered 410 because the joiner fell
// behind retention, it calls ReBootstrap with the {snapshot_url, as_of_hlc} the
// 410 advertised. ReBootstrap re-fetches the owner snapshot with the SAME pinned
// signed transport every federation request uses and overwrites the EXISTING
// local project in one transaction (snapshot.ReApply) — NOT creating a new
// project, and NEVER touching federation_outbox (R3 — the joiner's unsent edits
// survive and are flushed afterwards; events with HLC < as_of still flush and
// peer LWW resolves). It advances last_received_hlc to as_of and stamps the
// re-bootstrap marker (cutoff X) on the joined mapping row.
//
// snapshotURL is the absolute URL the 410 carried; asOfHLC is its as_of cutoff
// (recorded for diagnostics — the authoritative cutoff is the snapshot's own
// as_of line, which ReApply uses). The returned cutoff X is a REAL persisted
// value (the snapshot's as_of + the commit wall-clock), never a placeholder.
func (s *Service) ReBootstrap(ctx context.Context, localProjectID int64, ownerInstanceURL, snapshotURL, asOfHLC string) (*ReBootstrapResult, error) {
	if s.cipher == nil {
		return nil, ErrKeyMissing
	}
	if s.snap == nil || s.snap.snapshot == nil {
		return nil, fmt.Errorf("federation: snapshot deps not configured")
	}
	if s.sender == nil {
		return nil, fmt.Errorf("federation: no handshake sender configured")
	}
	owner := trimSlash(ownerInstanceURL)

	// Re-fetch the owner snapshot over the signed transport. The 410 carried a
	// fresh snapshot_url; the joiner's old 15-min handshake token is long expired
	// after a > retention offline, so the re-bootstrap fetch presents NO token —
	// the owner serves a re-snapshot to a signature-verified, non-revoked MEMBER
	// (the same membership the pull endpoint already trusts). An empty token query
	// is intentional; fetchSnapshot signs the request with the pinned transport.
	body, err := s.fetchSnapshot(ctx, snapshotURL, "")
	if err != nil {
		return nil, fmt.Errorf("re-fetch snapshot: %w", err)
	}

	res, err := snapshot.ReApply(ctx, snapshot.ApplyDeps{
		DB:          s.db,
		Projects:    s.projects,
		Sections:    s.snap.sections,
		Tasks:       s.snap.tasks,
		Contexts:    s.snap.contexts,
		FedProjects: s.fedProjects,
		Snapshot:    s.snap.snapshot,
	}, snapshot.ReApplyParams{
		LocalProjectID:   localProjectID,
		OwnerInstanceURL: owner,
		Reader:           bytes.NewReader(body),
		Now:              s.now,
	})
	if err != nil {
		return nil, fmt.Errorf("re-apply snapshot: %w", err)
	}
	return &ReBootstrapResult{
		LocalProjectID:   res.LocalProjectID,
		CutoffHLC:        res.CutoffHLC,
		RebootstrappedAt: res.RebootstrappedAt,
	}, nil
}

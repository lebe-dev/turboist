package federation

import (
	"context"
	"fmt"

	"github.com/lebe-dev/turboist/internal/repo"
)

// AuditQuery is the owner audit-view query (Federation v1 F6.3, US-7.4 AC1). An
// empty PeerInstanceURL/Kind matches everything; Limit/Offset paginate (the repo
// clamps Limit to a sane max).
type AuditQuery struct {
	PeerInstanceURL string
	Kind            string
	Limit           int
	Offset          int
}

// AuditView is one audit row for the owner UI (US-7.4 AC1). Timestamps are the
// stored TEXT ISO-8601 UTC strings. Detail is the short, NON-SENSITIVE coded
// reason — the writer never persists secrets/signatures/tokens (§7 F6.3).
type AuditView struct {
	ID              int64
	Kind            string
	Outcome         string
	PeerInstanceURL string
	Detail          string
	CreatedAt       string
}

// SignatureFailureAlert flags a peer whose recent signature-failure count crossed
// the configured threshold within the alert window (Federation v1 F6.3, US-7.4
// AC3 — the "possible attack on peer X" alert). Count is the observed count;
// Threshold is the configured trip point so the UI can render "N of M".
type SignatureFailureAlert struct {
	PeerInstanceURL string
	Count           int
	Threshold       int
}

// Audit returns the federation audit log newest-first for the owner audit view
// (Federation v1 F6.3, US-7.4 AC1), narrowed by the optional peer/kind filter and
// bounded by the page. Without an audit reader wired (a federation-off / partial
// build) it returns an empty list rather than erroring, so the endpoint stays a
// stable empty array.
func (s *Service) Audit(ctx context.Context, q AuditQuery) ([]AuditView, error) {
	if s.auditLog == nil {
		return []AuditView{}, nil
	}
	rows, err := s.auditLog.List(ctx, repo.AuditFilter{
		PeerInstanceURL: q.PeerInstanceURL,
		Kind:            q.Kind,
	}, repo.Page{Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	out := make([]AuditView, 0, len(rows))
	for _, r := range rows {
		out = append(out, AuditView{
			ID:              r.ID,
			Kind:            r.Kind,
			Outcome:         r.Outcome,
			PeerInstanceURL: r.PeerInstanceURL,
			Detail:          r.Detail,
			CreatedAt:       r.CreatedAt,
		})
	}
	return out, nil
}

// SignatureFailureAlerts returns one alert per peer whose signature-failure count
// within the configured alert window reached the threshold (Federation v1 F6.3,
// US-7.4 AC3). A non-positive threshold (or no audit reader) means alerts are off
// and the result is empty. A peer with an empty instance URL (a rejection before
// the instance header could be trusted) is never alerted — an alert names a peer.
func (s *Service) SignatureFailureAlerts(ctx context.Context) ([]SignatureFailureAlert, error) {
	if s.auditLog == nil || s.auditAlertThreshold <= 0 || s.auditAlertWindow <= 0 {
		return []SignatureFailureAlert{}, nil
	}
	since := s.now().Add(-s.auditAlertWindow)
	counts, err := s.auditLog.CountSignatureFailures(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("count signature failures: %w", err)
	}
	out := make([]SignatureFailureAlert, 0)
	for peer, n := range counts {
		if peer == "" || n < s.auditAlertThreshold {
			continue
		}
		out = append(out, SignatureFailureAlert{
			PeerInstanceURL: peer,
			Count:           n,
			Threshold:       s.auditAlertThreshold,
		})
	}
	return out, nil
}

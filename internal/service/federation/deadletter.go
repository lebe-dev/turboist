package federation

import (
	"context"
	"fmt"
)

// DeadLetterView is one parked, permanently-failed (peer, event) delivery for the
// owner's dead-letter diagnostics view (Federation v1 F4.4, US-4.4 AC3). It
// mirrors store.DeadLetterRow but is the service-layer DTO the JWT admin endpoint
// renders, so the handler never depends on the store row type directly.
type DeadLetterView struct {
	EventID         string
	PeerInstanceURL string
	LocalProjectID  int64
	StatusCode      int
	Reason          string
	FailedAt        string
}

// DeadLetter returns the parked dead-letter events newest-first for the owner's
// diagnostics view (Federation v1 F4.4, US-4.4 AC3). A non-positive limit uses
// the store's default cap. Without a sync store wired (a federation-off / partial
// build) it returns an empty list rather than erroring, so the endpoint stays a
// stable empty array. The payload bytes are intentionally NOT surfaced (the view
// carries only metadata — a future operator re-drive flow could fetch them).
func (s *Service) DeadLetter(ctx context.Context, limit int) ([]DeadLetterView, error) {
	if s.syncStore == nil {
		return []DeadLetterView{}, nil
	}
	rows, err := s.syncStore.ListDeadLetter(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list dead-letter: %w", err)
	}
	out := make([]DeadLetterView, 0, len(rows))
	for _, r := range rows {
		out = append(out, DeadLetterView{
			EventID:         r.EventID,
			PeerInstanceURL: r.PeerURL,
			LocalProjectID:  r.LocalProjectID,
			StatusCode:      r.StatusCode,
			Reason:          r.Reason,
			FailedAt:        r.FailedAt,
		})
	}
	return out, nil
}

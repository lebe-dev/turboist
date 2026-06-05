package federation

import (
	"context"
	"fmt"
)

// RestoreIdentityResult reports the outcome of the startup instance_url-change
// check (Federation v1 F6.5, US-8.5 AC2, R27).
type RestoreIdentityResult struct {
	// Changed is true when the persisted federation identity (the BASE_URL this
	// install enabled federation under) does NOT match the current instance_url —
	// i.e. the DB was restored under a new BASE_URL.
	Changed bool
	// PriorInstanceURL is the (first) persisted owner self-row URL that mismatched,
	// for the WARN log + the UI re-invite prompt. Empty when unchanged.
	PriorInstanceURL string
	// RowsMarked is how many federation mappings were marked lost=instance_url_changed
	// (history). 0 when unchanged.
	RowsMarked int64
}

// CheckRestoreIdentity detects whether this instance's DB was restored under a NEW
// BASE_URL and, if so, preserves the existing federation state as read-only history
// (Federation v1 F6.5, US-8.5 AC2, R27). It compares the current instance_url to the
// origin_instance_url persisted on the OWNER self-rows (the URL this install used
// when it enabled federation). On a mismatch it marks EVERY non-lost mapping
// lost=instance_url_changed — the rows are NOT deleted, so they render as history;
// outbound/inbound sync is halted (a lost mapping is skipped by the fan-out and
// rejected inbound); and the user is prompted to re-invite under the new URL. The
// keypair is preserved (no key regen). When the URL is unchanged (or there is no
// federated project) it is a no-op and the federation identity is kept verbatim
// (no re-handshake needed). It runs on the repo's own connection (no network I/O).
func (s *Service) CheckRestoreIdentity(ctx context.Context) (RestoreIdentityResult, error) {
	urls, err := s.fedProjects.OwnerSelfInstanceURLs(ctx)
	if err != nil {
		return RestoreIdentityResult{}, fmt.Errorf("read owner self urls: %w", err)
	}

	current := trimSlash(s.instanceURL)
	var prior string
	for _, u := range urls {
		if trimSlash(u) != current {
			prior = u
			break
		}
	}
	if prior == "" {
		// Unchanged (or nothing federated): identity preserved, no history marking.
		return RestoreIdentityResult{}, nil
	}

	n, err := s.fedProjects.MarkAllLostInstanceURLChanged(ctx)
	if err != nil {
		return RestoreIdentityResult{}, fmt.Errorf("mark instance_url_changed history: %w", err)
	}
	return RestoreIdentityResult{Changed: true, PriorInstanceURL: prior, RowsMarked: n}, nil
}

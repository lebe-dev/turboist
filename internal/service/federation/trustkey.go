package federation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrTrustKeyUnavailable is returned by TrustPeerKey when the trust-key
// collaborators (the security-incident repo + the peer-key cache + the .well-known
// fetcher) are not wired — a federation-off / partially-wired build. Handlers map
// it to CodeFederationKeyMissing.
var ErrTrustKeyUnavailable = errors.New("federation: trust-key not available (federation not fully configured)")

// TrustPeerKey is the manual operator action behind the "Trust new key" button
// (Federation v1 F5.6b, US-6.4 AC3). After a peer's key ROTATED and this instance
// detected the mismatch (rejecting the events 401, US-6.4 AC1, and recording an
// incident + sticky marker, AC2), an operator who has out-of-band confidence the
// rotation is legitimate clicks "Trust new key". This:
//
//  1. FETCHES the peer's CURRENT .well-known key (AC3 — the new key is taken from
//     the peer's published document, never blindly from the rejected event);
//  2. OVERWRITES the durable pinned key (federated_instances.public_key) and the
//     in-memory peer-key cache (Cache.Trust) so the next inbound event verifies;
//  3. CLEARS the sticky key_mismatch marker so the sync badge returns to healthy;
//  4. RESOLVES the open security incident with the newly-trusted key (the audit
//     trail of what was trusted, when).
//
// It is fetch-FIRST: if the .well-known fetch fails, nothing is mutated (the old
// key, marker, and open incident survive so the operator can retry). A missing
// project → ErrProjectNotFound (404); an unknown peer → ErrPeerNotFound (404);
// an unconfigured build → ErrTrustKeyUnavailable.
//
// NON-ATOMIC BOUNDARY (intentional): steps 2a (durable key write) → 2b (cache) →
// 3 (clear marker) → 4 (resolve incident) are SEPARATE writes, NOT one
// transaction. A crash/error after the durable key write but before the marker
// clear or incident resolve can leave the new key pinned while the sticky marker
// or the open incident still linger. This is RETRY-SAFE: every step is
// idempotent — re-running TrustPeerKey re-writes the (same) key, re-clears the
// (already-clear) marker, and re-resolves the (already-resolved) incident, all as
// no-ops — so the operator simply clicks "Trust new key" again to converge. The
// ordering (durable key first, audit-side state last) guarantees the only
// possible inconsistency is a cosmetic stale marker/incident, never a peer whose
// events are accepted without a pinned key.
func (s *Service) TrustPeerKey(ctx context.Context, projectID int64, peerInstanceURL string) error {
	const op = "federation.TrustPeerKey"
	if s.incidents == nil || s.peerKeys == nil || s.fetch == nil {
		return ErrTrustKeyUnavailable
	}

	// Verify the project exists (unknown project → 404, not a silent no-op).
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("load project: %w", err)
	}
	// Verify the peer is joined to this project (unknown peer → 404). This also
	// gives us the old key for the audit trail.
	if _, err := s.fedProjects.Get(ctx, projectID, peerInstanceURL); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrPeerNotFound
		}
		return fmt.Errorf("load peer: %w", err)
	}

	// (1) Fetch the peer's CURRENT published key FIRST (US-6.4 AC3). A failed fetch
	// aborts before any mutation so a transient network error is retryable and never
	// clears the incident. The fetch holds no DB connection (R1).
	inst, err := s.fetch(ctx, peerInstanceURL)
	if err != nil {
		return fmt.Errorf("%s: fetch peer well-known: %w", op, err)
	}
	if inst == nil || inst.PublicKey == "" {
		return fmt.Errorf("%s: peer well-known carried no public key", op)
	}
	// Validate the fetched key DECODES to a real Ed25519 key BEFORE any mutation, so
	// a non-empty-but-malformed .well-known key cannot corrupt the durable pinned key
	// (federated_instances.public_key). Cache.Trust (step 2b) decodes too, but only
	// AFTER the durable write — without this guard a malformed key persists durably
	// and diverges from the cache (Federation v1 F5.6b review fix). Fetch-first must
	// mean validate-first.
	if _, err := crypto.DecodePublicKey(inst.PublicKey); err != nil {
		return fmt.Errorf("%s: peer well-known public key is malformed: %w", op, err)
	}

	now := s.now()

	// (2a) Overwrite the durable pinned key. Done before the cache so the in-memory
	// trust can never outlive a failed durable write.
	if n, err := s.fedInstances.UpdatePublicKey(ctx, peerInstanceURL, inst.PublicKey, now); err != nil {
		return fmt.Errorf("update durable peer key: %w", err)
	} else if n == 0 {
		// The peer row exists in federated_projects but not in the instance
		// directory (a never-fully-handshaken peer): nothing pinned to overwrite.
		return ErrPeerNotFound
	}

	// (2b) Overwrite the in-memory cache so the signature middleware + per-event
	// validator immediately verify against the new key (Cache.Trust is the only
	// path that mutates a pinned key, US-6.4 AC3).
	if err := s.peerKeys.Trust(peerInstanceURL, inst.PublicKey, inst.DisplayName); err != nil {
		return fmt.Errorf("trust peer key in cache: %w", err)
	}

	// (3) Clear the sticky key_mismatch marker (the badge returns to healthy). On the
	// transition (a marker actually cleared) publish a ScopeFederation SSE so open
	// owner tabs flip the badge back.
	cleared, err := s.fedProjects.ClearKeyMismatch(ctx, projectID, peerInstanceURL)
	if err != nil {
		return fmt.Errorf("clear key mismatch marker: %w", err)
	}
	if cleared > 0 && s.statusNotifier != nil {
		s.statusNotifier.NotifyFederation(ctx)
	}

	// (4) Resolve the open incident with the newly-trusted key (the audit trail).
	if _, err := s.incidents.ResolveKeyChange(ctx, projectID, peerInstanceURL, inst.PublicKey, now); err != nil {
		return fmt.Errorf("resolve security incident: %w", err)
	}

	// Audit the manual key-trust (Federation v1 F6.3, US-7.4 AC1): a security-
	// relevant control-plane action that succeeded. Only the short key prefix is
	// referenced — never the full key bytes — keeping the audit detail non-sensitive.
	s.recordAudit(repo.AuditKindTrustKey, repo.AuditOutcomeAccepted, peerInstanceURL, "operator trusted new peer key")

	// Audit: the manual key-trust is a security-relevant action (US-6.4 AC3 — "action
	// logged"). Never log the key bytes themselves beyond a short prefix.
	logging.FromContext(ctx).InfoContext(ctx, "federation: peer key manually trusted",
		slog.String("op", op),
		slog.Int64("project_id", projectID),
		slog.String("peer", peerInstanceURL),
		slog.String("new_key_prefix", keyPrefix(inst.PublicKey)),
	)
	return nil
}

// keyPrefix returns a short, non-sensitive prefix of a base64 public key for audit
// logs (the full key is public, but the prefix keeps log lines tidy).
func keyPrefix(k string) string {
	if len(k) <= 8 {
		return k
	}
	return k[:8]
}

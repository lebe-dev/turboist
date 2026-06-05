package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// FederationSecurityIncidentRepo persists the append-only key-change incident log
// (Federation v1 F5.6b, US-6.4 AC2/AC3). When a peer's inbound event signature
// stops validating against the key this instance pinned for it (a key rotation or
// compromise), the F4.3 inbox-signature-check path records an INCIDENT here so the
// owner UI can surface the "Peer X signature failed — possible key rotation or
// compromise" alert (AC2) and so the manual "Trust new key" action has an audit
// trail of what was trusted and when (AC3). Distinct from the sticky transient
// federated_projects.key_mismatch_at flag (which trust-key CLEARS): this history
// SURVIVES the clear so a security review can see every key-change event.
type FederationSecurityIncidentRepo struct {
	db *sql.DB
}

func NewFederationSecurityIncidentRepo(db *sql.DB) *FederationSecurityIncidentRepo {
	return &FederationSecurityIncidentRepo{db: db}
}

// SecurityIncident is one key-change incident row (Federation v1 F5.6b). Optional
// timestamps / keys are empty strings when the underlying column is NULL.
// ResolvedAt is empty while the incident is still open.
type SecurityIncident struct {
	LocalProjectID  int64
	PeerInstanceURL string
	Kind            string
	DetectedAt      string
	OldPublicKey    string
	NewPublicKey    string
	ResolvedAt      string
}

// RecordKeyChange opens a new key-change incident for a (local project, peer) the
// FIRST time a signature mismatch is observed for that peer (US-6.4 AC2). It is
// IDEMPOTENT while an incident is open: the partial unique index pins at most one
// OPEN incident per (project, peer), and the INSERT uses ON CONFLICT DO NOTHING,
// so a flood of rejected events under one rotation records a single incident, not
// thousands (§7 F5.6b "incident write non-blocking under key-mismatch flood"). It
// returns whether a NEW incident OPENED (true) or one was already open (false) so
// the caller audit-logs only on the transition. oldPublicKey is the pinned key the
// rejected event failed to verify against, captured for the audit trail (may be
// empty when not known). A non-existent local project surfaces the FK error.
func (r *FederationSecurityIncidentRepo) RecordKeyChange(ctx context.Context, localProjectID int64, peerInstanceURL, oldPublicKey string, detectedAt time.Time) (bool, error) {
	const op = "repo.federation_security_incidents.RecordKeyChange"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	var oldKey any
	if oldPublicKey != "" {
		oldKey = oldPublicKey
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO federation_security_incidents (local_project_id, peer_instance_url, kind, detected_at, old_public_key)
		 VALUES (?, ?, 'key_change', ?, ?)
		 ON CONFLICT (local_project_id, peer_instance_url) WHERE resolved_at IS NULL DO NOTHING`,
		localProjectID, peerInstanceURL, model.FormatUTC(detectedAt), oldKey)
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("record key change: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("record key change rows: %w", err))
	}
	return n > 0, nil
}

// OpenIncident returns the currently-open key-change incident for a (local
// project, peer), or nil (NOT an error) when none is open — the common, healthy
// case. The partial unique index guarantees at most one open row, so this reads a
// single row.
func (r *FederationSecurityIncidentRepo) OpenIncident(ctx context.Context, localProjectID int64, peerInstanceURL string) (*SecurityIncident, error) {
	const op = "repo.federation_security_incidents.OpenIncident"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	row := r.db.QueryRowContext(ctx,
		`SELECT local_project_id, peer_instance_url, kind, detected_at,
		        COALESCE(old_public_key, ''), COALESCE(new_public_key, ''), COALESCE(resolved_at, '')
		   FROM federation_security_incidents
		  WHERE local_project_id = ? AND peer_instance_url = ? AND resolved_at IS NULL`,
		localProjectID, peerInstanceURL)
	var inc SecurityIncident
	err := row.Scan(&inc.LocalProjectID, &inc.PeerInstanceURL, &inc.Kind, &inc.DetectedAt,
		&inc.OldPublicKey, &inc.NewPublicKey, &inc.ResolvedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return &inc, nil
}

// ResolveKeyChange stamps resolved_at + the newly-trusted public key on the OPEN
// incident for a (local project, peer) when the operator clicks "Trust new key"
// (US-6.4 AC3 — the audit trail of what was trusted, when). It returns the
// affected-row count: 1 when an open incident was resolved, 0 when none was open
// (trust-key on a peer whose marker was never set is a clean no-op, not an error).
// Resolving frees the partial unique index so a LATER, distinct rotation opens a
// fresh incident — the history is append-only.
func (r *FederationSecurityIncidentRepo) ResolveKeyChange(ctx context.Context, localProjectID int64, peerInstanceURL, newPublicKey string, resolvedAt time.Time) (int, error) {
	const op = "repo.federation_security_incidents.ResolveKeyChange"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	res, err := r.db.ExecContext(ctx,
		`UPDATE federation_security_incidents
		    SET resolved_at = ?, new_public_key = ?
		  WHERE local_project_id = ? AND peer_instance_url = ? AND resolved_at IS NULL`,
		model.FormatUTC(resolvedAt), newPublicKey, localProjectID, peerInstanceURL)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("resolve key change: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("resolve key change rows: %w", err))
	}
	return int(n), nil
}

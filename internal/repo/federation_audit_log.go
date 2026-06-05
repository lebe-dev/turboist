package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// AuditKind is the security-relevant federation operation an audit row records
// (Federation v1 F6.3, US-7.4 AC1). The set is closed and matches the CHECK
// constraint on federation_audit_log.kind in migration 039.
type AuditKind string

const (
	// AuditKindHandshake — an inbound handshake was processed (accepted or rejected).
	AuditKindHandshake AuditKind = "handshake"
	// AuditKindRevoke — the owner revoked a peer's access to a project.
	AuditKindRevoke AuditKind = "revoke"
	// AuditKindTrustKey — the operator manually trusted a peer's new key.
	AuditKindTrustKey AuditKind = "trust_key"
	// AuditKindSignatureInvalid — a transport OR per-event Ed25519 signature
	// failed to verify (US-7.2 AC1). One of the three signature-failure kinds the
	// "possible attack on peer X" alert aggregates (US-7.4 AC3).
	AuditKindSignatureInvalid AuditKind = "signature_invalid"
	// AuditKindDigestMismatch — the request body digest did not match the
	// X-Federation-Digest header (transport leg, US-7.2 AC2). Counts toward the alert.
	AuditKindDigestMismatch AuditKind = "digest_mismatch"
	// AuditKindAuthorMismatch — an event's author.instance_url did not equal its
	// origin_instance (per-event, US-7.2 AC3). Counts toward the alert.
	AuditKindAuthorMismatch AuditKind = "author_mismatch"
	// AuditKindClockSkew — an event's HLC physical clock was outside the accepted
	// skew window (US-7.2 AC4).
	AuditKindClockSkew AuditKind = "clock_skew"
	// AuditKindReplay — a request nonce was already seen (US-7.3 AC1). Counts toward
	// the alert.
	AuditKindReplay AuditKind = "replay"
	// AuditKindTimestampStale — a request timestamp fell outside the ±5min window
	// (US-7.3 AC2). Counts toward the alert.
	AuditKindTimestampStale AuditKind = "timestamp_stale"
	// AuditKindKeyChange — a peer's pinned key changed (a verified-and-rejected
	// per-event signature → key rotation/compromise, US-6.4).
	AuditKindKeyChange AuditKind = "key_change"
)

// signatureFailureKinds are the audit kinds the "possible attack on peer X" alert
// counts (US-7.4 AC3): the transport + per-event rejections that a brute/forge
// attack would produce in a burst. A key_change is its OWN durable incident
// (F5.6b) so it is not double-counted here.
var signatureFailureKinds = []AuditKind{
	AuditKindSignatureInvalid,
	AuditKindDigestMismatch,
	AuditKindAuthorMismatch,
	AuditKindReplay,
	AuditKindTimestampStale,
}

// AuditOutcome records whether the operation succeeded or was refused (US-7.4
// AC1). It matches the CHECK on federation_audit_log.outcome.
type AuditOutcome string

const (
	AuditOutcomeAccepted AuditOutcome = "accepted"
	AuditOutcomeRejected AuditOutcome = "rejected"
)

// AuditEntry is one security-relevant federation operation to record (Federation
// v1 F6.3, US-7.4 AC1). Detail is a short, NON-SENSITIVE reason string — the
// caller MUST NEVER place a secret, raw signature, private seed, or invite token
// here (§7 F6.3 "never persist secrets/signatures/tokens"). CreatedAt is the
// wall-clock time of the operation; the repo formats it model.FormatUTC.
type AuditEntry struct {
	Kind            AuditKind
	Outcome         AuditOutcome
	PeerInstanceURL string
	Detail          string
	CreatedAt       time.Time
}

// AuditRow is one audit log row read back for the owner audit view (US-7.4 AC1).
// Timestamps are the stored TEXT ISO-8601 UTC strings.
type AuditRow struct {
	ID              int64
	Kind            string
	Outcome         string
	PeerInstanceURL string
	Detail          string
	CreatedAt       string
}

// AuditFilter narrows the audit list (US-7.4 AC1 owner audit view). Empty fields
// match everything; an empty filter returns the whole (paginated) log newest-first.
type AuditFilter struct {
	// PeerInstanceURL, when non-empty, returns only rows for this peer.
	PeerInstanceURL string
	// Kind, when non-empty, returns only rows of this kind.
	Kind string
}

// FederationAuditLogRepo persists the append-only federation audit log
// (Federation v1 F6.3, US-7.4). Every security-relevant federation operation —
// the transport/per-event rejections AND the owner control-plane trust actions —
// writes one row here so the owner can investigate anomalies. The async writer in
// internal/federation/audit drives Insert off the request path so logging never
// blocks a rejection (§7 F6.3 "async writer, failure-spam is worst-case load").
type FederationAuditLogRepo struct {
	db *sql.DB
}

// NewFederationAuditLogRepo constructs the audit repo over a *sql.DB.
func NewFederationAuditLogRepo(db *sql.DB) *FederationAuditLogRepo {
	return &FederationAuditLogRepo{db: db}
}

// Insert appends one audit row (US-7.4 AC1). The detail string is stored verbatim
// — the CALLER is responsible for keeping it non-sensitive (no secrets, signatures,
// or tokens), which the async writer guarantees by only ever passing coded reason
// strings.
func (r *FederationAuditLogRepo) Insert(ctx context.Context, e AuditEntry) error {
	const op = "repo.federation_audit_log.Insert"
	logQuery(ctx, op, string(e.Kind), e.PeerInstanceURL)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO federation_audit_log (kind, outcome, peer_instance_url, detail, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		string(e.Kind), string(e.Outcome), e.PeerInstanceURL, e.Detail, model.FormatUTC(e.CreatedAt))
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("insert audit: %w", err))
	}
	return nil
}

// List returns audit rows newest-first, narrowed by the optional peer/kind filter
// and bounded by the page (US-7.4 AC1 owner audit view). The (peer, created_at)
// index serves the per-peer filter + ordering.
func (r *FederationAuditLogRepo) List(ctx context.Context, f AuditFilter, page Page) ([]AuditRow, error) {
	const op = "repo.federation_audit_log.List"
	page = page.Normalize()
	logQuery(ctx, op, f.PeerInstanceURL, f.Kind, page.Limit, page.Offset)

	query := `SELECT id, kind, outcome, peer_instance_url, detail, created_at
	            FROM federation_audit_log
	           WHERE 1 = 1`
	args := []any{}
	if f.PeerInstanceURL != "" {
		query += " AND peer_instance_url = ?"
		args = append(args, f.PeerInstanceURL)
	}
	if f.Kind != "" {
		query += " AND kind = ?"
		args = append(args, f.Kind)
	}
	// Newest-first; id breaks ties for same-ms rows so pagination is stable.
	query += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, page.Limit, page.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list audit: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]AuditRow, 0, page.Limit)
	for rows.Next() {
		var row AuditRow
		if err := rows.Scan(&row.ID, &row.Kind, &row.Outcome, &row.PeerInstanceURL, &row.Detail, &row.CreatedAt); err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("scan audit: %w", err))
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("iterate audit: %w", err))
	}
	return out, nil
}

// CountSignatureFailures returns, per peer, how many signature-failure audit rows
// were recorded at-or-after since (Federation v1 F6.3, US-7.4 AC3). It powers the
// "possible attack on peer X" alert: the service flags any peer whose count
// crosses the configured threshold within the recent window. Only the
// signatureFailureKinds are counted; handshakes, revokes, key changes, and clock
// skew are excluded so a noisy-but-benign peer is not falsely flagged.
//
// SPOOF MITIGATION (F6.3 review C): a transport rejection is recorded with the
// CLAIMED X-Federation-Instance header, which an unauthenticated attacker controls
// (the signature FAILED, so the instance is unverified). To stop a stranger from
// raising a bogus "attack on peer X" alert for a URL we don't even federate with,
// the alert count is restricted to KNOWN peers (those with a row in
// federated_instances). The audit LOG still records every rejection verbatim (full
// trail); only the ALERT is de-noised here.
//
// RESIDUAL THREAT-MODEL GAP (accepted v1, R18-class): an attacker who spoofs the
// URL of an ALREADY-KNOWN peer can still inflate that peer's count — pre-auth
// transport rejections carry no verified identity, so they are inherently
// unattributable to a specific sender. This is documented, not closed; the operator
// should treat a per-peer alert as "anomalous traffic CLAIMING to be peer X",
// corroborated by the durable key-change incident (F5.6b) for a real rotation.
func (r *FederationAuditLogRepo) CountSignatureFailures(ctx context.Context, since time.Time) (map[string]int, error) {
	const op = "repo.federation_audit_log.CountSignatureFailures"
	logQuery(ctx, op, model.FormatUTC(since))

	// Build the IN-list placeholders for the signature-failure kinds.
	placeholders := ""
	args := []any{model.FormatUTC(since)}
	for i, k := range signatureFailureKinds {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
		args = append(args, string(k))
	}

	query := fmt.Sprintf(
		`SELECT peer_instance_url, COUNT(*)
		   FROM federation_audit_log
		  WHERE created_at >= ? AND kind IN (%s)
		    AND peer_instance_url IN (SELECT instance_url FROM federated_instances)
		  GROUP BY peer_instance_url`, placeholders)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("count signature failures: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := map[string]int{}
	for rows.Next() {
		var peer string
		var n int
		if err := rows.Scan(&peer, &n); err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("scan count: %w", err))
		}
		out[peer] = n
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("iterate counts: %w", err))
	}
	return out, nil
}

// DeleteOlderThan hard-DELETEs audit rows whose created_at predates cutoff (the
// 1-year retention GC, US-7.4 AC2). created_at is fixed-width TEXT ISO-8601 UTC so
// the comparison is a plain lexical string compare (§7 F6.3). Returns the number
// of rows removed.
func (r *FederationAuditLogRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	const op = "repo.federation_audit_log.DeleteOlderThan"
	cutoffStr := model.FormatUTC(cutoff)
	logQuery(ctx, op, cutoffStr)
	res, err := r.db.ExecContext(ctx, `DELETE FROM federation_audit_log WHERE created_at < ?`, cutoffStr)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("delete aged audit: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("delete aged audit rows: %w", err))
	}
	return n, nil
}

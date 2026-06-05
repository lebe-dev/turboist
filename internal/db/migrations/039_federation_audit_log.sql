-- +goose Up
-- Federation audit log (Federation v1 F6.3, US-7.4 Could).
--
-- US-7.4 lets the owner investigate federation anomalies (signature failures,
-- replay, key changes) by browsing an append-only audit log of security-relevant
-- federation operations. Every rejection the trust planes produce — and the
-- owner control-plane actions that change trust (handshake accepted, peer
-- revoked, key manually trusted) — writes ONE row here with a timestamp, the peer
-- instance, a kind, and an outcome (US-7.4 AC1). The log is retained one year and
-- the nightly GC hard-deletes anything older (US-7.4 AC2). A burst of signature
-- failures from one peer in a short window drives the "possible attack on peer X"
-- alert (US-7.4 AC3), computed by CountSignatureFailures over the recent window.
--
-- This is an INVENTED table (the design doc carries no audit schema — §7 F6.3
-- "invented table, agree columns"). The columns are the minimal AC1 set:
--   * kind          — the security-relevant operation
--                     (handshake|revoke|trust_key|signature_invalid|digest_mismatch|
--                      author_mismatch|clock_skew|replay|timestamp_stale|key_change).
--   * outcome       — accepted | rejected (an operation either succeeded or was
--                     refused; AC1 records the outcome alongside the kind).
--   * peer_instance_url — the calling/affected peer instance URL (may be empty
--                     when a request was rejected before the instance header could
--                     be trusted, e.g. a malformed-header signature failure).
--   * detail        — a short, NON-SENSITIVE reason string. The async writer MUST
--                     NEVER persist secrets, raw signatures, private seeds, or
--                     invite tokens here (§7 F6.3 "never persist secrets/
--                     signatures/tokens") — only a coded reason like
--                     "nonce replay" or "body digest mismatch".
--   * created_at    — TEXT ISO-8601 UTC (model.FormatUTC). Fixed-width so the
--                     1-year GC cutoff is a plain lexical string compare (§7 F6.3
--                     "fixed-width TEXT for lexical cutoff") and so per-peer
--                     windows order correctly.
--
-- Name-collision note (R21): every federation table is federation_*-prefixed;
-- federation_audit_log is free in the live schema (001..038).
-- +goose StatementBegin
CREATE TABLE federation_audit_log (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    -- The stable, instance-portable id required of every synchronized/federation
    -- table by §3 (keep int64 AUTOINCREMENT PK + ADD client_id TEXT UNIQUE ULID).
    -- Audit rows are local-only (never federated to a peer), but the column is kept
    -- for schema-uniformity and to satisfy the offline-sync overlay contract.
    client_id         TEXT UNIQUE,
    kind              TEXT NOT NULL CHECK (kind IN (
                          'handshake', 'revoke', 'trust_key',
                          'signature_invalid', 'digest_mismatch', 'author_mismatch',
                          'clock_skew', 'replay', 'timestamp_stale', 'key_change')),
    outcome           TEXT NOT NULL CHECK (outcome IN ('accepted', 'rejected')),
    peer_instance_url TEXT NOT NULL DEFAULT '',
    detail            TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL
);
-- +goose StatementEnd

-- GC sweep boundary: the nightly retention GC deletes rows older than the 1-year
-- cutoff (US-7.4 AC2) via a created_at < cutoff range scan.
CREATE INDEX idx_federation_audit_log_created ON federation_audit_log(created_at);

-- Per-peer query + the signature-failure burst aggregation (US-7.4 AC1/AC3): the
-- owner audit view filters by peer, and CountSignatureFailures counts a peer's
-- recent rejections, both keyed on (peer, created_at).
CREATE INDEX idx_federation_audit_log_peer ON federation_audit_log(peer_instance_url, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_federation_audit_log_peer;
DROP INDEX IF EXISTS idx_federation_audit_log_created;
DROP TABLE IF EXISTS federation_audit_log;

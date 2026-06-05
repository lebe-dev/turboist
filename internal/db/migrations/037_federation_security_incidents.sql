-- +goose Up
-- Federation security-incident log (Federation v1 F5.6b, US-6.4 AC2/AC3).
--
-- US-6.4 detects that a peer's published Ed25519 public_key has CHANGED: an
-- inbound event no longer verifies against the key this instance pinned for that
-- peer. F4.3 already SETS a sticky per-(project,peer) `key_mismatch_at` flag on
-- federated_projects (034) when that happens and the F0.3/F4.3 path already
-- refuses to auto-refetch a pinned key, so the rotated-key event is rejected 401
-- and never applied (US-6.4 AC1). What F5.6b adds is a durable INCIDENT RECORD so
-- the owner UI can render the "Peer X signature failed — possible key rotation or
-- compromise" alert (AC2) and so the manual "Trust new key" action has an audit
-- trail of what was trusted, when, and from which old key (AC3).
--
-- The sticky flag is a single transient timestamp that the trust-key action
-- CLEARS; this table is the append-only history that SURVIVES the clear, so a
-- security review can see every key-change event and its resolution. One open
-- incident per (local project, peer) at a time: the recorder is idempotent while
-- an incident is open (does NOT spam a row per rejected event under a flood, §7
-- F5.6b "incident write non-blocking under key-mismatch flood"); trust-key stamps
-- resolved_at + the newly-trusted key. local_project_id is the int64 projects.id
-- (the federation entity-identity deviation, §3); all wire timestamps are TEXT
-- ISO-8601 UTC (model.FormatUTC).
--
-- Name-collision note (R21): every federation table is federation_*-prefixed;
-- federation_security_incidents is free in the live schema (001..036).
-- +goose StatementBegin
CREATE TABLE federation_security_incidents (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    local_project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    peer_instance_url TEXT NOT NULL,
    kind              TEXT NOT NULL DEFAULT 'key_change' CHECK (kind IN ('key_change')),
    detected_at       TEXT NOT NULL,
    -- The old (pinned) key the rejected event failed to verify against, captured
    -- for the audit trail. NULL when not known at detection time.
    old_public_key    TEXT,
    -- The new key the operator trusted to resolve the incident (set by trust-key),
    -- NULL while the incident is still open.
    new_public_key    TEXT,
    -- When the operator resolved the incident via "Trust new key" (NULL = open).
    resolved_at       TEXT
);
-- +goose StatementEnd

-- A partial unique index pins AT MOST ONE open incident per (project, peer): the
-- recorder INSERTs with ON CONFLICT DO NOTHING so a flood of rejected events under
-- one rotation records a single incident, not thousands (idempotent recording).
-- Once trust-key stamps resolved_at the row drops out of the partial index, so a
-- LATER, distinct rotation opens a fresh incident.
CREATE UNIQUE INDEX idx_federation_security_incidents_open
    ON federation_security_incidents(local_project_id, peer_instance_url)
    WHERE resolved_at IS NULL;

-- Per-peer history lookup for the owner audit view.
CREATE INDEX idx_federation_security_incidents_peer
    ON federation_security_incidents(local_project_id, peer_instance_url);

-- +goose Down
DROP INDEX IF EXISTS idx_federation_security_incidents_peer;
DROP INDEX IF EXISTS idx_federation_security_incidents_open;
DROP TABLE IF EXISTS federation_security_incidents;

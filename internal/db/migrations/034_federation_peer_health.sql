-- +goose Up
-- Federation per-peer health marker (Federation v1 F4.3, US-4.3 AC4).
--
-- F4.3 surfaces a per-project sync-status indicator in the owner UI with four
-- server-derived states (US-4.3): synced (green), pending (yellow, >5min
-- undelivered outbox), unreachable (orange, a peer not contacted >24h), and
-- key_mismatch (red, STICKY). Three of the four derive from data that already
-- exists — pending from federation_outbox.created_at + delivered_to, unreachable
-- from federated_instances.last_contact_at. Only key_mismatch needs a NEW
-- persisted, sticky signal: when a peer's INBOUND event signature stops
-- validating (its Ed25519 key changed — the inbox signature check), F4.3 stamps
-- this column so the badge stays red and the event is NOT applied (US-4.3 AC4)
-- until an operator explicitly re-trusts the new key. The clear path (manual
-- trust-key) lands in F5.6b; F4.3 only SETS it, never auto-clears it (sticky).
--
-- key_mismatch_at is the wall-clock TEXT ISO-8601 UTC (model.FormatUTC) of the
-- first signature mismatch observed for this (local project, peer). NULL means
-- "no key mismatch" — the common case. It is a column on federated_projects (the
-- per-(local project, peer) mapping) so the marker is scoped to the specific
-- peer-on-project whose key changed, never the whole instance. local_project_id
-- is the int64 projects.id (the federation entity-identity deviation, §3).
ALTER TABLE federated_projects ADD COLUMN key_mismatch_at TEXT;

-- +goose Down
-- SQLite (modernc) supports ADD COLUMN but historically not DROP COLUMN; the Down
-- leg recreates federated_projects without the marker column, preserving rows. It
-- mirrors the 033 Down so the marker columns 033 added (rebootstrap_cutoff_hlc,
-- rebootstrapped_at) survive the round-trip — only key_mismatch_at is removed.
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_federated_projects_active;
DROP INDEX IF EXISTS idx_federated_projects_local;

CREATE TABLE federated_projects_pre034 (
    local_project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    peer_instance_url TEXT NOT NULL,
    remote_project_id TEXT NOT NULL DEFAULT '',
    is_owner          INTEGER NOT NULL DEFAULT 0 CHECK (is_owner IN (0, 1)),
    origin_instance_url TEXT NOT NULL,
    permissions       TEXT NOT NULL CHECK (permissions IN ('read', 'write', 'admin')),
    paused            INTEGER NOT NULL DEFAULT 0 CHECK (paused IN (0, 1)),
    revoked           INTEGER NOT NULL DEFAULT 0 CHECK (revoked IN (0, 1)),
    protocol_version  INTEGER NOT NULL DEFAULT 1,
    last_sent_hlc     TEXT,
    last_received_hlc TEXT,
    joined_at         TEXT NOT NULL,
    rebootstrap_cutoff_hlc TEXT,
    rebootstrapped_at TEXT,
    PRIMARY KEY (local_project_id, peer_instance_url)
);

INSERT INTO federated_projects_pre034 (
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at,
    rebootstrap_cutoff_hlc, rebootstrapped_at)
SELECT
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at,
    rebootstrap_cutoff_hlc, rebootstrapped_at
FROM federated_projects;

DROP TABLE federated_projects;
ALTER TABLE federated_projects_pre034 RENAME TO federated_projects;

CREATE INDEX idx_federated_projects_local ON federated_projects(local_project_id);
CREATE INDEX idx_federated_projects_active ON federated_projects(local_project_id) WHERE revoked = 0;
-- +goose StatementEnd

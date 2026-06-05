-- +goose Up
-- Federation re-bootstrap marker (Federation v1 F4.2, US-4.2 AC4).
--
-- When a peer falls behind retention, its pull is answered 410 stale_pull
-- (emitted by F3.3) carrying {snapshot_url, as_of_hlc}. F4.2 CONSUMES that 410:
-- it re-fetches the owner snapshot and overwrites local project state in one
-- transaction WITHOUT touching federation_outbox (the user's unsent edits must
-- survive — R3, the highest-impact F4.2 bug). After a re-bootstrap the UI must
-- surface a dismissible banner whose message names the cutoff X — the moment the
-- snapshot was taken — so the user knows "your unsent changes from before {X}
-- were preserved but may have been overridden" (US-4.2 AC4).
--
-- The cutoff X must be a REAL persisted value, not a placeholder string (R3):
--   rebootstrap_cutoff_hlc — the snapshot's as_of_hlc (the durable causal cutoff),
--   rebootstrapped_at       — the wall-clock TEXT ISO-8601 UTC (model.FormatUTC)
--                             the re-bootstrap committed at, the human-readable X
--                             the banner renders.
-- Both are NULL on a row that has only ever been INITIAL-bootstrapped (F2.3) —
-- that is how the joiner UI distinguishes a first bootstrap from a re-bootstrap.
-- They are columns on federated_projects (the per-(local project, peer) mapping)
-- so each joined peer copy carries its own re-sync history. local_project_id is
-- the int64 projects.id (the federation entity-identity deviation, §3).
ALTER TABLE federated_projects ADD COLUMN rebootstrap_cutoff_hlc TEXT;
ALTER TABLE federated_projects ADD COLUMN rebootstrapped_at TEXT;

-- +goose Down
-- SQLite (modernc) supports ADD COLUMN but historically not DROP COLUMN; the Down
-- leg recreates federated_projects without the two marker columns. It is gated on
-- the table existing so a partial migration stack rolls back cleanly.
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_federated_projects_active;
DROP INDEX IF EXISTS idx_federated_projects_local;

CREATE TABLE federated_projects_pre033 (
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
    PRIMARY KEY (local_project_id, peer_instance_url)
);

INSERT INTO federated_projects_pre033 (
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at)
SELECT
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at
FROM federated_projects;

DROP TABLE federated_projects;
ALTER TABLE federated_projects_pre033 RENAME TO federated_projects;

CREATE INDEX idx_federated_projects_local ON federated_projects(local_project_id);
CREATE INDEX idx_federated_projects_active ON federated_projects(local_project_id) WHERE revoked = 0;
-- +goose StatementEnd

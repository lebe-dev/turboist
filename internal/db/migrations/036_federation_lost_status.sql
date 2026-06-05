-- +goose Up
-- Federation lost-status marker (Federation v1 F5.4, US-6.2 AC3/AC4; reused by
-- F5.5 US-6.3 voluntary leave and F5.6a US-6.5 owner-death).
--
-- When the OWNER permanently revokes a peer (F5.4) it sends that peer a signed
-- `federation_revoke` control event. The peer, on receiving (or — if it was
-- offline when the revoke was sent — on the next rejected sync, US-6.2 AC4),
-- marks its JOINED copy of the project as `lost`: the trust link is permanently
-- gone and the local copy becomes read-only. The reason disambiguates WHY the
-- copy was lost so the three end-states that share this flag stay distinguishable:
--
--   'revoked'    — the owner revoked us (F5.4, US-6.2): copy is READ-ONLY.
--   'left'       — we voluntarily left (F5.5, US-6.3): copy becomes a plain
--                  editable local project (no outbound sync). Modelled NOW so the
--                  F5.5 milestone only adds behaviour, not schema.
--   'owner-dead' — the owner instance is permanently unreachable (F5.6a, US-6.5):
--                  read-only fallback. Modelled NOW for the same reason.
--
-- The empty string '' is the normal, NOT-lost state (the common case). The marker
-- is IRREVERSIBLE for 'revoked' (re-collaboration needs a fresh invite, US-6.2
-- AC5); there is no un-revoke path.
--
-- These are columns on federated_projects (the per-(local project, peer) mapping)
-- so a project federated with several owners/peers carries lost state per link.
-- On the JOINER the relevant row is its is_owner=0 mapping to the origin owner;
-- on the OWNER the peer's row also carries `revoked` (set by F5.4's DELETE peers).
-- local_project_id is the int64 projects.id (the federation entity-identity
-- deviation, §3). The CHECK constraint pins the closed reason vocabulary so a
-- typo can never persist an unknown lost-reason.
ALTER TABLE federated_projects ADD COLUMN lost INTEGER NOT NULL DEFAULT 0 CHECK (lost IN (0, 1));
ALTER TABLE federated_projects ADD COLUMN lost_reason TEXT NOT NULL DEFAULT '' CHECK (lost_reason IN ('', 'revoked', 'left', 'owner-dead'));

-- +goose Down
-- SQLite (modernc) supports ADD COLUMN but historically not DROP COLUMN; the Down
-- leg recreates federated_projects without the two lost columns, preserving rows
-- and every column added through 034 (rebootstrap_cutoff_hlc, rebootstrapped_at,
-- key_mismatch_at). It is gated on the table existing so a partial migration stack
-- rolls back cleanly. Mirrors the 033/034 Down legs.
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_federated_projects_active;
DROP INDEX IF EXISTS idx_federated_projects_local;

CREATE TABLE federated_projects_pre036 (
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
    key_mismatch_at   TEXT,
    PRIMARY KEY (local_project_id, peer_instance_url)
);

INSERT INTO federated_projects_pre036 (
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at,
    rebootstrap_cutoff_hlc, rebootstrapped_at, key_mismatch_at)
SELECT
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at,
    rebootstrap_cutoff_hlc, rebootstrapped_at, key_mismatch_at
FROM federated_projects;

DROP TABLE federated_projects;
ALTER TABLE federated_projects_pre036 RENAME TO federated_projects;

CREATE INDEX idx_federated_projects_local ON federated_projects(local_project_id);
CREATE INDEX idx_federated_projects_active ON federated_projects(local_project_id) WHERE revoked = 0;
-- +goose StatementEnd

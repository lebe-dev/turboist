-- +goose Up
-- Federation ops: runtime-reloadable retention settings + the instance_url_changed
-- lost-reason (Federation v1 F6.5, US-8.4 / US-8.5 AC2; R27).
--
-- Two concerns land together in this one migration:
--
--  1. federation_retention_settings — a single-row (id=1) persistence slot for the
--     admin-configurable retention windows (US-8.4): the tombstone, outbox, and
--     inbox retentions the GC sweeps with. Persisting them here (rather than only
--     in config.yml) lets the owner change them at runtime from the settings UI;
--     the live values are held behind an atomic.Pointer in the process so the GC
--     reads the latest without a restart. NULL means "fall back to the config /
--     compiled default", so a fresh install behaves exactly as before. The outbox
--     retention is HARD-CAPPED at 30 days in the service layer (§16.3) regardless
--     of what is stored, so a too-large value is clamped, never honored verbatim.
--     The row is seeded empty (all NULL) so the defaults apply until the owner
--     explicitly changes something.
--
--  2. federated_projects.lost_reason gains the 'instance_url_changed' value
--     (US-8.5 AC2, R27): when this instance is restored under a NEW BASE_URL the
--     existing federation mappings are NOT deleted — they are marked lost with
--     this reason so they render read-only HISTORY while the user re-invites under
--     the new URL (peers reject the new URL until then). The keypair is preserved
--     (no key regen). SQLite (modernc) cannot ALTER an existing column-level CHECK
--     in place, so the table is recreated with the widened vocabulary, preserving
--     every row and every column added through 036. The recreate mirrors the
--     033/034/036 Down legs.
-- +goose StatementBegin
CREATE TABLE federation_retention_settings (
    id                       INTEGER PRIMARY KEY CHECK (id = 1),
    tombstone_retention_days INTEGER,
    outbox_retention_days    INTEGER,
    inbox_retention_days     INTEGER,
    updated_at               TEXT
);
-- +goose StatementEnd

-- Seed the single row empty so the compiled/config defaults apply until the owner
-- changes a value. INSERT OR IGNORE keeps a re-run idempotent.
INSERT OR IGNORE INTO federation_retention_settings (id) VALUES (1);

-- Recreate federated_projects so lost_reason accepts 'instance_url_changed'.
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_federated_projects_active;
DROP INDEX IF EXISTS idx_federated_projects_local;

CREATE TABLE federated_projects_v040 (
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
    lost              INTEGER NOT NULL DEFAULT 0 CHECK (lost IN (0, 1)),
    lost_reason       TEXT NOT NULL DEFAULT '' CHECK (lost_reason IN ('', 'revoked', 'left', 'owner-dead', 'instance_url_changed')),
    PRIMARY KEY (local_project_id, peer_instance_url)
);

INSERT INTO federated_projects_v040 (
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at,
    rebootstrap_cutoff_hlc, rebootstrapped_at, key_mismatch_at, lost, lost_reason)
SELECT
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at,
    rebootstrap_cutoff_hlc, rebootstrapped_at, key_mismatch_at, lost, lost_reason
FROM federated_projects;

DROP TABLE federated_projects;
ALTER TABLE federated_projects_v040 RENAME TO federated_projects;

CREATE INDEX idx_federated_projects_local ON federated_projects(local_project_id);
CREATE INDEX idx_federated_projects_active ON federated_projects(local_project_id) WHERE revoked = 0;
-- +goose StatementEnd

-- +goose Down
-- Drop the retention table and recreate federated_projects with the pre-040
-- lost_reason vocabulary (without 'instance_url_changed'), preserving rows. A row
-- carrying the new reason would violate the narrowed CHECK, so it is first
-- downgraded to the closest pre-040 read-only reason ('owner-dead') so the Down
-- leg never fails mid-migration.
DROP TABLE IF EXISTS federation_retention_settings;

-- +goose StatementBegin
UPDATE federated_projects SET lost_reason = 'owner-dead' WHERE lost_reason = 'instance_url_changed';

DROP INDEX IF EXISTS idx_federated_projects_active;
DROP INDEX IF EXISTS idx_federated_projects_local;

CREATE TABLE federated_projects_pre040 (
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
    lost              INTEGER NOT NULL DEFAULT 0 CHECK (lost IN (0, 1)),
    lost_reason       TEXT NOT NULL DEFAULT '' CHECK (lost_reason IN ('', 'revoked', 'left', 'owner-dead')),
    PRIMARY KEY (local_project_id, peer_instance_url)
);

INSERT INTO federated_projects_pre040 (
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at,
    rebootstrap_cutoff_hlc, rebootstrapped_at, key_mismatch_at, lost, lost_reason)
SELECT
    local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url,
    permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at,
    rebootstrap_cutoff_hlc, rebootstrapped_at, key_mismatch_at, lost, lost_reason
FROM federated_projects;

DROP TABLE federated_projects;
ALTER TABLE federated_projects_pre040 RENAME TO federated_projects;

CREATE INDEX idx_federated_projects_local ON federated_projects(local_project_id);
CREATE INDEX idx_federated_projects_active ON federated_projects(local_project_id) WHERE revoked = 0;
-- +goose StatementEnd

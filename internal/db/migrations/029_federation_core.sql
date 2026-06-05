-- +goose Up
-- Federation core bookkeeping schema (Federation v1 F1.1).
--
-- This migration lands the relational backbone of federation: the peer-instance
-- trust directory, the per-project peer mapping, invites, and the outbound /
-- inbound event queues.
--
-- IMPORTANT name-collision note (R21): the design doc's generic table names
-- `inbox`/`outbox` (FEDERATION-ARCH.md §6, §13) COLLIDE with the existing GTD
-- `inbox` container (001_schema.sql:6, referenced by tasks.inbox_id). Reusing
-- those names would make this migration fail outright. Every federation table is
-- therefore prefixed `federation_`/`federated_`. This migration must apply
-- cleanly to a DB that already contains the GTD `inbox` table.
--
-- Entity identity deviation (§3): we keep int64 AUTOINCREMENT PKs on the local
-- domain tables and map them to the cross-instance identity here.
-- federated_projects.local_project_id is the int64 projects.id; the doc's TEXT
-- local_project_id is intentionally NOT used. All wire timestamps are TEXT
-- ISO-8601 UTC (model.FormatUTC), not the doc's INTEGER ms.

-- federated_instances: the trust directory of peer instances we have shaken
-- hands with. Keyed by instance_url (the federation identity). display_name is
-- the human-readable name the peer carried in its handshake (users has no
-- display_name, R24) and is the source for "display_name @ instance.tld".
-- +goose StatementBegin
CREATE TABLE federated_instances (
    instance_url    TEXT PRIMARY KEY,
    public_key      TEXT NOT NULL,
    display_name    TEXT NOT NULL DEFAULT '',
    last_contact_at TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
-- +goose StatementEnd

-- federated_projects: the per-(local project, peer) federation mapping. The
-- owner's own instance gets a self-row with is_owner=1 and
-- peer_instance_url=origin_instance_url=this instance's URL. permissions is the
-- peer's grant on this project (read|write|admin). protocol_version is the
-- version negotiated at handshake (F0.4). last_sent_hlc/last_received_hlc are
-- the per-peer sync cursors (populated by the sync workers in later phases).
-- The composite PK lets one local project federate with many peers, but never
-- holds two rows for the same (project, peer).
-- +goose StatementBegin
CREATE TABLE federated_projects (
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
-- +goose StatementEnd

-- Lookups by local project (peers-for-project) and a partial index for the
-- non-revoked fan-out path used by later sync milestones.
CREATE INDEX idx_federated_projects_local ON federated_projects(local_project_id);
CREATE INDEX idx_federated_projects_active ON federated_projects(local_project_id) WHERE revoked = 0;

-- federation_invites: per-project share invites. The secret is NEVER stored in
-- plaintext (US-1.2 AC2) — only secret_hash = SHA-256(secret). invite_id is a
-- ULID/UUIDv7 (TEXT). Lifecycle: active → consumed_at / revoked_at / expired.
-- +goose StatementBegin
CREATE TABLE federation_invites (
    invite_id        TEXT PRIMARY KEY,
    local_project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    secret_hash      TEXT NOT NULL,
    permissions      TEXT NOT NULL CHECK (permissions IN ('read', 'write', 'admin')),
    -- Single-use invariant backstop (US-1.2 AC3 / US-2.2 AC3): used_count may
    -- never exceed max_uses and max_uses must be at least 1. The handshake
    -- consume path enforces this in-process via a self-guarding UPDATE
    -- (used_count < max_uses); this CHECK is the defense-in-depth so even a
    -- buggy/unguarded writer can never over-consume a single-use invite.
    max_uses         INTEGER NOT NULL DEFAULT 1 CHECK (max_uses >= 1),
    used_count       INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0 AND used_count <= max_uses),
    expires_at       TEXT,
    revoked_at       TEXT,
    consumed_at      TEXT,
    created_at       TEXT NOT NULL
);
-- +goose StatementEnd

CREATE INDEX idx_federation_invites_project ON federation_invites(local_project_id);

-- federation_outbox: the transactional outbound event log. A canonical signed
-- event is written here in the SAME tx as the domain write (NFR-2 crash-safety).
-- delivered_to tracks per-peer delivery (JSON in later phases); the publisher
-- worker drains it. event_id is the cross-instance dedup key. (The publisher,
-- backoff, and chunking land in Phase 3 — this is the storage backbone.)
-- +goose StatementBegin
CREATE TABLE federation_outbox (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id         TEXT NOT NULL UNIQUE,
    local_project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    payload          TEXT NOT NULL,
    delivered_to     TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL
);
-- +goose StatementEnd

CREATE INDEX idx_federation_outbox_project ON federation_outbox(local_project_id);

-- federation_inbox: the inbound event dedup + apply log. POST /federation/events
-- inserts ON CONFLICT(event_id) DO NOTHING (at-least-once + dedup, NFR-2), then
-- the single inbox-apply goroutine processes pending rows. (Apply lands in
-- Phase 3 — this is the storage backbone.)
-- +goose StatementBegin
CREATE TABLE federation_inbox (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id          TEXT NOT NULL UNIQUE,
    peer_instance_url TEXT NOT NULL,
    local_project_id  INTEGER REFERENCES projects(id) ON DELETE CASCADE,
    payload           TEXT NOT NULL,
    applied_at        TEXT,
    received_at       TEXT NOT NULL
);
-- +goose StatementEnd

CREATE INDEX idx_federation_inbox_pending ON federation_inbox(received_at) WHERE applied_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_federation_inbox_pending;
DROP TABLE IF EXISTS federation_inbox;
DROP INDEX IF EXISTS idx_federation_outbox_project;
DROP TABLE IF EXISTS federation_outbox;
DROP INDEX IF EXISTS idx_federation_invites_project;
DROP TABLE IF EXISTS federation_invites;
DROP INDEX IF EXISTS idx_federated_projects_active;
DROP INDEX IF EXISTS idx_federated_projects_local;
DROP TABLE IF EXISTS federated_projects;
DROP TABLE IF EXISTS federated_instances;

-- +goose Up
-- Offline-sync / federation overlay (Federation v1 F0.1).
--
-- Every synchronized entity gains two columns:
--   * client_id  TEXT — a stable, instance-portable identifier (UUIDv7, used as
--     the cross-instance entity id by federation). Nullable so the ALTER does
--     not need a default expression; backfilled below for existing rows and a
--     partial UNIQUE index enforces uniqueness only over the non-NULL values.
--   * deleted_at TEXT — soft-delete tombstone (ISO-8601 UTC, model.FormatUTC).
--     NULL means live; a non-NULL value is a final tombstone (re-edit → 410).
--
-- This is an ALTER-only migration (no new tables) so it cannot collide with the
-- existing `inbox` table (001_schema.sql:6).

-- +goose StatementBegin
ALTER TABLE tasks ADD COLUMN client_id TEXT;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tasks ADD COLUMN deleted_at TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE projects ADD COLUMN client_id TEXT;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE projects ADD COLUMN deleted_at TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE project_sections ADD COLUMN client_id TEXT;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE project_sections ADD COLUMN deleted_at TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE labels ADD COLUMN client_id TEXT;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE labels ADD COLUMN deleted_at TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE contexts ADD COLUMN client_id TEXT;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE contexts ADD COLUMN deleted_at TEXT;
-- +goose StatementEnd

-- Backfill ULID-shaped client ids for existing rows. SQLite has no native UUID
-- generator, so we synthesise a 128-bit random hex id with a UUIDv7-style
-- time-ordered prefix: the first 48 bits are the current unix-milliseconds
-- (matching google/uuid.NewV7 layout) so backfilled ids sort by creation time,
-- the rest is randomblob. This keeps existing rows uniquely identifiable; the
-- application uses model.NewClientID (google/uuid v7) for all new rows.
-- +goose StatementBegin
UPDATE tasks SET client_id =
    printf('%012x', CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)) ||
    lower(hex(randomblob(10)))
    WHERE client_id IS NULL;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE projects SET client_id =
    printf('%012x', CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)) ||
    lower(hex(randomblob(10)))
    WHERE client_id IS NULL;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE project_sections SET client_id =
    printf('%012x', CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)) ||
    lower(hex(randomblob(10)))
    WHERE client_id IS NULL;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE labels SET client_id =
    printf('%012x', CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)) ||
    lower(hex(randomblob(10)))
    WHERE client_id IS NULL;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE contexts SET client_id =
    printf('%012x', CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)) ||
    lower(hex(randomblob(10)))
    WHERE client_id IS NULL;
-- +goose StatementEnd

-- Partial UNIQUE indexes: client_id must be unique across rows that have one,
-- but multiple NULLs remain allowed (mirrors UNIQUE(client_id) WHERE NOT NULL).
CREATE UNIQUE INDEX idx_tasks_client_id            ON tasks(client_id)            WHERE client_id IS NOT NULL;
CREATE UNIQUE INDEX idx_projects_client_id         ON projects(client_id)         WHERE client_id IS NOT NULL;
CREATE UNIQUE INDEX idx_project_sections_client_id ON project_sections(client_id) WHERE client_id IS NOT NULL;
CREATE UNIQUE INDEX idx_labels_client_id           ON labels(client_id)           WHERE client_id IS NOT NULL;
CREATE UNIQUE INDEX idx_contexts_client_id         ON contexts(client_id)         WHERE client_id IS NOT NULL;

-- Partial indexes on deleted_at to keep the "live rows only" filter cheap.
CREATE INDEX idx_tasks_deleted_at            ON tasks(deleted_at)            WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_projects_deleted_at         ON projects(deleted_at)         WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_project_sections_deleted_at ON project_sections(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_labels_deleted_at           ON labels(deleted_at)           WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_contexts_deleted_at         ON contexts(deleted_at)         WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_contexts_deleted_at;
DROP INDEX IF EXISTS idx_labels_deleted_at;
DROP INDEX IF EXISTS idx_project_sections_deleted_at;
DROP INDEX IF EXISTS idx_projects_deleted_at;
DROP INDEX IF EXISTS idx_tasks_deleted_at;

DROP INDEX IF EXISTS idx_contexts_client_id;
DROP INDEX IF EXISTS idx_labels_client_id;
DROP INDEX IF EXISTS idx_project_sections_client_id;
DROP INDEX IF EXISTS idx_projects_client_id;
DROP INDEX IF EXISTS idx_tasks_client_id;

ALTER TABLE contexts DROP COLUMN deleted_at;
ALTER TABLE contexts DROP COLUMN client_id;
ALTER TABLE labels DROP COLUMN deleted_at;
ALTER TABLE labels DROP COLUMN client_id;
ALTER TABLE project_sections DROP COLUMN deleted_at;
ALTER TABLE project_sections DROP COLUMN client_id;
ALTER TABLE projects DROP COLUMN deleted_at;
ALTER TABLE projects DROP COLUMN client_id;
ALTER TABLE tasks DROP COLUMN deleted_at;
ALTER TABLE tasks DROP COLUMN client_id;

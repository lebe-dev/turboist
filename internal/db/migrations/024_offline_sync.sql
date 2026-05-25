-- +goose Up
-- Offline-sync foundation: every syncable entity gains a stable client-generated
-- id (set by the frontend before the server assigns its surrogate INTEGER PK)
-- plus a tombstone column. client_id is nullable so legacy rows keep working;
-- a partial unique index guards against accidental duplicates only when set.

ALTER TABLE tasks            ADD COLUMN client_id  TEXT;
ALTER TABLE tasks            ADD COLUMN deleted_at TEXT;
CREATE UNIQUE INDEX idx_tasks_client_id    ON tasks(client_id)    WHERE client_id IS NOT NULL;
CREATE INDEX idx_tasks_deleted_at          ON tasks(deleted_at)   WHERE deleted_at IS NOT NULL;

ALTER TABLE projects         ADD COLUMN client_id  TEXT;
ALTER TABLE projects         ADD COLUMN deleted_at TEXT;
CREATE UNIQUE INDEX idx_projects_client_id ON projects(client_id) WHERE client_id IS NOT NULL;
CREATE INDEX idx_projects_deleted_at       ON projects(deleted_at) WHERE deleted_at IS NOT NULL;

ALTER TABLE project_sections ADD COLUMN client_id  TEXT;
ALTER TABLE project_sections ADD COLUMN deleted_at TEXT;
CREATE UNIQUE INDEX idx_sections_client_id ON project_sections(client_id) WHERE client_id IS NOT NULL;
CREATE INDEX idx_sections_deleted_at       ON project_sections(deleted_at) WHERE deleted_at IS NOT NULL;

ALTER TABLE labels           ADD COLUMN client_id  TEXT;
ALTER TABLE labels           ADD COLUMN deleted_at TEXT;
CREATE UNIQUE INDEX idx_labels_client_id   ON labels(client_id)   WHERE client_id IS NOT NULL;
CREATE INDEX idx_labels_deleted_at         ON labels(deleted_at)  WHERE deleted_at IS NOT NULL;

ALTER TABLE contexts         ADD COLUMN client_id  TEXT;
ALTER TABLE contexts         ADD COLUMN deleted_at TEXT;
CREATE UNIQUE INDEX idx_contexts_client_id ON contexts(client_id) WHERE client_id IS NOT NULL;
CREATE INDEX idx_contexts_deleted_at       ON contexts(deleted_at) WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_contexts_deleted_at;
DROP INDEX IF EXISTS idx_contexts_client_id;
ALTER TABLE contexts DROP COLUMN deleted_at;
ALTER TABLE contexts DROP COLUMN client_id;

DROP INDEX IF EXISTS idx_labels_deleted_at;
DROP INDEX IF EXISTS idx_labels_client_id;
ALTER TABLE labels DROP COLUMN deleted_at;
ALTER TABLE labels DROP COLUMN client_id;

DROP INDEX IF EXISTS idx_sections_deleted_at;
DROP INDEX IF EXISTS idx_sections_client_id;
ALTER TABLE project_sections DROP COLUMN deleted_at;
ALTER TABLE project_sections DROP COLUMN client_id;

DROP INDEX IF EXISTS idx_projects_deleted_at;
DROP INDEX IF EXISTS idx_projects_client_id;
ALTER TABLE projects DROP COLUMN deleted_at;
ALTER TABLE projects DROP COLUMN client_id;

DROP INDEX IF EXISTS idx_tasks_deleted_at;
DROP INDEX IF EXISTS idx_tasks_client_id;
ALTER TABLE tasks DROP COLUMN deleted_at;
ALTER TABLE tasks DROP COLUMN client_id;

-- +goose Up
-- Task comments (Federation v1 F0.2 — schema-only, deferrable).
--
-- A `comments` row is an immutable note attached to a task: the application
-- never UPDATEs the body, so cross-instance federation never has to merge a
-- comment body — only create and (soft-)delete participate in sync. The table
-- carries the offline-sync / federation overlay columns (client_id, deleted_at)
-- exactly like the five synced entities from migration 024.
--
-- `comments` is a net-new table name; it does not collide with any live table
-- (001_schema.sql…024). The task FK is ON DELETE CASCADE, but soft-delete of a
-- task no longer fires that cascade — child tombstones are written by the
-- service layer in a later milestone (F3.3). The cascade is kept for the
-- hard-DELETE / GC path.

CREATE TABLE comments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    body        TEXT NOT NULL,
    client_id   TEXT,
    deleted_at  TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE INDEX idx_comments_task ON comments(task_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_comments_client_id ON comments(client_id) WHERE client_id IS NOT NULL;
CREATE INDEX idx_comments_deleted_at ON comments(deleted_at) WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_comments_deleted_at;
DROP INDEX IF EXISTS idx_comments_client_id;
DROP INDEX IF EXISTS idx_comments_task;
DROP TABLE IF EXISTS comments;

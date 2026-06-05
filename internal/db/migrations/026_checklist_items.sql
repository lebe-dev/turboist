-- +goose Up
-- Task checklist items (Federation v1 F0.2 — schema-only, deferrable).
--
-- A checklist item is a small sub-todo on a task: a title, a completed flag and
-- an ordering. It carries the offline-sync / federation overlay columns
-- (client_id, deleted_at) like the five synced entities from migration 024.
--
-- Ordering uses BOTH an integer `position` (the local, renormalising order, like
-- project_sections) AND a nullable `frac_position` fractional-index string. The
-- fractional key is the conflict-free ordering federation will use (§5.6 / R9):
-- integer position is renormalised on local reorder, which is incompatible with
-- a fractional key, so the federated ordering path writes `frac_position` lazily
-- while local UI keeps using `position`. v1 only lands the column; the
-- conflict-free reorder algorithm is deferred.
--
-- `checklist_items` is a net-new table name; it does not collide with any live
-- table (001_schema.sql…025). The task FK is ON DELETE CASCADE for the
-- hard-DELETE / GC path; soft-delete child tombstones are written by the service
-- layer in a later milestone (F3.3).

CREATE TABLE checklist_items (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id       INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    is_completed  INTEGER NOT NULL DEFAULT 0 CHECK (is_completed IN (0, 1)),
    position      INTEGER NOT NULL DEFAULT 0,
    frac_position TEXT,
    client_id     TEXT,
    deleted_at    TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE INDEX idx_checklist_items_task ON checklist_items(task_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_checklist_items_client_id ON checklist_items(client_id) WHERE client_id IS NOT NULL;
CREATE INDEX idx_checklist_items_deleted_at ON checklist_items(deleted_at) WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_checklist_items_deleted_at;
DROP INDEX IF EXISTS idx_checklist_items_client_id;
DROP INDEX IF EXISTS idx_checklist_items_task;
DROP TABLE IF EXISTS checklist_items;

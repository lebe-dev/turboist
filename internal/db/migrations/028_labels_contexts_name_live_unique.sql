-- +goose NO TRANSACTION
-- Reconcile name uniqueness with soft-delete (Federation v1 F0.1 fix).
--
-- 001_schema.sql declared a full-table UNIQUE(name) on both `labels` and
-- `contexts`. 024_offline_sync.sql then made Delete a soft-delete: the physical
-- row survives carrying its `name`, so the table-level UNIQUE(name) slot stays
-- occupied by an invisible tombstone. Recreating an entity with the same name
-- afterwards hit the UNIQUE violation -> ErrConflict -> 409 "name already
-- exists" against a row the user can neither see (every read filters
-- deleted_at IS NULL) nor recover.
--
-- Fix: drop the full-table UNIQUE(name) and replace it with a *live-only*
-- partial unique index (UNIQUE(name) WHERE deleted_at IS NULL), so a tombstoned
-- name frees its slot and can be recreated, while the tombstone itself is left
-- intact for federation per-field LWW / retention GC (it must survive the
-- >=90-day window — reviving it would break resurrection-prevention, US-3.7 AC2).
--
-- SQLite cannot drop a table-level constraint via ALTER, so each table is
-- rebuilt with the official 12-step procedure (https://sqlite.org/lang_altertable.html).
-- This requires PRAGMA foreign_keys=OFF, which is a no-op inside a transaction,
-- hence the NO TRANSACTION annotation above. The rebuilt tables keep the exact
-- current column set (001 + 014 labels.is_private + 024 client_id/deleted_at) and the
-- 024 partial indexes are recreated against the new tables. FKs from projects,
-- tasks, task_labels, project_labels reference these tables by name and remain
-- valid after the RENAME.

-- +goose Up
-- +goose StatementBegin
PRAGMA foreign_keys = OFF;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE labels_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    color         TEXT NOT NULL,
    is_favourite  INTEGER NOT NULL DEFAULT 0 CHECK (is_favourite IN (0, 1)),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    is_private    INTEGER NOT NULL DEFAULT 0 CHECK (is_private IN (0, 1)),
    client_id     TEXT,
    deleted_at    TEXT,
    CHECK (
        color IN ('red','orange','yellow','green','teal',
                  'blue','purple','pink','grey','brown')
        OR (length(color) = 7
            AND substr(color,1,1) = '#'
            AND lower(substr(color,2)) GLOB '[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]')
    )
);
-- +goose StatementEnd
-- +goose StatementBegin
INSERT INTO labels_new (id, name, color, is_favourite, created_at, updated_at, is_private, client_id, deleted_at)
    SELECT id, name, color, is_favourite, created_at, updated_at, is_private, client_id, deleted_at FROM labels;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE labels;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE labels_new RENAME TO labels;
-- +goose StatementEnd
-- Live-only name uniqueness: a tombstoned name no longer blocks recreation.
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_labels_name_live ON labels(name) WHERE deleted_at IS NULL;
-- +goose StatementEnd
-- Recreate the 024 partial indexes against the rebuilt table.
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_labels_client_id ON labels(client_id) WHERE client_id IS NOT NULL;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_labels_deleted_at ON labels(deleted_at) WHERE deleted_at IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE contexts_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    color         TEXT NOT NULL,
    is_favourite  INTEGER NOT NULL DEFAULT 0 CHECK (is_favourite IN (0, 1)),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    client_id     TEXT,
    deleted_at    TEXT,
    CHECK (
        color IN ('red','orange','yellow','green','teal',
                  'blue','purple','pink','grey','brown')
        OR (length(color) = 7
            AND substr(color,1,1) = '#'
            AND lower(substr(color,2)) GLOB '[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]')
    )
);
-- +goose StatementEnd
-- +goose StatementBegin
INSERT INTO contexts_new (id, name, color, is_favourite, created_at, updated_at, client_id, deleted_at)
    SELECT id, name, color, is_favourite, created_at, updated_at, client_id, deleted_at FROM contexts;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE contexts;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE contexts_new RENAME TO contexts;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_contexts_name_live ON contexts(name) WHERE deleted_at IS NULL;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_contexts_client_id ON contexts(client_id) WHERE client_id IS NOT NULL;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_contexts_deleted_at ON contexts(deleted_at) WHERE deleted_at IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
PRAGMA foreign_key_check;
-- +goose StatementEnd
-- +goose StatementBegin
PRAGMA foreign_keys = ON;
-- +goose StatementEnd

-- +goose Down
-- NO TRANSACTION (declared at the top) applies to the whole file, so the Down
-- direction also runs outside a transaction and can toggle foreign_keys.
-- +goose StatementBegin
PRAGMA foreign_keys = OFF;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_contexts_deleted_at;
-- +goose StatementEnd
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_contexts_client_id;
-- +goose StatementEnd
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_contexts_name_live;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE contexts_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    color         TEXT NOT NULL,
    is_favourite  INTEGER NOT NULL DEFAULT 0 CHECK (is_favourite IN (0, 1)),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    client_id     TEXT,
    deleted_at    TEXT,
    CHECK (
        color IN ('red','orange','yellow','green','teal',
                  'blue','purple','pink','grey','brown')
        OR (length(color) = 7
            AND substr(color,1,1) = '#'
            AND lower(substr(color,2)) GLOB '[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]')
    )
);
-- +goose StatementEnd
-- +goose StatementBegin
INSERT INTO contexts_old (id, name, color, is_favourite, created_at, updated_at, client_id, deleted_at)
    SELECT id, name, color, is_favourite, created_at, updated_at, client_id, deleted_at FROM contexts;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE contexts;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE contexts_old RENAME TO contexts;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_contexts_client_id ON contexts(client_id) WHERE client_id IS NOT NULL;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_contexts_deleted_at ON contexts(deleted_at) WHERE deleted_at IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_labels_deleted_at;
-- +goose StatementEnd
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_labels_client_id;
-- +goose StatementEnd
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_labels_name_live;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE labels_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    color         TEXT NOT NULL,
    is_favourite  INTEGER NOT NULL DEFAULT 0 CHECK (is_favourite IN (0, 1)),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    is_private    INTEGER NOT NULL DEFAULT 0 CHECK (is_private IN (0, 1)),
    client_id     TEXT,
    deleted_at    TEXT,
    CHECK (
        color IN ('red','orange','yellow','green','teal',
                  'blue','purple','pink','grey','brown')
        OR (length(color) = 7
            AND substr(color,1,1) = '#'
            AND lower(substr(color,2)) GLOB '[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]')
    )
);
-- +goose StatementEnd
-- +goose StatementBegin
INSERT INTO labels_old (id, name, color, is_favourite, created_at, updated_at, is_private, client_id, deleted_at)
    SELECT id, name, color, is_favourite, created_at, updated_at, is_private, client_id, deleted_at FROM labels;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE labels;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE labels_old RENAME TO labels;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_labels_client_id ON labels(client_id) WHERE client_id IS NOT NULL;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_labels_deleted_at ON labels(deleted_at) WHERE deleted_at IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
PRAGMA foreign_key_check;
-- +goose StatementEnd
-- +goose StatementBegin
PRAGMA foreign_keys = ON;
-- +goose StatementEnd

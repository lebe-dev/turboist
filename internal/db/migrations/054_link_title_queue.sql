-- +goose Up
-- Link titles (internal/service/linktitle): a task created with nothing but an
-- https:// URL as its title is queued here, and a background worker replaces the
-- title with the page's own ("Article «…»") and moves the URL into the
-- description.
--
-- A row exists only while the task still waits for its title: the worker deletes
-- it once the task was rewritten, given up on, or edited by the user in the
-- meantime. Server-side bookkeeping, deliberately NOT replicated (no change_log
-- triggers) — a replica sees the outcome through the ordinary task upsert.
CREATE TABLE link_title_queue (
    task_id         INTEGER PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    url             TEXT    NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT    NOT NULL,
    last_error      TEXT    NULL,
    created_at      TEXT    NOT NULL
);
CREATE INDEX idx_link_title_queue_due ON link_title_queue(next_attempt_at);

-- +goose Down
DROP INDEX IF EXISTS idx_link_title_queue_due;
DROP TABLE IF EXISTS link_title_queue;

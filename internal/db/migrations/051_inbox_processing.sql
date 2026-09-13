-- +goose Up
-- LLM Inbox processing (internal/service/inboxproc).
--
-- auto_sorted_at marks a task the background job filed out of the Inbox; NULL
-- means a person placed it. Any manual move clears it (repo.TaskRepo.Move): once
-- the user has relocated the task themselves, the placement is their decision.
-- The existing trg_changelog_tasks_upd trigger covers the column, so a replica
-- learns about the marker through the ordinary task upsert.
ALTER TABLE tasks ADD COLUMN auto_sorted_at TEXT NULL;

-- Both tables below are server-side bookkeeping and are deliberately NOT
-- replicated: they carry no change_log triggers (like idempotency_keys). A
-- replica sees the outcome of a decision through the task row itself.

-- Processing state of a task that is STILL in the Inbox. No row = not looked at
-- yet. The fingerprint (sha256 of title + "\n" + description at decision time)
-- lets an edited task be reconsidered.
CREATE TABLE inbox_processing_state (
    task_id         INTEGER PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    fingerprint     TEXT    NOT NULL,
    status          TEXT    NOT NULL CHECK (status IN ('kept', 'failed', 'reverted')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    -- Only for 'failed'; NULL means "wait until the task is edited".
    next_attempt_at TEXT    NULL,
    last_error      TEXT    NULL,
    updated_at      TEXT    NOT NULL
);

-- Decision journal. A deleted task keeps its rows with a copy of the title.
CREATE TABLE inbox_processing_log (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id           INTEGER NULL REFERENCES tasks(id) ON DELETE SET NULL,
    task_title        TEXT    NOT NULL,
    outcome           TEXT    NOT NULL CHECK (outcome IN ('sorted', 'kept', 'failed')),
    model             TEXT    NOT NULL,
    reason            TEXT    NOT NULL DEFAULT '',
    confidence        REAL    NULL,
    -- JSON: {labelIds, priority, dueAt, dueHasTime}
    before_state      TEXT    NOT NULL,
    -- JSON: {contextId, projectId, labelIds, priority, dueAt}
    after_state       TEXT    NULL,
    error             TEXT    NULL,
    prompt_tokens     INTEGER NULL,
    completion_tokens INTEGER NULL,
    reverted_at       TEXT    NULL,
    created_at        TEXT    NOT NULL
);
CREATE INDEX idx_inbox_processing_log_created ON inbox_processing_log(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_inbox_processing_log_created;
DROP TABLE IF EXISTS inbox_processing_log;
DROP TABLE IF EXISTS inbox_processing_state;
ALTER TABLE tasks DROP COLUMN auto_sorted_at;

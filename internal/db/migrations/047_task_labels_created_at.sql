-- +goose Up
-- Tagging timestamp per task↔label edge.
--
-- Until now the join table carried no time at all, so "how often was this label
-- used last week" could only be approximated from tasks.created_at — wrong for
-- every label attached after the task was created (the weekly-review re-tagging
-- workflow). The label-usage stats page needs the real application time.
--
-- Nullable on purpose: SQLite cannot ADD COLUMN NOT NULL without a constant
-- default, and a '' sentinel would sort before every real timestamp and break
-- MIN/MAX. Existing rows are backfilled from the task's creation time — the best
-- approximation available for historical data; readers treat a NULL that somehow
-- survives as "unknown", i.e. outside every range.
ALTER TABLE task_labels ADD COLUMN created_at TEXT;

UPDATE task_labels
   SET created_at = (SELECT t.created_at FROM tasks t WHERE t.id = task_labels.task_id)
 WHERE created_at IS NULL;

-- The stats query buckets rows by created_at across three rolling windows.
CREATE INDEX idx_task_labels_created_at ON task_labels(created_at) WHERE created_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_task_labels_created_at;
ALTER TABLE task_labels DROP COLUMN created_at;

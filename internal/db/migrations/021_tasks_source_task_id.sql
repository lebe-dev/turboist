-- +goose Up
-- Link recurrence completion snapshots back to their parent recurring task so
-- advanceRecurring can detect when a snapshot for the current day already
-- exists (prevents duplicate "completed yesterday" rows after a double-click
-- or any other accidental re-completion).
ALTER TABLE tasks ADD COLUMN source_task_id INTEGER REFERENCES tasks(id) ON DELETE SET NULL;
CREATE INDEX idx_tasks_source_task_id ON tasks(source_task_id) WHERE source_task_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_tasks_source_task_id;
ALTER TABLE tasks DROP COLUMN source_task_id;

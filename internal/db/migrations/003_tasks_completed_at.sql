-- +goose Up
ALTER TABLE tasks ADD COLUMN completed_at TEXT;
CREATE INDEX idx_tasks_completed_at ON tasks(completed_at) WHERE completed_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_tasks_completed_at;
ALTER TABLE tasks DROP COLUMN completed_at;

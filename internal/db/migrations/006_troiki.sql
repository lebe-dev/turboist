-- +goose Up
ALTER TABLE tasks ADD COLUMN troiki_category TEXT CHECK (troiki_category IN ('important','medium','rest'));
CREATE INDEX idx_tasks_troiki ON tasks(troiki_category) WHERE troiki_category IS NOT NULL;
ALTER TABLE users ADD COLUMN troiki_medium_capacity INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN troiki_rest_capacity INTEGER NOT NULL DEFAULT 0;

-- +goose Down
DROP INDEX IF EXISTS idx_tasks_troiki;
ALTER TABLE tasks DROP COLUMN troiki_category;
ALTER TABLE users DROP COLUMN troiki_medium_capacity;
ALTER TABLE users DROP COLUMN troiki_rest_capacity;

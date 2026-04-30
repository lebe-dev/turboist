-- +goose Up
ALTER TABLE tasks ADD COLUMN postpone_count INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tasks DROP COLUMN postpone_count;

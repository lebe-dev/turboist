-- +goose Up
ALTER TABLE tasks ADD COLUMN troiki_capacity_granted INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tasks DROP COLUMN troiki_capacity_granted;

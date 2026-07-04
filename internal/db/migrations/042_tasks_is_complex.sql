-- +goose Up
ALTER TABLE tasks ADD COLUMN is_complex INTEGER NOT NULL DEFAULT 0 CHECK (is_complex IN (0, 1));

-- +goose Down
ALTER TABLE tasks DROP COLUMN is_complex;

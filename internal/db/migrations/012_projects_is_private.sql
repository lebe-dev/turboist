-- +goose Up
ALTER TABLE projects ADD COLUMN is_private INTEGER NOT NULL DEFAULT 0 CHECK (is_private IN (0, 1));

-- +goose Down
ALTER TABLE projects DROP COLUMN is_private;

-- +goose Up
ALTER TABLE users ADD COLUMN state TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE users DROP COLUMN state;

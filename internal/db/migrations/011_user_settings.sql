-- +goose Up
ALTER TABLE users ADD COLUMN settings TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE users DROP COLUMN settings;

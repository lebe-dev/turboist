-- +goose Up
ALTER TABLE users ADD COLUMN troiki_started INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE users DROP COLUMN troiki_started;

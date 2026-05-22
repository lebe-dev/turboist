-- +goose Up
ALTER TABLE users ADD COLUMN totp_last_used_step INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE users DROP COLUMN totp_last_used_step;

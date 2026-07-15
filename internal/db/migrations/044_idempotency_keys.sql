-- +goose Up
-- Idempotency keys let offline clients replay queued mutations safely: a retry
-- of an already-executed request returns the stored response instead of
-- executing the handler again. Rows are pruned after 48h (see cmd/turboist).
CREATE TABLE idempotency_keys (
    key         TEXT PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    method      TEXT NOT NULL,
    path        TEXT NOT NULL,
    -- 0 = in flight (reserved before the handler runs), else the replayed status.
    status      INTEGER NOT NULL DEFAULT 0,
    response    TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_idempotency_created ON idempotency_keys(created_at);

-- +goose Down
DROP TABLE idempotency_keys;

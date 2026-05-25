-- +goose Up
-- Idempotency-Key cache. The offline frontend retries POST/PATCH/DELETE with
-- the same Idempotency-Key when the previous attempt timed out or failed
-- mid-flight. The middleware (see internal/httpapi/idempotency.go) records the
-- final response under (user_id, key) and replays it on retry so a network
-- glitch never creates a duplicate task/project/etc.
--
-- Rows expire after 24h — long enough to cover a phone left in flight mode
-- overnight, short enough that the table stays small without a janitor.
-- No FK to users(id): the table is single-user-app local, but keyed by
-- user_id for future multi-tenant safety. A FK would block test fixtures
-- that exercise per-user isolation against the CHECK(id = 1) on users.
CREATE TABLE idempotency_keys (
    user_id       INTEGER NOT NULL,
    key           TEXT    NOT NULL,
    status_code   INTEGER NOT NULL,
    content_type  TEXT    NOT NULL DEFAULT 'application/json',
    response_body BLOB    NOT NULL,
    created_at    TEXT    NOT NULL,
    PRIMARY KEY (user_id, key)
);
CREATE INDEX idx_idempotency_keys_created_at ON idempotency_keys(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_idempotency_keys_created_at;
DROP TABLE IF EXISTS idempotency_keys;

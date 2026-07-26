-- +goose Up
-- Idempotency keys let offline clients replay queued mutations safely: a retry
-- of an already-executed request returns the stored response instead of
-- executing the handler again. Rows are pruned after 48h (see cmd/turboist).
--
-- Some databases already carry an `idempotency_keys` table left over from the
-- abandoned federation/sync branch (migrations 024-040: removed from the repo,
-- but still recorded in goose_db_version on those installs). Its shape is
-- incompatible with the repo below ((user_id, key) PK, status_code /
-- content_type / response_body). It only ever held a 48h response cache, so
-- dropping it loses nothing.
DROP TABLE IF EXISTS idempotency_keys;

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

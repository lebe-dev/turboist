-- +goose Up
-- Allow the 'android' client_kind for native Capacitor sessions. SQLite cannot
-- alter a CHECK constraint in place, so the table is rebuilt with the widened
-- constraint. Nothing references sessions(id), so the drop/rename is safe.
CREATE TABLE sessions_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash    TEXT NOT NULL UNIQUE,
    client_kind   TEXT NOT NULL CHECK (client_kind IN ('web', 'ios', 'cli', 'android')),
    user_agent    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    last_used_at  TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    revoked_at    TEXT,
    ip_address    TEXT NOT NULL DEFAULT ''
);
INSERT INTO sessions_new (id, user_id, token_hash, client_kind, user_agent, created_at, last_used_at, expires_at, revoked_at, ip_address)
    SELECT id, user_id, token_hash, client_kind, user_agent, created_at, last_used_at, expires_at, revoked_at, ip_address FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_new RENAME TO sessions;
CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_active  ON sessions(expires_at) WHERE revoked_at IS NULL;

-- +goose Down
-- Revert to the original constraint. Android sessions are dropped first so the
-- copy into the narrower CHECK does not fail.
DELETE FROM sessions WHERE client_kind = 'android';
CREATE TABLE sessions_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash    TEXT NOT NULL UNIQUE,
    client_kind   TEXT NOT NULL CHECK (client_kind IN ('web', 'ios', 'cli')),
    user_agent    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    last_used_at  TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    revoked_at    TEXT,
    ip_address    TEXT NOT NULL DEFAULT ''
);
INSERT INTO sessions_old (id, user_id, token_hash, client_kind, user_agent, created_at, last_used_at, expires_at, revoked_at, ip_address)
    SELECT id, user_id, token_hash, client_kind, user_agent, created_at, last_used_at, expires_at, revoked_at, ip_address FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_old RENAME TO sessions;
CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_active  ON sessions(expires_at) WHERE revoked_at IS NULL;

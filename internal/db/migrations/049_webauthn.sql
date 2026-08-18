-- +goose Up
-- Passkeys (WebAuthn). A passkey is an ADDITIONAL login method: the password
-- stays the account's recovery path, so nothing here touches `users`.

-- The WebAuthn user handle: an opaque, stable, random byte string the
-- authenticator stores next to the credential and echoes back on a
-- discoverable ("usernameless") login. It must NOT be derivable from anything
-- user-visible, hence its own row rather than a formula over users.id — and it
-- must survive secret rotation, hence a stored value rather than an HMAC.
CREATE TABLE webauthn_users (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    handle     TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

-- One row per registered credential. `credential` holds the JSON-serialised
-- go-webauthn Credential Record (public key, flags, attestation, AAGUID); it is
-- rewritten after every successful assertion because the signature counter and
-- backup-state flags live inside it. The columns beside it exist only for
-- lookup (`credential_id`) and for the settings UI (`name`, timestamps).
CREATE TABLE webauthn_credentials (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    credential    TEXT NOT NULL,
    sign_count    INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    last_used_at  TEXT
);
CREATE INDEX idx_webauthn_credentials_user ON webauthn_credentials(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_webauthn_credentials_user;
DROP TABLE IF EXISTS webauthn_credentials;
DROP TABLE IF EXISTS webauthn_users;

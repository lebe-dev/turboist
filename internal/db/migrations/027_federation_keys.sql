-- +goose Up
-- Federation v1 F0.3: single-row instance trust-plane keypair + identity.
--
-- One row (id=1) holds this instance's Ed25519 keypair and federation identity.
-- The keypair is the separate trust plane used to sign peer-to-peer federation
-- requests (distinct from the HS256 JWT / HMAC API-token planes). The private
-- seed is stored encrypted with the shared TokenCipher (FEDERATION_KEY env),
-- mirroring totp_secret (019) and the app_settings id=1 singleton (017).
--
-- Table name `federation_keys` was audited free against the live schema
-- (001..026); the GTD `inbox`/`outbox` collision does not apply here.
CREATE TABLE federation_keys (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    public_key      TEXT NOT NULL,
    private_seed_enc TEXT NOT NULL,
    -- node_id is a stable, generated install UUID used as the HLC tie-break id
    -- (R10). It is intentionally NOT derived from BASE_URL host, which changes
    -- behind proxies and would break the HLC total order.
    node_id         TEXT NOT NULL,
    -- display_name is this instance's human-readable name carried by the
    -- handshake. `users` has no display_name, so this is the only source for
    -- the "display_name @ instance.tld" rendering (US-1.4 AC2 / US-3.5 AC4).
    -- Defaults to the host of BASE_URL, set at lazy-gen time by the repo.
    display_name    TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS federation_keys;

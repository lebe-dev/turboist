-- +goose Up
-- Granular permissions for API tokens. Each token carries a JSON array of
-- "resource:action" strings (e.g. ["tasks:read","projects:write"]). The
-- special value ["*"] grants full access and is used as DEFAULT so existing
-- tokens keep working without a data migration. CHECK(json_valid) prevents
-- corrupt JSON from poisoning every request that loads the token.
ALTER TABLE api_tokens ADD COLUMN scopes TEXT NOT NULL DEFAULT '["*"]'
    CHECK(json_valid(scopes));

-- +goose Down
ALTER TABLE api_tokens DROP COLUMN scopes;

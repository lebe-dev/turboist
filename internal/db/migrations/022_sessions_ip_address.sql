-- +goose Up
-- Record the client IP at session creation/rotation so users can recognise
-- where a session is from in Settings → Sessions. NOT NULL DEFAULT '' keeps
-- existing rows valid; the value is set from Fiber's c.IP() (honours
-- ProxyHeader configured by the app) when issued.
ALTER TABLE sessions ADD COLUMN ip_address TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE sessions DROP COLUMN ip_address;

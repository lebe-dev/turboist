-- +goose Up
CREATE TABLE app_settings (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data TEXT NOT NULL DEFAULT '{}'
);
INSERT INTO app_settings (id, data) VALUES (1, '{}');

-- +goose Down
DROP TABLE IF EXISTS app_settings;

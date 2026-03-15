CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS user_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT OR IGNORE INTO user_state (key, value) VALUES ('pinned_tasks', '[]');
INSERT OR IGNORE INTO user_state (key, value) VALUES ('active_context_id', '');
INSERT OR IGNORE INTO user_state (key, value) VALUES ('active_view', 'all');
INSERT OR IGNORE INTO user_state (key, value) VALUES ('collapsed_ids', '[]');
INSERT OR IGNORE INTO user_state (key, value) VALUES ('sidebar_collapsed', 'false');
INSERT OR IGNORE INTO user_state (key, value) VALUES ('planning_open', 'false');

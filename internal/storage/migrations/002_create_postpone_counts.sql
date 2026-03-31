CREATE TABLE IF NOT EXISTS postpone_counts (
    task_id TEXT PRIMARY KEY,
    count   INTEGER NOT NULL DEFAULT 0
);

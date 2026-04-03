CREATE TABLE IF NOT EXISTS auto_remove_first_seen (
    task_id    TEXT NOT NULL,
    label      TEXT NOT NULL,
    first_seen TEXT NOT NULL,
    PRIMARY KEY (task_id, label)
);

-- +goose Up
-- Task templates: a reusable root task plus an ordered set of subtasks. Each
-- row (root and subtask) carries the editable task fields a template captures —
-- title/description/priority/day_part — and an optional set of labels. Templates
-- are single-user local configuration (like app settings): they are NOT part of
-- the federation/sync overlay, so they carry no client_id/deleted_at and are
-- hard-deleted. Label links cascade so deleting a label cleans up its template
-- references, and deleting a template cascades to its subtasks and label links.
CREATE TABLE task_templates (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    priority    TEXT    NOT NULL DEFAULT 'no-priority',
    day_part    TEXT    NOT NULL DEFAULT 'none',
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);

CREATE TABLE task_template_subtasks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id INTEGER NOT NULL REFERENCES task_templates(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL DEFAULT 0,
    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    priority    TEXT    NOT NULL DEFAULT 'no-priority',
    day_part    TEXT    NOT NULL DEFAULT 'none',
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);

CREATE INDEX idx_task_template_subtasks_template ON task_template_subtasks(template_id, position);

CREATE TABLE task_template_labels (
    template_id INTEGER NOT NULL REFERENCES task_templates(id) ON DELETE CASCADE,
    label_id    INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (template_id, label_id)
);

CREATE TABLE task_template_subtask_labels (
    subtask_id INTEGER NOT NULL REFERENCES task_template_subtasks(id) ON DELETE CASCADE,
    label_id   INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (subtask_id, label_id)
);

-- +goose Down
DROP TABLE task_template_subtask_labels;
DROP TABLE task_template_labels;
DROP INDEX idx_task_template_subtasks_template;
DROP TABLE task_template_subtasks;
DROP TABLE task_templates;

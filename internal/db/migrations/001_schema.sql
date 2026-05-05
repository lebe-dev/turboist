-- +goose Up
-- +goose StatementBegin
PRAGMA foreign_keys = ON;
-- +goose StatementEnd

CREATE TABLE inbox (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    created_at  TEXT NOT NULL
);
INSERT INTO inbox (id, created_at) VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

CREATE TABLE contexts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    color         TEXT NOT NULL,
    is_favourite  INTEGER NOT NULL DEFAULT 0 CHECK (is_favourite IN (0, 1)),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    CHECK (
        color IN ('red','orange','yellow','green','teal',
                  'blue','purple','pink','grey','brown')
        OR (length(color) = 7
            AND substr(color,1,1) = '#'
            AND lower(substr(color,2)) GLOB '[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]')
    )
);

CREATE TABLE labels (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    color         TEXT NOT NULL,
    is_favourite  INTEGER NOT NULL DEFAULT 0 CHECK (is_favourite IN (0, 1)),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    CHECK (
        color IN ('red','orange','yellow','green','teal',
                  'blue','purple','pink','grey','brown')
        OR (length(color) = 7
            AND substr(color,1,1) = '#'
            AND lower(substr(color,2)) GLOB '[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]')
    )
);

CREATE TABLE projects (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    context_id   INTEGER NOT NULL REFERENCES contexts(id) ON DELETE CASCADE,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    color        TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'open'
                  CHECK (status IN ('open', 'completed', 'archived', 'cancelled')),
    is_pinned    INTEGER NOT NULL DEFAULT 0 CHECK (is_pinned IN (0, 1)),
    pinned_at    TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    CHECK ((is_pinned = 1 AND pinned_at IS NOT NULL)
        OR (is_pinned = 0 AND pinned_at IS NULL)),
    CHECK (
        color IN ('red','orange','yellow','green','teal',
                  'blue','purple','pink','grey','brown')
        OR (length(color) = 7
            AND substr(color,1,1) = '#'
            AND lower(substr(color,2)) GLOB '[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]')
    )
);
CREATE INDEX idx_projects_context ON projects(context_id);
CREATE INDEX idx_projects_status  ON projects(status);
CREATE INDEX idx_projects_pinned  ON projects(pinned_at) WHERE is_pinned = 1;

CREATE TABLE project_sections (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX idx_sections_project ON project_sections(project_id);

CREATE TABLE tasks (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    title              TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',

    inbox_id           INTEGER REFERENCES inbox(id)             ON DELETE RESTRICT,
    context_id         INTEGER REFERENCES contexts(id)          ON DELETE CASCADE,
    project_id         INTEGER REFERENCES projects(id)          ON DELETE CASCADE,
    section_id         INTEGER REFERENCES project_sections(id)  ON DELETE SET NULL,
    parent_id          INTEGER REFERENCES tasks(id)             ON DELETE CASCADE,

    priority           TEXT NOT NULL DEFAULT 'no-priority'
                        CHECK (priority IN ('high', 'medium', 'low', 'no-priority')),
    status             TEXT NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open', 'completed', 'cancelled')),

    due_at             TEXT,
    due_has_time       INTEGER NOT NULL DEFAULT 0 CHECK (due_has_time IN (0, 1)),
    deadline_at        TEXT,
    deadline_has_time  INTEGER NOT NULL DEFAULT 0 CHECK (deadline_has_time IN (0, 1)),

    day_part           TEXT NOT NULL DEFAULT 'none'
                        CHECK (day_part IN ('none', 'morning', 'afternoon', 'evening')),
    plan_state         TEXT NOT NULL DEFAULT 'none'
                        CHECK (plan_state IN ('none', 'week', 'backlog')),

    is_pinned          INTEGER NOT NULL DEFAULT 0 CHECK (is_pinned IN (0, 1)),
    pinned_at          TEXT,

    recurrence_rule    TEXT,

    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,

    CHECK (
        (inbox_id IS NOT NULL AND context_id IS NULL)
     OR (inbox_id IS NULL     AND context_id IS NOT NULL)
    ),
    CHECK (inbox_id IS NULL OR (project_id IS NULL AND section_id IS NULL)),
    CHECK (inbox_id IS NULL OR parent_id IS NULL),
    CHECK (section_id IS NULL OR project_id IS NOT NULL),
    CHECK ((is_pinned = 1 AND pinned_at IS NOT NULL)
        OR (is_pinned = 0 AND pinned_at IS NULL)),
    CHECK (due_has_time = 0 OR due_at IS NOT NULL),
    CHECK (deadline_has_time = 0 OR deadline_at IS NOT NULL)
);

CREATE INDEX idx_tasks_inbox      ON tasks(inbox_id)   WHERE inbox_id IS NOT NULL;
CREATE INDEX idx_tasks_context    ON tasks(context_id) WHERE context_id IS NOT NULL;
CREATE INDEX idx_tasks_project    ON tasks(project_id) WHERE project_id IS NOT NULL;
CREATE INDEX idx_tasks_section    ON tasks(section_id) WHERE section_id IS NOT NULL;
CREATE INDEX idx_tasks_parent     ON tasks(parent_id)  WHERE parent_id  IS NOT NULL;
CREATE INDEX idx_tasks_status     ON tasks(status);
CREATE INDEX idx_tasks_plan_state ON tasks(plan_state) WHERE plan_state != 'none';
CREATE INDEX idx_tasks_due        ON tasks(due_at)     WHERE due_at IS NOT NULL;
CREATE INDEX idx_tasks_deadline   ON tasks(deadline_at) WHERE deadline_at IS NOT NULL;
CREATE INDEX idx_tasks_pinned     ON tasks(pinned_at)  WHERE is_pinned = 1;
CREATE INDEX idx_tasks_recurring  ON tasks(id)         WHERE recurrence_rule IS NOT NULL;

CREATE TABLE task_labels (
    task_id   INTEGER NOT NULL REFERENCES tasks(id)  ON DELETE CASCADE,
    label_id  INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, label_id)
);
CREATE INDEX idx_task_labels_label ON task_labels(label_id);

CREATE TABLE project_labels (
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    label_id    INTEGER NOT NULL REFERENCES labels(id)   ON DELETE CASCADE,
    PRIMARY KEY (project_id, label_id)
);
CREATE INDEX idx_project_labels_label ON project_labels(label_id);

-- +goose Down
DROP TABLE IF EXISTS project_labels;
DROP TABLE IF EXISTS task_labels;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS project_sections;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS labels;
DROP TABLE IF EXISTS contexts;
DROP TABLE IF EXISTS inbox;

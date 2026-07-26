-- +goose Up
-- Directed relations between two tasks.
--
-- `blocks`: source blocks target — the target cannot be completed while the
-- source is still `open`. A completed OR cancelled source no longer blocks,
-- otherwise a cancelled task would deadlock its dependents forever.
--
-- `related`: symmetric, purely informational. The service normalises the pair
-- so source_task_id < target_task_id, which lets the UNIQUE constraint dedupe
-- A↔B without a second lookup.
--
-- A surrogate id (rather than a composite PK like task_labels) so a relation can
-- be addressed directly by DELETE /api/v1/tasks/:id/relations/:relationId.
-- Both FKs cascade: tasks are hard-deleted, there is no tombstone to hang onto.
CREATE TABLE task_relations (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    source_task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    target_task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    type           TEXT NOT NULL CHECK (type IN ('related', 'blocks')),
    created_at     TEXT NOT NULL,
    UNIQUE (source_task_id, target_task_id, type),
    CHECK (source_task_id <> target_task_id)
);
-- Both directions are queried: the blocker lookup walks by target, the
-- relations list for a task walks by both ends.
CREATE INDEX idx_task_relations_source ON task_relations(source_task_id);
CREATE INDEX idx_task_relations_target ON task_relations(target_task_id);

-- +goose Down
DROP TABLE task_relations;

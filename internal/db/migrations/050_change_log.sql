-- +goose Up
-- Append-only changelog: the record of every committed mutation of a syncable
-- entity, so a replica that has been offline can learn what changed without
-- re-downloading everything — deletions included.
--
-- Written by triggers rather than by the repositories on purpose. Rows are
-- written by many paths — direct repo calls, service cascades (parking a parent
-- task in the backlog rewrites every open descendant), bulk statements that
-- touch dozens of rows at once, foreign-key cascades that nobody calls at all —
-- and repo-layer discipline would have to remember every one of them, today and
-- for every write path added later. As triggers, logging is a property of the
-- schema instead: a row cannot change without being logged.
--
-- The log stores pointers, never payloads: entity + id + op. Readers join back
-- to the live tables for the current state, which is also why a redundant row
-- (an updated_at bump that changed no user-visible field) costs nothing but a
-- refetch. Multiple rows per entity are expected — readers dedupe by
-- (entity, entity_id) keeping the highest seq.
--
-- Delete rows are the tombstones. Entity tables keep their hard deletes: there
-- is no deleted_at column anywhere, and a delete row here is all a replica needs.
--
-- AUTOINCREMENT (not a bare INTEGER PRIMARY KEY) so seq is strictly increasing
-- and never reused: pruning old rows must not hand a stale cursor a seq that
-- points at a newer change.
CREATE TABLE change_log (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    entity     TEXT    NOT NULL,
    entity_id  INTEGER NOT NULL,
    op         TEXT    NOT NULL CHECK (op IN ('upsert', 'delete')),
    changed_at TEXT    NOT NULL
);
CREATE INDEX idx_change_log_entity ON change_log(entity, entity_id);

-- The epoch invalidates every cursor at once. Truncating the log (a backup
-- restore rewrites history wholesale) bumps it; a replica presenting a cursor
-- from an older epoch is told to start over from a full snapshot instead of
-- silently diverging. It lives in its own column rather than inside the
-- app_settings JSON blob because it is sync bookkeeping, not a user-facing
-- setting, and the settings payload must keep its shape.
ALTER TABLE app_settings ADD COLUMN sync_epoch INTEGER NOT NULL DEFAULT 1;

-- Entities that map one-to-one onto a table row.

-- tasks -> 'task'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_tasks_ins AFTER INSERT ON tasks
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_tasks_upd AFTER UPDATE ON tasks
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_tasks_del AFTER DELETE ON tasks
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- projects -> 'project'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_projects_ins AFTER INSERT ON projects
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('project', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_projects_upd AFTER UPDATE ON projects
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('project', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_projects_del AFTER DELETE ON projects
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('project', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- project_sections -> 'section'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_project_sections_ins AFTER INSERT ON project_sections
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('section', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_project_sections_upd AFTER UPDATE ON project_sections
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('section', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_project_sections_del AFTER DELETE ON project_sections
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('section', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- contexts -> 'context'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_contexts_ins AFTER INSERT ON contexts
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('context', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_contexts_upd AFTER UPDATE ON contexts
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('context', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_contexts_del AFTER DELETE ON contexts
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('context', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- labels -> 'label'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_labels_ins AFTER INSERT ON labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('label', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_labels_upd AFTER UPDATE ON labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('label', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_labels_del AFTER DELETE ON labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('label', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- task_relations -> 'task_relation'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_relations_ins AFTER INSERT ON task_relations
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_relation', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_relations_upd AFTER UPDATE ON task_relations
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_relation', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_relations_del AFTER DELETE ON task_relations
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_relation', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- task_templates -> 'task_template'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_templates_ins AFTER INSERT ON task_templates
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_templates_upd AFTER UPDATE ON task_templates
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_templates_del AFTER DELETE ON task_templates
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- Child and join tables carry no entity of their own: the API serves them
-- inside their owner's payload, so a row change is logged as an upsert of the
-- owner. The delete triggers are guarded by the owner still existing — when the
-- owner is what got deleted, its own delete row is the change the client needs,
-- and a stale upsert of a vanished id would be noise.

-- task_labels -> upsert of the owning 'task'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_labels_ins AFTER INSERT ON task_labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task', NEW.task_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_labels_upd AFTER UPDATE ON task_labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task', NEW.task_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    SELECT 'task', OLD.task_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
     WHERE OLD.task_id IS NOT NEW.task_id
       AND EXISTS (SELECT 1 FROM tasks WHERE id = OLD.task_id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_labels_del AFTER DELETE ON task_labels
WHEN EXISTS (SELECT 1 FROM tasks WHERE id = OLD.task_id)
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task', OLD.task_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- project_labels -> upsert of the owning 'project'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_project_labels_ins AFTER INSERT ON project_labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('project', NEW.project_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_project_labels_upd AFTER UPDATE ON project_labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('project', NEW.project_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    SELECT 'project', OLD.project_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
     WHERE OLD.project_id IS NOT NEW.project_id
       AND EXISTS (SELECT 1 FROM projects WHERE id = OLD.project_id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_project_labels_del AFTER DELETE ON project_labels
WHEN EXISTS (SELECT 1 FROM projects WHERE id = OLD.project_id)
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('project', OLD.project_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- task_template_subtasks -> upsert of the owning 'task_template'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_template_subtasks_ins AFTER INSERT ON task_template_subtasks
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', NEW.template_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_template_subtasks_upd AFTER UPDATE ON task_template_subtasks
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', NEW.template_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    SELECT 'task_template', OLD.template_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
     WHERE OLD.template_id IS NOT NEW.template_id
       AND EXISTS (SELECT 1 FROM task_templates WHERE id = OLD.template_id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_template_subtasks_del AFTER DELETE ON task_template_subtasks
WHEN EXISTS (SELECT 1 FROM task_templates WHERE id = OLD.template_id)
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', OLD.template_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- task_template_labels -> upsert of the owning 'task_template'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_template_labels_ins AFTER INSERT ON task_template_labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', NEW.template_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_template_labels_upd AFTER UPDATE ON task_template_labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', NEW.template_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    SELECT 'task_template', OLD.template_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
     WHERE OLD.template_id IS NOT NEW.template_id
       AND EXISTS (SELECT 1 FROM task_templates WHERE id = OLD.template_id);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_template_labels_del AFTER DELETE ON task_template_labels
WHEN EXISTS (SELECT 1 FROM task_templates WHERE id = OLD.template_id)
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', OLD.template_id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- task_template_subtask_labels -> upsert of the owning 'task_template'
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_template_subtask_labels_ins AFTER INSERT ON task_template_subtask_labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', (SELECT template_id FROM task_template_subtasks WHERE id = NEW.subtask_id), 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_template_subtask_labels_upd AFTER UPDATE ON task_template_subtask_labels
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', (SELECT template_id FROM task_template_subtasks WHERE id = NEW.subtask_id), 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    SELECT 'task_template', (SELECT template_id FROM task_template_subtasks WHERE id = OLD.subtask_id), 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
     WHERE OLD.subtask_id IS NOT NEW.subtask_id
       AND (SELECT template_id FROM task_template_subtasks WHERE id = OLD.subtask_id) IS NOT NULL;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_task_template_subtask_labels_del AFTER DELETE ON task_template_subtask_labels
WHEN (SELECT template_id FROM task_template_subtasks WHERE id = OLD.subtask_id) IS NOT NULL
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('task_template', (SELECT template_id FROM task_template_subtasks WHERE id = OLD.subtask_id), 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd
-- The single user row carries both the preference blob and the opaque UI-state
-- blob, which the API serves as two separate resources — hence two entities off
-- one table. Any write logs a user_settings upsert (a password or 2FA change
-- logs a redundant one; the cost is one refetch of a small payload), and a write
-- that touches the state column logs the state entity as well.
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_users_ins AFTER INSERT ON users
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('user_settings', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('user_state', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_users_upd AFTER UPDATE ON users
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('user_settings', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_users_state_upd AFTER UPDATE OF state ON users
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('user_state', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_users_del AFTER DELETE ON users
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('user_settings', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('user_state', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- app_settings -> 'app_settings'. Only the data column is watched: sync_epoch
-- lives in the same row but is machinery, and bumping it must not look like a
-- settings change to the clients that are about to be told to resync anyway.
-- +goose StatementBegin
CREATE TRIGGER trg_changelog_app_settings_ins AFTER INSERT ON app_settings
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('app_settings', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_app_settings_upd AFTER UPDATE OF data ON app_settings
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('app_settings', NEW.id, 'upsert', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_changelog_app_settings_del AFTER DELETE ON app_settings
BEGIN
    INSERT INTO change_log (entity, entity_id, op, changed_at)
    VALUES ('app_settings', OLD.id, 'delete', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
END;
-- +goose StatementEnd

-- Deliberately not logged: sessions, api_tokens, totp_recovery_codes and the
-- webauthn tables (credentials that must never leave the server), the
-- idempotency_keys replay cache, the calendar tables (read-only integration
-- state fetched from the provider, not user data), and the single inbox marker
-- row, which never changes.

-- +goose Down
DROP TRIGGER IF EXISTS trg_changelog_tasks_ins;
DROP TRIGGER IF EXISTS trg_changelog_tasks_upd;
DROP TRIGGER IF EXISTS trg_changelog_tasks_del;
DROP TRIGGER IF EXISTS trg_changelog_projects_ins;
DROP TRIGGER IF EXISTS trg_changelog_projects_upd;
DROP TRIGGER IF EXISTS trg_changelog_projects_del;
DROP TRIGGER IF EXISTS trg_changelog_project_sections_ins;
DROP TRIGGER IF EXISTS trg_changelog_project_sections_upd;
DROP TRIGGER IF EXISTS trg_changelog_project_sections_del;
DROP TRIGGER IF EXISTS trg_changelog_contexts_ins;
DROP TRIGGER IF EXISTS trg_changelog_contexts_upd;
DROP TRIGGER IF EXISTS trg_changelog_contexts_del;
DROP TRIGGER IF EXISTS trg_changelog_labels_ins;
DROP TRIGGER IF EXISTS trg_changelog_labels_upd;
DROP TRIGGER IF EXISTS trg_changelog_labels_del;
DROP TRIGGER IF EXISTS trg_changelog_task_relations_ins;
DROP TRIGGER IF EXISTS trg_changelog_task_relations_upd;
DROP TRIGGER IF EXISTS trg_changelog_task_relations_del;
DROP TRIGGER IF EXISTS trg_changelog_task_templates_ins;
DROP TRIGGER IF EXISTS trg_changelog_task_templates_upd;
DROP TRIGGER IF EXISTS trg_changelog_task_templates_del;
DROP TRIGGER IF EXISTS trg_changelog_task_labels_ins;
DROP TRIGGER IF EXISTS trg_changelog_task_labels_upd;
DROP TRIGGER IF EXISTS trg_changelog_task_labels_del;
DROP TRIGGER IF EXISTS trg_changelog_project_labels_ins;
DROP TRIGGER IF EXISTS trg_changelog_project_labels_upd;
DROP TRIGGER IF EXISTS trg_changelog_project_labels_del;
DROP TRIGGER IF EXISTS trg_changelog_task_template_subtasks_ins;
DROP TRIGGER IF EXISTS trg_changelog_task_template_subtasks_upd;
DROP TRIGGER IF EXISTS trg_changelog_task_template_subtasks_del;
DROP TRIGGER IF EXISTS trg_changelog_task_template_labels_ins;
DROP TRIGGER IF EXISTS trg_changelog_task_template_labels_upd;
DROP TRIGGER IF EXISTS trg_changelog_task_template_labels_del;
DROP TRIGGER IF EXISTS trg_changelog_task_template_subtask_labels_ins;
DROP TRIGGER IF EXISTS trg_changelog_task_template_subtask_labels_upd;
DROP TRIGGER IF EXISTS trg_changelog_task_template_subtask_labels_del;
DROP TRIGGER IF EXISTS trg_changelog_users_ins;
DROP TRIGGER IF EXISTS trg_changelog_users_upd;
DROP TRIGGER IF EXISTS trg_changelog_users_state_upd;
DROP TRIGGER IF EXISTS trg_changelog_users_del;
DROP TRIGGER IF EXISTS trg_changelog_app_settings_ins;
DROP TRIGGER IF EXISTS trg_changelog_app_settings_upd;
DROP TRIGGER IF EXISTS trg_changelog_app_settings_del;
DROP INDEX IF EXISTS idx_change_log_entity;
DROP TABLE IF EXISTS change_log;
ALTER TABLE app_settings DROP COLUMN sync_epoch;

-- +goose Up
-- today_lock_exempt marks a project the Today focus lock never keeps out: work for
-- today may be added to it even when it has no open task on Today yet. The existing
-- trg_changelog_projects_upd trigger covers the column.
ALTER TABLE projects ADD COLUMN today_lock_exempt INTEGER NOT NULL DEFAULT 0 CHECK (today_lock_exempt IN (0, 1));

-- +goose Down
ALTER TABLE projects DROP COLUMN today_lock_exempt;

-- +goose Up
-- Add Troiki category to projects. The drop of tasks.troiki_category is deferred
-- to a follow-up migration once repo/service/handlers stop referencing it.
ALTER TABLE projects ADD COLUMN troiki_category TEXT CHECK (troiki_category IN ('important','medium','rest'));
CREATE INDEX idx_projects_troiki ON projects(troiki_category) WHERE troiki_category IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_projects_troiki;
ALTER TABLE projects DROP COLUMN troiki_category;

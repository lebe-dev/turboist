-- +goose Up
ALTER TABLE projects ADD COLUMN project_type TEXT NOT NULL DEFAULT 'generic' CHECK (project_type IN ('generic', 'software'));

-- +goose Down
ALTER TABLE projects DROP COLUMN project_type;

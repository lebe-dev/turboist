-- +goose Up
ALTER TABLE project_sections ADD COLUMN position INTEGER NOT NULL DEFAULT 0;

UPDATE project_sections AS s
SET position = (
    SELECT COUNT(*) FROM project_sections s2
    WHERE s2.project_id = s.project_id
      AND (s2.created_at < s.created_at
           OR (s2.created_at = s.created_at AND s2.id < s.id))
);

CREATE INDEX idx_sections_project_position ON project_sections(project_id, position);

-- +goose Down
DROP INDEX IF EXISTS idx_sections_project_position;
ALTER TABLE project_sections DROP COLUMN position;

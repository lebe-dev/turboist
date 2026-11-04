-- +goose Up
-- description explains what a label means, so people and AI agents reading a task
-- know why it carries the label. Plain text, empty when not set. The existing
-- trg_changelog_labels_upd trigger covers the column.
ALTER TABLE labels ADD COLUMN description TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE labels DROP COLUMN description;

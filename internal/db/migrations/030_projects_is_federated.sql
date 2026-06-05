-- +goose Up
-- Per-project federation enable flag (Federation v1 F1.1). Mirrors
-- 012_projects_is_private: a single 0/1 column with a CHECK constraint and a
-- default of 0 so every existing project starts non-federated. Enabling only
-- flips this flag (and inserts the is_owner=1 self-row in federated_projects);
-- the project is not actually syncable until the Phase 3 sync core lands.
ALTER TABLE projects ADD COLUMN is_federated INTEGER NOT NULL DEFAULT 0 CHECK (is_federated IN (0, 1));

-- +goose Down
ALTER TABLE projects DROP COLUMN is_federated;

-- +goose Up
-- auto_sort_undecided_at marks an Inbox task the LLM processor looked at but
-- could not file: the model kept it, was not confident enough, or named a project
-- that does not exist. Such a task is not sent to the model again. Editing its
-- title or description, or moving it, clears the mark (repo.TaskRepo.Update /
-- Move), so a reworded or relocated task gets a fresh look.
-- The existing trg_changelog_tasks_upd trigger covers the column.
ALTER TABLE tasks ADD COLUMN auto_sort_undecided_at TEXT NULL;

-- +goose Down
ALTER TABLE tasks DROP COLUMN auto_sort_undecided_at;

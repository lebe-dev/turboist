-- +goose Up
-- The pinned caps used to be a server-wide config key (max-pinned); they are now
-- per-user preferences in users.settings. Seed every existing row with the
-- default of 10 for both kinds. Rows whose blob still lacks the keys are
-- normalized to the same default on read (repo.UserRepo.GetSettings).
UPDATE users
SET settings = json_set(
    CASE WHEN json_valid(settings) THEN settings ELSE '{}' END,
    '$.maxPinnedTasks', 10,
    '$.maxPinnedProjects', 10
);

-- +goose Down
UPDATE users
SET settings = json_remove(
    CASE WHEN json_valid(settings) THEN settings ELSE '{}' END,
    '$.maxPinnedTasks',
    '$.maxPinnedProjects'
);

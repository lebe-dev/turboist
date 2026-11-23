-- +goose Up
-- The installation settings payload now carries the read-only weeklyLimit and
-- backlogLimit (copied from the weekly/backlog configuration) so a native replica
-- can repeat the planning budgets on the device. A replica that already holds a
-- document and a cursor would only ever learn them from a later settings edit,
-- which may never happen — and until it does, the bug the fields exist to fix
-- (a park applied on the device, refused by the server and silently returned to
-- the day it came from) would keep happening on every installed device.
--
-- Writing the value the row already holds fires trg_changelog_app_settings_upd,
-- which hands every replica the new shape on its next pull. Only a database that
-- already has a change history is nudged: an empty log means no replica holds a
-- cursor, and a replica without one is seeded from a snapshot, which carries the
-- new shape anyway. That also keeps a fresh database's cursor at zero.
UPDATE app_settings SET data = data
 WHERE id = 1 AND EXISTS (SELECT 1 FROM change_log);

-- +goose Down
-- Nothing to undo: the row carries exactly the value it carried before.

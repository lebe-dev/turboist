-- +goose Up
-- Federation pruned-floor high-water mark (Federation v1 F3.3, US-3.7 AC4 review fix).
--
-- The stale-pull 410 gate (handlers/federation_events.go) must tell a long-quiet
-- peer to re-snapshot when its since_hlc predates events that have already aged out
-- of the outbox. Anchoring that decision to the PRESENCE of federation_outbox rows
-- is unsafe: outbox retention (default 30d, hard-capped 30d) is SHORTER than
-- tombstone retention (default 90d). Once the daily GC purges a quiet project's
-- outbox entirely, the "oldest retained HLC" is empty and the gate would fall
-- through to serve 200 + an empty event list — silently telling a stale peer it is
-- caught up and defeating US-3.7 AC4 ("re-snapshot rather than silently miss the
-- pruned changes").
--
-- This table is the DURABLE record of what was pruned: a per-project monotonic
-- pruned-floor HLC. The GC advances it to the MAX event HLC of the outbox rows it
-- purges; the pull handler answers 410 whenever since_hlc < this floor, regardless
-- of whether any outbox rows currently remain. local_project_id is the int64
-- projects.id (the federation entity-identity deviation, §3).
-- +goose StatementBegin
CREATE TABLE federation_pruned_floor (
    local_project_id INTEGER PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    floor_hlc        TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS federation_pruned_floor;

-- +goose Up
-- Federation outbox backpressure persistence (Federation v1 F4.4, US-4.4 /
-- US-8.3). The in-memory per-peer backoff gate the F3.2 publisher worker already
-- carries is lost on restart: a peer that returned a 4xx permanent reject, or
-- that is mid-way through an exponential 5xx backoff, forgets that on the next
-- start and re-POSTs immediately — re-hammering a down/rejecting peer (the
-- "persist retry-not-before across restart" risk in §7 F4.4). This migration
-- lands the two durable backing tables F4.4 needs:
--
--   1. federation_dead_letter — the parking lot for events whose delivery to a
--      specific peer failed PERMANENTLY (a 4xx ≠ 429: a revoked-peer 403, an
--      author/origin-mismatch 400, a signature-rejected 401, a stale-tombstone
--      410). The event is NOT retried automatically (US-4.4 AC3); it surfaces in
--      the owner's dead-letter diagnostics view (GET /api/v1/federation/dead-letter)
--      and is EXCLUDED from the per-peer pending-delivery count (§7 F4.4 risk:
--      "dead-letter excluded from pending count").
--
--   2. federation_peer_retry — the durable per-peer retry gate: the not-before
--      wall-clock the peer may next be re-POSTed, the consecutive-transient
--      attempt counter that drives the 1s..1h exponential backoff (US-4.4 AC2),
--      and a permanent flag so a peer that has been fully dead-lettered is not
--      re-probed on restart until an operator intervention re-enables it. Loaded
--      once on worker start so backoff survives a restart.
--
-- Name-collision note (R21): every federation table is federation_*-prefixed;
-- federation_dead_letter and federation_peer_retry are both free in the live
-- schema (001..034). All wire timestamps are TEXT ISO-8601 UTC (model.FormatUTC).
-- local_project_id is the int64 projects.id (the federation entity-identity
-- deviation, §3).

-- federation_dead_letter: one row per (peer, event) that hit a permanent
-- delivery failure. event_id is the cross-instance dedup key; payload is the
-- verbatim canonical signed bytes (so the owner can inspect what failed and a
-- future operator action could re-drive it). The (peer_instance_url, event_id)
-- pair is unique so a re-classification of the same event for the same peer is
-- idempotent (ON CONFLICT DO NOTHING).
-- +goose StatementBegin
CREATE TABLE federation_dead_letter (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id          TEXT NOT NULL,
    peer_instance_url TEXT NOT NULL,
    local_project_id  INTEGER REFERENCES projects(id) ON DELETE CASCADE,
    payload           TEXT NOT NULL,
    status_code       INTEGER NOT NULL DEFAULT 0,
    reason            TEXT NOT NULL DEFAULT '',
    failed_at         TEXT NOT NULL,
    UNIQUE (peer_instance_url, event_id)
);
-- +goose StatementEnd

CREATE INDEX idx_federation_dead_letter_peer ON federation_dead_letter(peer_instance_url);

-- federation_peer_retry: the durable per-peer retry gate, keyed by peer
-- instance_url. not_before is the earliest wall-clock the peer may be re-POSTed
-- (a transient backoff window); attempt is the consecutive-transient failure
-- count driving the exponential window; permanent=1 means the peer has been
-- fully dead-lettered and must not be re-probed until an operator re-enables it.
-- A successful delivery DELETEs the row (the gate clears).
-- +goose StatementBegin
CREATE TABLE federation_peer_retry (
    peer_instance_url TEXT PRIMARY KEY,
    not_before        TEXT NOT NULL DEFAULT '',
    attempt           INTEGER NOT NULL DEFAULT 0,
    permanent         INTEGER NOT NULL DEFAULT 0 CHECK (permanent IN (0, 1)),
    updated_at        TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
DROP INDEX IF EXISTS idx_federation_dead_letter_peer;
DROP TABLE IF EXISTS federation_dead_letter;
DROP TABLE IF EXISTS federation_peer_retry;

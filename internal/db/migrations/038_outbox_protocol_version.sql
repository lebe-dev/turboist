-- +goose Up
-- Federation v1 F6.1: protocol_version dual-write seam on federation_outbox.
--
-- Each outbound event is emitted at a negotiated protocol version (F0.4). v1 is
-- the only version, so the column is fixed at 1, but it lands NOW as the durable
-- seam a FUTURE dual-write needs: when this instance speaks v2 to one peer and v1
-- to another, the publisher records which wire shape a row was serialised at so
-- the seam (internal/federation/protocol.Encode) can down-convert per peer without
-- a schema change. It is purely additive (R20) and applies cleanly on top of the
-- full live schema, including any federation_outbox rows already present (they
-- backfill to 1, the column being NOT NULL).
--
-- ALTER only — no new table, so the GTD inbox/outbox collision (R21) does not
-- apply here.
ALTER TABLE federation_outbox ADD COLUMN protocol_version INTEGER NOT NULL DEFAULT 1;

-- Backfill pre-existing rows to the v1 protocol version (the DEFAULT covers new
-- rows; this is explicit for clarity and for an ALTER engine that does not apply
-- the default retroactively).
UPDATE federation_outbox SET protocol_version = 1 WHERE protocol_version IS NULL;

-- +goose Down
-- SQLite's ALTER TABLE ... DROP COLUMN (3.35+, available via modernc) removes the
-- column while preserving every existing federation_outbox row.
ALTER TABLE federation_outbox DROP COLUMN protocol_version;

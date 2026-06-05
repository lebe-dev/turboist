-- +goose Up
-- Federation v1 F2.3: Hybrid Logical Clock (HLC) sidecar tables.
--
-- HLC is a federation-only sidecar (§3): the existing wall-clock updated_at
-- semantics are untouched for the non-federated 99% of usage. For federated
-- entities only, these two tables drive per-field Last-Writer-Wins ordering.
--
-- physical_ms is sourced from the SAME time.Now() that writes updated_at, so the
-- two never drift (§3 DEVIATE row / R11). node_id is a stable generated install
-- UUID (federation_keys.node_id, R10) — never derived from BASE_URL host.
--
-- Both table names were audited free against the live schema (001..030); the
-- GTD inbox/outbox collision does not apply here.

-- entity_field_hlc records the HLC of the last write to EACH field of a
-- federated entity (§5.4 per-field LWW). entity_id is the entity's client_id
-- (the cross-instance ULID/UUIDv7 identity, §3) — NOT the local int64 PK, since
-- the int64 id is instance-local and not portable. The composite PK
-- (entity_type, entity_id, field_name) is the natural key; WITHOUT ROWID keeps
-- the table compact and the lookups index-only (the apply path is the hot path,
-- NFR-1.3). hlc is the zero-padded, lexically-comparable HLC string.
-- +goose StatementBegin
CREATE TABLE entity_field_hlc (
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    field_name  TEXT NOT NULL,
    hlc         TEXT NOT NULL,
    PRIMARY KEY (entity_type, entity_id, field_name)
) WITHOUT ROWID;
-- +goose StatementEnd

-- hlc_state is the single-row (id=1) clock state for THIS instance: the last
-- physical_ms it minted and the logical counter at that millisecond, plus a copy
-- of the install node_id used for the tie-break suffix. Now() advances this row
-- under SetMaxOpenConns(1) (the pool serialises writes, so the read-modify-write
-- is race-free without a second mutex). The row is lazily inserted by the HLC
-- store on first Now(); node_id is sourced from federation_keys at that time.
-- +goose StatementBegin
CREATE TABLE hlc_state (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    last_physical_ms INTEGER NOT NULL DEFAULT 0,
    last_logical     INTEGER NOT NULL DEFAULT 0,
    node_id          TEXT NOT NULL DEFAULT ''
);
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS hlc_state;
DROP TABLE IF EXISTS entity_field_hlc;

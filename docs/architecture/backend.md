# Backend Architecture

Go 1.26 service built on Fiber v3 with an embedded SQLite store (`modernc.org/sqlite`, no CGO) and goose-managed migrations. Raw SQL throughout — no ORM.

## CLI flags

| Flag | Default | Description |
|---|---|---|
| `-config <path>` | `config.yml` | Path to business config file |
| `-db <path>` | `turboist.db` | Path to SQLite database file |

The server runs migrations from `internal/db/migrations` on every start. The schema is created on first boot; the singleton `users` row and `inbox` row (id=2) are seeded by migration `002_users_sessions.sql`. Boot is idempotent — no separate migration command.

## Endpoints

| Route | Auth | Description |
|---|---|---|
| `GET /healthz` | none | Liveness probe |
| `GET /version` | none | Build version |
| `POST /auth/setup` | none | Create singleton user (first-run only); subsequent calls return `setup_already_done` |
| `POST /auth/login` | — | Issue access token + refresh token |
| `POST /auth/refresh` | — | Rotate refresh token |
| `POST /auth/logout` | — | Invalidate current session |
| `POST /auth/logout-all` | — | Invalidate all sessions |
| `GET /auth/me` | JWT | Current user info |
| `POST /auth/passkey/login/{begin,finish}` | none | Discoverable passkey login (WebAuthn) |
| `/api/v1/passkeys/*` | JWT | Register, list, rename and remove passkeys |
| `/api/v1/{contexts,labels,sections,projects,inbox,tasks,search,config}` | Bearer | Authenticated REST resources |
| `POST\|DELETE /api/v1/tasks/:id/relations[/:relationId]` | Bearer | Task relation graph (write-only — reads ride on `GET /api/v1/tasks/:id?relations=true`) |
| `GET /api/v1/sync/{changes,snapshot}` | Bearer | Delta feed and full seed for clients holding a local copy of the workspace |

All `/api/v1/*` endpoints require `Authorization: Bearer <token>`. The token can be a 15-minute JWT access token or a long-lived API token (generated in Settings → API). Web clients also receive a 30-day refresh token in an HttpOnly cookie scoped to `/auth/refresh`. API tokens are accepted on every `/api/v1/*` route except `/api/v1/api-tokens/*`, which requires a JWT session.

See [API.md](../../API.md) for the full reference.

## Authentication

Single-user app. First request must be `POST /auth/setup` with `{username, password, clientKind}`. Login issues a 15-minute access token and 30-day refresh token. Up to 5 concurrent sessions per client kind (`web|ios|cli`) — older sessions are pruned automatically.

Optional TOTP 2FA (RFC 6238) with single-use recovery codes. Requires `TOTP_SECRET_KEY` env var.

### Passkeys (WebAuthn)

A second, additional way in — the password stays as the recovery path. Built on
`github.com/go-webauthn/webauthn`; the service lives in `internal/service/passkey`,
the credentials in `webauthn_credentials` (migration `049`).

- **Discoverable credentials.** Registration demands a resident key, so login is
  usernameless: the authenticator returns the user handle (`webauthn_users.handle`,
  a random 32-byte value generated on first enrollment) and the server resolves
  the account from it.
- **One factor covers both.** A passkey assertion never leads to a TOTP step: the
  authenticator holds the key *and* verifies the user, so an extra code would add
  friction without adding a factor.
- **Ceremony state is server-side and single-use.** `begin` stores the challenge
  in an in-memory store keyed by an opaque `ceremonyId` (5-minute TTL) and
  `finish` consumes it — a replay gets `passkey_ceremony_invalid`. In-memory is
  deliberate: single process, worthless data after five minutes, and a restart
  mid-ceremony just means tapping the button again.
- **Counter write-back.** Every successful assertion rewrites the stored
  credential record (signature counter, backup-state flags), which is what makes
  clone detection work.
- **Relying Party from `BASE_URL`.** `WEBAUTHN_RP_ID` / `WEBAUTHN_ORIGINS` override
  it. Credentials are bound to the RP ID — changing it invalidates all of them. A
  Relying Party the library rejects disables the routes entirely (they 404)
  rather than taking the server down.
- **JWT only.** `/api/v1/passkeys/*` sits behind `RequireJWTAuth`: enrolling a
  passkey mints a password-equivalent credential, so an API token must not reach
  it. Up to 20 passkeys per account.
- **Native apps** run the ceremony through platform APIs (`@capgo/capacitor-passkey`),
  since the WebView's origin is not the server's; that path needs the association
  files served under `/.well-known/` (`WELL_KNOWN_PATH`) and the Android app's
  `android:apk-key-hash:` origin in `WEBAUTHN_ORIGINS`. See `docs/mobile.md`.

## Real-time invalidation (SSE)

Clients keep their views fresh over a Server-Sent Events stream. EventSource cannot send an `Authorization` header, so a client first `POST`s `/api/v1/events/ticket` (JWT only) to mint a short-lived single-use ticket, then opens `GET /api/v1/events?ticket=...`. Events carry only a coarse `scope` (`tasks`, `projects`, `plan`, `inbox`, …); the client refetches the affected views via the regular REST endpoints. There is no persistence or replay — after a reconnect the client does a one-shot catch-up refetch.

Mutations publish their scopes through `PublishMiddleware` (after a successful `2xx` on a mutating method), fanning out to every active subscriber of that user via the in-memory `events.Hub`.

### Heartbeat and reconnect

An idle stream emits `event: ping` every 25 s (`sseHeartbeatInterval`). It keeps the connection alive behind nginx (default `proxy_read_timeout` is 60 s) *and* doubles as the client's liveness signal — hence a named event rather than a `:` comment, which EventSource never surfaces to JavaScript. A phone that suspends its radio, or a proxy that drops the connection, leaves `EventSource` in `OPEN` with no `error` event ever firing, so the client's watchdog re-handshakes once the heartbeat stops arriving (`LIVENESS_TIMEOUT_MS` in `lib/realtime/events.svelte.ts`). Raising the server interval means raising that timeout with it.

The ticket lives in the stream URL and is single-use, so the browser's own EventSource retry — which reuses that exact URL — can only ever get a `401`. The client therefore never relies on it: it tears the stream down on any error and re-handshakes for a fresh ticket on its own backoff.

### SSE echo suppression

Each browser tab generates a per-tab origin id and:

- sends it on every mutating request via the `X-Client-Origin` header, and
- passes the same origin in the body of `POST /api/v1/events/ticket`, binding it to its stream.

`Hub.Publish` skips any subscriber whose origin matches the mutation's `X-Client-Origin`. The tab that made a change therefore never receives the echo of its own mutation — it already applied the change from the mutation's own response, so re-fetching would only cost a round-trip and cause a visible re-render. Other tabs and devices still receive the event. An empty origin (older clients, server-side) disables suppression. To refresh data it can no longer derive from the echo, the originating client refetches the [`GET /api/v1/stats/sidebar`](../../API.md) bundle once after its own mutation.

Inbound events are coalesced client-side over a 200 ms window before anything is refetched (`lib/realtime/scopeCoalescer.ts`). One remote change usually fans out to several scopes — a bulk move emits `tasks` + `plan` + `inbox` + `projects` — and the shell used to issue one GET per scope handler. The coalesced burst now resolves to a single aggregate request: [`GET /api/v1/config`](../../API.md#get-apiv1config) when entity lists may have moved, or the smaller `/stats/sidebar` bundle when only counters did. The same `/config` refetch backs the catch-up after an SSE reconnect and after the offline outbox drains, replacing six per-store GETs. It is a conditional request, so an unchanged workspace costs a `304` with no body — which matters on mobile, where every unlock reconnects the stream.

## Idempotency

`IdempotencyMiddleware` (`internal/httpapi/idempotency_middleware.go`) makes mutating `/api/v1/*` requests safe to retry. When a request carries an `Idempotency-Key` header, the middleware reserves the key, runs the handler once, and stores the `2xx` response; a later request with the same key replays the stored response (`X-Idempotent-Replay: true`) without re-running the handler. A concurrent duplicate (the first request still in flight) is rejected with `409 idempotency_in_flight`; non-2xx responses are released so a corrected retry re-runs the handler.

The middleware sits **after** `APIAuthMiddleware` (it needs the resolved user id) and **before** `PublishMiddleware` in the `/api/v1` group, so a replay short-circuits before Publish and never re-emits an SSE invalidation. A nil `IdempotencyRepo` disables it (used in tests). Client-facing behaviour is documented in [API.md → Idempotency](../../API.md#idempotency).

Keys are persisted in the `idempotency_keys` table (migration `044_idempotency_keys.sql`: `key` PK, `user_id` FK, `method`, `path`, `status`, `response`, `created_at`; `status = 0` marks a reservation still in flight). A background prune runs in `cmd/turboist` — once at startup and then every 12 hours on the shared cleanup context — deleting rows older than 48 hours via `IdempotencyRepo.DeleteOlderThan`.

## Task relations

`task_relations` (migration `046_task_relations.sql`) stores directed edges between two tasks: `source_task_id`, `target_task_id`, `type` (`related` | `blocks`), plus a surrogate `id` so the API can address one edge (`DELETE /api/v1/tasks/:id/relations/:relationId`). Both FKs cascade — tasks are hard-deleted, so a deleted blocker takes its edges with it.

`related` is symmetric and normalised on write (lower id first), which lets the `UNIQUE (source, target, type)` constraint dedupe A↔B added from either side. `blocks` is directed and enforced: `CompleteService.completeAt` refuses a task with any `open` blocker, returning `service.TaskBlockedError` → `409 task_blocked` with the blocker ids in `details`. The guard sits at that one choke point deliberately — single complete, `bulk/complete` and the Troiki board all funnel through it. Blockers are inherited down the subtask tree: `TaskRelationsRepo.OpenBlockerIDs` walks the task's ancestor chain, so a subtask of a blocked parent is refused too (an inherited blocker sitting inside the task's own subtree is dropped — "child blocks parent" must not leave the child waiting for itself). Completed *and cancelled* blockers stop blocking, otherwise cancelling a task would deadlock its dependents forever. A cycle check (recursive CTE over `blocks` edges) rejects a pair that would leave both tasks permanently uncompletable.

Reads are shaped by the "one aggregate" rule: there is no relations endpoint. `TaskDTO` always carries `blockedByCount` / `relationCount`, hydrated by one batch query (`TaskRelationsRepo.SummaryByTaskIDs`) in `TaskRepo.Get` and in the single list funnel `listWithBaseArgsOrdered` — `blockedByCount` counts inherited blockers too, so the padlock in the lists agrees with the guard, while `relationCount` stays the task's own relations because those are what the detail page lists — so every list view, `GET /api/v1/config`'s `pinnedTasks` and `/stats/sidebar`'s `pinned` get them without a per-task query. The full relation list is opt-in via `GET /api/v1/tasks/:id?relations=true`, and both mutations answer with the updated task so no follow-up read is needed.

## Label usage stats

`GET /api/v1/labels/stats` powers the Labels page. One `GROUP BY` over
`labels LEFT JOIN task_labels LEFT JOIN tasks` (`LabelRepo.UsageStats`) returns
every label with counters for all three rolling windows — last 7 / 30 / 90 days,
each ending at the end of today in the server timezone — plus the equally long
preceding window for the trend, and the period-independent totals (open,
overdue, all-time tasks, distinct projects, last used).

Deliberately not folded into `GET /api/v1/config`: that aggregate is refetched on
every SSE burst on every device, and this is a heavier scan needed by exactly one
screen. It stays offline-capable regardless — the frontend's read-through cache
write-throughs every `/api/v1/*` GET and serves the stale copy on a network
error, so the page opens offline once it has been visited online.

The tagging timestamp it buckets by is `task_labels.created_at`
(migration `047_task_labels_created_at.sql`, nullable + backfilled from the
task's creation time). `TaskLabelsRepo.SetForTask` is therefore a diff, not a
delete-and-reinsert: surviving rows keep their timestamp, so editing an unrelated
field on a task does not move all of its tagging events into the current week.

## Change log

`change_log` (migration `050_change_log.sql`) is an append-only record of every
committed mutation of a replicable entity: `seq` (an `AUTOINCREMENT` cursor),
`entity`, `entity_id`, `op` (`upsert` | `delete`) and `changed_at`. A client
holding a full local copy of the data reads it forward from the last `seq` it
saw and learns what changed — deletions included, since the entity tables keep
their hard deletes and a `delete` row is the only tombstone there is.

The rows are written by SQLite `AFTER INSERT/UPDATE/DELETE` triggers, not by Go
code. Rows change through paths no single caller names: service cascades
(`CascadeBacklogToDescendants` rewrites a whole subtask subtree in one bulk
statement), group and bulk endpoints, and foreign-key cascades that fire with no
Go code involved at all. As triggers, logging is a property of the schema — a row
cannot change without being logged, today or through a write path added later.
**Adding a table therefore means deciding whether it is replicable, and if so
creating its three triggers in the same migration.**

The log stores pointers, never payloads: readers join back to the live tables for
the current state. That makes a redundant row (an `updated_at` bump that changed
nothing user-visible) cost one refetch and nothing else, and multiple rows per
entity are expected — readers dedupe by `(entity, entity_id)` keeping the highest
`seq`. `AUTOINCREMENT` rather than a bare rowid primary key so that pruning old
rows can never hand a stale cursor a `seq` pointing at a newer change.

Entities are the resources the API already serves, not the tables: `task`,
`project`, `section`, `context`, `label`, `task_relation`, `task_template`,
`user_settings`, `user_state`, `app_settings`. Join and child tables whose
contents the API serves inside their owner's payload have no entity of their own
— `task_labels` and `project_labels` log an upsert of the task or project, the
template subtask and label tables log an upsert of the template. Their delete
triggers are guarded by the owner still existing, so a child removed by a cascade
never logs an upsert of the row that is itself on its way out. `users` backs two
resources: any write logs a `user_settings` upsert, and a write touching the
`state` column logs `user_state` as well. Sessions, API tokens, TOTP recovery
codes, the WebAuthn tables, the idempotency replay cache and the calendar tables
are deliberately not logged — none of it belongs on a client replica.

`app_settings.sync_epoch` (default `1`, read and advanced through
`AppSettingsRepo.SyncEpoch` / `BumpSyncEpoch`) invalidates every cursor at once.
It is bumped whenever the history stops describing the data — a restore replaces
the rows wholesale — so a client presenting a cursor stamped with an older epoch
is told to start over instead of silently diverging. The bump touches only its
own column, never the settings blob, and is not itself logged as a settings
change.

### Reading it: the delta feed and the snapshot

Two endpoints serve the log to clients that keep a full local copy of the
workspace (`internal/httpapi/handlers/sync.go`, `sync_snapshot.go` over
`internal/repo/changelog.go`, `sync_snapshot.go`). The wire contract — parameters,
payloads, error bodies, the loop a client runs — is in
[API.md](../../API.md#sync); what follows is why the server side looks the way it
does.

`GET /api/v1/sync/changes` reads one page inside a single read transaction: the
epoch, the log window and every hydrated row come from one snapshot of the
database, so a page can never claim a cursor newer than the rows it carries. The
window is collapsed to one row per `(entity, entity_id)` — the highest `seq`,
the only one whose `op` still describes reality — and the surviving rows are then
hydrated from the live tables through the very loaders the REST endpoints use, so
a task served in a delta is indistinguishable from the same task served by
`GET /api/v1/tasks/:id`. An upsert whose row has since disappeared is rewritten
into a delete rather than dropped: the page must never tell a client to keep a row
the server no longer has. Paging asks for one row more than the limit to decide
`hasMore` without a second count.

`GET /api/v1/sync/snapshot` is the bootstrap the delta loop starts from and the
recovery a client falls back to when the feed refuses to resume. It is
deliberately unpaged: the dataset belongs to one person, and the completed-task
window keeps the only unbounded collection in check. Its cursor is read in the
same transaction as the rows, so a write that lands mid-read either is already
reflected or arrives with the next delta page — the worst case is one row applied
twice, which is free.

The completed-task window is `repo.SyncHistoryWindow` measured back from the
request. Two closures keep the seed referentially whole: the recursive task query
pulls in the ancestors of every kept task however old they are, so an open subtask
never arrives pointing at a parent the client has never seen, and the relation
query serves only edges whose both endpoints are inside the window.

Both endpoints require every read scope (`tasks`, `projects`, `sections`,
`contexts`, `labels`, `templates`, `settings`) rather than one of them: a single
response can carry the whole workspace, and reading it here must not be cheaper
than reading it endpoint by endpoint. JWT sessions bypass scope checks as usual,
so the apps are unaffected.

Nothing is pushed through these endpoints. Client writes go through the existing
mutation endpoints with an `Idempotency-Key`, which keeps every invariant enforced
exactly once, in `internal/service`.

### Retention

The log is trimmed to `repo.SyncHistoryWindow` (90 days) — the same window the
snapshot seeds completed tasks over, so a client is handed exactly the stretch of
history it is allowed to catch up across. `ChangeLogRepo.Prune` runs once at
startup and then daily on the shared cleanup context, alongside the session and
idempotency-key jobs (`cmd/turboist/cleanup.go`).

It deletes a contiguous prefix — every row up to the highest `seq` older than the
cutoff — rather than every row whose `changed_at` is old. The two are the same
log until a clock steps backwards, and only the prefix form keeps the expiry
answer honest: a resuming client is refused purely on `MIN(seq)`, so a hole
punched in the middle of the log would let a cursor below it resume and never
learn about the rows that were removed above. Nothing resets `sqlite_sequence`,
so a pruned-away `seq` is never handed out again, and a cursor sitting on the
last surviving row keeps resuming — routine trimming costs an up-to-date client
nothing.

### Restore

A restore replaces the dataset wholesale, so `repo.ResetSyncHistory` runs inside
the restore's own transaction (`BackupService.Restore`): it bumps the epoch and
then empties the log, in that order, so the rows the restore itself logged — the
wipe and every re-insert — go with the history they belong to instead of reaching
clients as a change-by-change replay. A restore that rolls back leaves both the
data and the log exactly as they were.

The two refusals a client meets are the same two states from the other side: a
cursor carrying a pre-restore epoch, and a cursor older than everything the log
still holds. Either way the remedy is a fresh snapshot.

## List view contract

The list views in `internal/repo/views.go` are re-implemented as local queries by
the native mobile client, which renders its lists offline instead of asking the
server. Two implementations of the same predicates drift silently, so both sides
are pinned to one shared dataset:

- `testdata/sync-contract/fixture.json` — contexts, projects, sections, labels
  and tasks covering every plan state, due/deadline shape, priority, pin, status,
  troiki category and subtask depth, plus the frozen clock every dated view takes
  its boundaries from. Entities are addressed by stable string keys, never by
  database ids, because each implementation assigns its own. Each task carries a
  `notes` field naming the invariant it exercises, and a distinct `createdAt`
  (completed tasks a distinct `completedAt`) so the shared sort is a total order.
- `testdata/sync-contract/golden/*.json` — the expected ordered task keys and
  count per view.
- `internal/repo/view_contract_test.go` — replays the fixture through the
  repositories, renders every view and diffs against the goldens. The mobile
  client renders the same fixture through its own queries and diffs the same
  files.

Changing a view predicate or the shared task sort turns this test red on purpose.
Rewrite the goldens with `go test ./internal/repo -run TestViewContract -update`
and read the diff: every changed line is a behaviour change the mobile client has
to follow. Never edit the fixture just to make a golden agree with new code.

## Storage

All data lives in the SQLite file pointed to by `-db`. WAL mode is enabled — back up `*.db`, `*.db-wal`, and `*.db-shm` together, or use `VACUUM INTO` for a single-file snapshot.

## Development

### Requirements

- Go 1.26+
- `golangci-lint` (for `just lint-backend`)
- `just` task runner

### Commands

```sh
just test           # go test ./...
just lint-backend   # golangci-lint run ./...
just coverage       # writes coverage.out and coverage.html
just build          # builds ./turboist
```

Repository tests run against an in-memory SQLite database with migrations applied; HTTP handlers are exercised via Fiber's `app.Test`.

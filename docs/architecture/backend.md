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
| `/api/v1/{contexts,labels,sections,projects,inbox,tasks,search,config}` | Bearer | Authenticated REST resources |
| `POST\|DELETE /api/v1/tasks/:id/relations[/:relationId]` | Bearer | Task relation graph (write-only — reads ride on `GET /api/v1/tasks/:id?relations=true`) |

All `/api/v1/*` endpoints require `Authorization: Bearer <token>`. The token can be a 15-minute JWT access token or a long-lived API token (generated in Settings → API). Web clients also receive a 30-day refresh token in an HttpOnly cookie scoped to `/auth/refresh`. API tokens are accepted on every `/api/v1/*` route except `/api/v1/api-tokens/*`, which requires a JWT session.

See [API.md](../../API.md) for the full reference.

## Authentication

Single-user app. First request must be `POST /auth/setup` with `{username, password, clientKind}`. Login issues a 15-minute access token and 30-day refresh token. Up to 5 concurrent sessions per client kind (`web|ios|cli`) — older sessions are pruned automatically.

Optional TOTP 2FA (RFC 6238) with single-use recovery codes. Requires `TOTP_SECRET_KEY` env var.

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

`related` is symmetric and normalised on write (lower id first), which lets the `UNIQUE (source, target, type)` constraint dedupe A↔B added from either side. `blocks` is directed and enforced: `CompleteService.completeAt` refuses a task with any `open` blocker, returning `service.TaskBlockedError` → `409 task_blocked` with the blocker ids in `details`. The guard sits at that one choke point deliberately — single complete, `bulk/complete` and the Troiki board all funnel through it. Completed *and cancelled* blockers stop blocking, otherwise cancelling a task would deadlock its dependents forever. A cycle check (recursive CTE over `blocks` edges) rejects a pair that would leave both tasks permanently uncompletable.

Reads are shaped by the "one aggregate" rule: there is no relations endpoint. `TaskDTO` always carries `blockedByCount` / `relationCount`, hydrated by one batch query (`TaskRelationsRepo.SummaryByTaskIDs`) in `TaskRepo.Get` and in the single list funnel `listWithBaseArgsOrdered` — so every list view, `GET /api/v1/config`'s `pinnedTasks` and `/stats/sidebar`'s `pinned` get them without a per-task query. The full relation list is opt-in via `GET /api/v1/tasks/:id?relations=true`, and both mutations answer with the updated task so no follow-up read is needed.

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

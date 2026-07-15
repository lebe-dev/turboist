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

All `/api/v1/*` endpoints require `Authorization: Bearer <token>`. The token can be a 15-minute JWT access token or a long-lived API token (generated in Settings → API). Web clients also receive a 30-day refresh token in an HttpOnly cookie scoped to `/auth/refresh`. API tokens are accepted on every `/api/v1/*` route except `/api/v1/api-tokens/*`, which requires a JWT session.

See [API.md](../../API.md) for the full reference.

## Authentication

Single-user app. First request must be `POST /auth/setup` with `{username, password, clientKind}`. Login issues a 15-minute access token and 30-day refresh token. Up to 5 concurrent sessions per client kind (`web|ios|cli`) — older sessions are pruned automatically.

Optional TOTP 2FA (RFC 6238) with single-use recovery codes. Requires `TOTP_SECRET_KEY` env var.

## Real-time invalidation (SSE)

Clients keep their views fresh over a Server-Sent Events stream. EventSource cannot send an `Authorization` header, so a client first `POST`s `/api/v1/events/ticket` (JWT only) to mint a short-lived single-use ticket, then opens `GET /api/v1/events?ticket=...`. Events carry only a coarse `scope` (`tasks`, `projects`, `plan`, `inbox`, …); the client refetches the affected views via the regular REST endpoints. There is no persistence or replay — after a reconnect the client does a one-shot catch-up refetch.

Mutations publish their scopes through `PublishMiddleware` (after a successful `2xx` on a mutating method), fanning out to every active subscriber of that user via the in-memory `events.Hub`.

### SSE echo suppression

Each browser tab generates a per-tab origin id and:

- sends it on every mutating request via the `X-Client-Origin` header, and
- passes the same origin in the body of `POST /api/v1/events/ticket`, binding it to its stream.

`Hub.Publish` skips any subscriber whose origin matches the mutation's `X-Client-Origin`. The tab that made a change therefore never receives the echo of its own mutation — it already applied the change from the mutation's own response, so re-fetching would only cost a round-trip and cause a visible re-render. Other tabs and devices still receive the event. An empty origin (older clients, server-side) disables suppression. To refresh data it can no longer derive from the echo, the originating client refetches the [`GET /api/v1/stats/sidebar`](../../API.md) bundle once after its own mutation.

## Idempotency

`IdempotencyMiddleware` (`internal/httpapi/idempotency_middleware.go`) makes mutating `/api/v1/*` requests safe to retry. When a request carries an `Idempotency-Key` header, the middleware reserves the key, runs the handler once, and stores the `2xx` response; a later request with the same key replays the stored response (`X-Idempotent-Replay: true`) without re-running the handler. A concurrent duplicate (the first request still in flight) is rejected with `409 idempotency_in_flight`; non-2xx responses are released so a corrected retry re-runs the handler.

The middleware sits **after** `APIAuthMiddleware` (it needs the resolved user id) and **before** `PublishMiddleware` in the `/api/v1` group, so a replay short-circuits before Publish and never re-emits an SSE invalidation. A nil `IdempotencyRepo` disables it (used in tests). Client-facing behaviour is documented in [API.md → Idempotency](../../API.md#idempotency).

Keys are persisted in the `idempotency_keys` table (migration `044_idempotency_keys.sql`: `key` PK, `user_id` FK, `method`, `path`, `status`, `response`, `created_at`; `status = 0` marks a reservation still in flight). A background prune runs in `cmd/turboist` — once at startup and then every 12 hours on the shared cleanup context — deleting rows older than 48 hours via `IdempotencyRepo.DeleteOlderThan`.

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

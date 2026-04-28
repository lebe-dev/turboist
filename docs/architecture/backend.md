# Backend architecture

Turboist's backend is a single Go binary (`./cmd/turboist`) that serves both
the embedded frontend assets and the JSON API documented in
`files/files/API.md`. The codebase is layered: HTTP at the edge, services for
cross-entity rules, repositories for raw-SQL persistence, and a domain model
shared by all of them.

## Layout

```
cmd/turboist/                 entrypoint: env → config → db → deps → Fiber
internal/
  config/                     yaml + env loader, validation
  logging/                    slog setup keyed off LOG_LEVEL
  db/
    db.go                     Open(), WithTx() helper
    migrations.go             goose runner over embed.FS
    migrations/               001_schema.sql, 002_users_sessions.sql
  model/                      enums, entities, time helpers (UTC `.000Z`)
  repo/                       raw-SQL repositories + shared test harness
    contexts.go labels.go projects.go sections.go project_labels.go
    tasks.go task_labels.go views.go search.go
    users.go sessions.go
    errors.go sqlerr.go util.go testdb_test.go
  auth/                       argon2id, JWT, refresh rotation, rate-limit
    password.go jwt.go refresh.go ratelimit.go cleanup.go
  service/                    business rules that span repos
    tasks_create.go auto_labels.go
    complete.go move.go plan.go pin.go
  httpapi/
    server.go errors.go middleware.go config.go
    dto/                      request/response shapes
    handlers/                 route handlers grouped by resource
```

## Layer responsibilities

- `model` is pure data and enum validation. No I/O. Times round-trip through
  `FormatUTC` / `ParseUTC` so the wire format stays `2006-01-02T15:04:05.000Z`.
- `repo` owns SQL. Each file is a single aggregate; nullable columns map to
  `sql.Null*` and are converted to pointers at the boundary. Listing helpers
  return `(items, total)` via two queries — no `GROUP_CONCAT`. `task_labels`
  and `project_labels` are hydrated in code.
- `auth` is independent of `repo` for its primitives (hash, JWT, rate-limit)
  and depends on `repo/users` + `repo/sessions` only for persistence. Refresh
  tokens are stored as sha256 hashes; rotation is performed in a single tx.
- `service` enforces invariants that cross repositories: placement rules,
  cycle detection on `move`, weekly/backlog/pin caps, auto-label resolution,
  RRULE advancement on recurring `complete`.
- `httpapi` translates HTTP ↔ services. The custom `ErrorHandler` maps the
  typed errors from `repo`/`service` to the `{error: {code, message, details}}`
  envelope and the status codes listed in `API.md`. Pagination, request-id,
  access logging, and bearer auth are middleware.

## Data flow

A typical write request — `POST /api/v1/projects/:id/tasks`:

1. Fiber routes the request through recover → request-id → access-log →
   `AuthMiddleware` (Bearer → JWT claims in context).
2. The handler decodes the DTO, resolves auto-labels (`service/auto_labels`),
   and calls `service/tasks_create`, which opens a transaction via
   `db.WithTx`.
3. Inside the tx the service calls into `repo/tasks` and `repo/task_labels`,
   then commits.
4. The handler hydrates labels for the response and renders JSON with UTC
   timestamps in the canonical format.

Reads follow the same shape minus the transaction; pagination is applied by
the repo layer and wrapped in the standard `{items, total, limit, offset}`
envelope.

## Persistence

- `modernc.org/sqlite` (cgo-free). DSN sets `foreign_keys=1`,
  `journal_mode=WAL`, `synchronous=NORMAL`.
- Migrations are embedded at build time and run by goose on every boot.
- The schema enforces the singleton `users` (id=1) and `inbox` (id=2) rows
  via `CHECK` constraints — attempts to insert a second row fail.
- All times in SQLite are stored as ISO-8601 UTC strings with millisecond
  precision so they round-trip with `model/time.go` unchanged.

## Auth

- Passwords: argon2id, PHC-encoded (`time=3, mem=64MB, threads=4, salt=16,
  key=32`).
- Access tokens: JWT HS256, 15-minute expiry, claims `sub`, `sid`, `iat`,
  `exp`.
- Refresh tokens: 32 random bytes, base64url; only the sha256 hash is stored.
  Rotation is single-use and detects reuse by comparing against the
  short-lived in-memory revocation map.
- Sessions are scoped per `client_kind` with a hard cap of 5; the oldest is
  evicted on the 6th login. Web clients receive the refresh token as an
  `HttpOnly; Secure; SameSite=Lax` cookie scoped to `/auth/refresh`; other
  clients receive it in the response body.
- A 24h ticker (`StartSessionCleanup`) deletes expired sessions and revoked
  rows older than 7 days.
- `IPLimiter` is a token-bucket per remote IP (rate `1/6s`, burst 10) with a
  10-minute TTL and a GC goroutine; it gates `setup` and `login`.

## Configuration

`internal/config` validates `config.yml` on load: timezone resolves through
`time.LoadLocation`; day-parts must cover 24h without overlap; limits must be
positive; the inbox overflow target must be a valid priority; auto-label
entries must have a non-empty `label`. `GET /api/v1/config` exposes only the
public subset listed in `API.md`.

## Testing

- Repos: real SQLite (in-memory by default) with migrations applied via the
  shared `setupTestDB(t)` helper in `repo/testdb_test.go`. No DB mocks.
- Services: same harness; assert on observable state and returned errors.
- Auth: round-trip hash/verify, JWT issue/verify, refresh rotation, session
  eviction, rate-limit allow/block.
- HTTP: Fiber `app.Test(req)`; covers the error envelope, auth middleware,
  pagination clamps, and a smoke test that boots the full app and walks
  `setup → login → context → project → task → complete → views`.

Coverage targets: `repo`, `service`, and `auth` ≥ 80%. The Justfile recipes
`just test`, `just lint-backend`, and `just coverage` are the gates used in
CI and locally.

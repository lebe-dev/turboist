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

# Turboist

Turboist is a task management app for the rest of us.

## Features

- Contexts, projects, sections, labels (with auto-label rules)
- Inbox with overflow handling
- Day phases (morning/day/evening/anytime)
- Weekly / backlog planning with per-bucket caps
- Pinned tasks and pinned projects (separate caps)
- Recurring tasks (RRULE, advanced on completion)
- Single-user JWT auth with refresh-token rotation
- [Troiki System support](docs/troiki-system.md)
- Localized UI (English / Russian) — see [docs/locales.md](docs/locales.md)
- Public View — hide private projects, tasks, and labels for screenshot-friendly sharing — see [docs/public-mode.md](docs/public-mode.md)
- [Public API](API.md)

## Nginx Configuration

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

## Backend

The backend is a Go 1.26 service built on Fiber v3 with an embedded SQLite store
(`modernc.org/sqlite`, no CGO) and goose-managed migrations. Raw SQL is used
throughout — there is no ORM. All public HTTP routes are documented in
`files/files/API.md`; layout details live in `docs/architecture/backend.md`.

### Requirements

- Go 1.26+
- `golangci-lint` (for `just lint-backend`)
- `just` task runner

### Configuration

Two configuration sources are merged at start-up:

- Environment (loaded from `.env` if present; see `.env.example`):
  - `BIND` — listen address, e.g. `0.0.0.0:8080` (required)
  - `BASE_URL` — public base URL used when building `Task.URL` (required)
  - `JWT_SECRET` — base64-encoded secret, ≥ 32 bytes (required)
  - `API_TOKEN_SALT` — HMAC salt for API tokens, ≥ 32 bytes (required); rotating it invalidates all existing tokens
  - `LOG_LEVEL` — `debug|info|warn|error`, default `info`
  - `GOOGLE_CALENDAR_CLIENT_ID` / `GOOGLE_CALENDAR_CLIENT_SECRET` — optional
    Google OAuth credentials for read-only calendar events. Configure the
    OAuth redirect URI as `<BASE_URL>/api/v1/calendars/google/callback`.
- `config.yml` — business config (timezone, day-parts, limits, auto-labels,
  inbox overflow, pin caps). See `config.example.yml` for the full schema.

### Run

```sh
cp .env.example .env       # fill JWT_SECRET, API_TOKEN_SALT, BASE_URL, BIND
cp config.example.yml config.yml
just run-backend           # go run ./cmd/turboist
```

CLI flags:

- `-config <path>` — path to `config.yml` (default `config.yml`)
- `-db <path>` — path to the SQLite database file (default `turboist.db`)

The server runs migrations from `internal/db/migrations` on every start. The
schema is created on first boot; the singleton `users` row and the singleton
`inbox` row (id=2) are seeded by migration `002_users_sessions.sql`. There is
no separate migration command — boot is idempotent.

### Endpoints

- `GET  /healthz` — liveness probe (no auth)
- `GET  /version` — build version (no auth)
- `POST /auth/setup` — create the singleton user (first-run only); subsequent
  calls return `setup_already_done`
- `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`,
  `POST /auth/logout-all`, `GET /auth/me`
- `/api/v1/{contexts,labels,sections,projects,inbox,tasks,search,config}` —
  authenticated REST resources

All `/api/v1/*` endpoints require an `Authorization: Bearer <token>` header.
The token can be either a 15-minute JWT access token or a long-lived API token
generated in Settings → API. Web clients additionally receive a 30-day refresh
token in an HttpOnly cookie scoped to `/auth/refresh`. API tokens are accepted
on every `/api/v1/*` route except `/api/v1/api-tokens/*`, which requires a JWT
session. See `files/files/API.md` for the full reference.

### Authentication

This is a single-user app. The first request must be `POST /auth/setup` with
`{username, password, clientKind}` to create the user. Login issues a 15-minute
access token and a 30-day refresh token; up to 5 concurrent sessions per client
kind (`web|ios|cli`) are kept — older sessions are pruned automatically.

### Storage

All data lives in the SQLite file pointed to by `-db` (default `turboist.db`,
created next to the binary). WAL mode is enabled, so back up `*.db`,
`*.db-wal`, and `*.db-shm` together — or use `VACUUM INTO` for a single-file
snapshot.

### Tests, lint, build

```sh
just test                  # go test ./...
just lint-backend          # golangci-lint run ./...
just coverage              # writes coverage.out and coverage.html
just build                 # builds ./turboist
```

Repository tests run against an in-memory SQLite database with migrations
applied; HTTP handlers are exercised via Fiber's `app.Test`.

## RoadMap

- Offline-first
- iOS Native App
- Feature: Constraints

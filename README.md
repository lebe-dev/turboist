# Turboist

Turboist is a task management app for the rest of us.

## Features

- Contexts, projects, sections, labels (with auto-label rules)
- Inbox with overflow handling
- Day phases (morning / day / evening / anytime)
- Weekly / backlog planning with per-bucket caps
- Pinned tasks and pinned projects (separate caps)
- Recurring tasks (RRULE, advanced on completion)
- Single-user JWT auth with refresh-token rotation
- Optional TOTP 2FA (RFC 6238) with single-use recovery codes
- [Troiki System](docs/troiki-system.md)
- Localized UI (English / Russian) — [docs/locales.md](docs/locales.md)
- Public View — [docs/public-mode.md](docs/public-mode.md)
- Google Calendar integration (read-only) — [docs/google-calendar.md](docs/google-calendar.md)
- [Public API](API.md)

<<<<<<< HEAD
## Quick start
=======
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
  - `LOG_LEVEL` — `debug|info|warn|error`, default `info`. Logs are emitted as
    JSON via `log/slog` with request-scoped fields (`request_id`, `user_id`,
    `auth_method`) attached by middleware. What each level emits:
    - `debug` — handler entry/exit with key params, service inputs and
      decision branches, repo query bind values and `sql.ErrNoRows` lookups,
      JWT issue/verify lifecycle, allowed rate-limit requests, access log for
      successful (<400) responses
    - `info` — significant state changes: successful auth events
      (`/auth/setup`, `/auth/login`, `/auth/refresh`, `/auth/logout`), session
      creation/rotation, successful mutations (create/update/delete/complete/
      move/restore) with resulting id, backup export/restore start/finish with
      row counts, access log for 4xx responses
    - `warn` — recoverable/expected failures: validation errors, wrong
      password, unknown user, expired/used/invalid refresh tokens, rate-limit
      hits (with client IP), business-rule rejections (plan cap exceeded,
      troiki slot full, forbidden placement, invalid RRULE), deferred
      `io.Closer` errors, access log for 5xx responses
    - `error` — unexpected failures: SQL errors that are not `sql.ErrNoRows`,
      transaction aborts, panics
  - `GOOGLE_CALENDAR_CLIENT_ID` / `GOOGLE_CALENDAR_CLIENT_SECRET` — optional
    Google OAuth credentials for read-only calendar events. Configure the
    OAuth redirect URI as `<BASE_URL>/api/v1/calendars/google/callback`.
  - `CALENDAR_TOKEN_KEY` — optional encryption key for stored calendar OAuth
    tokens. Defaults to `JWT_SECRET`; keep the chosen value stable.
- `config.yml` — business config (timezone, day-parts, limits, auto-labels,
  inbox overflow, pin caps). See `config.example.yml` for the full schema.

### Run
>>>>>>> 049dc85 (v1.6.0 (#32))

```sh
cp .env.example .env           # fill JWT_SECRET, API_TOKEN_SALT, BASE_URL
cp config.example.yml config.yml
docker compose up -d
```

See [docs/configuration.md](docs/configuration.md) for all environment variables and config options.

## Docs

- [Installation](docs/install.md) — nginx config
- [Configuration](docs/configuration.md) — env vars, log levels, config.yml
- [Backend architecture](docs/architecture/backend.md) — endpoints, auth, storage, dev commands
- [API reference](API.md)
- [Troiki System](docs/troiki-system.md)
- [Localization](docs/locales.md)
- [Public mode](docs/public-mode.md)
- [Google Calendar](docs/google-calendar.md)

## RoadMap

- Feature: Task templates
- Feature: Federated Project Synchronization (Bridge Protocol) for Multi-Instance Collaboration
- Offline-first
- iOS Native App
- Feature: Constraints

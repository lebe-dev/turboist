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

## Quick start

```sh
cp .env.example .env           # fill JWT_SECRET, API_TOKEN_SALT, BASE_URL
cp config.example.yml config.yml
docker compose up -d
```

See [docs/configuration.md](docs/configuration.md) for all environment variables and config options.

## Nginx

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

## Docs

- [Configuration](docs/configuration.md) — env vars, log levels, config.yml
- [Backend architecture](docs/architecture/backend.md) — endpoints, auth, storage, dev commands
- [API reference](API.md)
- [Troiki System](docs/troiki-system.md)
- [Localization](docs/locales.md)
- [Public mode](docs/public-mode.md)
- [Google Calendar](docs/google-calendar.md)

## RoadMap

- Feature: extended session management on Session page
- Feature: Task templates
- Feature: Federated Project Synchronization (Bridge Protocol) for Multi-Instance Collaboration
- Offline-first
- iOS Native App
- Feature: Constraints

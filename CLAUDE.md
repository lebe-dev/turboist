# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Turboist

Turboist is a web app that augments Todoist with extra features: context-based task filtering, weekly planning, backlog management, day parts, auto-labeling, auto-remove, quick capture, and self-discipline constraints. Go backend + SvelteKit frontend, deployed as a single binary with embedded static files.

## Production instance

SSH: kaiman

Host kaiman
  User root
  HostName 146.103.96.8
  Port 22
  IdentityFile ~/.ssh/id_ed25519
  
/opt/turboist

## Commands

All commands are in the Justfile. Key ones:

```bash
just build              # Build frontend + Go binary
just run-backend        # Run Go server (port 8080)
just run-frontend       # Run Vite dev server (port 4200)
just dev                # Run both concurrently (frontend proxied in dev mode)

just test               # Go tests (all)
just test <name>        # Go tests matching name
just test-frontend      # Vitest (all)
just test-frontend <n>  # Vitest matching name
just test-all           # Both

just lint               # golangci-lint + svelte-check
just lint-backend       # golangci-lint only
just lint-frontend      # svelte-check only
just format             # go fmt

just start-env          # docker compose up (for local dev)
just stop-env           # docker compose down
```

## Required environment

- `TODOIST_API_KEY` and `TURBOIST_ADMIN_PASSWORD` — set in `.env` file (auto-loaded)
- `DEV=true` — enables frontend proxy to Vite dev server at localhost:5173
- App config: `config.yml` (see `config.example.yml` for full reference)

## Architecture

### Backend (Go)

```
cmd/turboist/main.go    — entrypoint, wires everything together
internal/
  config/               — YAML + env config loading
  todoist/              — Todoist API client (REST for single ops, Sync for batch/fetch) + in-memory cache
  handler/              — HTTP handlers (Fiber v3): tasks, auth, config, state, constraints, health
  server/               — Fiber app setup, routing, middleware, static file serving
  ws/                   — WebSocket hub: channels, delta broadcasting, protocol
  scheduler/            — Background jobs: auto-remove, weekly-limit, label-project sync
  storage/              — SQLite persistence (user state, postpone counts, auto-remove tracking, constraints label blocks)
  context/              — Task filtering by context (labels, projects, sections)
  taskview/             — Task view/sort logic
  auth/                 — Session-based auth with cookie tokens
static.go               — embed directive for frontend/build/
```

**Data flow:** Backend polls Todoist API → in-memory cache → WebSocket broadcasts snapshots/deltas to connected clients. Mutations go through cache methods which call Todoist API then schedule a debounced refresh.

### Frontend (SvelteKit 2 + Svelte 5)

SPA mode (static adapter, no SSR). TailwindCSS 4 + bits-ui components.

```
frontend/src/
  routes/               — Pages: main, task detail, settings, login, labels, projects
  lib/
    api/                — BackendConnector interface + DefaultBackend (HTTP) + MockBackend
    stores/             — Svelte 5 rune-based stores (*.svelte.ts files)
    ws/                 — WebSocket client with auto-reconnect, ping/pong
    components/         — Domain components + UI primitives (bits-ui wrappers)
    utils/              — Shared utilities including task-tree conversion
    i18n.ts             — i18n setup (svelte-intl-precompile)
  locales/              — en.json, ru.json (update both when adding user-visible strings)
```

**Store pattern:** Factory function creating a singleton with `$state` encapsulated inside, exposed via getters and methods. All store files use `.svelte.ts` extension for rune reactivity.

**WebSocket protocol:** Two channels (`tasks`, `planning`). Server sends `snapshot` (full replace) or `delta` (upserted + removed IDs). Client subscribes with view/context params.

**Flat task model:** Tasks arrive as trees but are stored flat (`FlatTask[]`) for efficient reactive updates. `buildTree()` reconstructs the tree in store getters.

**Constraints store:** `constraints.svelte.ts` — singleton store hydrated from `config.constraints`. Enforcement queries: `isLabelBlocked(labels)`, `getBlockedLabelSeconds(labels)`, `isPostponeExhausted()`, `isPriorityBelowFloor(priority)`, `getDayPartCap(label)`. Mutation helpers: `incrementPostponeUsed()`, `updateDailyConstraints(response)`. Used by TaskItem, DayPartTaskList, DailyConstraintsDialog, DailyConstraintsBanner, +page.svelte, and settings/+page.svelte. TaskDropdownMenu receives constraint state as props from TaskItem.

**Constraints UI:** `DailyConstraintsDialog` opens automatically on the Today view when daily constraints need selection (pool > 0, not yet picked today). `DailyConstraintsBanner` shows confirmed constraints as a banner above the Today task list.

### Constraints API

```
GET  /api/constraints/daily        — daily constraints status (needs_selection, items, rerolls)
POST /api/constraints/daily/roll   — pick/re-roll random constraints from pool
POST /api/constraints/daily/swap   — swap one constraint item { index: N }
POST /api/constraints/daily/confirm — lock daily constraints selection
```

Label block status and config-driven constraints (day part caps, priority floor, postpone budget) are served via `GET /api/config` in the `constraints` field. Constraint pool is persisted via `PATCH /api/state` with `constraint_pool` key. All constraint endpoints require `constraints.enabled = true`; they return 400 when disabled.

**Postpone budget enforcement:** The task Update handler (`tasks.go`) detects date-forward changes as postpones. When `constraints.enabled` and `constraints.postpone_budget > 0` and the daily used count meets the limit, the handler returns `400 "Daily postpone limit reached"`. After a successful postpone, it increments the used count in `postpone_budget` user_state.

### Embedding

Frontend is built to `frontend/build/`, then embedded into the Go binary via `//go:embed all:frontend/build` in `static.go`. In dev mode (`DEV=true`), requests are proxied to Vite instead.

## Key conventions

- **i18n:** Always update both `frontend/locales/en.json` and `frontend/locales/ru.json`
- **Icons:** Import individually from `@lucide/svelte/icons/<name>`, never from barrel export
- **UI primitives:** bits-ui wrappers in `$lib/components/ui/`, imported as namespace (`import * as Dialog from '...'`)
- **Go tests:** Standard library assertions only (no testify). `TestFunc_CaseName` naming. Hand-written mocks
- **Go handlers:** One struct per domain, constructor injection, Fiber's `app.Test()` for HTTP tests
- **Todoist client:** REST API for single-entity mutations (add/update/delete/complete/move task/section), Sync API for data fetching (full/incremental sync) and batch operations (SetTasksLabels, DecomposeTask, BatchMove). Internal args types (`TaskAddArgs`, `TaskUpdateArgs`) in todoist package — never leak external library types into handlers/troiki
- **Cache mutations:** Use cache methods (they auto-refresh). If using `cache.Client()` directly, call `cache.RefreshAfterMutation()`
- **Svelte 5 only:** `$state`, `$derived`, `$effect`, snippets. No Svelte 4 APIs (`writable`, `readable`, `<slot>`)

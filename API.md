# Turboist API

The API is described by an OpenAPI 3.1 document. It is the single source of
truth for every endpoint, request and response schema, required auth and
API-token scope. This file covers only what the spec cannot express: how to
reach it, and the cross-cutting rules and concepts behind the endpoints.

## Where to get the spec

- **From a running server:** `GET /api/v1/openapi.json`. Requires
  authentication — a JWT session access token or any valid API token (no
  particular scope needed). Anonymous requests get `401`.
- **From the repository:** [`api/openapi.yaml`](api/openapi.yaml). Generated
  from the route table and the Go DTO types by `just openapi`; never edit it by
  hand.

```sh
BASE="http://localhost:8080"
TOKEN="your-api-token-or-jwt"

curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/openapi.json"
```

## Prompt for an AI agent

Give this to an agent that needs to work with a Turboist instance:

```text
You have access to a Turboist task manager API.
Base URL: <BASE_URL>
API token: <API_TOKEN>

Before calling any endpoint, download the OpenAPI spec:
  GET <BASE_URL>/api/v1/openapi.json
  Header: Authorization: Bearer <API_TOKEN>

Use it as the only reference for paths, parameters, request/response
schemas and required scopes. Send the same Authorization header on every
request. A 403 with "insufficient scope" means the token lacks the scope
listed for that operation in the spec — report it instead of retrying.
```

---

## Authentication

All authenticated endpoints accept either a **JWT access token** or a
**long-lived API token** in the same header:

```
Authorization: Bearer <token>
```

### API tokens

API tokens are created in the web app settings (or via `POST /api/v1/api-tokens`
with a JWT session) and never expire. The plaintext token is returned only once.

### JWT (session-based)

1. `POST /auth/login` → receive `access` (JWT, 15 min TTL) and `refresh` (30 days)
2. Send `Authorization: Bearer <access>` on every request
3. Refresh with `POST /auth/refresh` before expiry

`/auth/*` is rate limited: 10 requests/minute per IP.

### Setup check

There is no dedicated endpoint. A setup check sits in front of the whole
`/api/v1` group, so any call to it on an instance that has not been set up yet
short-circuits with `503` and `{"error": {"code": "setup_required", ...}}`
before authentication runs. The SPA probes `GET /api/v1/config` without
credentials for exactly this.

### Scopes

API tokens carry granular permissions. Each scope grants one action on one
resource; the scope every operation requires is listed in the spec. JWT sessions
are never checked against scopes.

| Scope | Description |
|-------|-------------|
| `tasks:read` | Read tasks: get by id, all task views, subtasks, sidebar and week stats, inbox list, tasks listed under a context / project / section / label |
| `tasks:write` | Create, update, delete tasks; complete / uncomplete / cancel; pin / unpin; move; plan; decompose; duplicate; task relations; bulk operations; grouping; create tasks in inbox / context / project / section |
| `projects:read` | Read projects: list, get by id, tasks/sections of a project, projects in a context or by label |
| `projects:write` | Create, update, delete projects; complete / uncomplete / cancel / archive / unarchive; pin / unpin; assign or clear Troiki category |
| `contexts:read` | Read contexts |
| `contexts:write` | Create, update, delete contexts |
| `labels:read` | Read labels, including search and usage stats |
| `labels:write` | Create, update, delete labels |
| `sections:read` | Read sections |
| `sections:write` | Create, update, delete, reorder sections |
| `templates:read` | Read task templates |
| `templates:write` | Create, update, delete task templates |
| `troiki:read` | Read the current Troiki view |
| `troiki:write` | Start a Troiki day; reset Troiki |
| `settings:read` | Read user settings, server config, persisted UI state |
| `settings:write` | Update user settings; merge UI state |
| `search:read` | Full-text search across tasks and projects |
| `calendars:read` | Read calendars and calendar events |
| `*` | Full access — covers all current and future scopes |

Rules:

- `write` does **not** imply `read`, and vice versa. Grant both when both are needed.
- `*` may only appear alone (`["*"]`); combining it with concrete scopes is rejected.
- Cross-resource endpoints are bound to the **target resource**, not the path
  parent: `POST /projects/:id/tasks` requires `tasks:write` only.
- A request whose scopes do not cover the endpoint gets `403` with
  `code = "forbidden"` and message `"insufficient scope"`. The missing scope is
  logged server-side but deliberately not returned, so probing endpoints does
  not reveal the scope catalog.

### JWT-only endpoints

Some endpoints reject API tokens with `401` regardless of scopes, including `*`:

| Endpoints | Reason |
|-----------|--------|
| `/api/v1/api-tokens` | Token management must come from a session |
| `/api/v1/sessions` | A leaked integration token must not be able to drop your browsers |
| `/api/v1/passkeys` | Registering a passkey mints a password-equivalent credential |
| `/auth/totp/*` | 2FA management |
| `GET /api/v1/backup`, `POST /api/v1/restore` | The backup contains the whole database |
| `GET /api/v1/events` (SSE) | The stream emits across all resources without per-scope filtering |
| Calendar mutations | Connecting and configuring accounts stays with the session |

---

## Conventions

### Timestamps

All timestamps are ISO-8601 UTC with millisecond precision:
`2024-01-15T09:30:00.000Z`.

### Pagination

List endpoints accept `limit` (default 50, max 200) and `offset`, and answer
with an envelope:

```json
{ "items": [...], "total": 100, "limit": 50, "offset": 0 }
```

### Errors

Every failed request answers with the same envelope:

```json
{ "error": { "code": "not_found", "message": "task not found", "details": null } }
```

| Code | HTTP |
|------|------|
| `validation_failed` | 400 or 422 |
| `auth_invalid` | 401 |
| `auth_expired` | 401 |
| `totp_invalid_code` | 401 |
| `forbidden` | 403 |
| `not_found` | 404 |
| `conflict` | 409 |
| `idempotency_in_flight` | 409 |
| `task_blocked` | 409 (`details.blockerIds` lists the open blockers) |
| `today_locked` | 409 (`details.lockedUntil`, `details.projectId`; see [Today focus lock](#today-focus-lock)) |
| `troiki_slot_full` | 409 |
| `totp_already_enabled` | 409 |
| `totp_not_enabled` | 409 |
| `passkey_ceremony_invalid` | 400 |
| `passkey_exists` | 409 |
| `calendar_reauth_required` | 409 |
| `sync_epoch_mismatch` | 409 (`details.epoch` is the current epoch) |
| `inbox_processing_disabled` | 409 |
| `inbox_processing_nothing_pending` | 409 |
| `setup_already_done` | 410 |
| `sync_cursor_expired` | 410 |
| `limit_exceeded` | 422 |
| `forbidden_placement` | 422 |
| `recurrence_invalid` | 422 |
| `auth_rate_limited` | 429 |
| `internal_error` | 500 |
| `setup_required` | 503 |

### Colors

Color fields accept named CSS colors or hex strings `#RRGGBB`.

### PATCH semantics

- Fields absent from the request body stay unchanged.
- Optional fields that can be cleared (e.g. `dueAt`, `deadlineAt`,
  `recurrenceRule`) are three-state: **absent** = unchanged, **`null`** = clear,
  **value** = set.

### Idempotency

Any mutating request (`POST`/`PUT`/`PATCH`/`DELETE`) to `/api/v1/*` may carry
an `Idempotency-Key` header, so a client can retry a request whose response it
never received without risking a duplicate side effect.

```sh
curl -X POST "$BASE/api/v1/tasks/42/complete" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: 3f0c1c2e-8a4b-4e9d-9d1a-2b7c9f6e0a11"
```

- **Key format**: 8–128 characters from `[A-Za-z0-9_-]` (UUIDs qualify). A
  malformed key returns `400 validation_failed`.
- **First request** with a key runs normally; its response is stored only if
  the status is `2xx`.
- **Replay** with the same key returns the stored response verbatim with
  `X-Idempotent-Replay: true`. The handler does not run again and no SSE
  invalidation is re-emitted.
- **Concurrent duplicate** — a second request while the first is still running
  — returns `409 idempotency_in_flight`.
- **Non-2xx responses are not stored**: a request that failed validation can be
  corrected and retried with the same key.
- **Retention**: keys are kept for 48 hours; a replay after that re-runs the
  handler.
- The header is optional and scoped per user.

### Conditional requests

`GET /api/v1/config` carries a strong `ETag` over the body bytes. Send it back
as `If-None-Match` and an unchanged workspace answers `304 Not Modified` with an
empty body. The ETag changes whenever any embedded section does.

---

## Concepts

### Passkeys (WebAuthn)

A passkey is an **additional** login method — the password stays as the
recovery path. A passkey assertion satisfies both factors at once, so it never
leads to a TOTP step, even with 2FA enabled.

Credentials are **discoverable**: login needs no username. The Relying Party is
derived from `BASE_URL` (`WEBAUTHN_RP_ID` / `WEBAUTHN_ORIGINS` override it — see
[docs/configuration.md](docs/configuration.md)); changing the RP ID invalidates
every registered passkey.

Every ceremony is two calls. `begin` returns `{ceremonyId, options}`, where
`options` goes to `navigator.credentials.create()`/`.get()` verbatim (binary
fields base64url-encoded); `finish` sends the authenticator's answer in the same
JSON form with the `ceremonyId`. The challenge lives server-side for **5
minutes** and is single-use — a replayed `finish` gets
`400 passkey_ceremony_invalid`. When WebAuthn is not configured, these routes
do not exist and answer `404`.

### Task relations

Directed links between two tasks:

| Type | Meaning |
|------|---------|
| `related` | Symmetric, informational. Adding it from either side produces the same single relation, so a duplicate is rejected with `409 conflict`. |
| `blocks` | Directed and **enforced**: the blocked task cannot be completed while the blocking task is `open` (`409 task_blocked`). |

`direction` is relative to the task in the path and only matters for `blocks`:
`incoming` — the other task blocks this one; `outgoing` — this task blocks the
other one. The relation's `task` is the **peer** end, so one stored relation
reads as `incoming` on one task and `outgoing` on the other.

There is no endpoint listing relations: they ride inside
`GET /api/v1/tasks/:id?relations=true`, and both mutations answer with the
updated task.

- Creating or removing a relation bumps `updatedAt` on **both** tasks.
- Tasks are hard-deleted and relations cascade — deleting a blocker releases
  everything it was blocking.
- `duplicate`, `decompose` and recurrence snapshots do **not** copy relations.
- Relations are included in backup export/restore with their ids preserved.

### Task templates

Reusable blueprints — a root task plus an ordered set of subtasks — that can be
materialized into any project. Each row captures title, description, priority,
day part and labels. A template can be seeded from an existing task with
`GET /api/v1/tasks/:id/template-draft`, which returns an unsaved draft to
prefill the editor.

### Inbox processing

An optional background job files open Inbox tasks into projects with an
OpenAI-compatible language model (`INBOX_PROCESSING_*` environment variables;
off by default). Its endpoints are mounted either way, so a client can tell a
disabled feature from an old server.

### Calendars

Read-only Google Calendar integration. Events are fetched live from Google and
cached in memory (default 10 min, `CALENDAR_CACHE_TTL`); they are never stored
in the database. See [docs/google-calendar.md](docs/google-calendar.md).

### Troiki

A 3-slot priority system (important / medium / rest) for daily project planning.

### App settings

Global, server-wide rules. The same payload is embedded as `appSettings` in
`GET /api/v1/config`.

- **Auto-labels** — when a task title contains `mask`, the listed labels are
  attached on create / title change.
- **Project suggestions** — when a task title contains `mask`, the listed
  projects are *offered* in quick-add (deduped, sorted A–Z, capped at 3). The
  server never assigns a project itself.

`ignoreCase: true` makes the mask match case-insensitively.

- **Link titles** (`linkTitles`) — `{enabled, template, rules: [{domain, template}]}`.
  When enabled, a task created with a bare `https://` URL as its title is renamed
  in the background from the page's title, and the URL moves into the
  description. `PUT /api/v1/app-settings/link-titles` takes any subset of the
  three keys; `rules` replaces the list. Templates must contain `{title}`;
  domains are normalised (`https://www.youtube.com/` → `youtube.com`). See
  [docs/link-titles.md](docs/link-titles.md).

### Today focus lock

`POST /api/v1/today/lock` (scope `settings:write`) locks Today to the projects already on
it until the next midnight in the server timezone, and answers `{"lockedUntil": "…"}`.
Locking again while a lock runs keeps it; there is no way to lift it early. While it runs,
any write that would put an open task due today or earlier into a project with no other
open work on Today — or into the inbox — is refused with `409 today_locked`. Work for
other days is unaffected. The end is served as `todayLockUntil` in the user settings
(read-only there). See [docs/today-lock.md](docs/today-lock.md).

### Harpoon

A per-user "jump pair": at most **two** task/project references. Slot 0 is the
first, slot 1 the second; attaching a third evicts the oldest. Titles are
hydrated on read, and references to deleted entities are silently dropped.

### State

A per-user key-value blob for UI state; values can be any JSON type. Setting a
key to `null` removes it.

---

## Sync

Two endpoints for clients that keep a **full local copy** of the workspace and
reconcile it in the background. They are additive: payloads are the objects the
REST API already serves, and a client without a local copy never needs them.

- `GET /api/v1/sync/snapshot` — seeds a replica.
- `GET /api/v1/sync/changes` — carries what changed since a cursor.

There is no push endpoint; writes go through the ordinary mutation endpoints.

### The loop

1. `GET /api/v1/sync/snapshot` — store every collection with the returned
   `epoch` and `cursor`.
2. `GET /api/v1/sync/changes?since=<cursor>&epoch=<epoch>` — apply the page and
   persist the new `cursor` in one local transaction; repeat while `hasMore`.
3. Pull again whenever something moves. Any SSE event (`GET /api/v1/events`) or
   reconnect means "ask for changes". The stream is JWT-only, so an API-token
   integration polls the change feed instead.
4. Write through the normal endpoints, each with an `Idempotency-Key`. Your own
   writes come back through the feed as upserts of state you already hold.
5. On `409 sync_epoch_mismatch` or `410 sync_cursor_expired`, go back to step 1.

Applying is idempotent: a change carries the entity's **current** row rather
than a diff, and a delete is a tombstone. Replaying a page after a crash costs
nothing but writing it twice.

### Entities

| `entity` | Payload | Snapshot field |
|----------|---------|----------------|
| `task` | Task object | `tasks` |
| `project` | Project object | `projects` |
| `section` | Section object | `sections` |
| `context` | Context object | `contexts` |
| `label` | Label object | `labels` |
| `task_relation` | Relation edge (below) | `taskRelations` |
| `task_template` | Template object | `taskTemplates` |
| `user_settings` | Settings object | `userSettings` |
| `user_state` | UI-state object | `userState` |
| `app_settings` | App settings object | `appSettings` |

Join and child rows have no entity of their own: a change to a task's or
project's labels is an `upsert` of the owner; a change to a template's subtasks
or their labels is an `upsert` of the `task_template`.

**Never replicated**: sessions, API tokens, TOTP recovery codes, passkeys,
idempotency keys and calendar data.

The `entity` strings are part of the wire contract: the set only grows, and a
name is never repurposed.

`task_relation` carries the stored edge (`sourceTaskId`, `targetTaskId`,
`type`) rather than the direction-relative relation object. For
`type: "blocks"`, the source blocks the target.

### Retention

The change history is kept for **90 days** — the same window the snapshot seeds
completed tasks over — and trimmed daily. A cursor that has fallen behind the
oldest retained change gets `410 sync_cursor_expired`. A cursor sitting exactly
on the trim boundary lost nothing and keeps working.

### Epoch

`epoch` names the history a cursor belongs to. It advances only when the data is
replaced wholesale by `POST /api/v1/restore`. A client sending the old `epoch`
gets `409 sync_epoch_mismatch`; one that omits it gets
`410 sync_cursor_expired`, because the restore pushes the history past every
earlier cursor. Either way the remedy is a fresh snapshot.

### Scopes

Both endpoints require **every read scope**: `tasks:read`, `projects:read`,
`sections:read`, `contexts:read`, `labels:read`, `templates:read` and
`settings:read`, so reading the workspace through sync is never cheaper than
reading it endpoint by endpoint.

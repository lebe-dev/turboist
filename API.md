# Turboist API Reference

All authenticated endpoints accept either a **JWT access token** or a **long-lived API token**.

## Authentication

### Using an API Token

```
Authorization: Bearer <token>
```

API tokens are created via `POST /api/v1/api-tokens` (requires a JWT session) and never expire. Store the plaintext token securely — it is only returned once.

#### Scopes

API tokens carry **granular permissions** (scopes). Each scope grants access to one resource and one action.

- Scope format: `resource:action`, where `action` is either `read` or `write`
- `write` is **not** implied by `read` and vice versa — both must be granted independently if both are needed
- Special wildcard `*` grants full access (all current and future scopes)
- A request whose scopes do not cover the called endpoint returns `403 Forbidden` with `code = "forbidden"` and message `"insufficient scope"` (the missing scope name is logged server-side but not exposed in the response)
- Some endpoints are reserved for JWT sessions and are **never** reachable with an API token (token management, session management, TOTP, backup/restore, SSE `/events`) — calling them with a Bearer API token returns `401`

JWT sessions are unaffected by scopes — a logged-in session always has full access. Only API tokens are checked against scopes.

See the [Scopes Reference](#scopes-reference) below for the full list of scopes and the [Endpoint → Scope Mapping](#endpoint--scope-mapping) for the required scope of every endpoint.

### Using JWT (session-based)

1. `POST /auth/login` → receive `access` (JWT, 15 min TTL) and `refresh` (30 days)
2. Send `Authorization: Bearer <access>` on every request
3. Refresh with `POST /auth/refresh` before expiry

The examples below use shell variables:

```sh
BASE="http://localhost:8080"
TOKEN="your-api-token-or-jwt"
```

---

## Breaking changes

### v1.15

- **`GET /api/v1/stats/plan` is removed.** It returned `{week, backlog}`, a strict
  subset of both `GET /api/v1/stats/sidebar` (as `planStats`) and
  `GET /api/v1/config` (likewise). Use either of those.
- **`GET /api/v1/config` requires more scopes.** It previously accepted a lone
  `settings:read`, which let such a token read every task, project, label,
  context, template and the Troiki board through the aggregate. It now requires
  `settings:read` **and** `tasks:read`, `projects:read`, `labels:read`,
  `contexts:read`, `troiki:read`, `templates:read`. `*` tokens and JWT sessions
  are unaffected.
- **`GET /api/v1/config` gained `harpoon` and `taskTemplates`** and now answers
  `304` to a matching `If-None-Match`. Both are additive.
- **`POST /auth/refresh` gained a `user` object.** Additive.

---

## Conventions

### Timestamps

All timestamps are ISO-8601 UTC with millisecond precision: `2024-01-15T09:30:00.000Z`.

### Pagination

List endpoints accept `limit` (default 50, max 200) and `offset` query params. Response envelope:

```json
{
  "items": [...],
  "total": 100,
  "limit": 50,
  "offset": 0
}
```

### Error Response

```json
{
  "error": {
    "code": "CodeNotFound",
    "message": "task not found",
    "details": null
  }
}
```

| Code | HTTP |
|------|------|
| `CodeNotFound` | 404 |
| `CodeAuthInvalid` | 401 |
| `CodeAuthRateLimited` | 429 |
| `CodeValidation` | 422 |
| `CodeForbidden` | 403 |
| `CodeConflict` | 409 |
| `idempotency_in_flight` | 409 |
| `CodeLimitExceeded` | 409 |
| `CodeForbiddenPlacement` | 422 |
| `totp_invalid_code` | 401 |
| `totp_already_enabled` | 409 |
| `totp_not_enabled` | 409 |
| `calendar_reauth_required` | 409 |
| `task_blocked` | 409 |
| `CodeInternalError` | 500 |

#### `403 Forbidden`

Returned when an API token is missing a scope required by the endpoint. JWT sessions never receive this response.

```json
{
  "error": {
    "code": "forbidden",
    "message": "insufficient scope"
  }
}
```

The response intentionally omits the required scope name — the missing scope is logged server-side but is not surfaced to the client, to avoid leaking the internal scope catalog via endpoint probing.

### Enum Values

| Field | Values |
|-------|--------|
| `priority` | `high`, `medium`, `low`, `no-priority` |
| task `status` | `open`, `completed`, `cancelled` |
| project `status` | `open`, `completed`, `cancelled`, `archived` |
| `projectType` | `generic`, `software` |
| `dayPart` | `none`, `morning`, `afternoon`, `evening` |
| `planState` | `none`, `week`, `backlog` |
| `troikiCategory` | `important`, `medium`, `rest` |
| `clientKind` | `web`, `ios`, `cli` |

### Colors

Color fields accept named CSS colors or hex strings `#RRGGBB`.

### PATCH semantics

- Fields absent from the request body → unchanged
- Optional fields that support null-clearing (e.g. `dueAt`, `deadlineAt`, `recurrenceRule`) use three-state semantics: **absent** = unchanged, **`null`** = clear, **string** = set value

### Idempotency

Any mutating request (`POST`/`PUT`/`PATCH`/`DELETE`) to `/api/v1/*` may carry an `Idempotency-Key` header. It lets a client safely retry a request whose response it never received (e.g. the network dropped after the server committed the change) without risking a duplicate side effect.

```sh
curl -X POST "$BASE/api/v1/tasks/42/complete" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: 3f0c1c2e-8a4b-4e9d-9d1a-2b7c9f6e0a11"
```

- **Key format**: 8–128 characters from `[A-Za-z0-9_-]` (UUIDs qualify). A malformed key returns `400 validation_failed`.
- **First request** with a given key runs normally; its response is stored only if the status is `2xx`.
- **Replay** — a later request with the **same** key — returns the stored response verbatim (same status and body) with the header `X-Idempotent-Replay: true`. The handler does not run again and no SSE invalidation is re-emitted.
- **Concurrent duplicate** — a second request arriving while the first is still executing returns `409 idempotency_in_flight`.
- **Non-2xx responses are not stored**: a request that failed validation can be corrected and retried with the same key.
- **Retention**: keys are kept for 48 hours, then pruned; a replay after expiry re-runs the handler.
- The header is optional and scoped per user; omitting it preserves the previous behaviour with zero overhead.

---

## Public Endpoints (no auth required)

### `GET /healthz`

```json
{ "status": "ok" }
```

```sh
curl "$BASE/healthz"
```

### `GET /version`

```json
{ "version": "1.3.0", "commit": "", "buildTime": "" }
```

```sh
curl "$BASE/version"
```

### `GET /api/config`

Public runtime configuration for the SPA (kept unauthenticated so the browser can initialise error reporting before login). A blank `dsn` means the frontend leaves Sentry disabled.

```json
{ "sentry": { "dsn": "https://<key>@<host>/<project>", "environment": "production" } }
```

```sh
curl "$BASE/api/config"
```

---

## Auth

> Rate limited: 10 requests/minute per IP.

### Checking whether setup is needed

There is no dedicated endpoint. `SetupCheckMiddleware` sits in front of the whole
`/api/v1` group, so any call to it on an un-set-up instance short-circuits with
`503` and `{"error": {"code": "setup_required", ...}}` before authentication runs.
The SPA probes `GET /api/v1/config` without credentials for exactly this.

```sh
curl -i "$BASE/api/v1/config"
```

### `POST /auth/setup`

First-time setup. Creates the single user account. Fails if a user already exists.

**Request:**
```json
{
  "username": "alice",
  "password": "secret",
  "clientKind": "cli"
}
```

**Response:** same as `/auth/login`.

```sh
curl -X POST "$BASE/auth/setup" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret","clientKind":"cli"}'
```

### `POST /auth/login`

**Request:**
```json
{
  "username": "alice",
  "password": "secret",
  "clientKind": "cli"
}
```

**Response:**
```json
{
  "access": "<jwt>",
  "refresh": "<token>",
  "user": { "id": 1, "username": "alice" }
}
```

Web clients (`clientKind: "web"`) receive the refresh token in an `HttpOnly` cookie instead of the body.

If the account has TOTP enabled, `/auth/login` returns an OTP challenge instead
of the regular response — both with HTTP `200`:

```json
{ "otpRequired": true, "ticket": "<short-lived JWT>" }
```

The ticket is valid for 5 minutes, is bound to the chosen `clientKind`, and is
only usable on `/auth/login/otp`. Clients should keep it in memory only.

```sh
curl -X POST "$BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret","clientKind":"cli"}'
```

### `POST /auth/login/otp`

Completes a two-step login. Accepts either a TOTP code or one of the user's
unused recovery codes.

**Request:**
```json
{
  "ticket": "<ticket from /auth/login>",
  "code": "123456"
}
```

**Response** (200) — identical to `/auth/login`:
```json
{
  "access": "<jwt>",
  "refresh": "<token>",
  "user": { "id": 1, "username": "alice" }
}
```

Errors: `auth_invalid` (401) for missing/expired ticket; `totp_invalid_code`
(401) for a wrong TOTP or recovery code; `auth_rate_limited` (429) when the IP
limiter trips. The endpoint shares the same per-IP backoff as `/auth/login`.

### `POST /auth/refresh`

Exchange a refresh token for a new access token. Send either:

- Body: `{ "refresh": "<token>" }`
- Or cookie `refresh=<token>` (web clients)

**Response:**
```json
{
  "access": "<jwt>",
  "refresh": "<new-token>",
  "user": { "id": 1, "username": "admin", "totpEnabled": false }
}
```

The `user` object lets a booting client skip a follow-up `GET /auth/me`; that
endpoint remains available.

Reusing an already-rotated refresh token revokes the session (theft detection).

```sh
# via request body
curl -X POST "$BASE/auth/refresh" \
  -H "Content-Type: application/json" \
  -d '{"refresh":"<refresh-token>"}'

# via cookie (web clients)
curl -X POST "$BASE/auth/refresh" \
  -b "refresh=<refresh-token>"
```

### `POST /auth/logout` *(requires JWT)*

Revokes the current session. Returns `204 No Content`.

```sh
curl -X POST "$BASE/auth/logout" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /auth/logout-all` *(requires JWT)*

Revokes all sessions for the user. Returns `204 No Content`.

```sh
curl -X POST "$BASE/auth/logout-all" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /auth/logout-others` *(requires JWT)*

Revokes every active session of the current user except the one that issued the request — useful for kicking forgotten devices while staying logged in here. Returns `204 No Content`.

```sh
curl -X POST "$BASE/auth/logout-others" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /auth/me` *(requires JWT)*

```json
{ "user": { "id": 1, "username": "alice", "totpEnabled": false } }
```

`totpEnabled` reflects whether the account has confirmed TOTP 2FA enrollment.

```sh
curl "$BASE/auth/me" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /auth/totp/setup` *(requires JWT)*

Begin TOTP enrollment. Generates a fresh secret, encrypts and persists it
without enabling 2FA, and returns the data needed to display a QR code.

**Response** `200`:
```json
{
  "secret": "JBSWY3DPEHPK3PXP",
  "otpauthUrl": "otpauth://totp/Turboist:alice?secret=...&issuer=Turboist",
  "qrPngBase64": "<base64 PNG>"
}
```

Errors: `totp_already_enabled` (409) if 2FA is already on.

### `POST /auth/totp/confirm` *(requires JWT)*

Verify the user-supplied 6-digit code against the pending secret. On success
enables 2FA and returns 8 single-use recovery codes — these are the only time
they are visible.

**Request:**
```json
{ "code": "123456" }
```

**Response** `200`:
```json
{ "recoveryCodes": ["ABCDEFGHJK", "..."] }
```

Errors: `totp_invalid_code` (401), `totp_already_enabled` (409),
`totp_not_enabled` (409) when no pending setup exists, `auth_rate_limited`
(429).

### `POST /auth/totp/disable` *(requires JWT)*

Disable 2FA. The supplied code may be either a current TOTP code or an unused
recovery code.

**Request:**
```json
{ "code": "123456" }
```

**Response:** `204 No Content`.

Errors: `totp_invalid_code` (401), `totp_not_enabled` (409),
`auth_rate_limited` (429).

---

## API Tokens

> These endpoints require a **JWT session** — API token authentication is rejected here.

### `POST /api/v1/api-tokens`

Create a new long-lived API token with a fixed set of scopes. The plaintext token is returned **only in this response**.

**Request:**
```json
{
  "name": "my-script",
  "scopes": ["tasks:read", "tasks:write", "projects:read"]
}
```

- `name` — required, non-empty
- `scopes` — required, non-empty. Each entry must be a [valid scope](#scopes-reference) or the wildcard `"*"`. Validation is strict:
  - Duplicates → `422`
  - `"*"` combined with any other scope → `422`
  - `<resource>:write` without `<resource>:read` for the same resource → `422`
  - Unknown scope → `422`
- Scopes are **immutable** after creation. To change permissions, delete the token and create a new one.

**Response** `201`:
```json
{
  "id": 1,
  "name": "my-script",
  "scopes": ["tasks:read", "tasks:write", "projects:read"],
  "token": "abc123...",
  "createdAt": "2024-01-15T09:30:00.000Z"
}
```

```sh
# Read+write on tasks, read-only on projects
curl -X POST "$BASE/api/v1/api-tokens" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-script","scopes":["tasks:read","tasks:write","projects:read"]}'

# Full access (wildcard)
curl -X POST "$BASE/api/v1/api-tokens" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"admin-cli","scopes":["*"]}'
```

### `GET /api/v1/api-tokens`

List all tokens (metadata only — plaintext is never returned after creation). Each entry includes its scopes.

**Response:**
```json
[
  {
    "id": 1,
    "name": "my-script",
    "scopes": ["tasks:read", "tasks:write", "projects:read"],
    "createdAt": "2024-01-15T09:30:00.000Z"
  },
  {
    "id": 2,
    "name": "admin-cli",
    "scopes": ["*"],
    "createdAt": "2024-01-15T09:30:00.000Z"
  }
]
```

Tokens created before scopes were introduced are returned with `scopes: ["*"]` (full access) for backwards compatibility.

```sh
curl "$BASE/api/v1/api-tokens" \
  -H "Authorization: Bearer $JWT"
```

### `DELETE /api/v1/api-tokens/:id`

Revoke a token. Returns `204 No Content`.

```sh
curl -X DELETE "$BASE/api/v1/api-tokens/1" \
  -H "Authorization: Bearer $JWT"
```

### Scopes Reference

The 16 concrete scopes (plus the wildcard `*`) accepted by `POST /api/v1/api-tokens`:

| Scope | Description |
|-------|-------------|
| `tasks:read` | Read tasks: get by id, all task views (`today`, `tomorrow`, `overdue`, `week`, `backlog`, `pinned`, `completed`), subtasks list, `stats/sidebar`, `stats/week-summary`, inbox list, tasks listed under a context / project / section / label |
| `tasks:write` | Create, update, delete tasks; complete / uncomplete / cancel; pin / unpin; move; plan; decompose; duplicate; add / remove task relations; bulk operations (`bulk/complete`, `bulk/move`, `bulk/priority`); `tasks/group`; create tasks in inbox / context / project / section |
| `projects:read` | Read projects: list, get by id, list tasks/sections of a project, list projects in a context or by label |
| `projects:write` | Create, update, delete projects; complete / uncomplete / cancel / archive / unarchive; pin / unpin; assign or clear Troiki category |
| `contexts:read` | Read contexts: list, get by id |
| `contexts:write` | Create, update, delete contexts |
| `labels:read` | Read labels: list (with search), get by id |
| `labels:write` | Create, update, delete labels |
| `sections:read` | Read sections: get by id, list sections of a project |
| `sections:write` | Create, update, delete sections; reorder |
| `troiki:read` | Read current Troiki view |
| `troiki:write` | Start a Troiki day; reset Troiki |
| `settings:read` | Read user settings, server config, persisted UI state |
| `settings:write` | Update user settings; merge UI state |
| `search:read` | Full-text search across tasks and projects |
| `calendars:read` | Read calendars and calendar events |
| `*` | Full access — covers all current and future scopes |

**Rules:**

- `write` does **not** imply `read`. To both read and write a resource, grant both scopes explicitly.
- The wildcard `*` may only appear alone (`["*"]`); combining it with concrete scopes is rejected.
- Cross-resource endpoints are bound to the **target resource**, not the path parent. For example, `POST /projects/:id/tasks` requires `tasks:write` only — `projects:read` is **not** required even though the URL contains a project id.

### Endpoint → Scope Mapping

Required scope for every authenticated endpoint. Endpoints marked **JWT only** reject API tokens with `401` regardless of scopes — they are reserved for the session that owns them.

#### Tasks

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/tasks/:id` | `tasks:read` |
| `PATCH /api/v1/tasks/:id` | `tasks:write` |
| `DELETE /api/v1/tasks/:id` | `tasks:write` |
| `GET /api/v1/tasks/:id/subtasks` | `tasks:read` |
| `GET /api/v1/tasks/:id/template-draft` | `tasks:read` |
| `POST /api/v1/tasks/:id/subtasks` | `tasks:write` |
| `POST /api/v1/tasks/:id/duplicate` | `tasks:write` |
| `POST /api/v1/tasks/:id/decompose` | `tasks:write` |
| `POST /api/v1/tasks/:id/complete` | `tasks:write` |
| `POST /api/v1/tasks/:id/uncomplete` | `tasks:write` |
| `POST /api/v1/tasks/:id/cancel` | `tasks:write` |
| `POST /api/v1/tasks/:id/pin` | `tasks:write` |
| `POST /api/v1/tasks/:id/unpin` | `tasks:write` |
| `POST /api/v1/tasks/:id/move` | `tasks:write` |
| `POST /api/v1/tasks/:id/plan` | `tasks:write` |
| `POST /api/v1/tasks/:id/relations` | `tasks:write` |
| `DELETE /api/v1/tasks/:id/relations/:relationId` | `tasks:write` |
| `GET /api/v1/tasks/today` | `tasks:read` |
| `GET /api/v1/tasks/tomorrow` | `tasks:read` |
| `GET /api/v1/tasks/overdue` | `tasks:read` |
| `GET /api/v1/tasks/week` | `tasks:read` |
| `GET /api/v1/tasks/backlog` | `tasks:read` |
| `GET /api/v1/tasks/pinned` | `tasks:read` |
| `GET /api/v1/tasks/completed` | `tasks:read` |
| `GET /api/v1/stats/week-summary` | `tasks:read` |
| `POST /api/v1/tasks/bulk/complete` | `tasks:write` |
| `POST /api/v1/tasks/bulk/move` | `tasks:write` |
| `POST /api/v1/tasks/bulk/priority` | `tasks:write` |
| `POST /api/v1/tasks/group` | `tasks:write` |

#### Inbox

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/inbox` | `tasks:read` |
| `POST /api/v1/inbox/tasks` | `tasks:write` |

#### Projects

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/projects` | `projects:read` |
| `GET /api/v1/projects/:id` | `projects:read` |
| `GET /api/v1/projects/:id/bundle` | `projects:read` + `sections:read` + `tasks:read` |
| `PATCH /api/v1/projects/:id` | `projects:write` |
| `DELETE /api/v1/projects/:id` | `projects:write` |
| `GET /api/v1/projects/:id/tasks` | `tasks:read` |
| `POST /api/v1/projects/:id/tasks` | `tasks:write` |
| `GET /api/v1/projects/:id/sections` | `sections:read` |
| `POST /api/v1/projects/:id/sections` | `sections:write` |
| `POST /api/v1/projects/:id/complete` | `projects:write` |
| `POST /api/v1/projects/:id/uncomplete` | `projects:write` |
| `POST /api/v1/projects/:id/cancel` | `projects:write` |
| `POST /api/v1/projects/:id/archive` | `projects:write` |
| `POST /api/v1/projects/:id/unarchive` | `projects:write` |
| `POST /api/v1/projects/:id/pin` | `projects:write` |
| `POST /api/v1/projects/:id/unpin` | `projects:write` |
| `POST /api/v1/projects/:id/troiki` | `projects:write` |

#### Contexts

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/contexts` | `contexts:read` |
| `GET /api/v1/contexts/:id` | `contexts:read` |
| `POST /api/v1/contexts` | `contexts:write` |
| `PATCH /api/v1/contexts/:id` | `contexts:write` |
| `DELETE /api/v1/contexts/:id` | `contexts:write` |
| `GET /api/v1/contexts/:id/tasks` | `tasks:read` |
| `POST /api/v1/contexts/:id/tasks` | `tasks:write` |
| `GET /api/v1/contexts/:id/projects` | `projects:read` |
| `POST /api/v1/contexts/:id/projects` | `projects:write` |

#### Labels

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/labels` | `labels:read` |
| `GET /api/v1/labels/:id` | `labels:read` |
| `POST /api/v1/labels` | `labels:write` |
| `PATCH /api/v1/labels/:id` | `labels:write` |
| `DELETE /api/v1/labels/:id` | `labels:write` |
| `GET /api/v1/labels/:id/tasks` | `tasks:read` |
| `GET /api/v1/labels/:id/projects` | `projects:read` |

#### Task Templates

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/task-templates` | `templates:read` |
| `GET /api/v1/task-templates/:id` | `templates:read` |
| `POST /api/v1/task-templates` | `templates:write` |
| `PATCH /api/v1/task-templates/:id` | `templates:write` |
| `DELETE /api/v1/task-templates/:id` | `templates:write` |
| `POST /api/v1/task-templates/:id/instantiate` | `tasks:write` |

#### Sections

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/sections/:id` | `sections:read` |
| `PATCH /api/v1/sections/:id` | `sections:write` |
| `DELETE /api/v1/sections/:id` | `sections:write` |
| `POST /api/v1/sections/:id/reorder` | `sections:write` |
| `GET /api/v1/sections/:id/tasks` | `tasks:read` |
| `POST /api/v1/sections/:id/tasks` | `tasks:write` |

#### Troiki

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/troiki` | `troiki:read` |
| `POST /api/v1/troiki/start` | `troiki:write` |
| `POST /api/v1/troiki/reset` | `troiki:write` |

#### Settings / Config / State

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/settings` | `settings:read` |
| `PATCH /api/v1/settings` | `settings:write` |
| `GET /api/v1/config` | `settings:read` **and** `tasks:read`, `projects:read`, `labels:read`, `contexts:read`, `troiki:read`, `templates:read` (see below) |
| `GET /api/v1/state` | `settings:read` |
| `PATCH /api/v1/state` | `settings:write` |
| `GET /api/v1/harpoon` | `settings:read` |
| `POST /api/v1/harpoon/attach` | `settings:write` |
| `POST /api/v1/harpoon/detach` | `settings:write` |

#### Search

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/search` | `search:read` |

#### Calendars

| Endpoint | Scope |
|----------|-------|
| `GET /api/v1/calendars` | `calendars:read` |
| `GET /api/v1/calendars/events` | `calendars:read` |
| `GET /api/v1/calendars/google/start` | `calendars:read` |
| other `/api/v1/calendars` subroutes | `calendars:read` |

#### JWT-only endpoints (no API-token access)

These endpoints reject Bearer API tokens with `401`. They cannot be granted via any scope, including `*`.

| Endpoint | Reason |
|----------|--------|
| `POST /api/v1/api-tokens`, `GET /api/v1/api-tokens`, `DELETE /api/v1/api-tokens/:id` | Token management — must come from a JWT session |
| `GET /api/v1/sessions`, `DELETE /api/v1/sessions/:id` | Session management |
| `POST /auth/totp/setup`, `POST /auth/totp/confirm`, `POST /auth/totp/disable` | 2FA management |
| `GET /api/v1/backup`, `POST /api/v1/restore` | Backup contains the whole database |
| `GET /api/v1/events` (SSE) | The event stream emits across all resources without per-scope filtering — exposing it to tokens would bypass read-scoping |

---

## Sessions

Active sessions for the current user. All endpoints require JWT auth — long-lived API tokens are rejected so a leaked integration token can't drop your browsers.

### `GET /api/v1/sessions`

Returns active (non-revoked, non-expired) sessions ordered by `lastUsedAt` descending. The `isCurrent` flag marks the session that issued the request.

```json
[
  {
    "id": 12,
    "clientKind": "web",
    "userAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
    "displayName": "Chrome on macOS",
    "ipAddress": "203.0.113.10",
    "createdAt": "2026-05-01T10:00:00.000Z",
    "lastUsedAt": "2026-05-24T09:30:00.000Z",
    "isCurrent": true
  }
]
```

`ipAddress` is captured at session creation from Fiber's `c.IP()` and is not refreshed on token rotation — it represents *where the session was opened from*. Empty for sessions created before this field was added.

```sh
curl "$BASE/api/v1/sessions" \
  -H "Authorization: Bearer $JWT"
```

### `DELETE /api/v1/sessions/:id`

Revoke a single session by id. Only sessions owned by the current user can be revoked; unknown ids return `404`. Returns `204 No Content`.

```sh
curl -X DELETE "$BASE/api/v1/sessions/12" \
  -H "Authorization: Bearer $JWT"
```

---

## Tasks

### Task Object

```json
{
  "id": 42,
  "title": "Write tests",
  "description": "",
  "inboxId": null,
  "contextId": 1,
  "projectId": 10,
  "sectionId": null,
  "parentId": null,
  "priority": "high",
  "status": "open",
  "dueAt": "2024-01-20T00:00:00.000Z",
  "dueHasTime": false,
  "deadlineAt": null,
  "deadlineHasTime": false,
  "dayPart": "morning",
  "planState": "week",
  "isPinned": false,
  "pinnedAt": null,
  "isPrivate": false,
  "isComplex": false,
  "completedAt": null,
  "recurrenceRule": null,
  "postponeCount": 0,
  "labels": [{ "id": 3, "name": "bug", "color": "red", "isFavourite": false, "isPrivate": false, "createdAt": "...", "updatedAt": "..." }],
  "blockedByCount": 0,
  "relationCount": 0,
  "url": "https://example.com/task/42",
  "createdAt": "2024-01-15T09:30:00.000Z",
  "updatedAt": "2024-01-15T09:30:00.000Z"
}
```

A task belongs to exactly one placement: `inboxId`, `contextId`, `projectId`, or `sectionId`. `parentId` identifies a subtask relationship.

`blockedByCount` is how many still-open tasks block this one (see [Task Relations](#task-relations)); a non-zero value means completion is refused with `task_blocked`. `relationCount` is every relation touching the task, both directions and both types. Both are present on **every** task-returning endpoint — the single get, all list and view endpoints, and `GET /api/v1/config`'s `pinnedTasks` — so a client never has to ask separately whether a task is blocked.

`relations` is present only where noted below (`GET /api/v1/tasks/:id?relations=true` and the two relation mutations).

### `GET /api/v1/tasks/:id`

```sh
curl "$BASE/api/v1/tasks/42" \
  -H "Authorization: Bearer $TOKEN"
```

Pass `?subtasks=true` to receive the children inline under a `subtasks` paged envelope — the task detail page uses this so it can render in one round-trip instead of two.

Pass `?relations=true` to receive the task's relations inline under `relations` (see [Task Relations](#task-relations)). Both flags combine, so the detail page fetches the task, its subtree and its relation graph in a single request. Relations are opt-in because every list view shares this repository path and would otherwise pay for a join nobody reads there.

```sh
curl "$BASE/api/v1/tasks/42?subtasks=true&relations=true" \
  -H "Authorization: Bearer $TOKEN"
```

### `PATCH /api/v1/tasks/:id`

All fields are optional. Omit a field to leave it unchanged.

```json
{
  "title": "Updated title",
  "description": "Some notes",
  "priority": "high",
  "dueAt": "2024-01-20T00:00:00.000Z",
  "dueHasTime": false,
  "deadlineAt": "2024-02-01T00:00:00.000Z",
  "deadlineHasTime": false,
  "dayPart": "morning",
  "planState": "week",
  "recurrenceRule": "RRULE:FREQ=DAILY",
  "labels": ["bug", "urgent"],
  "removedAutoLabels": ["auto-label-name"],
  "isPrivate": false,
  "isComplex": false
}
```

Pass `null` for `dueAt`, `deadlineAt`, or `recurrenceRule` to clear the value.

> Tasks in a Troiki-bound project have their `priority` managed automatically — direct priority edits are rejected.

```sh
# Update title and priority
curl -X PATCH "$BASE/api/v1/tasks/42" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Updated title","priority":"high"}'

# Clear due date
curl -X PATCH "$BASE/api/v1/tasks/42" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dueAt":null}'

# Set recurrence rule
curl -X PATCH "$BASE/api/v1/tasks/42" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"recurrenceRule":"RRULE:FREQ=WEEKLY;BYDAY=MO"}'
```

### `DELETE /api/v1/tasks/:id`

Returns `204 No Content`.

```sh
curl -X DELETE "$BASE/api/v1/tasks/42" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/tasks/:id/subtasks`

Returns a paged list of subtasks.

```sh
curl "$BASE/api/v1/tasks/42/subtasks" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/tasks/:id/template-draft`

Builds an **unsaved** [task template](#task-templates) draft from the task and its
whole subtree. Deeper nesting is flattened into a single subtask level
(depth-first pre-order), and each captured task carries its hydrated labels. The
root task's title becomes the template `name`. IDs, position and timestamps are
zero/empty — the response is meant to prefill the template editor; nothing is
persisted. Save it with `POST /api/v1/task-templates`.

**Response:** [TaskTemplate](#task-templates) shape (with `id: 0`).

```sh
curl "$BASE/api/v1/tasks/42/template-draft" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/tasks/:id/subtasks`

Create a subtask. Inherits the parent's labels if `labels` is omitted. Cannot create subtasks in the inbox.

**Request:** [CreateTaskRequest](#createtaskrequest)

```sh
curl -X POST "$BASE/api/v1/tasks/42/subtasks" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Subtask one","priority":"medium"}'
```

### `POST /api/v1/tasks/:id/duplicate`

Creates a copy of the task with title suffixed `(2)`. Subtasks are cloned recursively under the new task (keeping their original titles). Returns `201` with the new task.

```sh
curl -X POST "$BASE/api/v1/tasks/42/duplicate" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/tasks/:id/decompose`

Replaces the task with N sibling tasks from supplied titles. The original task is deleted. New tasks inherit placement, priority, due/deadline, labels, description, day part, plan state, recurrence, and privacy.

**Request:**
```json
{ "titles": ["Subtask A", "Subtask B", "Subtask C"] }
```

**Response** `201`:
```json
{ "created": [TaskObject, ...] }
```

```sh
curl -X POST "$BASE/api/v1/tasks/42/decompose" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"titles":["Design","Implement","Test"]}'
```

---

## Task Actions

### `POST /api/v1/tasks/:id/complete`

Mark a task complete. Optional body to specify exact completion time (useful for recording overdue completions):

```json
{ "completedAt": "2024-01-15T08:00:00.000Z" }
```

Returns the updated task. If the task has a recurrence rule, a new task is scheduled and returned.

Fails with `409 task_blocked` when the task has at least one **open** task blocking it (see [Task Relations](#task-relations)). The blocker ids travel in `details.blockerIds`:

```json
{
  "error": {
    "code": "task_blocked",
    "message": "task is blocked by an incomplete task",
    "details": { "blockerIds": [7, 9] }
  }
}
```

Only `open` blockers count — a `completed` or `cancelled` blocker releases its dependents, so cancelling a task never deadlocks whatever it was holding back. Re-completing an already-completed task skips the check (that retry path exists to recover a lost Troiki capacity grant), and `uncomplete` / `cancel` are never blocked.

```sh
# Complete now
curl -X POST "$BASE/api/v1/tasks/42/complete" \
  -H "Authorization: Bearer $TOKEN"

# Complete with explicit timestamp
curl -X POST "$BASE/api/v1/tasks/42/complete" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"completedAt":"2024-01-15T08:00:00.000Z"}'
```

### `POST /api/v1/tasks/:id/uncomplete`

Reverts a completed task to open.

```sh
curl -X POST "$BASE/api/v1/tasks/42/uncomplete" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/tasks/:id/cancel`

Marks a task as cancelled.

```sh
curl -X POST "$BASE/api/v1/tasks/42/cancel" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/tasks/:id/pin`

Pins the task. Fails with `CodeLimitExceeded` if the max-pinned limit is reached.

```sh
curl -X POST "$BASE/api/v1/tasks/42/pin" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/tasks/:id/unpin`

```sh
curl -X POST "$BASE/api/v1/tasks/42/unpin" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/tasks/:id/move`

Move a task to a different placement. Exactly one of `inboxId`, `contextId`, `projectId`, or `sectionId` must be non-null (or `parentId` for subtask placement). Fails with `CodeForbiddenPlacement` for invalid placement or cycles.

```json
{
  "inboxId": null,
  "contextId": 1,
  "projectId": 10,
  "sectionId": null,
  "parentId": null
}
```

```sh
# Move to a project
curl -X POST "$BASE/api/v1/tasks/42/move" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"contextId":1,"projectId":10}'

# Move back to inbox
curl -X POST "$BASE/api/v1/tasks/42/move" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"inboxId":1}'

# Make a subtask of another task
curl -X POST "$BASE/api/v1/tasks/42/move" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"contextId":1,"projectId":10,"parentId":7}'
```

### `POST /api/v1/tasks/:id/plan`

Set the plan state. `state` is one of `none`, `week`, `backlog`. Fails with `CodeLimitExceeded` if the plan limit is exceeded.

```json
{ "state": "week" }
```

```sh
# Add to weekly plan
curl -X POST "$BASE/api/v1/tasks/42/plan" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"state":"week"}'

# Remove from plan
curl -X POST "$BASE/api/v1/tasks/42/plan" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"state":"none"}'
```

---

## Task Relations

Directed links between two tasks. Two types:

| Type | Meaning |
|------|---------|
| `related` | Symmetric, purely informational. Adding it from either side produces the same single relation (the pair is normalised), so a duplicate is rejected with `409 conflict`. |
| `blocks` | Directed and **enforced**: the blocked task cannot be completed while the blocking task is still `open`. |

`direction` is interpreted **relative to the task in the path** and only matters for `blocks`:

- `incoming` — the other task blocks this one ("blocked by"),
- `outgoing` — this task blocks the other one ("blocks").

Both endpoints below answer with the **updated task**, with `relations` hydrated. There is deliberately no `GET` for relations: they ride inside `GET /api/v1/tasks/:id?relations=true`, and the mutations return the task, so a client never needs a follow-up read.

### Relation Object

```json
{
  "id": 3,
  "type": "blocks",
  "direction": "incoming",
  "createdAt": "2026-07-20T10:00:00.000Z",
  "task": { "id": 7, "title": "Ship the migration", "status": "open", "...": "full Task object" }
}
```

`task` is the **peer** end — the task at the other side of the relation, not the one in the path. The same stored relation therefore serialises as `incoming` on one of its tasks and `outgoing` on the other.

### `POST /api/v1/tasks/:id/relations`

```json
{ "targetTaskId": 7, "type": "blocks", "direction": "incoming" }
```

`direction` defaults to `outgoing` and is ignored for `type: "related"`.

Returns `200` with the updated task (`relations` hydrated). Errors:

| Condition | Response |
|-----------|----------|
| Either task does not exist | `404 not_found` |
| `targetTaskId` equals the path id | `400 validation_failed` |
| Unknown `type` or `direction` | `400 validation_failed` |
| The relation already exists | `409 conflict` |
| A `blocks` relation would close a loop in the blocking graph | `400 validation_failed` |

The cycle check matters: `A blocks B blocks A` would leave both tasks permanently uncompletable. Only `blocks` edges participate — a `related` link cannot deadlock anything.

```sh
# "Task 42 is blocked by task 7"
curl -X POST "$BASE/api/v1/tasks/42/relations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"targetTaskId":7,"type":"blocks","direction":"incoming"}'

# "Task 42 blocks task 9"
curl -X POST "$BASE/api/v1/tasks/42/relations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"targetTaskId":9,"type":"blocks","direction":"outgoing"}'

# "Task 42 is related to task 11"
curl -X POST "$BASE/api/v1/tasks/42/relations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"targetTaskId":11,"type":"related"}'
```

### `DELETE /api/v1/tasks/:id/relations/:relationId`

Removes a relation. Works from **either** of its two tasks; a relation that does not touch `:id` returns `404 not_found` rather than silently succeeding. Returns `200` with the updated task.

```sh
curl -X DELETE "$BASE/api/v1/tasks/42/relations/3" \
  -H "Authorization: Bearer $TOKEN"
```

### Notes

- Creating or removing a relation bumps `updatedAt` on **both** tasks, so a client watching either one sees the change.
- Tasks are hard-deleted and relations cascade with them — deleting a blocker releases everything it was blocking.
- `duplicate`, `decompose` and recurrence snapshots do **not** copy relations; a cloned dependency graph would be ambiguous.
- Relations are included in backup export/restore, with their ids preserved.
- Relation mutations publish the `tasks` SSE scope like any other task write.

---

## Task Views

All view endpoints support optional query filters:
- `contextId` — filter by context
- `projectId` — filter by project
- `labelId` — filter by label
- `priority` — filter by priority (`high`, `medium`, `low`, `no-priority`)

Views returning paginated results also accept `limit` and `offset`.

### `GET /api/v1/tasks/today`

Tasks due today. Returns paged response.

```sh
curl "$BASE/api/v1/tasks/today" \
  -H "Authorization: Bearer $TOKEN"

# With filters
curl "$BASE/api/v1/tasks/today?contextId=1&priority=high" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/tasks/tomorrow`

Tasks due tomorrow. Returns paged response.

```sh
curl "$BASE/api/v1/tasks/tomorrow" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/tasks/overdue`

Tasks past due. Returns paged response.

```sh
curl "$BASE/api/v1/tasks/overdue" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/tasks/week`

Tasks planned for the week.

```json
{ "items": [...], "total": 12 }
```

```sh
curl "$BASE/api/v1/tasks/week" \
  -H "Authorization: Bearer $TOKEN"

# Filtered by label
curl "$BASE/api/v1/tasks/week?labelId=3" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/tasks/backlog`

Tasks in the backlog.

```json
{ "items": [...], "total": 5 }
```

```sh
curl "$BASE/api/v1/tasks/backlog" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/tasks/pinned`

```json
{ "items": [...], "total": 3 }
```

```sh
curl "$BASE/api/v1/tasks/pinned" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/tasks/completed`

Tasks completed within a date window. Query params:
- `days` — number of days back (1–90, default 1). Today is always included.

Returns paged response.

```sh
# Today only (default)
curl "$BASE/api/v1/tasks/completed" \
  -H "Authorization: Bearer $TOKEN"

# Last 7 days
curl "$BASE/api/v1/tasks/completed?days=7" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/stats/sidebar`

Bundles every aggregate the app shell shows in the sidebar — plan counters, the
inbox badge and the pinned list — in one round-trip. A client that just mutated
its own data refetches this once (its own SSE echo is suppressed — see
_Real-time invalidation_ in [docs/architecture/backend.md](docs/architecture/backend.md))
instead of issuing separate per-aggregate requests. The plan counters it carries
are also embedded in `GET /api/v1/config`; there is no standalone plan-counter
endpoint (see [Breaking changes](#breaking-changes)).

```json
{
  "planStats": { "week": 8, "backlog": 14 },
  "inboxStats": { "count": 3, "warnThresholdExceeded": false },
  "pinned": { "items": [], "total": 0 }
}
```

```sh
curl "$BASE/api/v1/stats/sidebar" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/stats/week-summary`

Backs the weekly summary review page. Returns the current-week range (Mon..next
Mon, configured timezone, as UTC instants), headline counters, and the full list
of tasks completed in that range. The list includes subtasks and
recurrence-completion snapshots (every row marked completed in range); the
client derives the by-priority / by-project / by-context breakdowns from it.
`stats.completedCount` is the authoritative total, `plannedOpen` counts open
tasks still on the week board, `overdue` counts open tasks past their due date.

`troiki` is present only when the Troiki system is enabled (else `null`). It
leads the page because the methodology takes priority over plain projects/tasks.
`slots` is ordered important → medium → rest; each slot reports its `capacity`,
the number of `projects` assigned, the count of `open` tasks remaining in those
projects, and how many of its tasks were `completed` during the week.

```json
{
  "range": { "start": "2026-06-29T00:00:00.000Z", "end": "2026-07-06T00:00:00.000Z" },
  "stats": { "completedCount": 12, "plannedOpen": 5, "overdue": 2 },
  "completed": [],
  "troiki": {
    "started": true,
    "slots": [
      { "category": "important", "capacity": 3, "projects": 2, "open": 4, "completed": 7 },
      { "category": "medium", "capacity": 5, "projects": 3, "open": 9, "completed": 3 },
      { "category": "rest", "capacity": 2, "projects": 1, "open": 6, "completed": 2 }
    ]
  }
}
```

```sh
curl "$BASE/api/v1/stats/week-summary" \
  -H "Authorization: Bearer $TOKEN"
```

---

## Task Bulk Operations

### `POST /api/v1/tasks/bulk/complete`

Complete up to 100 tasks. Partial failures are reported per-item.

**Request:**
```json
{ "ids": [1, 2, 3] }
```

**Response:**
```json
{
  "succeeded": [1, 2],
  "failed": [{ "id": 3, "error": { "code": "CodeNotFound", "message": "task not found" } }]
}
```

```sh
curl -X POST "$BASE/api/v1/tasks/bulk/complete" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3]}'
```

### `POST /api/v1/tasks/bulk/move`

Move up to 100 tasks to the same target placement.

**Request:**
```json
{
  "ids": [1, 2, 3],
  "contextId": 1,
  "projectId": 10,
  "sectionId": null,
  "inboxId": null,
  "parentId": null
}
```

**Response:** same bulk envelope as bulk/complete.

```sh
# Move several tasks to a project
curl -X POST "$BASE/api/v1/tasks/bulk/move" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3],"contextId":1,"projectId":10}'

# Move several tasks to inbox
curl -X POST "$BASE/api/v1/tasks/bulk/move" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3],"inboxId":1}'
```

### `POST /api/v1/tasks/bulk/priority`

Set the priority of up to 100 tasks. `priority` is one of `high`, `medium`,
`low`, `no-priority`.

**Request:**
```json
{
  "ids": [1, 2, 3],
  "priority": "high"
}
```

**Response:** same bulk envelope as bulk/complete. A task in a Troiki-categorised
project has its priority pinned by the category; setting a mismatching priority
fails that id with a `validation_failed` error while the rest succeed.

```sh
curl -X POST "$BASE/api/v1/tasks/bulk/priority" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3],"priority":"high"}'
```

### `POST /api/v1/tasks/group`

Create a parent task and reparent a set of child tasks under it. Children inherit the parent's labels and priority.

**Request:**
```json
{
  "title": "Epic",
  "description": "",
  "priority": "high",
  "contextId": 1,
  "projectId": 10,
  "sectionId": null,
  "labels": ["bug"],
  "childIds": [5, 6, 7]
}
```

**Response** `201`:
```json
{
  "parent": TaskObject,
  "succeeded": [5, 6],
  "failed": [{ "id": 7, "error": { "code": "...", "message": "..." } }]
}
```

```sh
curl -X POST "$BASE/api/v1/tasks/group" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Epic","priority":"high","contextId":1,"projectId":10,"labels":["bug"],"childIds":[5,6,7]}'
```

---

## Contexts

### Context Object

```json
{
  "id": 1,
  "name": "Work",
  "color": "blue",
  "isFavourite": false,
  "createdAt": "...",
  "updatedAt": "..."
}
```

### `GET /api/v1/contexts`

List contexts. Supports `limit`, `offset`.

```sh
curl "$BASE/api/v1/contexts" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/contexts`

```json
{ "name": "Work", "color": "blue", "isFavourite": false }
```

Returns `201`.

```sh
curl -X POST "$BASE/api/v1/contexts" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Work","color":"blue","isFavourite":false}'
```

### `GET /api/v1/contexts/:id`

```sh
curl "$BASE/api/v1/contexts/1" \
  -H "Authorization: Bearer $TOKEN"
```

### `PATCH /api/v1/contexts/:id`

```json
{ "name": "Personal", "color": "#FF5733", "isFavourite": true }
```

```sh
curl -X PATCH "$BASE/api/v1/contexts/1" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Personal","color":"#FF5733","isFavourite":true}'
```

### `DELETE /api/v1/contexts/:id`

Returns `204`.

```sh
curl -X DELETE "$BASE/api/v1/contexts/1" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/contexts/:id/projects`

List projects in a context. Supports `limit`, `offset`, `status` filter.

```sh
curl "$BASE/api/v1/contexts/1/projects" \
  -H "Authorization: Bearer $TOKEN"

# Only open projects
curl "$BASE/api/v1/contexts/1/projects?status=open" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/contexts/:id/tasks`

List tasks in a context. Supports `limit`, `offset`, `status`, `priority`, `q` (search), `labelId`.

```sh
curl "$BASE/api/v1/contexts/1/tasks" \
  -H "Authorization: Bearer $TOKEN"

# Search with filters
curl "$BASE/api/v1/contexts/1/tasks?q=meeting&priority=high&limit=20" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/contexts/:id/tasks`

Create a task in a context. **Request:** [CreateTaskRequest](#createtaskrequest). Returns `201`.

```sh
curl -X POST "$BASE/api/v1/contexts/1/tasks" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"New task","priority":"medium"}'
```

---

## Projects

### Project Object

```json
{
  "id": 10,
  "contextId": 1,
  "title": "Website Redesign",
  "description": "",
  "color": "green",
  "status": "open",
  "projectType": "generic",
  "isPinned": false,
  "pinnedAt": null,
  "isPrivate": false,
  "troikiCategory": null,
  "labels": [],
  "createdAt": "...",
  "updatedAt": "..."
}
```

### `GET /api/v1/projects`

List projects. Query params: `contextId`, `status`, `limit`, `offset`.

```sh
curl "$BASE/api/v1/projects" \
  -H "Authorization: Bearer $TOKEN"

# Filter by context and status
curl "$BASE/api/v1/projects?contextId=1&status=open" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/projects/:id`

```sh
curl "$BASE/api/v1/projects/10" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/projects/:id/bundle`

Single round-trip aggregate for the project page. Returns the project, its
sections and all its tasks (subtasks included, flattened — re-parent client-side
by `parentId`) instead of three separate `GET` calls. Because it exposes data
across three domains, an API token must hold **all** of `projects:read`,
`sections:read` and `tasks:read` (JWT sessions are unrestricted).

```sh
curl "$BASE/api/v1/projects/10/bundle" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "project": { "id": 10, "title": "...", "...": "..." },
  "sections": { "items": [ { "id": 3, "title": "Backlog", "...": "..." } ], "total": 1, "limit": 200, "offset": 0 },
  "tasks": { "items": [ { "id": 42, "title": "...", "parentId": null, "...": "..." } ], "total": 12, "limit": 500, "offset": 0 }
}
```

### `PATCH /api/v1/projects/:id`

```json
{
  "title": "New Title",
  "description": "...",
  "color": "blue",
  "labels": ["urgent"],
  "isPrivate": false,
  "projectType": "software"
}
```

```sh
curl -X PATCH "$BASE/api/v1/projects/10" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"New Title","color":"blue","labels":["urgent"]}'
```

### `DELETE /api/v1/projects/:id`

Returns `204`.

```sh
curl -X DELETE "$BASE/api/v1/projects/10" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/contexts/:id/projects`

Create a project in a context.

```json
{
  "title": "My Project",
  "description": "",
  "color": "blue",
  "labels": [],
  "projectType": "generic"
}
```

Returns `201`.

```sh
curl -X POST "$BASE/api/v1/contexts/1/projects" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"My Project","color":"blue","projectType":"generic"}'
```

### `GET /api/v1/projects/:id/sections`

List sections. Supports `limit`, `offset`.

```sh
curl "$BASE/api/v1/projects/10/sections" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/projects/:id/sections`

```json
{ "title": "Phase 1" }
```

Returns `201`.

```sh
curl -X POST "$BASE/api/v1/projects/10/sections" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Phase 1"}'
```

### `GET /api/v1/projects/:id/tasks`

List tasks in a project. Query params: `status`, `priority`, `labelId`, `limit`, `offset`.

```sh
curl "$BASE/api/v1/projects/10/tasks" \
  -H "Authorization: Bearer $TOKEN"

# Open high-priority tasks only
curl "$BASE/api/v1/projects/10/tasks?status=open&priority=high" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/projects/:id/tasks`

Create a task in a project. **Request:** [CreateTaskRequest](#createtaskrequest). Returns `201`.

```sh
curl -X POST "$BASE/api/v1/projects/10/tasks" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Implement login","priority":"high","planState":"week"}'
```

### `POST /api/v1/projects/:id/complete`
### `POST /api/v1/projects/:id/uncomplete`
### `POST /api/v1/projects/:id/cancel`
### `POST /api/v1/projects/:id/archive`
### `POST /api/v1/projects/:id/unarchive`

Status transition endpoints. Return the updated project.

```sh
curl -X POST "$BASE/api/v1/projects/10/complete" \
  -H "Authorization: Bearer $TOKEN"

curl -X POST "$BASE/api/v1/projects/10/uncomplete" \
  -H "Authorization: Bearer $TOKEN"

curl -X POST "$BASE/api/v1/projects/10/cancel" \
  -H "Authorization: Bearer $TOKEN"

curl -X POST "$BASE/api/v1/projects/10/archive" \
  -H "Authorization: Bearer $TOKEN"

curl -X POST "$BASE/api/v1/projects/10/unarchive" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/projects/:id/pin`

Only open projects can be pinned. Fails with `CodeLimitExceeded` if max is reached.

```sh
curl -X POST "$BASE/api/v1/projects/10/pin" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/projects/:id/unpin`

```sh
curl -X POST "$BASE/api/v1/projects/10/unpin" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/projects/:id/troiki`

Assign or clear a Troiki category for a project. Set `category` to `null` to clear. Returns the updated project.

```json
{ "category": "important" }
```

```sh
# Assign category
curl -X POST "$BASE/api/v1/projects/10/troiki" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"category":"important"}'

# Clear category
curl -X POST "$BASE/api/v1/projects/10/troiki" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"category":null}'
```

---

## Sections

### Section Object

```json
{
  "id": 5,
  "projectId": 10,
  "title": "Phase 1",
  "position": 0,
  "createdAt": "...",
  "updatedAt": "..."
}
```

### `GET /api/v1/sections/:id`

```sh
curl "$BASE/api/v1/sections/5" \
  -H "Authorization: Bearer $TOKEN"
```

### `PATCH /api/v1/sections/:id`

```json
{ "title": "New Title" }
```

```sh
curl -X PATCH "$BASE/api/v1/sections/5" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Phase 2"}'
```

### `DELETE /api/v1/sections/:id`

Returns `204`.

```sh
curl -X DELETE "$BASE/api/v1/sections/5" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/sections/:id/tasks`

List tasks in a section. Supports `limit`, `offset`.

```sh
curl "$BASE/api/v1/sections/5/tasks" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/sections/:id/tasks`

Create a task in a section. **Request:** [CreateTaskRequest](#createtaskrequest). Returns `201`.

```sh
curl -X POST "$BASE/api/v1/sections/5/tasks" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Task in section","priority":"low"}'
```

### `POST /api/v1/sections/:id/reorder`

Change a section's position within its project.

```json
{ "position": 2 }
```

Returns the updated section.

```sh
curl -X POST "$BASE/api/v1/sections/5/reorder" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"position":2}'
```

---

## Labels

### Label Object

```json
{
  "id": 3,
  "name": "bug",
  "color": "red",
  "isFavourite": false,
  "isPrivate": false,
  "createdAt": "...",
  "updatedAt": "..."
}
```

### `GET /api/v1/labels`

List labels. Query params: `q` (name filter), `limit`, `offset`.

```sh
curl "$BASE/api/v1/labels" \
  -H "Authorization: Bearer $TOKEN"

# Search by name
curl "$BASE/api/v1/labels?q=bug" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/labels`

```json
{ "name": "urgent", "color": "red", "isFavourite": false }
```

Returns `201`.

```sh
curl -X POST "$BASE/api/v1/labels" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"urgent","color":"red","isFavourite":false}'
```

### `GET /api/v1/labels/:id`

```sh
curl "$BASE/api/v1/labels/3" \
  -H "Authorization: Bearer $TOKEN"
```

### `PATCH /api/v1/labels/:id`

```json
{ "name": "critical", "color": "#FF0000", "isFavourite": true, "isPrivate": false }
```

```sh
curl -X PATCH "$BASE/api/v1/labels/3" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"critical","color":"#FF0000","isFavourite":true}'
```

### `DELETE /api/v1/labels/:id`

Returns `204`.

```sh
curl -X DELETE "$BASE/api/v1/labels/3" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/labels/:id/tasks`

Tasks with this label. Supports `limit`, `offset`.

```sh
curl "$BASE/api/v1/labels/3/tasks" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/labels/:id/projects`

Projects with this label. Supports `limit`, `offset`.

```sh
curl "$BASE/api/v1/labels/3/projects" \
  -H "Authorization: Bearer $TOKEN"
```

---

## Task Templates

Reusable blueprints — a root task plus an ordered set of subtasks — that can be
materialized into any project. Each template row (root and subtask) captures
`title`/`description`/`priority`/`dayPart` plus a set of labels. Templates are
single-user local configuration: they are not federated and are hard-deleted.

A template can also be seeded from an existing task with
[`GET /api/v1/tasks/:id/template-draft`](#get-apiv1tasksidtemplate-draft), which
returns an unsaved draft (the task plus its flattened subtree) to prefill the
editor before saving via `POST`.

### Template Object

```json
{
  "id": 1,
  "name": "Onboard client",
  "description": "Kick off a new client engagement",
  "priority": "high",
  "dayPart": "morning",
  "position": 0,
  "labels": [LabelObject, ...],
  "subtasks": [
    {
      "id": 10,
      "title": "Schedule kickoff call",
      "description": "",
      "priority": "medium",
      "dayPart": "none",
      "labels": [LabelObject, ...]
    }
  ],
  "createdAt": "2026-06-21T10:00:00.000Z",
  "updatedAt": "2026-06-21T10:00:00.000Z"
}
```

### `GET /api/v1/task-templates`

Lists all templates (paged envelope; returns every template). Ordered by
`position`, then `name`.

```sh
curl "$BASE/api/v1/task-templates" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/task-templates`

Create a template. `name` is required and is used as the root task's title.
`subtasks` is optional; each subtask requires a non-empty `title`. `priority`
defaults to `no-priority`, `dayPart` to `none`. Returns `201`.

```sh
curl -X POST "$BASE/api/v1/task-templates" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Onboard client",
        "description": "",
        "priority": "high",
        "dayPart": "morning",
        "labelIds": [3],
        "subtasks": [
          {"title": "Schedule kickoff call", "priority": "medium", "labelIds": [3]},
          {"title": "Send contract"}
        ]
      }'
```

### `GET /api/v1/task-templates/:id`

Single template with subtasks and labels.

### `PATCH /api/v1/task-templates/:id`

Full replace: the body has the same shape as `POST` and rewrites the template's
fields, labels and subtasks wholesale (there is no granular subtask API).

### `DELETE /api/v1/task-templates/:id`

Hard-deletes the template; subtasks and label links cascade. Returns `204`.

### `POST /api/v1/task-templates/:id/instantiate`

Materialize the template into a project: creates the root task and each subtask
under it (auto-labels and Troiki priority coercion apply, as for normal task
creation). **Request:** `{"projectId": 5}`. Returns `201` with the created root
task and subtasks.

```sh
curl -X POST "$BASE/api/v1/task-templates/1/instantiate" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"projectId": 5}'
```

```json
{
  "root": TaskObject,
  "subtasks": [TaskObject, ...]
}
```

---

## Inbox

### `GET /api/v1/inbox`

```json
{
  "count": 5,
  "warnThresholdExceeded": false,
  "tasks": [TaskObject, ...]
}
```

Returns up to 200 tasks. `warnThresholdExceeded` indicates the inbox is getting full (threshold configured server-side).

```sh
curl "$BASE/api/v1/inbox" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/inbox/tasks`

Create a task in the inbox. **Request:** [CreateTaskRequest](#createtaskrequest). Returns `201`.

```sh
curl -X POST "$BASE/api/v1/inbox/tasks" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Buy milk"}'
```

---

## Calendars

Read-only Google Calendar integration. Events are fetched live from Google and
cached in-memory server-side (default 10 min, `CALENDAR_CACHE_TTL`); they are
never stored in the database. See [docs/google-calendar.md](docs/google-calendar.md).

### `GET /api/v1/calendars`

Integration status, connected accounts and calendar sources.

```json
{
  "enabled": true,
  "googleConfigured": true,
  "googleClientIdConfigured": true,
  "googleClientSecretConfigured": true,
  "accounts": [],
  "sources": []
}
```

### `GET /api/v1/calendars/events`

Events in a time range. Both `start` and `end` are **required** (ISO-8601 UTC),
`end` must be after `start`, and the range may not exceed **92 days**. Returns an
empty list when the integration is disabled or unconfigured. When the stored
Google credentials have expired the call fails with `calendar_reauth_required`.

```sh
curl "$BASE/api/v1/calendars/events?start=2026-07-01T00:00:00.000Z&end=2026-07-08T00:00:00.000Z" \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/calendars/google/start`

Returns `{ "url": "..." }`, the Google OAuth authorize URL. **Has a side effect:**
it persists an OAuth state row.

### `GET /api/v1/calendars/google/callback`

Public OAuth redirect target. Accepts `code`, `state`, `error`; responds with a
302 back to the settings page. Not called directly by clients.

### Mutations *(JWT only)*

`PATCH /api/v1/calendars/settings`, `PATCH|DELETE /api/v1/calendars/google/config`,
`POST /api/v1/calendars/google/sync`, `PATCH /api/v1/calendars/sources/:id`,
`DELETE /api/v1/calendars/accounts/:id`.

---

## Search

### `GET /api/v1/search`

Full-text search across tasks and/or projects.

Query params:
- `q` — search query (minimum 2 characters, required)
- `type` — `tasks`, `projects`, or `all` (default: `all`)
- `limit`, `offset`

**Response:**
```json
{
  "tasks": { "items": [...], "total": 3, "limit": 50, "offset": 0 },
  "projects": { "items": [...], "total": 1, "limit": 50, "offset": 0 }
}
```

Only the keys for requested types are included in the response.

```sh
# Search everything
curl "$BASE/api/v1/search?q=redesign" \
  -H "Authorization: Bearer $TOKEN"

# Tasks only
curl "$BASE/api/v1/search?q=redesign&type=tasks" \
  -H "Authorization: Bearer $TOKEN"

# Projects only, paginated
curl "$BASE/api/v1/search?q=redesign&type=projects&limit=10&offset=0" \
  -H "Authorization: Bearer $TOKEN"
```

---

## Troiki

Troiki is a 3-slot priority system (important / medium / rest) for daily project planning.

### `GET /api/v1/troiki`

Current Troiki view.

```json
{
  "important": {
    "capacity": 1,
    "projects": [{ "id": 10, "title": "...", "tasks": [...], ... }]
  },
  "medium": { "capacity": 2, "projects": [] },
  "rest": { "capacity": 3, "projects": [] },
  "started": true
}
```

```sh
curl "$BASE/api/v1/troiki" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/troiki/start`

Start a new Troiki day. Returns the updated view.

```sh
curl -X POST "$BASE/api/v1/troiki/start" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/troiki/reset`

Reset the current Troiki session. Returns the updated view.

```sh
curl -X POST "$BASE/api/v1/troiki/reset" \
  -H "Authorization: Bearer $TOKEN"
```

---

## Configuration

### `GET /api/v1/config`

**The workspace bootstrap aggregate.** Beyond the static server configuration it
embeds the caller's contexts, projects, labels, settings, app settings, UI state,
the Troiki view, the sidebar counters, the pinned tasks, the harpoon pair and the
task templates — everything the SPA needs to render, in one round-trip. The SPA
also refetches it as its single steady-state refresh (see
[Conditional requests](#conditional-requests) below).

**Scopes.** Because the payload spans every read domain, an API token must hold
**all** of `settings:read`, `tasks:read`, `projects:read`, `labels:read`,
`contexts:read`, `troiki:read` and `templates:read`. A `*` token qualifies. JWT
sessions bypass scope checks entirely.

`contexts`, `projects` and `labels` are capped at **500** rows each. Note that
`taskTemplates` is a **bare array**, not the paged envelope
`GET /api/v1/task-templates` returns.

```json
{
  "timezone": "Europe/Moscow",
  "maxPinned": 5,
  "weekly": { "limit": 20 },
  "backlog": { "limit": 50 },
  "inbox": {
    "warnThreshold": 10,
    "overflowTask": { "title": "Clear inbox", "priority": "high" }
  },
  "dayParts": {
    "morning": { "start": 6, "end": 12 },
    "afternoon": { "start": 12, "end": 18 },
    "evening": { "start": 18, "end": 23 }
  },
  "totpAvailable": true,

  "contexts": [ { "id": 1, "name": "Work", "...": "..." } ],
  "projects": [ { "id": 2, "title": "Inbox", "...": "..." } ],
  "labels":   [ { "id": 3, "name": "bug", "...": "..." } ],
  "settings":    { "locale": "en", "...": "..." },
  "appSettings": { "autoLabels": [], "projectSuggestions": [] },
  "userState":   { "activeContextId": null },
  "troiki":      { "important": { "capacity": 3, "projects": [] },
                   "medium": { "capacity": 3, "projects": [] },
                   "rest": { "capacity": 3, "projects": [] },
                   "started": false },
  "planStats":  { "week": 4, "backlog": 11 },
  "inboxStats": { "count": 2, "warnThresholdExceeded": false },
  "pinnedTasks": [],
  "harpoon": { "slots": [] },
  "taskTemplates": []
}
```

```sh
curl "$BASE/api/v1/config" \
  -H "Authorization: Bearer $TOKEN"
```

#### Conditional requests

The response carries a strong `ETag` over the body bytes. Send it back as
`If-None-Match` and an unchanged workspace answers `304 Not Modified` with an
empty body — useful for clients that poll or that refetch on reconnect.

```sh
curl -i "$BASE/api/v1/config" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'If-None-Match: "6f1c..."'
```

The ETag is derived from the serialized payload, so it changes whenever any
embedded section does.

---

## State

Per-user key-value blob for storing UI state. Values can be any JSON type.

### `GET /api/v1/state`

Returns the stored JSON object (empty `{}` if nothing stored).

```sh
curl "$BASE/api/v1/state" \
  -H "Authorization: Bearer $TOKEN"
```

### `PATCH /api/v1/state`

Shallow-merge a JSON object into the stored state. Keys with `null` values are removed. Max payload: 64 KiB.

```json
{ "sidebarOpen": true, "selectedContextId": 1, "oldKey": null }
```

Returns the full merged state.

```sh
# Set keys
curl -X PATCH "$BASE/api/v1/state" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"sidebarOpen":true,"selectedContextId":1}'

# Remove a key by setting it to null
curl -X PATCH "$BASE/api/v1/state" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"oldKey":null}'
```

---

## Settings

### `GET /api/v1/settings`

```json
{
  "weeklyUnplannedExcludedLabelIds": [],
  "bugLabelIds": [],
  "locale": "en",
  "publicView": false,
  "bannerText": "",
  "bannerPublished": false,
  "bannerDayPart": ""
}
```

```sh
curl "$BASE/api/v1/settings" \
  -H "Authorization: Bearer $TOKEN"
```

### `PATCH /api/v1/settings`

All fields optional. `locale` must be `"en"`, `"ru"`, or `""` (client decides).

`bannerDayPart` scopes the Today banner to a single day phase — `"morning"`,
`"afternoon"`, `"evening"`, or `""` for all day (the default). When a phase is
set, the banner is shown only while that phase is active: it stays hidden until
the phase begins and disappears once it is over. `"none"` is rejected.

```json
{
  "weeklyUnplannedExcludedLabelIds": [3, 7],
  "bugLabelIds": [3],
  "locale": "ru",
  "publicView": false,
  "bannerText": "Under maintenance",
  "bannerPublished": true,
  "bannerDayPart": "morning"
}
```

```sh
# Change locale
curl -X PATCH "$BASE/api/v1/settings" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"locale":"ru"}'

# Set excluded labels for weekly view
curl -X PATCH "$BASE/api/v1/settings" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"weeklyUnplannedExcludedLabelIds":[3,7]}'
```

---

## App Settings

Global, server-wide rules (single-row `app_settings` table). Reads require the
`settings:read` scope, writes `settings:write`. The same payload is embedded as
`appSettings` in `GET /api/v1/config`.

- **Auto-labels** — when a task title contains `mask`, the listed labels are
  attached automatically on create / title change.
- **Project suggestions** — when a task title contains `mask`, the listed
  projects are *offered* in the quick-add dialog (deduped across matching rules,
  sorted A-Z, capped at 3). Advisory only: the server never assigns a project.

`ignoreCase: true` makes the mask match case-insensitively.

### `GET /api/v1/app-settings`

```json
{
  "autoLabels": [
    { "mask": "buy", "labelIds": [3], "ignoreCase": true }
  ],
  "projectSuggestions": [
    { "mask": "deploy", "projectIds": [4, 7], "ignoreCase": true }
  ]
}
```

```sh
curl "$BASE/api/v1/app-settings" \
  -H "Authorization: Bearer $TOKEN"
```

### `PUT /api/v1/app-settings/auto-labels`

Replaces the whole auto-label rules list. Each `mask` must be non-empty after
trimming and each `labelIds` must contain at least one existing label id
(duplicates within a rule are dropped). Returns the full app settings.

```sh
curl -X PUT "$BASE/api/v1/app-settings/auto-labels" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"autoLabels":[{"mask":"buy","labelIds":[3],"ignoreCase":true}]}'
```

### `PUT /api/v1/app-settings/project-suggestions`

Replaces the whole project-suggestion rules list. Each `mask` must be non-empty
after trimming and each `projectIds` must contain at least one existing project
id (duplicates within a rule are dropped). Returns the full app settings.

```sh
curl -X PUT "$BASE/api/v1/app-settings/project-suggestions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"projectSuggestions":[{"mask":"deploy","projectIds":[4,7],"ignoreCase":true}]}'
```

---

## Harpoon

A per-user "jump pair": at most **two** task/project references the user can hop
between with one click. Order is significant — slot 0 is the first member, slot 1
the second. Attaching a third reference evicts the oldest (FIFO). References are
persisted in user settings; titles are hydrated on read, and references to deleted
entities are silently dropped (self-healing).

All three endpoints return the same hydrated shape:

```json
{
  "slots": [
    { "kind": "task", "id": 42, "title": "Do thing" },
    { "kind": "project", "id": 7, "title": "My project" }
  ]
}
```

### `GET /api/v1/harpoon`

```sh
curl "$BASE/api/v1/harpoon" \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/v1/harpoon/attach`

Adds a reference (idempotent). `kind` is `"task"` or `"project"`; the target must
exist (`404` otherwise).

```sh
curl -X POST "$BASE/api/v1/harpoon/attach" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"task","id":42}'
```

### `POST /api/v1/harpoon/detach`

Removes a reference (idempotent — removing an absent reference is a no-op).

```sh
curl -X POST "$BASE/api/v1/harpoon/detach" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"task","id":42}'
```

---

## Appendix

### CreateTaskRequest

Used by all task-creation endpoints.

```json
{
  "title": "Buy milk",
  "description": "",
  "priority": "medium",
  "dueAt": "2024-01-20T00:00:00.000Z",
  "dueHasTime": false,
  "deadlineAt": null,
  "deadlineHasTime": false,
  "dayPart": "none",
  "planState": "none",
  "recurrenceRule": null,
  "labels": ["shopping"],
  "removedAutoLabels": []
}
```

`title` is required. All other fields are optional.

`recurrenceRule` must be a valid RRULE string (e.g. `"RRULE:FREQ=WEEKLY;BYDAY=MO"`).

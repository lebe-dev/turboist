# Turboist API Reference

All authenticated endpoints accept either a **JWT access token** or a **long-lived API token**.

## Authentication

### Using an API Token

```
Authorization: Bearer <token>
```

API tokens are created via `POST /api/v1/api-tokens` (requires a JWT session) and never expire. Store the plaintext token securely — it is only returned once.

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
| `CodeConflict` | 409 |
| `CodeLimitExceeded` | 409 |
| `CodeForbiddenPlacement` | 422 |
| `CodeInternalError` | 500 |

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

---

## Auth

> Rate limited: 10 requests/minute per IP.

### `GET /auth/setup-required`

Returns whether initial setup is needed.

```json
{ "required": true }
```

```sh
curl "$BASE/auth/setup-required"
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

```sh
curl -X POST "$BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret","clientKind":"cli"}'
```

### `POST /auth/refresh`

Exchange a refresh token for a new access token. Send either:

- Body: `{ "refresh": "<token>" }`
- Or cookie `refresh=<token>` (web clients)

**Response:**
```json
{ "access": "<jwt>", "refresh": "<new-token>" }
```

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

### `GET /auth/me` *(requires JWT)*

```json
{ "user": { "id": 1, "username": "alice" } }
```

```sh
curl "$BASE/auth/me" \
  -H "Authorization: Bearer $TOKEN"
```

---

## API Tokens

> These endpoints require a **JWT session** — API token authentication is rejected here.

### `POST /api/v1/api-tokens`

Create a new long-lived API token. The plaintext token is returned **only in this response**.

**Request:**
```json
{ "name": "my-script" }
```

**Response** `201`:
```json
{
  "id": 1,
  "name": "my-script",
  "token": "abc123...",
  "createdAt": "2024-01-15T09:30:00.000Z"
}
```

```sh
curl -X POST "$BASE/api/v1/api-tokens" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-script"}'
```

### `GET /api/v1/api-tokens`

List all tokens (metadata only — plaintext is never returned after creation).

**Response:**
```json
[
  { "id": 1, "name": "my-script", "createdAt": "2024-01-15T09:30:00.000Z" }
]
```

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
  "completedAt": null,
  "recurrenceRule": null,
  "postponeCount": 0,
  "labels": [{ "id": 3, "name": "bug", "color": "red", "isFavourite": false, "isPrivate": false, "createdAt": "...", "updatedAt": "..." }],
  "url": "https://example.com/task/42",
  "createdAt": "2024-01-15T09:30:00.000Z",
  "updatedAt": "2024-01-15T09:30:00.000Z"
}
```

A task belongs to exactly one placement: `inboxId`, `contextId`, `projectId`, or `sectionId`. `parentId` identifies a subtask relationship.

### `GET /api/v1/tasks/:id`

```sh
curl "$BASE/api/v1/tasks/42" \
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
  "isPrivate": false
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

Creates a copy of the task with title suffixed `(2)`. Returns `201` with the new task.

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

### `GET /api/v1/stats/plan`

```json
{ "week": 8, "backlog": 14 }
```

```sh
curl "$BASE/api/v1/stats/plan" \
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

Server configuration.

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
  "autoLabels": [
    { "mask": "*bug*", "label": "bug", "ignoreCase": true }
  ]
}
```

```sh
curl "$BASE/api/v1/config" \
  -H "Authorization: Bearer $TOKEN"
```

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
  "bannerPublished": false
}
```

```sh
curl "$BASE/api/v1/settings" \
  -H "Authorization: Bearer $TOKEN"
```

### `PATCH /api/v1/settings`

All fields optional. `locale` must be `"en"`, `"ru"`, or `""` (client decides).

```json
{
  "weeklyUnplannedExcludedLabelIds": [3, 7],
  "bugLabelIds": [3],
  "locale": "ru",
  "publicView": false,
  "bannerText": "Under maintenance",
  "bannerPublished": true
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

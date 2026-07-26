---
paths:
  - "internal/httpapi/**"
---

# Go Handler Conventions

## Layered architecture

`internal/httpapi/handlers/*.go` are thin Fiber adapters. Cross-repo invariants (caps, placement, RRULE advance, auto-labels, troiki capacity) live in `internal/service/*`. Raw SQL lives in `internal/repo/*`. Handlers MUST go through `service` for mutations that have invariants — never write raw SQL or call repos directly when a service exists.

## Handler structs

- One struct per domain: `TaskHandler`, `ProjectHandler`, `ContextHandler`, `LabelHandler`, `AuthHandler`, `InboxHandler`, `TaskRelationHandler`, etc.
- Constructor: `NewXxxHandler(deps) *XxxHandler` — all dependencies injected, stored in unexported fields
- Each handler owns its routes: `func (h *XxxHandler) Register(r fiber.Router)`. Handler methods themselves are unexported receiver methods returning `error` (Fiber v3 signature) — only `Register` and the constructor are exported
- Message literals shared by more than one handler file live in `consts.go` (`msgTaskNotFound`, `msgInvalidRequestBody`, …) — reuse them instead of re-typing the string

## Fiber context usage

- Query params: `c.Query("key")`, `c.Query("key", "default")`
- Path params: `c.Params("id")`
- Parse body: `c.Bind().JSON(&req)` (Fiber v3 binders)
- Success: `c.JSON(value)` or `c.SendStatus(fiber.StatusNoContent)`
- Errors: return typed errors from `internal/httpapi/errors.go` — the central `ErrorHandler` maps them to `{error: {code, message, details}}`
- Pass context to services/repos: always `c.Context()` — it already carries the request-scoped logger and auth fields (`logging.FromContext`). Fiber v3 has no `UserContext()`

## DTOs

- Request/response shapes belong in `internal/httpapi/dto` (shared across handlers) or as unexported structs in the handler file when single-use
- Pointer fields for optional/patchable values (e.g., `*string`, `*int`)
- DTO field names mirror the frontend `types.ts` (camelCase via JSON tags); times are ISO-8601 UTC with millisecond precision (`model.FormatUTC`)

## Route registration

`server.go`'s `RegisterRoutes(app, deps)` mounts the unauthenticated endpoints (`/healthz`, `/version`, `/api/config` — the SPA's Sentry bootstrap) and returns the `/api/v1` group with the middleware chain already attached, in this order:

`SetupCheckMiddleware` → `APIAuthMiddleware` → `IdempotencyMiddleware` → `PublishMiddleware`

The order is load-bearing: idempotency needs the resolved user id from auth, and must sit *before* publish so a replay short-circuits without re-emitting SSE. It returns the group (rather than registering handlers itself) to avoid an import cycle between `httpapi` and `httpapi/handlers`.

`cmd/turboist/main.go` then calls `Register` on each handler, passing either the bare group or a sub-group:

```go
handlers.NewTaskHandler(...).Register(api)
handlers.NewLabelHandler(...).Register(api.Group("/labels"))
handlers.NewSessionHandler(...).Register(api.Group("/sessions", httpapi.RequireJWTAuth()))
```

- Static routes before parameterized ones (`/tasks/bulk/complete` before `/tasks/:id`) — within a single `Register` and across handlers, so registration order in `main.go` matters
- Per-route authorization is declarative: `httpapi.RequireScope(auth.ScopeTasksRead)` for API-token scopes, `httpapi.RequireJWTAuth()` (group level) for endpoints an API token must never reach — sessions, api-tokens, backup
- Tests wire the same way via `setupAPIEnv(t)` (`handlers/testhelper_test.go`)

## Logging

- Use `slog.DebugContext(c.Context(), ...)` / `InfoContext` / `WarnContext` /
  `ErrorContext` everywhere so request-scoped fields (`request_id`, `user_id`,
  `auth_method`) attached by middleware appear automatically
- Levels: DEBUG = handler entry with key params and branching decisions;
  INFO = successful mutations (create/update/delete) with resulting id;
  WARN = validation failures and business-rule rejections before returning a
  typed error; ERROR = unexpected repo/service failures
- Field convention: `slog.String("op", "handler.<Domain>.<Action>")`,
  `slog.Int64("task_id", id)`, `slog.String("err", err.Error())`. Never log
  secrets (passwords, raw tokens, JWT strings, salts) — mask tokens to the
  first 6 chars if needed
- Never `_ = closer.Close()` — use `defer logging.LogClose(ctx, "<op>", c)` so
  Close errors surface at WARN instead of being silently swallowed

## Auth

- The app is single-user — `users` row id=1 is seeded by migration `002_users_sessions.sql`
- First call must be `POST /auth/setup`; thereafter login → access (15 min JWT HS256) + refresh (rotated, sha256-hashed, single-use)
- Sessions are scoped by `client_kind` — `web`/`ios`/`android`/`cli` (`model.ClientKind`, `android` added by migration `043`) — max 5 per kind (oldest evicted)
- Web refresh token lives in an `HttpOnly` cookie; other clients (native iOS/Android, CLI) get it in the response body
- API tokens carry granular `resource:action` scopes (`["*"]` = full access); enforce them with `RequireScope` at the route, never by hand inside the handler

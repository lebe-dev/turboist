---
paths:
  - "internal/httpapi/**"
---

# Go Handler Conventions

## Layered architecture

`internal/httpapi/handlers/*.go` are thin Fiber adapters. Cross-repo invariants (caps, placement, RRULE advance, auto-labels, troiki capacity) live in `internal/service/*`. Raw SQL lives in `internal/repo/*`. Handlers MUST go through `service` for mutations that have invariants — never write raw SQL or call repos directly when a service exists.

## Handler structs

- One struct per domain: `TasksHandler`, `ProjectsHandler`, `ContextsHandler`, `LabelsHandler`, `AuthHandler`, `InboxHandler`, etc.
- Constructor: `NewXxxHandler(deps) *XxxHandler` — all dependencies injected, stored in unexported fields
- Handler methods are exported receiver methods returning `error` (Fiber v3 signature)

## Fiber context usage

- Query params: `c.Query("key")`, `c.Query("key", "default")`
- Path params: `c.Params("id")`
- Parse body: `c.Bind().JSON(&req)` (Fiber v3 binders)
- Success: `c.JSON(value)` or `c.SendStatus(fiber.StatusNoContent)`
- Errors: return typed errors from `internal/httpapi/errors.go` — the central `ErrorHandler` maps them to `{error: {code, message, details}}`
- Pass context to services/repos: `c.Context()` (or `c.UserContext()` if request-scoped values are needed)

## DTOs

- Request/response shapes belong in `internal/httpapi/dto` (shared across handlers) or as unexported structs in the handler file when single-use
- Pointer fields for optional/patchable values (e.g., `*string`, `*int`)
- DTO field names mirror the frontend `types.ts` (camelCase via JSON tags); times are ISO-8601 UTC with millisecond precision (`model.FormatUTC`)

## Route registration (`server.go`)

- Static routes before parameterized routes (e.g., `/api/v1/tasks/bulk/complete` before `/api/v1/tasks/:id`)
- Group routes by handler under `/api/v1/<domain>` prefixes
- Authentication middleware is applied at the group level

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
- Sessions are scoped by `client_kind` (web/ios/cli), max 5 per kind (oldest evicted)
- Web refresh token lives in an `HttpOnly` cookie; other clients get it in the response body

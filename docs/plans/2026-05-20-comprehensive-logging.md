# Comprehensive logging coverage

## Overview
Add structured logging across all layers (handlers, service, repo, auth, middleware) so that errors are never silently swallowed and DEBUG-level details are available for investigations. Use context-propagated logger via `logging.FromContext(ctx)` so we don't have to thread `*slog.Logger` through every constructor.

## Context
- Files involved:
  - `internal/logging/logger.go` — add `FromContext`/`WithLogger` helpers and a `LogCloser` helper
  - `internal/httpapi/middleware.go` — attach request-scoped logger (with `request_id`, `user_id`, `auth_method`) to `c.Context()`; expand `AccessLogMiddleware` to log auth failures, panics, and 4xx at DEBUG/WARN
  - `internal/httpapi/server.go` — already logs 5xx with cause; ensure error handler also logs 4xx at DEBUG
  - `internal/httpapi/handlers/*.go` — add DEBUG entry/exit for each handler with key params and INFO/WARN at decision points
  - `internal/service/*.go` — add DEBUG for inputs/decisions, WARN for business-rule rejections, ERROR for unexpected repo errors
  - `internal/repo/*.go` — add DEBUG for query bind values where useful, ERROR on unexpected SQL failures, log `rows.Close()` errors instead of swallowing them
  - `internal/auth/*.go` — WARN on failed login, expired/invalid refresh, invalid API-token lookup; INFO on session creation/rotation; DEBUG on JWT issue/verify lifecycle
  - `cmd/turboist/main.go` — set `slog.SetDefault(log)` so context-less code paths still log to the configured handler
- Related patterns: existing `AccessLogMiddleware`, `makeErrorHandler` 5xx logging, `auth/cleanup.go` background-task logging
- Dependencies: none new (stdlib `log/slog`)

## Development Approach
- Testing approach: Regular (code first, then tests)
- Use `slog.DebugContext(ctx, ...)` / `InfoContext` / `WarnContext` / `ErrorContext` everywhere so request-scoped fields (request_id, user_id) appear automatically
- Introduce `logging.FromContext(ctx) *slog.Logger` (falls back to `slog.Default()`) and `logging.WithLogger(ctx, log) context.Context`
- Introduce `logging.LogClose(ctx, name, c io.Closer)` helper — call in `defer` to log non-nil Close errors at WARN instead of `_ =`
- Logged fields convention: `slog.String("op", "service.Plan.SetPlanState")`, `slog.Int64("task_id", id)`, `slog.String("err", err.Error())` — never log secrets (passwords, raw tokens, JWT strings, salts)
- DEBUG = inputs, branching decisions, query bind values; INFO = significant state changes (login ok, plan moved, backup created); WARN = recoverable/expected failures (validation, rate limit, business-rule reject, defer Close error); ERROR = unexpected (repo SQL error, transaction failure)
- CRITICAL: every task MUST include new/updated tests
- CRITICAL: all tests must pass before starting next task

## Implementation Steps

### Task 1: Logging foundation — context helpers and default logger wiring

**Files:**
- Modify: `internal/logging/logger.go`
- Create: `internal/logging/context.go`
- Modify: `cmd/turboist/main.go`
- Modify: `internal/logging/logger_test.go`
- Create: `internal/logging/context_test.go`

- [x] add `WithLogger(ctx, *slog.Logger) context.Context` and `FromContext(ctx) *slog.Logger` (fallback `slog.Default()`)
- [x] add `LogClose(ctx context.Context, name string, c io.Closer)` helper that logs non-nil close errors at WARN with `op=name`
- [x] in `cmd/turboist/main.go`, call `slog.SetDefault(log)` right after `logging.New(...)` so context-less paths use the configured JSON handler
- [x] write unit tests for `FromContext` (default), `WithLogger`/`FromContext` round-trip, and `LogClose` (records WARN on error, nothing on nil error)
- [x] run `just test logging` — must pass before task 2

### Task 2: Request-scoped logger in HTTP middleware

**Files:**
- Modify: `internal/httpapi/middleware.go`
- Modify: `internal/httpapi/middleware_test.go`
- Modify: `internal/httpapi/server.go`

- [x] in `RequestIDMiddleware`, after generating/propagating request_id, attach a child logger (`log.With("request_id", rid)`) to `c.Context()` using `logging.WithLogger`; store request-scoped logger via `c.SetUserContext` (Fiber v3) so downstream layers see it via `c.Context()`
- [x] after `AuthMiddleware`/`APIAuthMiddleware` resolve user_id and auth_method, enrich the request-scoped logger with `user_id` and `auth_method`
- [x] expand `AccessLogMiddleware` to log at DEBUG when status < 400, INFO for 4xx, WARN for 5xx; include `bytes_out`, `user_id` if available
- [x] in `makeErrorHandler`, also log 4xx (non-`auth_*`) at DEBUG with cause when present (still no client leak)
- [x] in `AuthMiddleware`/`APIAuthMiddleware`, log WARN on `missing header`, `bad format`, `invalid token`, `expired token` with masked token prefix (first 6 chars) — never log full token
- [x] write/update middleware tests asserting log records (use `slog.NewTextHandler` to a `bytes.Buffer` and parse, or `slog.Handler` test double)
- [x] run `just test httpapi` — must pass before task 3

### Task 3: Auth package — login, refresh, API-token, JWT, rate limit

**Files:**
- Modify: `internal/auth/jwt.go`, `internal/auth/refresh.go`, `internal/auth/api_token.go`, `internal/auth/password.go`, `internal/auth/ratelimit.go`, `internal/auth/cleanup.go`
- Modify: `internal/httpapi/handlers/auth.go`
- Modify: corresponding `_test.go` files

- [x] `auth.go` handler: INFO on successful `/auth/setup`, `/auth/login`, `/auth/refresh`, `/auth/logout`; WARN on wrong password, unknown user, expired/used/invalid refresh token, rate-limit hit; include `user_id` and `client_kind` but never the password or raw token
- [x] `jwt.go`: DEBUG on `Issue` (user_id, exp), DEBUG on `Verify` failure reason (expired vs malformed) — use `slog.Default()` since these are stateless funcs without ctx; alternatively accept ctx
- [x] `api_token.go`: WARN on token hash lookup miss in `APIAuthMiddleware` path (already covered in task 2)
- [x] `ratelimit.go`: DEBUG when a request is allowed; WARN when blocked (include client ip)
- [x] `cleanup.go`: already logs — convert to DEBUG for the routine `removed=0` case, keep INFO when `removed>0`, ERROR stays
- [x] add tests asserting log emission for failed login, expired refresh, rate-limited request
- [x] run `just test auth` and `just test httpapi/handlers` — must pass before task 4

### Task 4: Repo layer — DEBUG queries, log defer Close, ERROR on unexpected SQL

**Files:**
- Modify: all `internal/repo/*.go` files containing `defer func() { _ = rows.Close() }()` or SQL execution
- Modify: corresponding `_test.go` to assert error logging where reasonable

- [x] replace every `defer func() { _ = rows.Close() }()` (non-test files: `sections.go:88`, `sessions.go:205`, `labels.go:117`, `contexts.go`, `projects.go`, `tasks.go`, `views.go`, `search.go`, `api_tokens.go`, `task_labels.go`, `project_labels.go`, `users.go`, `app_settings.go`) with `defer logging.LogClose(ctx, "<repo>.<op>.rows", rows)`
- [x] same for `internal/service/backup_codec.go:34` (`zr.Close`), `backup_export.go` 7× `rows.Close`, `backup_restore.go:177` `rows.Close`
- [x] for each repo function with a single primary query, add `slog.DebugContext(ctx, "repo.<table>.<op>", slog.Any("args", args))` before exec; ERROR on non-`sql.ErrNoRows` errors with `op` and `err`
- [x] keep `sql.ErrNoRows` paths quiet (caller decides) — but DEBUG-log them with `op` and `id` so we can trace lookups
- [x] write repo tests that capture logs for one happy-path and one error-path per repo (TaskRepo, ProjectRepo, SessionRepo, APITokenRepo at minimum) — full coverage across every repo would be excessive; use a `testlog` helper buffer
- [x] run `just test repo` — must pass before task 5

### Task 5: Service layer — DEBUG inputs, WARN business-rule rejects, ERROR unexpected

**Files:**
- Modify: every `internal/service/*.go` (auto_labels, backup*, complete, group, move, pin, plan, tasks_create, troiki)
- Modify: corresponding `_test.go`

- [x] for each exported service method: at top, `slog.DebugContext(ctx, "service.<Svc>.<Method>", ...key inputs)`; before returning a known business-rule error (`ErrPlanLimitExceeded`, `ErrNoContextForInbox`, troiki slot full, forbidden placement, cap exceeded, RRULE invalid), log at WARN with `op`, relevant ids, and the reason
- [x] on unexpected repo errors (anything not pre-classified), log at ERROR with `op` and `err` before returning; the error still propagates up
- [x] backup service: INFO on export start/finish with row counts; INFO on restore start/finish with row counts; WARN on per-row sanitize/skip events; ERROR on transaction abort
- [x] update existing service tests to assert WARN logs on business-rule rejections for at least one path per service
- [x] run `just test service` — must pass before task 6

### Task 6: Handlers — DEBUG entry, INFO on mutations, WARN on validation failures

**Files:**
- Modify: every `internal/httpapi/handlers/*.go`
- Modify: `internal/httpapi/handlers/*_test.go`

- [x] each handler: `slog.DebugContext(c.Context(), "handler.<Domain>.<Action>", ...key params)` at start; on validation errors (bad JSON, missing field, bad id), log WARN before returning `ErrValidation`
- [x] for mutations (create/update/delete/complete/move/restore), log INFO with the resulting id and user_id on success
- [x] backup handler: INFO on export download with size, INFO on restore with payload size and outcome
- [x] api-tokens handler: INFO on token create (with token id, never the token value) and revoke
- [x] update at least one test per handler file to assert log emission for the WARN/INFO path
- [x] run `just test httpapi/handlers` — must pass before task 7

### Task 7: Verify acceptance criteria

- [x] run `just test` — full suite green
- [x] run `just lint` — no new lint errors (especially `errcheck` on Close)
- [x] grep for remaining `_ = .*\.Close\(\)` in non-test code — must be zero outside intentional cases (document any in code comment)
- [x] manually run the server with `LOG_LEVEL=debug`, hit `/auth/login` (bad password), `/api/v1/tasks` (list and create), backup export — confirm DEBUG/INFO/WARN records appear with `request_id` and `user_id` (skipped - manual, not automatable)
- [x] manually run with `LOG_LEVEL=info` — confirm DEBUG records disappear, INFO/WARN/ERROR remain (skipped - manual, not automatable)
- [x] verify test coverage for `internal/logging` and middleware ≥ 80% (logging 100%, middleware 94.7%)

### Task 8: Update documentation

- [x] update `README.md` "Configuration" section to document `LOG_LEVEL` values (`debug`, `info`, `warn`, `error`) and what each level emits
- [x] update `.claude/rules/go-handlers.md` with a short "Logging" subsection: use `slog.<Lvl>Context(c.Context(), ...)`, never `_ = closer.Close()` — use `logging.LogClose`
- [x] add a CHANGELOG entry describing the new logging coverage

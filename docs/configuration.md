# Configuration

Two configuration sources are merged at start-up.

## Environment variables

Load from `.env` if present (copy `.env.example` to get started):

| Variable | Required | Description |
|---|---|---|
| `BIND` | ✓ | Listen address, e.g. `0.0.0.0:8080` |
| `BASE_URL` | ✓ | Public base URL used when building `Task.URL` |
| `JWT_SECRET` | ✓ | Base64-encoded secret, ≥ 32 bytes |
| `API_TOKEN_SALT` | ✓ | HMAC salt for API tokens, ≥ 32 bytes. Rotating it invalidates all existing tokens. |
| `LOG_LEVEL` | — | `debug\|info\|warn\|error`, default `info` |
| `GOOGLE_CALENDAR_CLIENT_ID` | — | Google OAuth client ID for read-only calendar integration |
| `GOOGLE_CALENDAR_CLIENT_SECRET` | — | Google OAuth client secret. Redirect URI: `<BASE_URL>/api/v1/calendars/google/callback` |
| `CALENDAR_TOKEN_KEY` | — | Encryption key for stored calendar OAuth tokens. Defaults to `JWT_SECRET`; keep stable. |
| `CALENDAR_CACHE_TTL` | — | Server-side TTL for the in-memory calendar event cache, as a Go duration (e.g. `30s`, `2m`). Default `10m`. Lower it to see edits made in Google Calendar sooner, at the cost of more API calls (and, on mobile, more radio wake-ups: the day/week views refetch events on every catch-up refresh). Must be positive. |
| `TOTP_SECRET_KEY` | — | Encryption key (≥ 32 bytes) for TOTP secrets at rest. Required to enable 2FA; if empty, `/auth/totp/*` returns an error. Keep stable — rotating invalidates all enrolled secrets. |
| `SENTRY_DSN` | — | Backend Sentry DSN. When set, the server reports recovered panics, every 5xx response, and 400 Bad Request (with the underlying cause) to Sentry. Expected client errors (401/403/404/409/429/…) are not reported. Empty disables backend reporting. |
| `SENTRY_FRONTEND_DSN` | — | Browser Sentry DSN. Served to the SPA at runtime via `GET /api/config` (never baked into the static bundle), so toggling it needs no frontend rebuild. Use a separate Sentry project from the backend. |
| `SENTRY_ENVIRONMENT` | — | Environment label applied to both backend and frontend events (e.g. `production`, `staging`). |

### Sentry error reporting

Both planes are errors-only (no performance tracing) and fully optional — leave a DSN blank to disable that side. The backend captures recovered panics and any request resolving to HTTP ≥ 400, plus background-goroutine and startup failures. The frontend captures uncaught browser errors, unhandled promise rejections, and errors surfaced through SvelteKit's client pipeline; it fetches its DSN from the public `GET /api/config` endpoint on startup.

### Log levels

What each level emits:

- **debug** — handler entry/exit with key params, service inputs and decision branches, repo query bind values and `sql.ErrNoRows` lookups, JWT issue/verify lifecycle, allowed rate-limit requests, access log for successful (<400) responses
- **info** — significant state changes: successful auth events, session creation/rotation, successful mutations with resulting id, backup export/restore start/finish with row counts, access log for 4xx responses
- **warn** — recoverable failures: validation errors, wrong password, expired/used/invalid tokens, rate-limit hits with client IP, business-rule rejections (plan cap exceeded, troiki slot full, forbidden placement, invalid RRULE), deferred `io.Closer` errors, access log for 5xx responses
- **error** — unexpected failures: SQL errors that are not `sql.ErrNoRows`, transaction aborts, panics

## config.yml

Business configuration: timezone, day-parts, limits, auto-labels, inbox overflow, pin caps.

```sh
cp config.example.yml config.yml
```

See `config.example.yml` for the full schema.

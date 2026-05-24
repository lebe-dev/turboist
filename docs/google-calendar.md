# Google Calendar Integration

## Overview

Turboist integrates with Google Calendar in read-only mode. Events are fetched live from the Google Calendar API — they are **not** stored in the application database. An in-memory cache (2-minute TTL) reduces redundant API calls within a session.

## What is stored in the database

| Table | Contents |
|---|---|
| `calendar_oauth_configs` | Per-user OAuth credentials (client ID + client secret, AES-encrypted) |
| `calendar_accounts` | Connected Google account (email, display name, OAuth tokens encrypted) |
| `calendar_sources` | List of calendars from the user's Google Calendar list, with `selected` flag |
| `oauth_states` | Short-lived CSRF state tokens used during the OAuth flow (auto-expired) |

Events themselves are never written to the database.

## Request flow

1. `GET /api/v1/calendars/events?start=...&end=...`
2. Handler builds a cache key: `userID | start | end | source1_id:external_id | ...`
3. **Cache hit** — returns cached events immediately (no API call).
4. **Cache miss** — calls `FetchGoogleEvents()`, which pages through all selected calendar sources via the Google Calendar API, then stores the result in cache before responding.

The maximum request range is 92 days. The API call has a 20-second timeout.

## In-memory event cache

`EventCache` (`internal/service/calendar/cache.go`) is a thread-safe in-memory map with a **2-minute TTL**. It is process-local — the cache is lost on restart.

Cache entries for a user are invalidated whenever the user's calendar configuration changes:

- OAuth credentials updated or deleted
- Calendar source toggled (selected/deselected)
- Account disconnected
- Manual sync triggered (`POST /api/v1/calendars/google/sync`)
- OAuth callback completes (account re-connected)

## OAuth flow

1. User saves their own Google OAuth client ID and secret via Settings → Calendars.
2. `GET /api/v1/calendars/google/start` — generates a random state token, stores it in the DB with a 10-minute expiry, and returns the Google authorization URL.
3. User is redirected to Google, approves access.
4. `GET /api/v1/calendars/google/callback` — validates the state token, exchanges the authorization code for an access + refresh token, saves the account and the full calendar list to the DB, and auto-enables the calendar feature in user settings.

Tokens are encrypted at rest using AES (via `crypto.TokenCipher`). If a token is expired, `FreshGoogleToken()` refreshes it automatically and persists the new value back to the DB.

## Manual sync

`POST /api/v1/calendars/google/sync` refreshes the calendar source list from the Google Calendar API (adds new calendars, keeps existing ones). It does **not** pre-fetch events — events are always fetched lazily on the next `GET /events` request.

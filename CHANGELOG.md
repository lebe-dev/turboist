# Turboist v1.5.1

## New Features

- **Backup & Restore** — You can now export a full backup of your data and restore it later. The backup is protected against corrupted settings JSON, validates input on import, and enforces size limits to keep your data safe.

## Improvements

- **Settings page** — Refreshed look and improved layout, with much better compatibility on mobile screens.
- **Inbox page** — Cleaner UX, with the duplicated page title removed.
- **Task page** — Polished layout and interactions. The recurrence widget now opens in a drawer for a smoother editing experience.
- **Projects page** — Project descriptions now support multiple lines so you can write richer notes.
- **Sidebar menu** — The PINNED section uses a softer gray instead of yellow, and the backlog counter is now revealed on hover.
- **Troiki page** — Various visual and usability tweaks.
- **Context items** — Cleaner, more consistent appearance.
- **Public mode** — "Move to project" and the Settings page now correctly respect public mode.
- **Date handling** — Returning to a tab the next day now refreshes the current date correctly, so tasks appear in the right buckets.

## Bug Fixes

- Fixed an issue where the task title could not be edited.
- Added a safeguard to prevent accidental context removal.

## Under the Hood

- **Comprehensive logging** — Structured JSON logging now covers every layer
  (handlers, services, repositories, auth, middleware). Each request carries a
  `request_id` and, after authentication, a `user_id` and `auth_method`, so
  related log records can be traced end-to-end. Set `LOG_LEVEL=debug` for full
  visibility into handler entry, service decisions, and SQL query parameters;
  the default `info` level keeps successful mutations, auth events, and backup
  operations while filtering out the noisier debug records. Validation
  failures, business-rule rejections, expired tokens, and rate-limit hits now
  log at `warn`, and unexpected database/transaction errors log at `error`
  before being returned — silent failures are no longer swallowed.

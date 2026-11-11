# Today focus lock

The Today focus lock keeps the day to the projects already planned for it. It is turned on
from the Today screen, lasts until the next midnight in the server timezone, and cannot be
lifted early.

## What it does

While the lock runs, a task cannot be **taken into Today** — given a due date of today or
earlier while open — unless its project already has open work on Today (due today or
overdue). The inbox and tasks with no project are never allowed in.

Refused, while the lock runs:

- creating a task dated today (or earlier) in another project or in the inbox — subtasks,
  duplicates and quick capture included;
- setting another project's task to today, by editing the date or by dragging it;
- moving a task that is on Today into another project, or out of the inbox into one;
- reopening a completed task of another project whose date is today.

Always allowed:

- adding tasks to any project for another day, or with no date;
- editing a task that is already on Today, including moving it to another hour;
- taking work off Today (another day, no date);
- completing, cancelling and deleting anything.

The set of allowed projects is read live, at every write. A project leaves the set once its
last open task for today is done, and from then on it cannot take more work today. A
repeating task moving on to its next occurrence is the schedule the user already chose and
is never refused.

The inbox processor follows the same rule: a proposed due date that the lock would refuse is
left off, and the task is still filed.

## Turning it on

The padlock next to the Today title (web) or in the Today screen's top bar (Android) shows a
tooltip explaining the lock. Tapping it asks for confirmation; once confirmed, the padlock
closes and only explains itself until midnight. On Android the lock is an ordinary queued
write, so it applies at once even without a network. The device's own midnight is used
until the server's answer arrives with the next sync.

## API

- `POST /api/v1/today/lock` (scope `settings:write`) turns the lock on and answers
  `{"lockedUntil": "<ISO-8601 UTC>"}`. Locking again while a lock runs keeps it.
- The lock's end is `todayLockUntil` in the user settings (`GET /api/v1/settings`,
  `settings` in `GET /api/v1/config`, `user_settings` in the sync feed); `null` means the lock
  was never turned on and a moment in the past means no lock runs. `PATCH /api/v1/settings`
  ignores the field.
- A refused write answers `409 today_locked` with `details.lockedUntil` and
  `details.projectId` (`null` for the inbox). Bulk moves report the refused tasks per item
  with the same code.
- Turning the lock on publishes the `settings` SSE scope, so other web tabs refetch
  `/api/v1/config` and native clients pull.

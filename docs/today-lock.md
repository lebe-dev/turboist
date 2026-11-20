# Today focus lock

The Today focus lock keeps the day to the projects already planned for it. It is turned on
from the Today screen, lasts until the next midnight in the server timezone, and cannot be
lifted early.

## What it does

While the lock runs, a task cannot be **taken into Today** — given a due date of today or
earlier while open — unless its project already has open work on Today (due today or
overdue). The inbox and tasks with no project are never allowed in.

The lock also decides what may be **finished**. Only the work of the projects that are part
of the day can be ticked off: a task finished elsewhere is work done outside what the day was
planned for. A task already standing on Today carries its own project into the day, which is
why the last task of a project can always be completed — and why that project is closed for
the rest of the day afterwards. Cancelling, deleting and taking work off Today stay allowed.

Refused, while the lock runs:

- creating a task dated today (or earlier) in another project or in the inbox — subtasks,
  duplicates and quick capture included;
- setting another project's task to today, by editing the date or by dragging it;
- moving a task that is on Today into another project, or out of the inbox into one;
- reopening a completed task of another project whose date is today;
- completing a task whose project has no open work on Today, including an inbox task.

A project can be marked **always allowed on Today** (`todayLockExempt`, set from the
project menu in Android Native or with `PATCH /api/v1/projects/:id`). Such a project counts as
having open work on Today at all times, so tasks that appear spontaneously — a household
project, say — can still be added for today while the lock runs, and their work can still be
finished. The task move dialogs on Android say that the lock is on, since some projects will
refuse a task dated today.

The week is the exception. An open task already on the Week screen may be taken into
Today whatever its project:

- a task planned for the week;
- a task dated a later day of the current week (Monday to Sunday, server timezone).

It is the task that qualifies, not its project: other work of that project stays out until
the project has open work on Today of its own. A completed task is not on the Week screen,
so reopening one onto Today is still judged by its project. The exception is about taking
work into the day, not about finishing it: a week task has to be taken into Today first, and
that is what puts its project into the day.

Always allowed:

- adding tasks to any project for another day, or with no date;
- editing a task that is already on Today, including moving it to another hour;
- moving a task from this week's plan, or dated later this week, to today;
- taking work off Today (another day, no date);
- cancelling and deleting anything.

The set of allowed projects is read live, at every write and at every completion. A project
leaves the set once its last open task for today is done, and from then on it can neither
take more work today nor have more of its work finished. Finishing a task on Today takes its
open subtasks with it, dated or not: they are the same work, and the parent's own place on
Today is what let the whole of it be completed. A repeating task moving on to its next
occurrence is the schedule the user already chose and is never refused.

The inbox processor follows the same rule: a proposed due date that the lock would refuse is
left off, and the task is still filed.

The quick-add dialog (web and Android) disables its submit button while the draft would be
refused, so such a task cannot be added from there at all; pick one of today's projects or
another day. A refused completion says so in a sentence of its own — the day belongs to its
projects, so their work is the only work that can be finished — and nothing on screen changes.
On Android a bulk completion leaves those tasks open and reports how many were left out.
Dropping a task into a completed project moves it and then leaves it open with a note, since
the move is what the user asked for and the completion was only the rule that follows from it.

## Turning it on

The padlock next to the Today title (web and Android) shows a
tooltip explaining the lock. Tapping it asks for confirmation; once confirmed, the padlock
closes and only explains itself until midnight. On Android the lock is an ordinary queued
write, so it applies at once even without a network. The device's own midnight is used
until the server's answer arrives with the next sync.

## API

- `POST /api/v1/today/lock` (scope `settings:write`) turns the lock on and answers
  `{"lockedUntil": "<ISO-8601 UTC>"}`. Locking again while a lock runs keeps it.
- `GET /api/v1/today/lock` (scope `settings:read`) answers `{"active", "lockedUntil",
  "projectIds"}` — the projects that may still take work for today. The web quick-add dialog
  reads it each time it opens under a lock, so it refuses a draft on every screen (task,
  project, Today), not only where the Today list is on show.
- The lock's end is `todayLockUntil` in the user settings (`GET /api/v1/settings`,
  `settings` in `GET /api/v1/config`, `user_settings` in the sync feed); `null` means the lock
  was never turned on and a moment in the past means no lock runs. `PATCH /api/v1/settings`
  ignores the field.
- A refused write answers `409 today_locked` with `details.lockedUntil` and
  `details.projectId` (`null` for the inbox). Bulk moves and bulk completions report the
  refused tasks per item with the same code. Every completion endpoint — the single task,
  the bulk action and the Troiki board — answers it too, since they all go through one
  completion path.
- Turning the lock on publishes the `settings` SSE scope, so other web tabs refetch
  `/api/v1/config` and native clients pull.

# Task relations

Relations link two tasks together. There are two types:

| Type | Symmetric? | Enforced? | Meaning |
|------|-----------|-----------|---------|
| **Related** | Yes | No | A plain cross-reference. Both tasks show a link to the other; nothing changes about how either behaves. |
| **Blocks** | No | Yes | A dependency. The blocked task **cannot be completed** while the task blocking it is still open. |

## Direction

`Blocks` is directed, and the UI always states it from the point of view of the task you are looking at:

- **Blocked by** — the other task holds this one back.
- **Blocks** — this task holds the other one back.

Both are the *same* relation seen from its two ends. Adding "task 42 is blocked by task 7" on task 42 makes task 7 show "blocks task 42" — you do not add it twice, and removing it from either side removes it entirely.

`Related` has no direction, so it is stored once no matter which of the two tasks you add it from.

## The blocking rule

A task is **blocked** when at least one task blocking it is still `open`.

- Completing or cancelling a blocker releases everything it was holding back. Cancelling counts on purpose: a cancelled task would otherwise deadlock its dependents forever.
- `Related` links never block anything.
- Blocking is checked one level deep. If A blocks B and B blocks C, completing C requires B to be closed — and B in turn requires A. The chain resolves itself as you work down it; C is not reported as blocked by A directly.
- `Uncomplete` and `Cancel` are never blocked. Only completion is.
- Re-completing a task that is already complete stays allowed even if a blocker was added afterwards, so a task never gets stuck in a half-completed state.

## In the UI

**On a task page** a *Relations* section lists the relations in three groups — Blocked by, Blocks, Related — each row linking to the other task and showing whether it is already done. The **Add relation** button opens a picker where you choose the type and then find the task either by typing part of its title or by entering its numeric **ID**.

The task's own ID is shown under the title with a copy button, and the task actions menu (`···`) has a **Copy ID** item — handy for grabbing an ID to paste into the picker.

**In every task list** a blocked task shows a filled padlock in place of its checkbox, coloured by the task's priority, and the checkbox is not clickable. Any task with relations also shows a small link icon with the relation count next to its other badges.

**In bulk actions** completing a selection skips the blocked tasks and reports them individually — the rest of the selection still completes.

## What is not allowed

| Attempt | Result |
|---------|--------|
| Relating a task to itself | Rejected |
| The same relation twice (including `related` added from the opposite side) | Rejected as a duplicate |
| A `blocks` relation that would close a loop (A blocks B blocks … blocks A) | Rejected — every task in such a loop would be permanently uncompletable |

Only `blocks` relations are considered when checking for loops, so a `related` link between two tasks that already block each other's neighbours is always fine.

## Offline

Relations are visible offline on any task page you have opened online (they are cached with the task), but **adding and removing them requires a connection** — they are not part of the small set of writes the offline outbox queues.

Completing a blocked task is refused offline too: the check runs against the cached task before the operation is queued, so you get an immediate "unavailable offline" rather than a task that looks done and then bounces back. One consequence to be aware of: completing a *blocker* while offline does not release its dependents until you reconnect, because the offline cache has no way to know which tasks that blocker was holding back. See [docs/offline.md](offline.md).

## Interaction with other features

- **Deleting a task** removes its relations along with it, so deleting a blocker releases whatever it was blocking.
- **Recurring tasks** are subject to the rule like any other: a blocked recurring task is refused rather than silently advanced to its next occurrence.
- **Duplicate, Decompose and recurrence history snapshots do not copy relations.** Cloning a dependency graph is ambiguous, so new tasks start with none.
- **Backup** includes relations, and restoring preserves their identifiers.
- **Troiki** is unaffected: relations do not grant or consume slot capacity.

## API

Two write endpoints, both answering with the updated task:

```
POST   /api/v1/tasks/:id/relations
DELETE /api/v1/tasks/:id/relations/:relationId
```

There is no separate endpoint for reading relations. Every task carries `blockedByCount` and `relationCount` on all endpoints that return tasks, and the full list arrives inline via `GET /api/v1/tasks/:id?relations=true`. Completing a blocked task answers `409` with the code `task_blocked` and the blocker ids in `details.blockerIds`.

See [API.md → Task Relations](../API.md#task-relations) for request bodies, error cases and examples.

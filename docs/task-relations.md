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
- **Subtasks inherit their parent's blockers.** A subtask of a blocked task cannot be completed either, at any depth — finishing it would start work the parent is still waiting on. Releasing the parent's blocker releases the whole subtree at once. The subtask shows the padlock like any blocked task, but its own *Relations* section stays empty: the relation belongs to the parent, and the subtask's relation count keeps counting only its own links. One exception keeps the rule from turning on itself — a blocker that sits *inside* the subtask's own subtree is not inherited downwards, otherwise "child blocks parent" would leave the child waiting for itself.
- `Uncomplete` and `Cancel` are never blocked. Only completion is.
- Re-completing a task that is already complete stays allowed even if a blocker was added afterwards, so a task never gets stuck in a half-completed state.

## In the UI

**On a task page** a *Relations* section lists the relations in three groups — Blocked by, Blocks, Related — each row linking to the other task and showing whether it is already done. The **Add relation** button opens a picker where you choose the type and then find the task either by typing part of its title or by entering its numeric **ID**.

**Drag and drop between subtasks.** On a task page you can drag one open subtask onto another; where you drop it decides what happens. Drop it on the target row's **top or bottom edge** to make the dragged one depend on it (the target blocks it) — the row is outlined in blue and shows a padlock, and a tooltip says "Will depend on …". Drop it on the row's **middle** to nest it as that row's own subtask instead — the row is outlined in violet and shows an indent arrow, and the tooltip says "Will become a subtask of …". On a desktop, drag with the mouse; on a phone — the web app and both mobile apps — press and hold a subtask, then move it; the same edge/middle split applies to a finger's position over the row.

When a drop is not allowed, the row turns red and the tooltip says why. For a dependency: the target is the subtask's own parent or its own subtask (blockers are inherited down the tree, so such a dependency makes no sense), the dependency already exists, or it would close a loop. For nesting: the target is inside the dragged task's own subtree (nesting it there would close a cycle no tree can resolve), or the target is already done. Nesting under one of the dragged task's own ancestors is allowed — that is just moving it up a level — and dropping it back onto the row that is already its parent does nothing, since there is nothing to change. Releasing over a refused row does nothing either way. After a successful drop a notification offers **Undo**, which removes the dependency or moves the subtask back, respectively.

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

**In the web app** relations are visible offline on any task page you have opened online (they are cached with the task), but **adding and removing them requires a connection** — they are not part of the small set of writes the offline outbox queues.

Completing a blocked task is refused offline too: the check runs against the cached task before the operation is queued, so you get an immediate "unavailable offline" rather than a task that looks done and then bounces back. One consequence to be aware of: completing a *blocker* while offline does not release its dependents until you reconnect, because the offline cache has no way to know which tasks that blocker was holding back. See [docs/offline.md](offline.md).

**In the native Android client** the whole relation graph is on the device, so relations behave the same with or without a connection: links are added and removed offline and queued like any other write, and completing or cancelling a blocker releases its dependents immediately — the padlocks clear on the spot rather than on reconnect. The three refusals above are made on the device as well, before anything is queued, so a link the app turns down never becomes a request that fails later. See [docs/mobile.md](mobile.md).

## Interaction with other features

- **Deleting a task** removes its relations along with it, so deleting a blocker releases whatever it was blocking.
- **Recurring tasks** are subject to the rule like any other: a blocked recurring task is refused rather than silently advanced to its next occurrence.
- **Duplicate, Decompose and recurrence history snapshots do not copy relations.** Cloning a dependency graph is ambiguous, so new tasks start with none.
- **Backup** includes relations, and restoring preserves their identifiers.
- **Troiki** is unaffected: relations do not grant or consume slot capacity.

## API

Two write endpoints, both answering with the updated task, plus one dry-run read:

```
POST   /api/v1/tasks/:id/relations
DELETE /api/v1/tasks/:id/relations/:relationId
GET    /api/v1/tasks/:id/relations/blocker-check?candidates=1,2,3
```

`blocker-check` writes nothing: it says which candidates could not be made blockers of the task (already blocking it, or it would close a loop). The web app asks it once when a subtask drag starts; the native Android client answers the same question from its own copy of the relation graph, with no network.

There is no separate endpoint for listing relations. Every task carries `blockedByCount` and `relationCount` on all endpoints that return tasks, and the full list arrives inline via `GET /api/v1/tasks/:id?relations=true`. Completing a blocked task answers `409` with the code `task_blocked` and the blocker ids in `details.blockerIds`.

See [API.md → Task Relations](../API.md#task-relations) for request bodies, error cases and examples.

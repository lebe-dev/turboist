# Public View

Public View is a screenshot-friendly mode that hides everything you've explicitly marked as private from the UI.

## When to use

You're about to share a screenshot of Turboist with someone and want to make sure no sensitive project, task, or label shows up. Toggle Public View on, take the screenshot, toggle it back off — nothing is deleted.

## Marking entities as private

Three entities can be marked private: **projects**, **tasks**, and **labels**. Each carries an `is_private` flag.

- Open the entity's actions menu (⋯ button on the project / task / label).
- Pick **Mark as private**. Pick **Mark as public** to undo.

When Public View is **off**, private entities are still visible — a small `LockSimple` icon next to the title indicates the private flag.

## Toggling Public View

Settings → **Privacy** tab → **Public view** switch. The setting is per-user, persisted on the server (`users.settings.publicView`).

## What gets hidden when Public View is on

| Action | Result |
|---|---|
| Project marked private | The project disappears from the sidebar, all listings, pinned section. **All its tasks are also hidden** (cascade). |
| Task marked private | The task disappears from every view. **All its subtasks are also hidden** (cascade). |
| Label marked private | The label disappears from the sidebar. Tasks tagged with it remain visible — labels are tags, not parents. |

Direct links to a private entity (`/project/:id`, `/task/:id`, `/label/:id`) redirect to `/today` with a toast.

## How it works

- **Backend** always returns `isPrivate` on each entity DTO and `publicView` in `/api/v1/settings`. There is **no server-side filtering** — the API stays uniform.
- **Frontend** filters at render time via `lib/utils/visibility.ts` (`isProjectVisible`, `isLabelVisible`, `isTaskVisible`). The task helper walks up `projectId` and the parent-task chain to honour the cascade rules.
- Toggling the setting re-renders instantly; nothing is re-fetched.

## Persistence

| Field | Where | Type |
|---|---|---|
| `is_private` | `projects`, `tasks`, `labels` tables | `INTEGER NOT NULL DEFAULT 0` |
| `publicView` | `users.settings` JSON | `bool` |

Migrations: `012_projects_is_private.sql`, `013_tasks_is_private.sql`, `014_labels_is_private.sql`.

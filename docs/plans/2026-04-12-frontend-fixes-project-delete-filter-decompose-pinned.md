# Frontend Fixes: Project Delete, Filter Leak, Decompose Dialog, Pinned Sort

## Overview

Four frontend improvements: (1) fix project page not updating after task deletion, (2) fix filter state leaking from All Tasks to Troiki page, (3) add date/priority quick buttons to DecomposeDialog, (4) sort pinned tasks by priority.

## Context

- Files involved:
  - `frontend/src/lib/stores/project-tasks.svelte.ts` - project tasks store (HTTP-only, no WS)
  - `frontend/src/lib/components/TaskItem.svelte` - task component with delete/decompose handlers
  - `frontend/src/lib/components/DecomposeDialog.svelte` - bulk subtask creation dialog
  - `frontend/src/lib/components/ContextSwitcher.svelte` - sidebar with pinned tasks display
  - `frontend/src/lib/stores/label-filter.svelte.ts` - global label filter store
  - `frontend/src/lib/api/types.ts` - TypeScript API types
  - `frontend/src/lib/api/backend.ts` - backend connector interface
  - `frontend/src/lib/api/default-backend.ts` - default HTTP backend
  - `frontend/src/lib/api/mock-backend.ts` - mock backend for tests
  - `internal/handler/tasks.go` - Go handler for decompose endpoint
  - `internal/todoist/cache.go` - cache DecomposeTask method
  - `internal/todoist/client.go` - Todoist API DecomposeTask method
  - `internal/todoist/testing.go` - noopCacheClient signature
  - `internal/todoist/cache_test.go` - mockCacheClient signature
  - `frontend/src/routes/troiki/+page.svelte` - Troiki page
  - `frontend/src/routes/+page.svelte` - main page with All Tasks filter
- Related patterns: optimistic removal via `pendingRemovals` in `tasksStore`, existing date/priority UI in `TaskDetailPanel.svelte` and `CreateTaskDialog.svelte`

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Follow existing store and component patterns (Svelte 5 runes, factory stores)
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Fix project page not updating after task deletion

**Files:**
- Modify: `frontend/src/lib/stores/project-tasks.svelte.ts`
- Modify: `frontend/src/lib/components/TaskItem.svelte`

- [x] Add `removeTaskLocal(id: string)` method to `projectTasksStore` that filters the task out of `flatTasks`
- [x] In `TaskItem.handleDelete()`, also call `projectTasksStore.removeTaskLocal(taskId)` (safe no-op if task isn't in the store)
- [x] In `TaskItem.handleComplete()`, also call `projectTasksStore.removeTaskLocal(taskId)`
- [x] Write tests: verify `removeTaskLocal` removes a task from `getProjectTasks` result, verify no-op when task ID doesn't exist
- [x] Run `just test-frontend` - must pass before task 2

### Task 2: Fix filter leak from All Tasks to Troiki page

**Files:**
- Modify: `frontend/src/routes/troiki/+page.svelte`
- Modify: `frontend/src/lib/stores/label-filter.svelte.ts`

The most likely cause: `labelFilterStore.activeLabel` is global singleton state that persists across route navigation. When a label filter is active on the All Tasks page, navigating to Troiki doesn't clear it. The Troiki page itself uses its own data source (troiki WS channel), but the global label filter state persists and could affect the main page's $effect when returning.

- [x] Investigate the exact reproduction path: set a label filter on All Tasks, navigate to Troiki, observe the behavior
- [x] In `troiki/+page.svelte` onMount, clear the labelFilterStore: `labelFilterStore.clear()`
- [x] Verify the All Tasks page's $effect correctly resets filters when returning from Troiki (the existing code at line 166-190 handles this, but confirm it works with the label filter cleared)
- [x] Write test: verify that labelFilterStore is cleared when entering Troiki page
- [x] Run `just test-frontend` - must pass before task 3

### Task 3: Add date and priority buttons to DecomposeDialog

**Files:**
- Modify: `frontend/src/lib/components/DecomposeDialog.svelte`
- Modify: `frontend/src/lib/components/TaskItem.svelte` (handleDecompose callback)
- Modify: `frontend/src/lib/api/types.ts` (DecomposeTaskRequest)
- Modify: `frontend/src/lib/api/mock-backend.ts` (update mock)
- Modify: `internal/handler/tasks.go` (decomposeTaskRequest struct + handler)
- Modify: `internal/todoist/cache.go` (DecomposeTask signature)
- Modify: `internal/todoist/client.go` (DecomposeTask - apply overrides)
- Modify: `internal/todoist/testing.go` (noopCacheClient signature)
- Modify: `internal/todoist/cache_test.go` (mockCacheClient signature)

Backend changes:
- [x] Add `Priority *int` and `DueDate *string` fields to `decomposeTaskRequest` in `tasks.go`
- [x] Update `Decompose` handler to pass priority/due_date overrides to `cache.DecomposeTask`
- [x] Add `DecomposeOpts` struct (Priority *int, DueDate *string) to cache/client, update `DecomposeTask` signatures in cache.go, client.go, testing.go, cache_test.go
- [x] In `client.DecomposeTask`, when opts.Priority is set, use it instead of `src.Priority`; when opts.DueDate is set, use `map[string]any{"date": *opts.DueDate}` instead of `src.Due`
- [x] Write Go test: handler test verifying priority/due_date fields are passed through

Frontend changes:
- [x] Add `priority` and `due_date` optional fields to `DecomposeTaskRequest` in `types.ts`
- [x] Add `dueDate` and `priority` state variables to `DecomposeDialog.svelte`
- [x] Add Today/Tomorrow toggle buttons (follow pattern from TaskDetailPanel lines 1198-1214) below the textarea
- [x] Add priority picker buttons (P1-P4, follow pattern from TaskDetailPanel lines 1215-1240) next to date buttons
- [x] Update `onConfirm` prop type to pass `{ tasks, priority?, due_date? }`
- [x] Update `TaskItem.handleDecompose` to pass priority/due_date from the dialog to the API call
- [x] Reset dueDate and priority state when dialog opens
- [x] Update i18n: add any new keys to both `en.json` and `ru.json`
- [x] Update mock-backend if needed
- [x] Run `just test` and `just test-frontend` - must pass before task 4

### Task 4: Sort pinned tasks by priority

**Files:**
- Modify: `frontend/src/lib/components/ContextSwitcher.svelte`

- [x] Add a `$derived` that sorts `pinnedStore.items` by priority descending, using `pinnedPriority()` helper (which already exists at line 31): `const sortedPinned = $derived([...pinnedStore.items].sort((a, b) => (pinnedPriority(b) ?? 1) - (pinnedPriority(a) ?? 1)))`
- [x] Replace `{#each pinnedStore.items as pinned (pinned.id)}` with `{#each sortedPinned as pinned (pinned.id)}` at line 103
- [x] Write test: verify pinned tasks are rendered in priority order (P1 first, P4 last)
- [x] Run `just test-frontend` - must pass before task 5

### Task 5: Verify acceptance criteria

- [x] Run full test suite: `just test-all`
- [x] Run linter: `just lint`

### Task 6: Update documentation

- [x] Update CLAUDE.md if internal patterns changed (e.g., if DecomposeOpts pattern is notable) — no update needed, DecomposeOpts follows existing TaskAddArgs/TaskUpdateArgs convention

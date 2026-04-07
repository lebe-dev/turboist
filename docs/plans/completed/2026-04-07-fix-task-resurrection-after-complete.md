---

# Fix task resurrection after complete/delete

## Overview

Completed/deleted tasks reappear after ~10-15 seconds due to a two-layer timing bug: the frontend's optimistic removal (`pendingRemovals`) gets cleaned up too early, and the backend's eviction grace period (15s) expires before the next background poll cycle can confirm the task is gone from Todoist.

## Root Cause

The bug is a race condition across two protection layers:

1. **Frontend** (`pendingRemovals` cleanup is too aggressive): When the user completes a task, `removeTaskLocal(id)` adds the ID to `pendingRemovals`. The first WS delta (from `evictTask`) removes it from `flatTasks`. On the next getter call, `applyPendingRemovals()` sees the ID is no longer in the tree and **deletes it from pendingRemovals** (line 60-61 of tasks.svelte.ts). This leaves no frontend protection against subsequent upserts.
2. **Backend** (`evictGracePeriod` < `poll_interval`): `evictGracePeriod=15s` but the default `poll_interval=30s`. After 15s the eviction entry expires. The next poller refresh (at ~20-30s) fetches from Todoist — for recurring tasks the same ID returns with a new due date, and `filterEvicted` no longer suppresses it. The delta upserts the task back to the client.

The combination: frontend protection lost at ~0.2s (first delta), backend protection lost at ~15s, task reappears on next refresh after 15s.

## Context

- Files involved:
  - `frontend/src/lib/stores/tasks.svelte.ts` — pendingRemovals cleanup logic
  - `internal/todoist/cache.go` — evictGracePeriod constant, filterEvicted logic
  - `internal/todoist/cache_test.go` — existing eviction tests
  - `frontend/src/lib/stores/tasks.test.ts` — existing store tests

## Development Approach

- **Testing approach**: TDD — write failing tests that reproduce the race condition first, then fix
- Complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Frontend — make pendingRemovals time-based

**Files:**
- Modify: `frontend/src/lib/stores/tasks.svelte.ts`
- Modify: `frontend/src/lib/stores/tasks.test.ts`

- [x] Write test: after removeTaskLocal + delta removing task from flatTasks + subsequent delta upserting the same task within 30s, the task should remain hidden
- [x] Write test: after removeTaskLocal + 30s+ elapsed, a delta upserting the task should show it (recurring task next occurrence)
- [x] Change `pendingRemovals` from `Set<string>` to `Map<string, number>` storing removal timestamp
- [x] In `applyPendingRemovals()`: instead of deleting entries when task is absent from tree, delete entries only when `Date.now() - timestamp > PENDING_REMOVAL_GRACE_MS` (30 seconds)
- [x] In `handleTasksDelta` upsert section (lines 117-123): only clear pendingRemovals for tasks whose grace period has expired (allows recurring task next occurrence to appear after grace period)
- [x] Run frontend tests: `just test-frontend`

### Task 2: Backend — increase evictGracePeriod to exceed poll_interval

**Files:**
- Modify: `internal/todoist/cache.go`
- Modify: `internal/todoist/cache_test.go`

- [x] Write test: eviction grace period should survive at least one full poll cycle (filterEvicted should suppress a task for longer than the default poll interval)
- [x] Change `evictGracePeriod` from `15 * time.Second` to `45 * time.Second` — ensures coverage across at least one full 30s poll cycle with margin
- [x] Update existing eviction tests if they depend on the old 15s constant
- [x] Run backend tests: `just test`

### Task 3: Verify acceptance criteria

- [x] Run full test suite: `just test-all`
- [x] Run linter: `just lint`

### Task 4: Update documentation

- [x] Move this plan to `docs/plans/completed/`

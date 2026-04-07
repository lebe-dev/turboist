# Remove offline-first and PWA from frontend

## Overview

Strip all offline-first infrastructure (Yjs persistence, IndexedDB action queue, QueuedBackend, PWA/service worker) from the frontend, making the app online-first. No backend changes needed — the API is already thematic (separate endpoints for config, tasks, state, WS).

## Context

- Files to delete: ~15 files across lib/state/, lib/sync/, lib/pwa/, lib/api/, lib/components/
- Files to modify: ~15 files (stores, layout, settings, vite config, package.json, i18n, docs)
- No backend changes: GET /api/config, task endpoints, PATCH /api/state, and WebSocket channels already cover all needs
- FlatTask model (flat storage + buildTree on read) stays — it's a useful reactive pattern independent of Yjs

## Development Approach

- **Testing approach**: Regular (code first, then tests). Existing tests that mock deleted modules will be updated/removed.
- Complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Relocate FlatTask utilities

FlatTask, buildTree, flattenTasks etc. currently live in lib/state/types.ts (Yjs-specific dir). Move to lib/utils/ since they're independently useful for the reactive store model.

**Files:**
- Create: `frontend/src/lib/utils/task-tree.ts`
- Delete: `frontend/src/lib/state/types.ts`
- Modify: all files importing from `$lib/state/types` (~6 files)

- [ ] Create `frontend/src/lib/utils/task-tree.ts` with FlatTask interface + taskToFlat, flatToTask, flattenTasks, buildTree functions (copy from lib/state/types.ts)
- [ ] Update imports in: tasks.svelte.ts, planning.svelte.ts, project-tasks.svelte.ts, and test files
- [ ] Delete `frontend/src/lib/state/types.ts`
- [ ] Run `just test-frontend` — must pass

### Task 2: Remove PWA plugin and components

Remove service worker, manifest config, install/update banners.

**Files:**
- Modify: `frontend/vite.config.ts`
- Delete: `frontend/src/lib/pwa/install.svelte.ts`, `frontend/src/lib/pwa/update.svelte.ts`
- Delete: `frontend/src/lib/components/UpdateBanner.svelte`, `frontend/src/lib/components/InstallBanner.svelte`
- Modify: `frontend/src/routes/+layout.svelte`
- Modify: `frontend/package.json`

- [ ] Remove SvelteKitPWA plugin from vite.config.ts (import + plugin usage + manifest config)
- [ ] Delete `frontend/src/lib/pwa/` directory
- [ ] Delete `UpdateBanner.svelte` and `InstallBanner.svelte`
- [ ] Remove `<UpdateBanner />` and `<InstallBanner />` from +layout.svelte
- [ ] Remove `@vite-pwa/sveltekit` and `vite-plugin-pwa` from package.json, run yarn
- [ ] Run `just test-frontend` — must pass

### Task 3: Remove offline infrastructure files

Delete the offline queue, IndexedDB layer, Yjs persistence, connectivity check, QueuedBackend, and ConnectionIndicator.

**Files:**
- Delete: `frontend/src/lib/sync/action-queue.svelte.ts`, `frontend/src/lib/sync/action-queue.test.ts`
- Delete: `frontend/src/lib/sync/db.ts`
- Delete: `frontend/src/lib/sync/connectivity.ts`, `frontend/src/lib/sync/connectivity.test.ts`
- Delete: `frontend/src/lib/api/queued-backend.ts`, `frontend/src/lib/api/queued-backend.test.ts`
- Delete: `frontend/src/lib/api/flush-before-read.test.ts`
- Delete: `frontend/src/lib/state/index.svelte.ts`, `frontend/src/lib/state/index.test.ts`
- Delete: `frontend/src/lib/components/ConnectionIndicator.svelte`
- Modify: `frontend/src/lib/api/backend.ts` — remove setBackend/getBackend, export DefaultBackendConnector directly

- [ ] Delete all files listed above
- [ ] Simplify `backend.ts`: remove `setBackend()`/`getBackend()` dynamic swap; export `DefaultBackendConnector` instance directly (keep `BackendConnector` interface for mock tests)
- [ ] Run `just test-frontend` to identify remaining broken imports (expected — stores will be fixed in Task 4)

### Task 4: Simplify stores and app initialization for online-first

Remove all persistence calls, offline state, action queue overlay logic from stores.

**Files:**
- Modify: `frontend/src/lib/stores/app.svelte.ts`
- Modify: `frontend/src/lib/stores/tasks.svelte.ts`
- Modify: `frontend/src/lib/stores/planning.svelte.ts`
- Modify: `frontend/src/lib/stores/project-tasks.svelte.ts`
- Modify: `frontend/src/lib/stores/collapsed.svelte.ts`
- Modify: `frontend/src/lib/stores/sections.svelte.ts`
- Modify: `frontend/src/lib/stores/pinned.svelte.ts`
- Modify: `frontend/src/lib/stores/contexts.svelte.ts`
- Modify: `frontend/src/lib/stores/sidebar.svelte.ts`

app.svelte.ts:
- [ ] Remove imports: QueuedBackend, actionQueue, initState, destroyState, loadPersistedUI, saveAppConfig, loadAppConfig
- [ ] Remove QueuedBackend wrapping — use DefaultBackendConnector directly via backend.ts export
- [ ] Remove actionQueue.init(), initState(), auto-flush setup, flushNow()
- [ ] Remove IDB config fallback (cfg = await loadAppConfig) — fail fast if API unreachable
- [ ] Remove loadPersistedUI() call in hydrateFromConfig — init collapsedStore with server state directly
- [ ] Remove destroyState() from destroy()

tasks.svelte.ts:
- [ ] Remove imports: actionQueue, loadCompletedTasks, saveCompletedTasks, all lib/state imports
- [ ] Remove OFFLINE_GRACE_MS, isOffline state, offlineTimer, cancelOfflineTimer, scheduleOfflineCheck
- [ ] Remove pendingUpdateMap(), overlayUpdate(), walkWithUpdates(), applyPendingUpdates() — these overlaid queued mutation data
- [ ] Remove captureTempTasks(), reinjectTempTasks() — these preserved temp tasks across snapshots for queued creates
- [ ] Remove persistTasks/persistMeta calls from snapshot/delta handlers and local mutation methods
- [ ] Remove cached tasks loading from start() (loadPersistedTasks)
- [ ] Simplify fetchCompleted: remove IDB cache fallback (throw on error)
- [ ] Remove offline/reconnect logic from wsClient.onStateChange handler
- [ ] Keep: flatTasks model, buildTree in getter, pendingRemovals (useful for optimistic deletes), reconciledIds (for temp->real ID mapping), isStale indicator
- [ ] Remove isOffline getter from return object

planning.svelte.ts:
- [ ] Remove imports from lib/state
- [ ] Remove all persistTasks/persistMeta calls

project-tasks.svelte.ts:
- [ ] Remove imports from lib/state
- [ ] Remove loadFromCache() and loadPersistedTasks/persistTasks calls
- [ ] Simplify start() — just fetchFromServer(), no cache

collapsed.svelte.ts, sections.svelte.ts, pinned.svelte.ts, contexts.svelte.ts, sidebar.svelte.ts:
- [ ] Remove isStateReady and persistUI imports and calls from each
- [ ] Update existing store tests (app.test.ts, tasks.test.ts) — remove mocks for deleted modules
- [ ] Run `just test-frontend` — must pass

### Task 5: Clean up UI components, i18n, and settings

**Files:**
- Modify: `frontend/src/routes/settings/+page.svelte`
- Modify: `frontend/src/lib/components/TaskDetailPanel.svelte`
- Modify: `frontend/src/routes/+layout.svelte`
- Modify: `frontend/src/lib/components/Sidebar.svelte`
- Modify: `frontend/locales/en.json`, `frontend/locales/ru.json`

- [ ] Settings page: remove actionQueue import and "Pending Actions" section (queue management UI)
- [ ] TaskDetailPanel: remove actionQueue import and flushNow() call — subtask creation works without queue flush since mutations go directly to server now
- [ ] +layout.svelte: remove ConnectionIndicator import and usage (in mobile header)
- [ ] Sidebar.svelte: remove ConnectionIndicator import and usage
- [ ] Remove i18n keys from en.json and ru.json: all `pwa.*` keys, all `connectivity.*` keys
- [ ] Run `just test-frontend` — must pass

### Task 6: Remove npm dependencies and update documentation

**Files:**
- Modify: `frontend/package.json`
- Modify: `CLAUDE.md`
- Modify: `frontend/.claude/rules/frontend-api.md` (or `.claude/rules/frontend-api.md`)
- Modify: `.claude/rules/svelte-stores.md`
- Delete: `.claude/rules/frontend-state.md`
- Modify: `docs/architecture/frontend.md` (if exists)

- [ ] Remove from package.json: `yjs`, `y-indexeddb`, `idb`
- [ ] Run `cd frontend && yarn` to update lockfile
- [ ] Update `.claude/rules/frontend-api.md`: remove QueuedBackend, offline queue, IndexedDB stores sections
- [ ] Update `.claude/rules/svelte-stores.md`: remove Y.Doc persistence section, update init pattern description
- [ ] Delete `.claude/rules/frontend-state.md` (entire file was about Yjs persistence)
- [ ] Update `CLAUDE.md`: remove "offline-first via Yjs + IndexedDB" from frontend description, update data flow description, remove sync/state references
- [ ] Update `docs/architecture/frontend.md` if it exists

### Task 7: Verify acceptance criteria

- [ ] Run full test suite: `just test-all`
- [ ] Run linter: `just lint`
- [ ] Verify no remaining imports from deleted modules: grep for `lib/state`, `lib/sync`, `lib/pwa`, `queued-backend`, `action-queue`, `ConnectionIndicator`
- [ ] Verify no orphaned i18n keys referencing `pwa.*` or `connectivity.*`

### Task 8: Update documentation

- [ ] Update README.md if user-facing changes
- [ ] Update CLAUDE.md if internal patterns changed (covered in Task 6)
- [ ] Move this plan to `docs/plans/completed/`

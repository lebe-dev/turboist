# Constraints Feature

## Overview

Implement a system of self-imposed limitations: label blocking, daily self-discipline constraints, day part caps, priority floor, and postpone budget. Backend adds config parsing, SQLite storage, and new API endpoints. Frontend adds a constraints store, selection dialog, banner, and enforcement UI across TaskItem and DayPartTaskList.

## Context

- Files involved:
  - Backend: `internal/config/config.go`, `internal/storage/`, `internal/handler/`, `internal/server/server.go`, `cmd/turboist/main.go`
  - Frontend: `frontend/src/lib/stores/`, `frontend/src/lib/api/`, `frontend/src/lib/components/`, `frontend/src/routes/+page.svelte`, `frontend/src/routes/settings/+page.svelte`
  - Config: `config.example.yml`
  - i18n: `frontend/locales/en.json`, `frontend/locales/ru.json`
- Related patterns: Config loaded via yamlFile intermediate struct then mapped to AppConfig. Storage uses embedded SQL migrations (NNN_create_xxx.sql). Handlers: one struct per domain, constructor injection, Fiber v3. Stores: Svelte 5 rune singletons. State persisted via PATCH /api/state fire-and-forget. Config delivered to frontend via GET /api/config response.
- Dependencies: No new external dependencies needed.

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Follow existing patterns exactly: config yamlFile mapping, storage migration naming, handler struct style, store singleton pattern
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Config Parsing and Storage Foundation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.example.yml`
- Create: `internal/storage/migrations/005_create_constraints.sql`
- Create: `internal/storage/constraints.go`

- [x] Add `ConstraintsConfig` struct to config.go with all sub-feature fields: `Enabled bool`, `LabelBlocks []LabelBlockConfig`, `Daily DailyConstraintsConfig`, `DayPartCaps []DayPartCapConfig`, `PriorityFloor int`, `PostponeBudget int`
- [x] Add `LabelBlockConfig` (Label string, Duration time.Duration), `DailyConstraintsConfig` (MaxConstraints int, MaxRerolls int), `DayPartCapConfig` (Label string, MaxTasks int)
- [x] Add corresponding yamlFile fields and mapping logic in ParseAppConfig with defaults (Enabled=true, PriorityFloor=4, PostponeBudget=0, MaxConstraints=3, MaxRerolls=2)
- [x] Parse label block durations from strings like "14d" — add day-suffix parsing since time.ParseDuration does not support "d"
- [x] Create migration 005_create_constraints.sql: `constraints_label_blocks` table (label TEXT PK, started_at TEXT NOT NULL)
- [x] Create storage/constraints.go with methods: `GetLabelBlocks()`, `UpsertLabelBlock(label, startedAt)`, `DeleteLabelBlock(label)`, `DeleteExpiredLabelBlocks(labels []string)` — for syncing config entries with DB
- [x] Add storage methods for daily constraints state via user_state: helpers to get/set `constraint_pool`, `daily_constraints`, `postpone_budget` JSON keys
- [x] Update config.example.yml with the full constraints section
- [x] Write tests for config parsing (duration "14d" parsing, defaults, edge cases like PriorityFloor=4, PostponeBudget=0)
- [x] Write tests for storage methods (label block CRUD, user_state JSON round-trip)
- [x] Run project test suite — must pass before task 2

### Task 2: Label Blocking Backend

**Files:**
- Modify: `internal/handler/config.go`
- Modify: `cmd/turboist/main.go`

- [x] On startup in main.go: after storage init and config load, sync label blocks — for each configured label_block, upsert started_at if not present; delete DB entries for labels no longer in config; delete expired entries
- [x] Extend `appConfigResponse` in config handler with a `Constraints` field containing: `Enabled bool`, `LabelBlocks []labelBlockStatus` (label, remaining_seconds int), `DayPartCaps`, `PriorityFloor`, `PostponeBudget`, `PostponeBudgetUsed`
- [x] In Config handler, compute label block status: load blocks from storage, compare now vs started_at + duration, return remaining seconds (skip expired ones)
- [x] Load postpone budget state from user_state, check date matches today, return used count (or 0 if date mismatch = new day)
- [x] Write tests for config handler response including constraints block
- [x] Run project test suite — must pass before task 3

### Task 3: Daily Constraints Backend

**Files:**
- Create: `internal/handler/constraints.go`
- Modify: `internal/handler/state.go`
- Modify: `internal/server/server.go`
- Modify: `cmd/turboist/main.go`

- [x] Create ConstraintsHandler struct with storage and config deps
- [x] Implement `GET /api/constraints/daily`: read `daily_constraints` from user_state, if date != today or missing return `{ needs_selection: true, pool_size: N }`; otherwise return `{ needs_selection: false, items: [...], rerolls_used: N, max_rerolls: N }`
- [x] Implement `POST /api/constraints/daily/roll`: read constraint_pool, randomly pick max_constraints items, save to daily_constraints with today's date and rerolls_used incremented (reject if rerolls_used >= max_rerolls with 400). On first roll of the day (no existing entry or different date), do not count as a reroll
- [x] Implement `POST /api/constraints/daily/swap`: accept `{ index: N }`, replace that item with a random one not in current selection, increment rerolls_used (reject if exhausted)
- [x] Implement `POST /api/constraints/daily/confirm`: set a `confirmed` flag in daily_constraints JSON
- [x] Extend state handler PATCH to accept `constraint_pool` key (JSON array of strings)
- [x] Register routes in server.go: `GET /api/constraints/daily`, `POST /api/constraints/daily/roll`, `POST /api/constraints/daily/swap`, `POST /api/constraints/daily/confirm`
- [x] Wire ConstraintsHandler in main.go
- [x] Write tests for all 4 endpoints (happy path, reroll exhaustion, swap out-of-bounds, confirm idempotency, date rollover)
- [x] Write test for state handler accepting constraint_pool
- [x] Run project test suite — must pass before task 4

### Task 4: Postpone Budget Backend Enforcement

**Files:**
- Modify: `internal/handler/tasks.go`

- [x] In task Update handler, where postpone is detected (due date moved forward): load postpone_budget from user_state, check date matches today, if used >= config.PostponeBudget (and PostponeBudget > 0), return 400 with error "Daily postpone limit reached"
- [x] After successful postpone, increment used count in postpone_budget state
- [x] Write tests: postpone allowed when under budget, rejected when exhausted, budget resets on new day, unlimited when PostponeBudget=0
- [x] Run project test suite — must pass before task 5

### Task 5: Frontend API Types and Constraints Store

**Files:**
- Modify: `frontend/src/lib/api/types.ts`
- Modify: `frontend/src/lib/api/backend.ts`
- Modify: `frontend/src/lib/api/default-backend.ts`
- Modify: `frontend/src/lib/api/client.ts`
- Create: `frontend/src/lib/stores/constraints.svelte.ts`
- Modify: `frontend/src/lib/stores/app.svelte.ts`

- [x] Add constraints types to types.ts: `ConstraintsConfig` (enabled, label_blocks, day_part_caps, priority_floor, postpone_budget, postpone_budget_used), `LabelBlockStatus` (label, remaining_seconds), `DayPartCap` (label, max_tasks), `DailyConstraintsResponse` (needs_selection, items, rerolls_used, max_rerolls, pool_size)
- [x] Extend AppConfig type with `constraints: ConstraintsConfig`
- [x] Add BackendConnector methods: `getDailyConstraints()`, `rollDailyConstraints()`, `swapDailyConstraint(index)`, `confirmDailyConstraints()`
- [x] Implement in DefaultBackendConnector and export from client.ts
- [x] Create constraintsStore with $state fields: `enabled`, `labelBlocks` (Map<string, number> — label to remaining seconds), `dailyConstraints` (items array + needs_selection + rerolls_used + max_rerolls), `postponeBudget` ({ limit, used }), `dayPartCaps` (Map<string, number> — label to max), `priorityFloor` (number)
- [x] Add methods: `init(config)`, `isLabelBlocked(labels: string[]): boolean`, `isPostponeExhausted(): boolean`, `isPriorityBelowFloor(priority: number): boolean`, `getDayPartCap(label: string): number | null`
- [x] Hydrate constraintsStore from appStore.hydrateFromConfig() using config.constraints
- [x] Add i18n strings to en.json and ru.json for all constraint-related UI text (blocked label tooltip, priority floor tooltip, postpone limit tooltip, day part cap messages, daily constraints dialog labels, banner text)
- [x] Write tests for constraintsStore methods (isLabelBlocked with multiple labels, priority floor edge cases, postpone budget logic)
- [x] Run frontend test suite — must pass before task 6

### Task 6: Label Blocking and Priority Floor Frontend

**Files:**
- Modify: `frontend/src/lib/components/TaskItem.svelte`
- Modify: `frontend/src/lib/components/TaskDropdownMenu.svelte`

- [x] In TaskItem: when any of task's labels is blocked, apply dimmed style (opacity-60) and show lock icon next to label badges
- [x] In TaskDropdownMenu: accept new props `labelBlocked` and `priorityBlocked`. When labelBlocked: disable Today and Tomorrow buttons (greyed out with tooltip showing remaining days). When priorityBlocked: disable only Today button (tooltip: "Priority too low")
- [x] In TaskItem: compute `labelBlocked` via `constraintsStore.isLabelBlocked(task.labels)` and `priorityBlocked` via `constraintsStore.isPriorityBelowFloor(task.priority)`, pass to TaskDropdownMenu
- [x] Write tests for TaskDropdownMenu disabled states
- [x] Run frontend test suite — must pass before task 7

### Task 7: Daily Constraints Frontend (Dialog + Banner + Settings)

**Files:**
- Create: `frontend/src/lib/components/DailyConstraintsDialog.svelte`
- Create: `frontend/src/lib/components/DailyConstraintsBanner.svelte`
- Modify: `frontend/src/routes/+page.svelte` (Today page)
- Modify: `frontend/src/routes/settings/+page.svelte`

- [x] Create DailyConstraintsDialog: modal that shows randomly picked constraints, "Re-roll all" button (shows remaining count), individual "Swap" buttons per item, "Confirm" button to lock selection. Calls constraintsStore methods which delegate to API. Opens automatically when constraintsStore.dailyConstraints.needs_selection is true on Today view
- [x] Create DailyConstraintsBanner: sticky red/rose banner (bg-red-500/10 border-red-500/30) showing confirmed constraints as bulleted list. Not dismissible. Visible only on Today view when constraints are confirmed
- [x] In +page.svelte (Today page): on mount/view-change to today, fetch daily constraints status. If needs_selection and pool_size > 0, show DailyConstraintsDialog. Show DailyConstraintsBanner above existing banner when constraints are confirmed
- [x] In settings/+page.svelte: add "Daily Constraints" section with text input + "Add" button to manage constraint pool. List existing items with delete buttons. Save via patchState({ constraint_pool: [...] })
- [x] Write tests for DailyConstraintsDialog interaction logic (roll, swap, confirm)
- [x] Run frontend test suite — must pass before task 8

### Task 8: Postpone Budget and Day Part Caps Frontend

**Files:**
- Modify: `frontend/src/lib/components/TaskDropdownMenu.svelte`
- Modify: `frontend/src/lib/components/DayPartTaskList.svelte`
- Modify: `frontend/src/routes/+page.svelte`

- [x] Postpone budget indicator: in +page.svelte Today view, show a small badge near task list header area: "Postpones: N/M". When exhausted, badge turns red. Use constraintsStore.postponeBudget state
- [x] In TaskDropdownMenu: when postpone is exhausted (constraintsStore.isPostponeExhausted()), disable date picker / forward-date buttons with tooltip "Daily postpone limit reached". Only apply on Today view tasks
- [x] In DayPartTaskList: read day part caps from constraintsStore.getDayPartCap(label). Show progress indicator in section header: "N/M" with visual fill. When full: progress bar turns red, disable "+" button, tooltip "Day part limit reached"
- [x] Disable move-to-day-part in dropdown when target day part is at cap
- [x] Write tests for day part cap enforcement and postpone budget UI state
- [x] Run frontend test suite — must pass before task 9

### Task 9: Verify Acceptance Criteria

- [x] Run full backend test suite (`just test`)
- [x] Run full frontend test suite (`just test-frontend`)
- [x] Run linter (`just lint`)

### Task 10: Update Documentation

- [x] Update config.example.yml with constraints section (already done in task 1, verify completeness)
- [x] Update CLAUDE.md: add constraints handler to architecture section, mention constraints store, document new API endpoints
- [x] Move this plan to `docs/plans/completed/`

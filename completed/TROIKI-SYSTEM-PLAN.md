# Plan: Troiki System

## Overview
Add a new productivity feature — Troiki System — that organizes tasks into 3 priority classes (Important, Medium, Rest) within a dedicated Todoist project. Each class holds max 3 root tasks. A token-based unlock chain controls replenishment of lower classes.

## Validation Commands
- `just test`
- `just test-frontend`
- `just lint`

---

### Task 1: Backend Config
- [x] Add `TroikiConfig` and `TroikiSectionsConfig` structs to `internal/config/config.go`
- [x] Add `TroikiSystem TroikiConfig` field to `AppConfig`
- [x] Add YAML parsing in `yamlFile` with tag `troiki_system`
- [x] Apply defaults: `enabled=false`, `max_tasks_per_section=3`, section names: "Важное"/"Среднее"/"Остальное"
- [x] Add config test cases in `internal/config/config_test.go`
- [x] Document in `config.example.yml`
- [x] Mark completed

Config struct:
```go
type TroikiSectionsConfig struct {
    Important string `yaml:"important"` // default "Важное"
    Medium    string `yaml:"medium"`    // default "Среднее"
    Rest      string `yaml:"rest"`      // default "Остальное"
}

type TroikiConfig struct {
    Enabled            bool                 `yaml:"enabled"`
    ProjectName        string               `yaml:"project_name"`
    Sections           TroikiSectionsConfig `yaml:"sections"`
    MaxTasksPerSection int                  `yaml:"max_tasks_per_section"` // default 3
}
```

**Files:** `internal/config/config.go`, `internal/config/config_test.go`, `config.example.yml`

---

### Task 2: Storage — Token Persistence
- [x] Create migration `internal/storage/migrations/004_create_troiki_tokens.sql`
- [x] Add token methods to `internal/storage/troiki.go` (new file)
- [x] Add tests in `internal/storage/troiki_test.go`
- [x] Mark completed

Migration:
```sql
CREATE TABLE IF NOT EXISTS troiki_capacity (
    section_class TEXT NOT NULL PRIMARY KEY,  -- 'medium' or 'rest'
    capacity INTEGER NOT NULL DEFAULT 0       -- earned capacity, only grows
);
```

Store methods on `*Store`:
```go
func (s *Store) GetAllTroikiCapacity() (map[string]int, error)
func (s *Store) IncrementTroikiCapacity(sectionClass string) error // +1, capped at max
```

Note: No "spend" method. Capacity only grows (earned by completing upstream tasks). Deletion frees a slot naturally (task count decreases), no capacity change needed.

**Files:** `internal/storage/migrations/004_create_troiki_tokens.sql`, `internal/storage/troiki.go`, `internal/storage/troiki_test.go`

---

### Task 3: Todoist Client — AddSection
- [x] Add `AddSection(ctx, name, projectID) (string, error)` to `internal/todoist/client.go`
- [x] Add corresponding wrapper to `internal/todoist/cache.go`
- [x] Mark completed

Uses Todoist Sync API `section_add` command, similar to existing `AddTask`.

**Files:** `internal/todoist/client.go`, `internal/todoist/cache.go`

---

### Task 4: Troiki Domain Service
- [x] Create `internal/troiki/` package
- [x] Define `SectionClass` constants: `Important`, `Medium`, `Rest`
- [x] Implement `Service` struct with `Init`, `ComputeState`, `CanAddTask`, `OnTaskCompleted`, `AddTask`
- [x] Write comprehensive tests in `internal/troiki/troiki_test.go`
- [x] Mark completed

Core types:
```go
type SectionClass string
const (
    Important SectionClass = "important"
    Medium    SectionClass = "medium"
    Rest      SectionClass = "rest"
)

type SectionState struct {
    Class     SectionClass    `json:"class"`
    SectionID string          `json:"section_id"`
    Name      string          `json:"name"`
    Tasks     []*todoist.Task `json:"tasks"`
    RootCount int             `json:"root_count"`
    MaxTasks  int             `json:"max_tasks"`
    Capacity  int             `json:"capacity"`  // earned capacity (Important always = max)
    CanAdd    bool            `json:"can_add"`
}

type State struct {
    ProjectID string         `json:"project_id"`
    Sections  []SectionState `json:"sections"` // [Important, Medium, Rest]
}
```

**Capacity model** (not token-spend):
- `capacity` = total slots ever unlocked for a section (only grows, never decremented)
- Important: capacity is always `maxTasks` (always open)
- Medium/Rest: capacity starts at 0, increases by 1 each time an upstream task is completed
- `CanAdd = rootCount < min(capacity, maxTasks)`
- Deleting a task frees the slot (rootCount decreases) — can add replacement without needing new capacity
- Completing a task frees the slot AND increases downstream capacity

Key logic:
- **`Init(ctx)`**: Resolves project by name from cache → finds/creates 3 sections → stores resolved IDs
- **`ComputeState()`**: Gets tasks from cache filtered by troiki project → groups by section → counts roots (ParentID==nil) → reads capacity from SQLite → computes `CanAdd`
- **`CanAddTask(class)`**: Important: `rootCount < max`. Medium/Rest: `rootCount < min(capacity, max)`
- **`OnTaskCompleted(task)`**: Important task → `IncrementCapacity("medium")`. Medium task → `IncrementCapacity("rest")`. Rest task → no-op
- **`AddTask(ctx, class, content, description)`**: Validates CanAdd → calls `cache.AddTask` with resolved projectID + sectionID (no token spending)

Capacity chain: Important → Medium → Rest (linear, each class unlocks the next)

Test cases:
- `TestCanAddTask_ImportantOpen`, `_ImportantFull`
- `TestCanAddTask_MediumWithCapacity`, `_MediumNoCapacity`, `_MediumFull`
- `TestOnCompleted_ImportantUnlocksMedium`, `_MediumUnlocksRest`, `_RestNoOp`
- `TestAddTask_NoCapacitySpent`, `_SubtasksNotCounted`
- `TestCapacityAccumulation`
- `TestDeleteFreesSlot_NoCapacityChange`

**Files:** `internal/troiki/troiki.go`, `internal/troiki/troiki_test.go`

---

### Task 5: Backend Handler
- [x] Create `internal/handler/troiki.go` with `TroikiHandler`
- [x] `GET /api/troiki` — returns `State` via `service.ComputeState()`
- [x] `POST /api/troiki/tasks` — creates task with enforcement (409 if limit/no tokens)
- [x] Hook into `TasksHandler.Complete` for token generation on troiki task completion
- [x] Add tests
- [x] Mark completed

Endpoints:
- **`GET /api/troiki`** → `service.ComputeState()` → JSON response
- **`POST /api/troiki/tasks`** → body: `{ "section_class": "important", "content": "...", "description": "..." }` → `service.AddTask()` → 201 or 409 (no capacity is spent, only slot availability checked)

Completion hook in `internal/handler/tasks.go`:
```go
func (h *TasksHandler) Complete(c fiber.Ctx) error {
    id := c.Params("id")
    // BEFORE completing: look up task to know its section
    task := h.cache.TaskByID(id)
    if err := h.cache.CompleteTask(c.Context(), id); err != nil {
        return todoistErrorResponse(c, err)
    }
    // AFTER completing: check if troiki task → increase downstream capacity
    if h.troikiService != nil && task != nil {
        h.troikiService.OnTaskCompleted(task)
    }
    return c.SendStatus(fiber.StatusOK)
}
```

Add `troikiService *troiki.Service` field to `TasksHandler` (nil when disabled). Extend `NewTasksHandler` with optional param.

**Files:** `internal/handler/troiki.go`, `internal/handler/troiki_test.go`, `internal/handler/tasks.go`

---

### Task 6: WebSocket — Troiki Channel
- [x] Add `ChannelTroiki = "troiki"` to `internal/ws/protocol.go`
- [x] Add `TroikiSubscription`, `troikiSub`, `lastTroikiSnap` to `internal/ws/client.go`
- [x] Handle subscribe/unsubscribe for troiki channel
- [x] Add `broadcastTroiki` to `internal/ws/hub.go` (follows planning channel pattern)
- [x] Add troiki service reference to Hub
- [x] Mark completed

The Hub needs a `troikiService` field. `broadcastTroiki` calls `service.ComputeState()`, computes delta vs previous snapshot, sends snapshot or delta. Follows the exact pattern of `broadcastPlanning`.

**Files:** `internal/ws/protocol.go`, `internal/ws/client.go`, `internal/ws/hub.go`

---

### Task 7: Server Wiring + main.go
- [x] Register troiki routes in `internal/server/server.go` (conditional on `troiki.Enabled`)
- [x] Wire troiki service creation and `Init()` in `cmd/turboist/main.go`
- [x] Pass troiki service to `NewTasksHandler`, `NewHub`, route registration
- [x] Mark completed

In `main.go` after cache warmup:
```go
var troikiService *troiki.Service
if cfg.App.TroikiSystem.Enabled {
    troikiService = troiki.NewService(cache, cfg.App.TroikiSystem, store)
    if err := troikiService.Init(ctx); err != nil {
        log.Fatal("troiki init failed", "err", err)
    }
}
```

**Files:** `cmd/turboist/main.go`, `internal/server/server.go`

---

### Task 8: Config Handler — Expose Troiki Config
- [x] Add `troikiConfigResponse` to `internal/handler/config.go`
- [x] Include in `appConfigResponse`
- [x] Mark completed

```go
type troikiConfigResponse struct {
    Enabled            bool   `json:"enabled"`
    ProjectID          string `json:"project_id,omitempty"`
    ProjectName        string `json:"project_name,omitempty"`
    MaxTasksPerSection int    `json:"max_tasks_per_section,omitempty"`
}
```

**Files:** `internal/handler/config.go`

---

### Task 9: Frontend Types + API Layer
- [ ] Add `TroikiState`, `TroikiSectionState`, `TroikiConfig`, `CreateTroikiTaskRequest` to `frontend/src/lib/api/types.ts`
- [ ] Add `getTroikiState()`, `createTroikiTask()` to `BackendConnector` interface and implementations
- [ ] Mark completed

**Files:** `frontend/src/lib/api/types.ts`, `frontend/src/lib/api/backend.ts`, `frontend/src/lib/api/default-backend.ts`, `frontend/src/lib/api/client.ts`

---

### Task 10: Frontend WS — Troiki Channel
- [ ] Add troiki channel to WS types in `frontend/src/lib/ws/types.ts`
- [ ] Update `client.svelte.ts` to support `troiki` channel subscribe/unsubscribe
- [ ] Mark completed

**Files:** `frontend/src/lib/ws/types.ts`, `frontend/src/lib/ws/client.svelte.ts`

---

### Task 11: Frontend Troiki Store
- [ ] Create `frontend/src/lib/stores/troiki.svelte.ts`
- [ ] `enter()` subscribes to WS troiki channel, `exit()` unsubscribes
- [ ] Handle snapshot (full replace) and delta (update changed sections)
- [ ] `addTask(sectionClass, content, description)` calls API
- [ ] Expose `sections`, `loading`, `active` via getters
- [ ] Mark completed

Follows the planning store pattern: factory function → singleton with `$state` → WS-driven updates.

**Files:** `frontend/src/lib/stores/troiki.svelte.ts`

---

### Task 12: Frontend App Store — Troiki Config
- [ ] Add `troikiConfig` state to `app.svelte.ts`
- [ ] Hydrate from `cfg.troiki` in `hydrateFromConfig()`
- [ ] Expose `troikiEnabled` and `troikiConfig` getters
- [ ] Mark completed

**Files:** `frontend/src/lib/stores/app.svelte.ts`

---

### Task 13: Frontend — Troiki Page
- [ ] Create `frontend/src/routes/troiki/+page.svelte`
- [ ] 3 card layout: Important / Medium / Rest (stacked mobile, side-by-side desktop)
- [ ] Each card: section name, task list (TaskItem), slot counter (N/3), token badge (Medium/Rest)
- [ ] Inline add-task input (disabled when `can_add=false` with explanation)
- [ ] Call `troikiStore.enter()` on mount, `troikiStore.exit()` on destroy
- [ ] Mark completed

Layout: Vertical list of 3 sections, each separated by a horizontal divider with section name and task counter (`N/3`):

```
──────── Важное (2/3) ────────
  [ ] Task 1
  [ ] Task 2
  [+ Add task]

──────── Среднее (1/3) ── capacity: 2 ────────
  [ ] Task 1
  [+ Add task]

──────── Остальное (0/3) ── 🔒 locked ────────
  [Complete a task in Среднее to unlock]
```

- Each section: horizontal divider line with section name centered + counter badge (`N/3`)
- Medium/Rest: capacity badge next to counter (e.g., "capacity: 2/3")
- Task list: reuse `TaskItem` component
- Inline add-task input (disabled when `can_add=false` with explanation text)
- Disabled state shows: "Complete a task in [Previous Section] to unlock"

**Files:** `frontend/src/routes/troiki/+page.svelte`

---

### Task 14: Frontend — Sidebar Integration
- [ ] Add troiki link to `ContextSwitcher.svelte` as first item in PROJECTS section
- [ ] Conditional on `appStore.troikiEnabled`
- [ ] Use `Layers3` icon from lucide
- [ ] Mark completed

```svelte
{#if appStore.troikiEnabled}
    <a href="/troiki" class="...">
        <Layers3Icon class="h-4 w-4 shrink-0 opacity-60" />
        {#if !collapsed}
            <span class="truncate">{$t('troiki.title')}</span>
        {/if}
    </a>
{/if}
```

**Files:** `frontend/src/lib/components/ContextSwitcher.svelte`

---

### Task 15: i18n
- [ ] Add troiki strings to `frontend/locales/en.json`
- [ ] Add troiki strings to `frontend/locales/ru.json`
- [ ] Mark completed

Keys: `troiki.title`, `troiki.important`, `troiki.medium`, `troiki.rest`, `troiki.capacity`, `troiki.slotsUsed`, `troiki.sectionLocked`, `troiki.sectionFull`, `troiki.addTask`, `troiki.alwaysOpen`

**Files:** `frontend/locales/en.json`, `frontend/locales/ru.json`

---

### Task 16: Integration Testing + Lint
- [ ] Run `just test` — all Go tests pass
- [ ] Run `just test-frontend` — all frontend tests pass
- [ ] Run `just lint` — no lint errors
- [ ] Manual test: enable troiki in config, verify sections auto-created, add tasks, complete tasks, verify token flow
- [ ] Mark completed

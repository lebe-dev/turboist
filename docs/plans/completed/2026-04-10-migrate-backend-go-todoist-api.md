# Migrate backend to github.com/lebe-dev/go-todoist-api v0.1.0

## Overview

Replace CnTeng/todoist-api-go (Sync API-centric) with lebe-dev/go-todoist-api (REST API + Sync API). Use REST API for single-entity mutations and completed tasks. Keep Sync API for data fetching (full/incremental sync) and batch operations. Define internal args types to eliminate external library type leakage from handlers and troiki.

## Context

- Files involved:
  - `internal/todoist/client.go` — complete rewrite (REST for single ops, Sync for batch/fetch)
  - `internal/todoist/models.go` — add internal args types, sync response structs, update conversions
  - `internal/todoist/sync_handler.go` — DELETE (sync token moves into client)
  - `internal/todoist/cache.go` — update cacheClient interface and type references
  - `internal/handler/tasks.go` — replace synctodoist types with internal types
  - `internal/troiki/troiki.go` — replace synctodoist types with internal types
  - `internal/todoist/models_test.go` — update for new sync response types
  - `internal/todoist/cache_test.go` — update mock and arg types
  - `internal/todoist/sync_handler_test.go` — DELETE (replaced by sync token tests in client)
  - `internal/handler/troiki_test.go` — update mock types
  - `internal/troiki/troiki_test.go` — update mock types
  - `go.mod` / `go.sum` — swap dependency
- Related patterns: cacheClient interface, hand-written mocks, conversion functions (TaskFromSync, etc.)
- Dependencies: `github.com/lebe-dev/go-todoist-api v0.1.0` (replaces `github.com/CnTeng/todoist-api-go v0.2.4`)

## Key architectural decisions

- **REST API for single mutations**: AddTask, UpdateTask, DeleteTask, CloseTask (handles both recurring and non-recurring), MoveTask, MoveTaskToProject, AddSection. Simpler code, typed responses, no sync token interaction.
- **Sync API for data fetching**: FetchAll (sync_token=*), FetchIncremental (stored token). Essential for incremental sync architecture.
- **Sync API for batch operations**: DecomposeTask, SetTasksLabels, BatchMoveTasksToProject, BatchMoveTasks. Efficiency — single request for N operations.
- **Internal args types**: Define `TaskAddArgs` and `TaskUpdateArgs` in internal/todoist package. Eliminates external library leakage into handlers/troiki. Due dates use simple string fields instead of nested struct.
- **Sync token as client field**: Replace sync_handler.go with a mutex-protected field on Client. Token only advances on data sync responses, not command responses (same behavior as before, but simpler implementation).
- **Sync response parsing**: Define internal structs (`syncItem`, `syncProject`, etc.) to unmarshal raw JSON from SyncResponse fields.

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Define internal types and sync response structs

**Files:**
- Modify: `internal/todoist/models.go`
- Modify: `internal/todoist/models_test.go`

- [x] Add `TaskAddArgs` struct (Content, Description, ProjectID, SectionID, ParentID, Labels, Priority, DueDate string, DueString string, DueLang string) — replaces leaked `synctodoist.TaskAddArgs`
- [x] Add `TaskUpdateArgs` struct (ID, Content *string, Description *string, Labels []string, Priority *int, DueDate *string, DueString *string, DueLang *string) — replaces leaked `synctodoist.TaskUpdateArgs`
- [x] Add unexported sync response types for JSON parsing: `syncItem` (with id, content, description, project_id, section_id, parent_id, labels, priority, due, checked, is_deleted, added_at, completed_at), `syncProject` (id, name, inbox_project, is_deleted, is_archived), `syncSection` (id, name, project_id, is_deleted), `syncLabel` (id, name, is_deleted), `syncDue` (date string, is_recurring bool, string, lang)
- [x] Update `TaskFromSync` to accept `*syncItem` instead of `*sync.Task`
- [x] Update `ProjectFromSync` to accept `*syncProject` instead of `*sync.Project`
- [x] Update `SectionFromSync` to accept `*syncSection` instead of `*sync.Section`
- [x] Update `LabelFromSync` to accept `*syncLabel` instead of `*sync.Label`
- [x] Update models_test.go: rewrite TestTaskFromSync_* tests to use `syncItem` and `syncDue` instead of `sync.Task` and `sync.Due`
- [x] Run `just test models` — must pass

### Task 2: Rewrite client.go with new library

**Files:**
- Modify: `internal/todoist/client.go`
- Delete: `internal/todoist/sync_handler.go`
- Delete: `internal/todoist/sync_handler_test.go`

- [x] Add dependency: `go get github.com/lebe-dev/go-todoist-api@v0.1.0`
- [x] Replace Client struct: store `*todoist.Client` (lebe-dev), sync token (`string`) with mutex protection
- [x] Rewrite `NewClient(apiKey)`: create lebe-dev client, initialize sync token to `"*"`
- [x] Rewrite `FetchAll`: call `client.Sync()` with token=`"*"` and resource types [items, projects, sections, labels], unmarshal raw JSON fields into internal sync types, update stored sync token, return `*SyncResult`
- [x] Rewrite `FetchIncremental`: call `client.Sync()` with stored token, detect full_sync flag, parse delta or full result, update sync token only for data syncs (no SyncStatus entries)
- [x] Add `parseSyncResponse(*todoist.SyncResponse) (*SyncResult, error)` — unmarshal Items/Projects/Sections/Labels from raw JSON, filter deleted/checked
- [x] Rewrite `AddTask` using REST: convert internal `TaskAddArgs` to `todoist.AddTaskArgs`, call `client.AddTask()`, return task.ID
- [x] Rewrite `UpdateTask` using REST: convert internal `TaskUpdateArgs` to `todoist.UpdateTaskArgs`, call `client.UpdateTask()`
- [x] Rewrite `CompleteTask` using REST: call `client.CloseTask(id)` — REST close handles both recurring and non-recurring
- [x] Remove `CloseTask` method — unified into `CompleteTask` via REST close endpoint
- [x] Rewrite `DeleteTask` using REST: call `client.DeleteTask(id)`
- [x] Rewrite `MoveTask` using REST: call `client.MoveTask(id, &todoist.MoveTaskArgs{ParentID: &parentID})`
- [x] Rewrite `MoveTaskToProject` using REST: call `client.MoveTask(id, &todoist.MoveTaskArgs{ProjectID: &projectID})`
- [x] Rewrite `AddSection` using REST: call `client.AddSection(&todoist.AddSectionArgs{...})`, return section.ID
- [x] Rewrite `FetchCompletedTasks` using REST: call `client.GetCompletedTasksByCompletionDate()` with appropriate args
- [x] Rewrite `FetchCompletedBySection` using REST: same API with project/section filters
- [x] Rewrite `FetchCompletedSubtasks` using REST: filter by parent_id from completed tasks
- [x] Rewrite `SetTasksLabels` using Sync API: build SyncCommands with `item_update` type and labels args, call `client.Sync()`
- [x] Rewrite `DecomposeTask` using Sync API: build batch SyncCommands (item_add * N + item_delete), call `client.Sync()`
- [x] Rewrite `BatchMoveTasksToProject` using Sync API: build batch SyncCommands with `item_move`, call `client.Sync()`
- [x] Rewrite `BatchMoveTasks` using Sync API: build batch SyncCommands with `item_move`, call `client.Sync()`
- [x] Update `IsRateLimited`: adapt to new library's error types (check for `TodoistRequestError` with HTTP 429)
- [x] Delete `internal/todoist/sync_handler.go` and `internal/todoist/sync_handler_test.go`
- [x] Run `just test` — must compile (tests may fail pending cache/handler updates)

### Task 3: Update cache.go and its tests

**Files:**
- Modify: `internal/todoist/cache.go`
- Modify: `internal/todoist/cache_test.go`

- [x] Update `cacheClient` interface: replace `*synctodoist.TaskAddArgs` with `*TaskAddArgs`, replace `*synctodoist.TaskUpdateArgs` with `*TaskUpdateArgs`, remove `CloseTask` method (unified into CompleteTask)
- [x] Update `Cache.AddTask` signature: `(ctx, args *TaskAddArgs) (string, error)`
- [x] Update `Cache.UpdateTask` signature: `(ctx, args *TaskUpdateArgs) error`
- [x] Simplify `Cache.CompleteTask`: remove recurring/non-recurring branching — just call `client.CompleteTask(id)` which now handles both via REST close
- [x] Remove `synctodoist` import from cache.go
- [x] Update `mockCacheClient` in cache_test.go: match new interface (TaskAddArgs, TaskUpdateArgs, no CloseTask)
- [x] Update all test functions to use internal types instead of synctodoist types
- [x] Run `just test cache` — must pass

### Task 4: Update handlers and troiki

**Files:**
- Modify: `internal/handler/tasks.go`
- Modify: `internal/troiki/troiki.go`
- Modify: `internal/handler/troiki_test.go`
- Modify: `internal/troiki/troiki_test.go`

- [x] In handler/tasks.go: remove `synctodoist` import, replace `&synctodoist.TaskAddArgs{...}` with `&todoist.TaskAddArgs{...}`, replace `&synctodoist.TaskUpdateArgs{...}` with `&todoist.TaskUpdateArgs{...}`
- [x] Update due date handling in handler Create: replace `args.Due = &synctodoist.Due{Date: &dueDate}` with `args.DueDate = dueDate.Format("2006-01-02")`
- [x] Update due date handling in handler Update: replace nested Due struct patterns with string fields (DueDate, DueString, DueLang)
- [x] Update due date handling in handler Duplicate: same pattern as Create
- [x] In troiki/troiki.go: remove `synctodoist` import, update cache interface type, replace `&synctodoist.TaskAddArgs{...}` with `&todoist.TaskAddArgs{...}`
- [x] In handler/troiki_test.go: remove `synctodoist` import, update mock to use `todoist.TaskAddArgs`
- [x] In troiki/troiki_test.go: remove `synctodoist` import, update mock to use `todoist.TaskAddArgs`
- [x] Run `just test` — all tests must pass

### Task 5: Dependency cleanup and full verification

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [x] Run `go mod tidy` to remove CnTeng/todoist-api-go and clean up
- [x] Verify no remaining imports of CnTeng: `rg 'CnTeng' --type go`
- [x] Run `just test` — full test suite must pass
- [x] Run `just lint` — must pass
- [x] Run `just build` — must compile

### Task 6: Update documentation

- [x] Update CLAUDE.md if any internal patterns changed (e.g., new args types, removed sync_handler)
- [x] Move this plan to `docs/plans/completed/`

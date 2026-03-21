# Frontend Architecture

Svelte 5 + SvelteKit 2 SPA (static adapter, no SSR). TailwindCSS 4 + bits-ui for UI.

## Layer Overview

```
┌─────────────────────────────────┐
│  Routes / Components (Svelte 5) │
├─────────────────────────────────┤
│  Stores ($state singletons)     │
├──────────┬──────────────────────┤
│ WS Client│  API Layer           │
│ (real-   │  DefaultBackend      │
│  time)   │  → QueuedBackend     │
│          │  → ActionQueue (IDB) │
├──────────┴──────────────────────┤
│  Yjs Y.Doc + y-indexeddb        │
│  (offline persistence)          │
└─────────────────────────────────┘
```

## Data Flow

1. Backend polls Todoist API → in-memory cache → broadcasts via WebSocket
2. WS delivers **snapshot** (full task list) or **delta** (upserted + removed IDs)
3. Stores update `$state` + persist to Yjs Y.Doc (y-indexeddb auto-saves)
4. On page reload: Y.Doc loads from IndexedDB → stores show cached data instantly → WS delivers fresh data

Mutations: component → `QueuedBackend` → `actionQueue` (IndexedDB) → flush via HTTP → backend → Todoist. Optimistic UI via `$state` overlays.

## Directory Structure

```
src/
├── routes/              # SvelteKit pages (main, task detail, settings, login)
├── lib/
│   ├── api/             # Backend interface + implementations
│   │   ├── backend.ts       # BackendConnector interface
│   │   ├── default-backend  # HTTP implementation
│   │   ├── queued-backend   # Offline queue decorator
│   │   └── client.ts        # Thin delegation wrapper
│   ├── state/           # Yjs persistence layer
│   │   ├── index.svelte.ts  # Y.Doc + y-indexeddb lifecycle + persist/load helpers
│   │   └── types.ts         # FlatTask type + tree conversion (flattenTasks, buildTree)
│   ├── stores/          # Svelte 5 rune-based stores
│   │   ├── app            # Init orchestration
│   │   ├── tasks          # WS-driven task state + optimistic mutations
│   │   ├── planning       # Weekly/backlog planning (separate WS channel)
│   │   ├── contexts       # Active context + view selection
│   │   ├── pinned         # Pinned task list
│   │   ├── collapsed      # Collapsed subtree IDs
│   │   └── sidebar        # Sidebar toggle
│   ├── sync/            # Offline mutation queue
│   │   ├── action-queue     # IndexedDB-backed FIFO queue with coalescing
│   │   └── db.ts            # IDB schema (actionQueue, completedTasksCache, appConfig)
│   ├── ws/              # WebSocket client
│   │   ├── client.svelte.ts # Auto-reconnect, resubscribe, ping/pong
│   │   ├── types.ts         # Protocol: subscribe, snapshot, delta, ping
│   │   └── merge.ts         # Tree merge utilities
│   └── components/      # ~22 domain + ~52 UI primitives (bits-ui based)
```

## Flat Task Model

Tasks arrive as trees (`children: Task[]`) from the backend. Stored flat for CRDT compatibility:

```
FlatTask: { id, content, parent_id, due_date, due_recurring, labels[], ... }
```

- `flattenTasks(tree)` → flat array (depth-first)
- `buildTree(flat)` → tree (group by `parent_id`, orphans become roots)
- Conversion runs in store getters on every read — O(n), fast for typical task counts

## Offline Persistence (Yjs + y-indexeddb)

Y.Doc stores task arrays and UI state. y-indexeddb auto-persists to IndexedDB.

```
Y.Doc
├── Y.Array 'tasks'          # FlatTask[]
├── Y.Array 'backlogTasks'   # FlatTask[]
├── Y.Array 'weeklyTasks'    # FlatTask[]
├── Y.Map   'meta'           # context, weekly/backlog limits + counts
├── Y.Map   'planningMeta'   # same shape for planning view
└── Y.Map   'ui'             # active_context_id, active_view, sidebar, pinned, collapsed
```

Stores own reactive `$state`; Y.Doc is write-behind persistence only. Foundation for future y-websocket sync when backend supports Yjs protocol.

## Init Sequence

```
appStore.init()
  1. DefaultBackend → QueuedBackend setup
  2. actionQueue.init()          — load pending mutations from IDB
  3. migrateLocalStorage()       — one-time legacy migration
  4. initState()                 — Y.Doc + y-indexeddb, await IDB sync
  5. getAppConfig()              — API fetch (IDB fallback)
  6. hydrateFromConfig()         — init all child stores
  7. wsClient.connect()
  8. tasksStore.start()          — load from Y.Doc cache, subscribe WS
  9. actionQueue.startAutoFlush()
 10. actionQueue.flushNow()      — replay pending mutations
```

## WebSocket Protocol

Two channels: `tasks` (main view) and `planning` (weekly/backlog).

| Direction | Type | Payload |
|-----------|------|---------|
| Client → Server | `subscribe` | `{ channel, view?, context? }` |
| Client → Server | `unsubscribe` | `{ channel }` |
| Server → Client | `snapshot` | Full task list + meta |
| Server → Client | `delta` | `{ upserted: Task[], removed: string[], meta }` |
| Both | `ping/pong` | Keep-alive |

## Optimistic Mutation Strategy

- **Remove**: `pendingRemovals` Set overlay — tasks filtered out in getter, cleared when server confirms
- **Update**: `pendingUpdates` from actionQueue overlaid on top of server data in getter
- **Add/Insert**: directly modifies `$state` flat array + persists to Y.Doc

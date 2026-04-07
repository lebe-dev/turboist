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
│  time)   │  (HTTP)              │
└──────────┴──────────────────────┘
```

## Data Flow

1. Backend polls Todoist API → in-memory cache → broadcasts via WebSocket
2. WS delivers **snapshot** (full task list) or **delta** (upserted + removed IDs)
3. Stores update `$state` reactively
4. On page reload: WS delivers fresh data after reconnect

Mutations: component → `DefaultBackend` (HTTP) → backend → Todoist. Optimistic UI via `$state` updates.

## Directory Structure

```
src/
├── routes/              # SvelteKit pages (main, task detail, settings, login)
├── lib/
│   ├── api/             # Backend interface + implementations
│   │   ├── backend.ts       # BackendConnector interface + DefaultBackend
│   │   └── client.ts        # Thin delegation wrapper
│   ├── utils/           # Shared utilities
│   │   └── task-tree.ts     # FlatTask type + tree conversion (flattenTasks, buildTree)
│   ├── stores/          # Svelte 5 rune-based stores
│   │   ├── app            # Init orchestration
│   │   ├── tasks          # WS-driven task state + optimistic mutations
│   │   ├── planning       # Weekly/backlog planning (separate WS channel)
│   │   ├── contexts       # Active context + view selection
│   │   ├── pinned         # Pinned task list
│   │   ├── collapsed      # Collapsed subtree IDs
│   │   └── sidebar        # Sidebar toggle
│   ├── ws/              # WebSocket client
│   │   ├── client.svelte.ts # Auto-reconnect, resubscribe, ping/pong
│   │   ├── types.ts         # Protocol: subscribe, snapshot, delta, ping
│   │   └── merge.ts         # Tree merge utilities
│   └── components/      # ~22 domain + ~52 UI primitives (bits-ui based)
```

## Flat Task Model

Tasks arrive as trees (`children: Task[]`) from the backend. Stored flat for efficient reactive updates:

```
FlatTask: { id, content, parent_id, due_date, due_recurring, labels[], ... }
```

- `flattenTasks(tree)` → flat array (depth-first)
- `buildTree(flat)` → tree (group by `parent_id`, orphans become roots)
- Conversion runs in store getters on every read — O(n), fast for typical task counts

## Init Sequence

```
appStore.init()
  1. getAppConfig()              — API fetch
  2. hydrateFromConfig()         — init all child stores
  3. wsClient.connect()
  4. tasksStore.start()          — subscribe WS
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
- **Add/Insert**: directly modifies `$state` flat array
- **Update**: modifies `$state` flat array directly

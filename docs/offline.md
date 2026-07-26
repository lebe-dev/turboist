# Offline support

Turboist keeps working when the network drops. This is **Strategy 1** — a read-through
cache plus a small semantic outbox layered around the `ApiClient` — not a full offline
database. It ships in one place (`frontend/src/lib/offline/`) and, because that code sits
in the shared JS layer, it works identically on all three shells: the web app, the
installed web-PWA, and the native iOS/Android apps.

> The idempotency guarantees the outbox relies on are documented on the backend side:
> see [API.md](../API.md#idempotency) and
> [docs/architecture/backend.md](architecture/backend.md#idempotency).

## What works offline

1. **Reading previously-viewed screens.** Every screen you opened while online
   (today / tomorrow / week / inbox / projects / labels / contexts / search / …) reopens
   without a network connection and shows the last known state. An offline banner appears
   with the "data as of {time}" timestamp. The background cache warmer additionally
   pre-fetches the primary sidebar destinations even if you never opened them —
   today / tomorrow / week / **next-week (backlog)** / inbox / **the Troiki board** — plus
   the board (sections + tasks) of every **pinned project**, so those open offline out of
   the box. The projects, labels, contexts and pinned-task lists are not warmed
   separately: they arrive inside the `/api/v1/config` bootstrap aggregate, which
   write-throughs into the same cache on every boot. A non-pinned project
   still only opens offline once you have visited it online; an unvisited one shows a clear
   "no connection" message rather than a raw fetch error.
2. **A small whitelist of writes** — queued locally and replayed when the network returns:
   - `task.complete` — complete a task,
   - `task.uncomplete` — undo a completion,
   - `task.createInbox` — quick-add a task to the Inbox.
3. **Offline session.** If the server is unreachable at launch, the app renders from the
   cache instead of bouncing you to `/login`.

Everything else (moving/reordering tasks, editing fields, pinning, project/label/context
mutations, and any task that was *itself* created offline and not yet synced) is **not**
available offline — attempting it shows an "Unavailable offline" toast and the UI does not
change.

## How it works

The whole data layer (cache + outbox) lives in `frontend/src/lib/offline/` and wraps the
`ApiClient`, so it is platform-shared:

- **Read-through cache.** GET responses are cached in IndexedDB (`db.ts`, `readCache.ts`).
  On a network error the client serves the cached copy (cache-first with a background
  probe) and flips the offline banner; a cache miss surfaces a clear "needs connection"
  error rather than an infinite spinner. `GET /api/v1/tasks/:id` (the task detail page)
  has one extra fallback: when its own response was never cached, the payload is rebuilt
  from the task as it appears in any cached list, with its subtasks gathered by
  `parentId` — so any task visible offline can also be opened.
- **Where a response keeps its tasks.** Cache entries are keyed by request path, and each
  entry stores that path. `readCache.ts` uses it to pick the right extractor per endpoint
  (`EXTRACTORS`), because response shapes are a property of the endpoint, not something
  inferable from the payload: `/api/v1/config` keeps tasks under `pinnedTasks` and, two
  levels down, under `troiki.*.projects[].tasks`; `/stats/sidebar` under `pinned.items`;
  `/stats/week-summary` under `completed`; list views under `items`. An unregistered path
  falls back to probing every key any known shape uses, so an entry written by an older
  build stays traversable. Getting this wrong is silent — the task simply cannot be found
  offline, and an offline complete of it does not survive a restart, because the cache
  patcher never rewrites that entry.
- **Schema version.** `SCHEMA_VERSION` in `db.ts` guards the *shape* of cached values
  (`DB_VERSION` guards the object stores). On a mismatch the cached responses are wiped
  and incompatible outbox rows are quarantined; pending ops at the current version
  survive. Bump it whenever a cached payload gains a field the new code reads
  unconditionally — v1.15 did, when `/api/v1/config` grew `harpoon` and `taskTemplates`.
  The cost is one cold-cache launch after upgrading.
- **Semantic outbox.** A whitelisted mutation issued offline is matched to one of the three
  ops, synthesized into an optimistic response, applied to the cache (so it survives a
  restart), and enqueued (`outbox.ts`, `ops/`). The replay engine drains the queue FIFO
  when the network returns.
- **Idempotency.** Every mutation carries an `Idempotency-Key` header. On replay the *same*
  key is reused, so a request whose response was lost never creates a duplicate — the
  backend returns its stored response (`X-Idempotent-Replay: true`). See the backend docs
  linked above.
- **Status + banner.** `status.svelte.ts` tracks online/offline, the pending-op count, and
  the last-sync time; `OfflineBanner.svelte` renders it and offers a manual **Retry**.
- **Warm cache / offline session.** `warmCache.ts` and the auth boot path let the app open
  from cache when the server is unreachable.

### Graceful degradation

The offline bridge is a progressive enhancement. When IndexedDB is unavailable (e.g.
private-mode Safari) the bridge degrades to a **no-op** and the app behaves exactly like
the purely-online build — reads hit the network, offline writes are not queued.

## Native shells

`CapacitorHttp` routes `fetch`/XHR through the native HTTP stack, so on iOS/Android
requests **do not pass through the service worker**. That is why the cache and outbox live
in the JS layer — they work on native automatically. The service worker is used **only** to
deliver the web shell and never touches `/api/*`. See [docs/mobile.md](mobile.md) for the
native specifics.

## Known limitations (FEATURE-OFFLINE-ARCH.md §5.4)

- **Safari / iOS ITP eviction.** Cache Storage and IndexedDB may be evicted after ~7 days
  of not using the site. This degrades to "needs connection", not a crash. Installing the
  PWA to the home screen softens it; native apps are not affected.
- **No Background Sync.** The Background Sync API is not used at all (it does not exist on
  iOS). Replay runs **only while the app is open/foregrounded** — it kicks on app start, on
  network recovery, and on tab focus, never in the background.
- **Offline-created tasks are locked.** A task created offline holds a temporary
  client-side id until it syncs; complete/uncomplete on it is blocked ("Unavailable
  offline") until it has a real server id.

## Offline — manual acceptance checklist (FEATURE-OFFLINE-ARCH.md §6)

Automated unit/integration tests cover the internals of every scenario below
(`frontend/src/lib/offline/*.test.ts`, `frontend/src/lib/api/client.test.ts`,
`internal/httpapi/handlers/idempotency_test.go`). This checklist is the manual
end-to-end pass across the real UI and the three shells (web browser, installed
web-PWA, native iOS/Android). There is no Playwright/E2E harness — run this by hand.

Legend: **Web** = desktop/mobile browser · **PWA** = installed web app (home screen) ·
**Native** = iOS/Android Capacitor build (airplane mode).

Toggling "offline":
- Web/PWA: DevTools → Network → **Offline**, or the OS network toggle.
- Native: enable **Airplane mode** on the device/simulator (do NOT just kill Wi-Fi in a way
  that leaves a captive portal — true airplane mode is the honest test).

Before each run: be logged in and online once so the cache is warm.

---

### 1. Read offline (§6.1)
- [ ] Open **Today** while online (let it load fully).
- [ ] Go offline.
- [ ] Reload / relaunch the app.
- [ ] **Today renders from cache** — no infinite spinner.
- [ ] The **offline banner** is visible and shows the "data as of {time}" timestamp.
- [ ] Navigate to a project you visited online → it renders from cache.
- [ ] Navigate to a **pinned project** you never opened this session → it still renders
      from cache (its board is warmed automatically).
- [ ] Open **Next week** and the **Troiki board** without having visited them online this
      session → both render from cache (backlog/week and the Troiki view are warmed).
- [ ] Open a **task page** for a task visible in a cached list — including a task on the
      **Troiki board** — but never opened online → it renders from cache (title, fields,
      subtasks), not a "fetch failed" toast.
- [ ] Navigate to a non-pinned project you did **not** visit online → a clear localized
      "no connection" message appears (NOT a raw "Load failed", NOT an infinite spinner,
      NOT a blank screen).
- [ ] Go back online → banner clears, a background refetch pulls live data.

### 2. Complete a task offline, survive restart (§6.2)
- [ ] Go offline.
- [ ] Complete a task → the UI shows it completed **immediately**.
- [ ] The banner's pending counter shows **"1 change waiting to be sent"**.
- [ ] Relaunch the app **while still offline** → the task is **still completed**
      (served from the patched cache).
- [ ] Repeat with a **pinned** task, and with a task on the **Troiki board**: both live
      only inside the `/api/v1/config` aggregate, so they exercise the path→extractor
      registry. Relaunch offline → still completed, in the sidebar and on the board.
- [ ] Go back online → auto-replay fires; the pending counter drops to 0.
- [ ] The banner clears and SSE/refetch reconciles server truth.
- [ ] On a **second device**, the task shows completed.
- [ ] Server DB has **exactly one** completion (no duplicate) — verify via a
      second-device view or API.

### 3. Lost response on the wire (§6.3)
> Hard to reproduce by hand; primarily proven by automated tests
> (`outbox.replay.test.ts` + `idempotency_test.go`). To attempt manually:
- [ ] Go offline, complete a task (it queues with an `Idempotency-Key`).
- [ ] Using DevTools request-throttling / a proxy, allow the replay request to reach
      the server but drop the **response**.
- [ ] Trigger another replay (toggle online, or the banner's **Retry**).
- [ ] The server returns its stored response (`X-Idempotent-Replay: true`) — verify in
      the Network tab.
- [ ] **No duplicate** task/completion is created on the server.

### 4. Conflict — deleted elsewhere (§6.4)
- [ ] Go offline, complete task **A**.
- [ ] On a **second device (online)**, delete task **A**.
- [ ] Bring the first device back online.
- [ ] Replay of A's completion gets a 404 → A moves to **"Unsent changes"** and the
      user sees a failure notification/toast.
- [ ] Any **other** queued ops still drain successfully (the queue keeps going — the
      conflict does not stall it).

### 5. Create in inbox offline, then try to complete it (§6.5)
- [ ] Go offline, create a task in the **Inbox**.
- [ ] The new task appears with a **"waiting to send"** badge (it has a temporary,
      client-side id).
- [ ] While still offline, try to **complete** that just-created task →
      **"Unavailable offline"** (the client answers `offline_unsupported`; nothing is
      queued for it).
- [ ] Go back online → the badge disappears and the task now has a real **server id**.
- [ ] Completing it now works normally.

### 6. Long offline — refresh token expired (§6.6)
- [ ] With unsent changes queued, stay offline long enough that the refresh token
      expires (>30 days; simulate by clearing/expiring the session server-side, or
      testing the boot path).
- [ ] Relaunch: the app does **not** silently drop you — an expired refresh routes to
      the **login** screen.
- [ ] The **outbox is NOT cleared** by hitting the login prompt.
- [ ] Log back in → the queued changes replay and drain.
- [ ] (Contrast) A pure **network** failure at boot must NOT bounce to `/login` — it
      renders from cache with the offline banner (offline session).

### 7. Non-whitelisted mutation offline (§6.7)
- [ ] Go offline.
- [ ] Attempt an action that is **not** in the offline whitelist — e.g. **move/reorder**
      a task, pin, edit a task's fields, or any project/label/context mutation.
- [ ] A toast shows **"Unavailable offline"**.
- [ ] The **UI does not change** — the optimistic edit rolls back (existing page-level
      rollback), the list looks exactly as before the attempt.

### 8. Web-PWA installed (§6.8)
- [ ] In a supported browser, **install** Turboist to the home screen / dock
      (Add to Home Screen / Install app).
- [ ] Launch it once online so the shell + data cache warm.
- [ ] Go offline (OS toggle).
- [ ] **Launch from the home-screen icon** → the app shell opens (service worker serves
      the precached `index.html`/assets) and shows cached data with the offline banner.
- [ ] Confirm `/api/*` and `/auth/*` are **not** served by the service worker
      (DevTools → Application → Service Workers; data comes from the `lib/offline`
      IndexedDB cache, not the SW).
- [ ] Known limitation (do not fail the run on this): on Safari/iOS, Cache Storage +
      IndexedDB may be evicted after ~7 days of non-use (ITP) → degrades to
      "needs connection", not a crash. Milder for an installed PWA.

---

### Native iOS / Android — airplane-mode pass

Run scenarios 1, 2, 4, 5, 7 on a device/simulator in **airplane mode**. Native shells
differ from web in two ways worth checking explicitly:

- [ ] **Read offline (1)**: airplane mode → relaunch the app → Today renders from cache
      with the banner. (The shell is local — `webDir: 'build'` — so only *data* can be
      missing, never the app itself.)
- [ ] **Complete offline + relaunch (2)**: complete a task in airplane mode → force-quit
      and relaunch **still in airplane mode** → still completed → disable airplane mode →
      auto-replay drains → second device shows one completion.
- [ ] **Create-in-inbox then complete blocked (5)**: as web — badge, then
      "Unavailable offline" on complete, then real id after sync.
- [ ] **Non-whitelist blocked (7)**: move/edit in airplane mode → "Unavailable offline",
      UI unchanged.
- [ ] **Auth boot in airplane mode**: a logged-in user relaunching in airplane mode with
      a valid stored refresh token → renders offline (does NOT bounce to login). A
      genuinely logged-out user (no stored token) → login screen.
- [ ] **Logout guard**: with unsent changes queued, tap **Log out** → the "You have N
      unsent changes — sign out and discard them?" confirmation appears. **Cancel** →
      session + outbox intact. **Confirm** → logs out and the offline data is cleared.
- [ ] Note: there is **no Background Sync** on iOS — replay only runs while the app is
      **open/foregrounded**. Verify replay kicks on app foreground and on network
      recovery, not in the background.

# Quick switcher

One panel to jump to any task or project, or to write a new task down, without leaving the
current screen. Available in the web app (desktop and the Capacitor shells) and in the
native Android client.

## Opening it

| Client | How |
|---|---|
| Web, desktop | `⌘P` on macOS, `Ctrl+P` elsewhere (it replaces the browser's print shortcut inside the app), or the ⌘ button in the top bar |
| Web on a phone / Capacitor shells | the ⌘ button in the top bar |
| Native Android | the search-list button in the top bar of every list screen |

`/` still opens the full search page, and the Android search screen stays as it was:
the switcher is for reaching one thing quickly, the search page is for browsing results.

## What it shows

- **Nothing typed:** the tasks and projects you opened most recently on this device,
  newest first.
- **Something typed:** tasks and projects whose title, label or description contains what
  you typed, case-insensitively in every script (`отп` finds `Отпуск`). Recently opened
  matches come first, newest first; everything else follows by where the words hit:
  title, then a label, then the description. Within one rank projects come before tasks,
  and open work before finished work.
- **Two "add" rows** at the bottom, whenever something is typed:
  - **Add to Inbox** — an undated task in the Inbox.
  - **Add for today** — a task due today, filed into the project you opened most recently.
    If you have not opened a project on this device yet, it goes to the Inbox, still due
    today. The row names the target before you choose it.

  Auto-label rules apply exactly as they do for any other new task.

### Keyboard (web)

| Key | Action |
|---|---|
| `↑` / `↓` | move the highlight |
| `Enter` | open the highlighted row (or run the highlighted "add" row) |
| `Shift+Enter` | add to Inbox |
| `⌘/Ctrl+Shift+Enter` | add for today |
| `Esc` (or `⌘/Ctrl+P` again) | close |

On Android the keyboard's action key opens the first row, or adds to the Inbox when
nothing matched.

## "Recently opened"

A task or project counts as opened when its page is shown — from the switcher, a list, a
link or anywhere else. The list is kept on the device only (web: `localStorage`,
`turboist:recentItems`; Android: its own DataStore file) and is never synced: it is a
navigation habit, not workspace data. Android forgets it on sign-out together with the
replica.

## Offline

- **Android** answers entirely from the on-device replica (full-text index plus the
  label tables), and creating a task goes through the outbox, so the switcher works the
  same with no connection.
- **Web / Capacitor:** projects are matched locally from memory. Tasks come from
  `GET /api/v1/search`; when the network is down, that request is answered from every
  task already in the offline cache, ranked by the same rules (see
  [offline.md](offline.md)). Recently opened rows are shown and filtered from their stored
  snapshot. "Add to Inbox" is queued in the offline outbox; "Add for today" into a
  project is not an offline-capable write, so offline it falls back to the Inbox with
  today's due date (also queued).

## Server side

`GET /api/v1/search` matches case-insensitively in every script (a `casefold` SQL function
registered with SQLite, because SQLite's own `LIKE` folds ASCII only), also matches label
names, and ranks title → label → description, open before finished. See
[architecture/backend.md](architecture/backend.md#search).

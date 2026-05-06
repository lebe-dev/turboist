# Locales

Turboist ships with two interface languages — English (`en`) and Russian (`ru`)
— and is wired so additional languages can be added without schema changes.
This document describes how the system is laid out end-to-end and what to do
when adding strings or new locales.

## Overview

```
┌──────────────┐  PATCH /api/v1/settings  ┌─────────────────┐
│ Settings UI  │ ───────────────────────► │  Go handler     │
│ (settings    │                          │  (whitelist)    │
│  /+page)     │ ◄─────────────────────── │                 │
└──────────────┘   GET /api/v1/settings   └────────┬────────┘
       ▲                                           │
       │                                           ▼
       │                                  ┌─────────────────┐
       │                                  │  users.settings │
       │                                  │  (JSON column)  │
       │                                  └─────────────────┘
       │
┌──────┴───────────────────┐
│ svelte-intl-precompile   │  ←─ frontend/locales/{en,ru}.json
│ via $lib/i18n            │
└──────────────────────────┘
```

The chosen locale is the **single source of truth on the server** (one column
of the singleton `users` row). The frontend bootstraps with the browser locale,
then once `settingsStore.load()` completes it switches to whatever the server
remembers. There is no `localStorage` mirror.

Supported values: `"en"`, `"ru"`, plus the empty string `""` which means
"not set — let the client decide".

## Backend

### Storage

Locale lives in the JSON column `users.settings` introduced by migration
`011_user_settings.sql`. Adding/renaming a locale **does not** require a
migration: the JSON column simply gains another value.

The Go shape:

```go
// internal/model/settings.go
type UserSettings struct {
    WeeklyUnplannedExcludedLabelIDs []int64 `json:"weeklyUnplannedExcludedLabelIds"`
    Locale                          string  `json:"locale"`
}
```

`repo.UserRepo.GetSettings` / `SetSettings` (`internal/repo/users.go`)
serialize this struct to/from the JSON column; an empty/`{}` value parses to a
zero `UserSettings` with `Locale = ""`.

### HTTP API

Two endpoints, both authenticated and registered under `/api/v1`:

| Method  | Path             | Body                                  | Returns                    |
| ------- | ---------------- | ------------------------------------- | -------------------------- |
| `GET`   | `/api/v1/settings` | —                                     | `UserSettings` JSON object |
| `PATCH` | `/api/v1/settings` | partial `UserSettings` (any subset)   | full updated `UserSettings` |

Locale is patched as `{"locale": "ru"}` (or `"en"`, or `""` to reset).
The handler whitelists values:

```go
// internal/httpapi/handlers/settings.go
var supportedLocales = map[string]struct{}{
    "":   {},
    "en": {},
    "ru": {},
}
```

Anything outside the whitelist returns
`400 validation_failed`:

```json
{ "error": { "code": "validation_failed", "message": "unsupported locale" } }
```

`Locale *string` in the patch DTO is a pointer so the handler can distinguish
"field not present" from "field set to empty string".

### Tests

`internal/httpapi/handlers/settings_test.go` covers:

- `TestSettingsHandler_GetDefault` — fresh user → `locale: ""`
- `TestSettingsHandler_PatchLocale` — round-trip with `"ru"`
- `TestSettingsHandler_PatchLocaleEmpty` — `""` resets cleanly
- `TestSettingsHandler_PatchLocaleInvalid` — `"de"` is rejected with `400`
- `TestSettingsHandler_PatchLocalePreservesOtherFields` — patching locale does
  not clobber `weeklyUnplannedExcludedLabelIds`

Run focused tests:

```sh
just test SettingsHandler
```

## Frontend

### Stack

- [`svelte-intl-precompile`](https://github.com/cibernox/svelte-intl-precompile) (`^0.12.3`)
  — compiles each `locales/*.json` into a Svelte-friendly module at build time
- Vite plugin: `svelte-intl-precompile/sveltekit-plugin`. It exposes a virtual
  module `$locales` with `registerAll()` and `availableLocales`
- Wrapper module: `frontend/src/lib/i18n.ts` — the **only** import path
  components should use; never import from `svelte-intl-precompile` directly.

### Wrapper API (`$lib/i18n`)

```ts
export { availableLocales, locale, t } from 'svelte-intl-precompile';

export const SUPPORTED_LOCALES = ['en', 'ru'] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

export function isSupportedLocale(v: unknown): v is SupportedLocale;
export function initI18n(preferred: string | null): void;   // idempotent
export function setLocale(value: SupportedLocale): void;
export function localeLabel(value: SupportedLocale): string;
```

- `initI18n(preferred)` runs `registerAll()` and `init({ fallbackLocale: 'en', initialLocale })`.
  Resolution order for `initialLocale`: `preferred` → first segment of
  `navigator.language` → `'en'`. Subsequent calls are no-ops.
- `setLocale(value)` swaps the active locale. The settings store calls it after
  a successful `PATCH /api/v1/settings`.

### Bootstrap flow

1. **Root layout** (`frontend/src/routes/+layout.svelte`) calls
   `initI18n(null)` synchronously in its `<script>`. From this point onward
   every component can use `$t(...)`.
2. **App layout** (`frontend/src/routes/(app)/+layout.svelte`) loads stores in
   parallel, including `settingsStore.load()`. After `Promise.all` resolves it
   does:

   ```ts
   if (isSupportedLocale(settingsStore.locale)) {
     setLocale(settingsStore.locale);
   }
   ```

   So unauthenticated screens render in browser locale, and authenticated
   screens render in the user's saved locale.

### Settings store

```ts
// frontend/src/lib/stores/settings.svelte.ts
async setLocale(loc: SupportedLocale): Promise<void> {
    const updated = await settingsApi.patch(getApiClient(), { locale: loc });
    this.value = updated;
    setLocale(loc);   // re-export from $lib/i18n
}
```

The settings page uses `useFormDialog` semantics manually: button shows a
busy state, awaits the patch, then shows a localized toast
(`settings.language.updated` or `settings.language.updateFailed`).

### Using translations in components

Markup:

```svelte
<script lang="ts">
  import { t } from '$lib/i18n';
</script>

<h1>{$t('settings.title')}</h1>
<button aria-label={$t('topbar.search')}>…</button>
```

Inside `<script>` you read the store as a function:

```ts
import { t } from '$lib/i18n';
// Note the leading $ store-deref
const message = $t('task.toast.subtaskAdded');
```

Interpolation values:

```ts
$t('sidebar.unpinAria', { values: { name: project.title } })
```

In the locale JSON: `"unpinAria": "Unpin {name}"`.

### Conventions

- Add **every** new key to `frontend/locales/en.json`. `en` is the
  `fallbackLocale`, so anything missing in another locale gracefully falls
  back to English instead of showing the raw key.
- `ru.json` is allowed to be incomplete — only translate the keys that
  visibly differ. As coverage grows, fill it in alongside the English copy.
- Hierarchical keys, snake-style segments combined with camelCase for word
  endings: `settings.language.updateFailed`, `task.toast.addedToProject`.
  One screen-fragment = one key; structure follows the page or component
  surface, not the technical implementation.
- **Translate at the callsite**, never inside generic hooks. `useFormDialog`
  and `usePageLoad` accept already-translated strings — passing `$t('…')`
  inside the hook would trap them in the wrong reactivity context.
- Toast and error fallbacks follow the same rule:

  ```ts
  toast.error(describeError(err, $t('task.toast.failedAdd')));
  ```

## Adding a new key

1. Pick a hierarchical key under the relevant scope (e.g.
   `nav.archive`, `settings.theme.eink`).
2. Add the English copy to `frontend/locales/en.json`.
3. (Optional but encouraged) add a translation to `frontend/locales/ru.json`.
4. Use it as `$t('your.new.key')` (or `{$t('…')}` in markup).
5. Run `just lint` and `just test-all`.

`$t` is reactive — the same component re-renders automatically when
`setLocale(...)` is called, so you don't need any extra wiring.

## Adding a new locale

The whitelist must be widened in three places. They are intentionally separate
so the backend can reject bad input even before the JSON is loaded.

1. **Backend whitelist** — `internal/httpapi/handlers/settings.go`:

   ```go
   var supportedLocales = map[string]struct{}{
       "": {}, "en": {}, "ru": {}, "de": {}, // ← add here
   }
   ```

   Add a test case to `settings_test.go` covering the new value.

2. **Frontend supported list** — `frontend/src/lib/i18n.ts`:

   ```ts
   export const SUPPORTED_LOCALES = ['en', 'ru', 'de'] as const;
   ```

   Extend `localeLabel(...)` with the human-readable name.

3. **Locale file** — create `frontend/locales/de.json`. It can start with a
   subset; all missing keys fall back to `en`.

That's it — the Settings page renders one toggle per `SUPPORTED_LOCALES` entry
automatically, so the new option appears the moment the array grows.

## Migrating existing hardcoded strings

The initial i18n landing covered: settings page (full), root and app layouts,
sidebar, topbar, login/setup, document title, empty states for
`today/tomorrow/inbox/week/backlog/completed`, and the toasts emitted from
`(app)/+layout.svelte`.

A long tail of feature components, dialogs, and detail routes still contains
hardcoded English. To migrate one:

1. `import { t } from '$lib/i18n';`
2. Replace each user-visible string with `{$t('…')}` (or `$t('…')` in script).
3. Add the new keys to **both** `en.json` and `ru.json` (English at minimum).
4. `just lint && just test-all` to verify no regressions.

There is no compile-time check that a key exists — typos surface as the raw
key in the UI. When in doubt, search for the key in `en.json` after wiring it
up in markup.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| Raw key (e.g. `settings.title`) renders in the UI | Key missing from `en.json`, or typo at call-site |
| `Failed to resolve import "$locales"` in tests | `vitest.config.ts` is missing the `svelteIntlPrecompile('locales')` plugin |
| Locale doesn't persist across page reloads | `setLocale` was called but `settingsStore.setLocale` (which performs the PATCH) was bypassed |
| `400 validation_failed` on PATCH | Value not in the backend whitelist; widen `supportedLocales` |
| `t` is not reactive (text frozen on locale switch) | The component imported `t` from `'svelte-intl-precompile'` directly without using the store-deref `$t` syntax. Use `$lib/i18n` and `$t(...)` |

## Reference files

| Concern | File |
| --- | --- |
| DB column | `internal/db/migrations/011_user_settings.sql` |
| Go model | `internal/model/settings.go` |
| Repo I/O | `internal/repo/users.go` (`GetSettings`, `SetSettings`) |
| HTTP handler + whitelist | `internal/httpapi/handlers/settings.go` |
| Backend tests | `internal/httpapi/handlers/settings_test.go` |
| Locale JSON | `frontend/locales/en.json`, `frontend/locales/ru.json` |
| Wrapper module | `frontend/src/lib/i18n.ts` |
| Vite plugin | `frontend/vite.config.ts`, `frontend/vitest.config.ts` |
| TS DTO | `frontend/src/lib/api/types.ts` (`UserSettings`) |
| API client | `frontend/src/lib/api/endpoints/settings.ts` |
| Frontend store | `frontend/src/lib/stores/settings.svelte.ts` |
| App bootstrap | `frontend/src/routes/+layout.svelte`, `frontend/src/routes/(app)/+layout.svelte` |
| Settings UI | `frontend/src/routes/(app)/settings/+page.svelte` |

# Frontend Architecture

Turboist web-клиент — SPA на SvelteKit 2 (Svelte 5 runes) + Tailwind 4 + shadcn-svelte. Собранная статика встраивается в Go-бинарь и отдаётся бэкендом на корне; все API-вызовы — клиентский `fetch` к `/api/v1/*` и `/auth/*`.

## Режим работы — SPA

- `@sveltejs/adapter-static` с `fallback: 'index.html'`.
- В корневом `+layout.ts` выставлено `export const ssr = false; export const prerender = false;` — клиентский рендеринг для всех маршрутов.
- SvelteKit `load`-функции не используются для авторизованных запросов: токен хранится только в памяти JS, поэтому SSR/prerender в принципе не имеют доступа к нужному состоянию.
- Сборка кладётся в `frontend/build/`, бэкенд (`cmd/turboist/main.go` + `internal/httpapi`) встраивает её через `embed.FS` и отдаёт SPA-fallback на не-API маршрутах.

## API-клиент (`src/lib/api`)

- `client.ts` экспортирует `apiFetch<T>(path, init)` — единая точка входа.
  - Добавляет `Authorization: Bearer <accessToken>` из auth-store.
  - Парсит body (`Content-Type: application/json`) и при ошибке оборачивает её в `ApiError(code, message, details, status)` из единого error envelope `{error:{code,message,details}}`.
  - На 401 с кодом `auth_expired` запускает refresh-rotation: один `POST /auth/refresh` для всех конкурирующих запросов (singleflight через shared `Promise`), после успеха retry'ит исходный запрос. На повторный 401 — финальный `logout()`.
- `endpoints/*` — тонкие типизированные обёртки на каждое семейство роутов: `auth`, `config`, `contexts`, `projects`, `sections`, `tasks`, `labels`, `views`, `search`. Сгруппированы по доменам, чтобы pivot-route мог импортировать ровно одно семейство.
- `types.ts` — DTO и enums (`Priority`, `Status`, `ColorToken`, `DayPart`), повторяющие контракты из `files/files/structs.md` и `files/files/API.md`. Даты — `string` (ISO-8601 UTC `.000Z`).

## Auth (`src/lib/auth`)

- `store.svelte.ts` — runes-store: `{ user, accessToken, status: 'loading' | 'guest' | 'authenticated' }`.
- Методы: `bootstrap()` (GET `/auth/setup-required` + попытка `/auth/refresh`), `login`, `setup`, `logout`, `logoutAll`.
- `guard.ts` — helper `requireAuth()` для редиректа из защищённых route-`+page`-компонентов.

Схема жизненного цикла:

```
boot → /auth/setup-required → (нет user) → /setup
                            ↘ (есть user) → /auth/refresh → 200 → status=authenticated
                                                          → 401 → status=guest → /login
```

## Layout и навигация

- `routes/(auth)/` — public-`+layout` с центрированной формой; `/login` и `/setup` отображаются по результату `setup-required`.
- `routes/(app)/+layout.svelte` — Sidebar + main; на `onMount` инициирует stores `config`, `contexts`, `projects`, `labels`. Sidebar содержит:
  - Системные views: Inbox, Today, Tomorrow, Week (с лимитом из `config.weekly.limit`), Backlog, Overdue, Search.
  - Pinned Projects.
  - Контексты (collapsible) → свои проекты.
  - Labels (favourites first).
- Topbar с quick-add (горячая клавиша Q) и user-menu (logout / logout-all).
- Тема dark/light — `mode-watcher`.

## Глобальные stores (`src/lib/stores`)

Runes-обёртки `*.svelte.ts` для редко меняющихся справочников: `config`, `contexts`, `projects`, `labels`. Грузятся один раз при входе в `(app)`-layout и обновляются после CRUD-действий оптимистично; на ошибке валидации — toast (`svelte-sonner`) и rollback.

Состояние задач не глобальное — каждый view-маршрут запрашивает соответствующий `views.*` endpoint при монтировании.

## Views и pivot-маршруты

- `inbox`, `today`, `tomorrow`, `week`, `backlog`, `overdue` — соответствуют backend-views и используют общий `TaskTree`-рендер.
  - Today/Tomorrow группируются по `dayPart` (morning/afternoon/evening/anytime).
  - Week — группировка по дням, бейдж лимита, блокировка добавления при `limit_exceeded`.
  - Overdue — действия «перенести на сегодня/завтра/в backlog» через `POST /tasks/:id/plan`.
  - Inbox — warn-баннер при превышении `warnThreshold`.
- `project/[id]`, `context/[id]`, `label/[id]`, `search` — pivot-маршруты с собственными header-компонентами и actions (complete/cancel/archive/pin, reorder секций, поиск с debounce).

## Task primitives (`src/lib/components/task`)

- `TaskItem` — checkbox, title, метки, project/section badge, priority dot, dueDate, day-part icon, pin/edit/delete; индент для подзадач.
- `TaskTree` — рекурсивный рендер дерева, построенного `utils/taskTree.ts:buildTree` из плоского списка по `parentId`/`position`.
- `QuickAddDialog` — title + project + priority + due + labels.
- `TaskEditorSheet` — полная форма: description, priority, dueDate, dayPart, plannedFor, labels, removedAutoLabels, parentId, sectionId, RRULE.

## Хуки (`src/lib/hooks`)

Переиспользуемые Svelte 5 runes-хуки, устраняющие boilerplate в маршрутах и диалогах.

- `usePageLoad(fetcher, opts?)` — оборачивает асинхронный запрос: `requestSeq`-счётчик для сброса устаревших ответов при быстрой навигации, реактивный `loading`, метод `refetch()`. На ошибке вызывает `toast.error` или пользовательский `onError`. `autoLoad: false` отключает автоматический вызов при монтировании — caller сам зовёт `refetch()` через `$effect`.
- `useFormDialog()` — возвращает `submitting` и `submit(fn, messages)`: guard от двойного сабмита, toast при успехе и ошибке.
- `useListMutator<T>()` — держит `$state`-массив элементов; возвращает геттер `items` и `{ replace, remove }`.
- `is-mobile.svelte.ts` — реактивный breakpoint-хелпер на базе `MediaQuery`.

Barrel-экспорт: `$lib/hooks`.

## View-компоненты (`src/lib/components/view`)

Переиспользуемые UI-блоки для view-маршрутов.

- `ViewContent` — трёхсостоятельная обёртка: `{#if loading}` спиннер, `{:else if isEmpty}` `<EmptyState>`, `{:else}` children. Принимает `loading`, `isEmpty`, `emptyIcon`, `emptyTitle`, `emptyDescription` и `children` snippet.
- `ViewHeader` — заголовок страницы с кнопками действий.
- `EmptyState` — центрированный плейсхолдер с иконкой, заголовком, описанием.
- `DayPartSection` / `DayPartSectionHeader` — секция с иконкой и временным интервалом дня; используется в Today/Tomorrow.
- `CompletedTodayFooter` — сворачиваемый список задач, завершённых сегодня; загружает `GET /api/v1/views/completed-today` лениво при раскрытии.
- `LimitBadge` — бейдж числового лимита (используется в Week).

Barrel-экспорт: `$lib/components/view`.

## CRUD-диалоги (`src/lib/components/dialog`)

`ContextDialog`, `ProjectDialog`, `SectionDialog`, `LabelDialog`, `ConfirmDestructiveDialog` — переиспользуются из Sidebar и pivot-headers. На сабмите — через `useFormDialog()`: guard от двойного сабмита, toast при успехе и ошибке, оптимистичное обновление store.

## Тестирование

- `vitest` + `@testing-library/svelte` (jsdom).
- Покрытие сосредоточено на логике: `api/client` (refresh-singleflight, error-envelope, retry, финальный logout), `auth/store`, `utils/taskTree`, `utils/format`, stores.
- UI-компоненты — точечные smoke-тесты для критичных потоков.
- ESLint игнорирует `src/lib/components/ui/**` (shadcn-каталог read-only).

## Команды

Рецепты `Justfile`:

- `just frontend-dev` — vite dev-сервер.
- `just frontend-build` — статическая сборка.
- `just frontend-lint` — `svelte-check` + ESLint.
- `just frontend-test` — vitest run.

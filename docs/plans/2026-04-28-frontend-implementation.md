# Frontend MVP: SvelteKit 2 + Svelte 5 + Tailwind 4 + shadcn-svelte

## Overview
Реализовать полнофункциональный web-клиент Turboist поверх готового скелета `frontend/` (SvelteKit 2, Svelte 5 runes, Tailwind 4, shadcn-svelte уже в `src/lib/components/ui`). Объём — Full MVP: auth (setup/login/refresh-rotation), layout с sidebar, все основные views (Inbox, Today, Tomorrow, Week, Backlog, Overdue, Project, Context, Label, Search), CRUD задач/проектов/секций/контекстов/меток. Режим работы клиента — SPA (`adapter-static` + SPA fallback), потому что бэкенд встраивает статику и access-токен живёт только в памяти JS. Все данные тянутся клиентскими `fetch`-запросами; SvelteKit `load` не используется для авторизованных запросов.

## Context
- Backend API: `files/files/API.md`, `files/files/auth.md`, `files/files/structs.md`, `files/files/business-rules.md`, `files/files/config.md`
- Backend code: `cmd/turboist/main.go`, `internal/httpapi/`, `internal/service/`
- Backend doc: `docs/architecture/backend.md`, `docs/plans/completed/2026-04-27-backend-implementation.md`
- Существующее: `frontend/svelte.config.js`, `frontend/vite.config.ts`, `frontend/tsconfig.json`, `frontend/src/app.html`, `frontend/src/routes/+layout.svelte`, `frontend/src/lib/components/ui/*` (shadcn — не трогаем, исключаем из ESLint)
- Конвенции: camelCase JSON, ISO-8601 UTC `.000Z`, Bearer access, refresh в HttpOnly cookie с `Path=/auth/refresh`, единый error envelope `{error:{code,message,details}}`
- Justfile в корне (использовать `just` команды для фронта; добавить рецепты при необходимости)

## Development Approach
- Testing: Regular (код → vitest unit-тесты для api-client, stores, утилит; Playwright/component-тесты опционально только для критичных потоков)
- Каждая Task — атомарна, тесты обязательны для логики (api-client, auth-store, task tree-builder, RRULE-helper, фильтры view), UI-компоненты покрываются точечно
- Все тесты должны проходить (`just test` / `yarn check`) перед переходом к следующей таске
- Никаких добавлений к `src/lib/components/ui/**` — shadcn-каталог read-only
- ESLint игнорирует `src/lib/components/ui/**`

## Implementation Steps

### Task 1: Tooling — adapter-static, ESLint/Prettier, vitest, Justfile

**Files:**
- Modify: `frontend/package.json`, `frontend/svelte.config.js`, `frontend/vite.config.ts`, `frontend/tsconfig.json`, `Justfile`
- Create: `frontend/eslint.config.js`, `frontend/.prettierrc`, `frontend/.prettierignore`, `frontend/vitest.config.ts`, `frontend/src/routes/+layout.ts` (с `export const ssr = false; export const prerender = false;`)

- [x] заменить `@sveltejs/adapter-auto` на `@sveltejs/adapter-static` с `fallback: 'index.html'`
- [x] добавить ESLint flat-config (typescript-eslint + eslint-plugin-svelte) с `ignores: ['src/lib/components/ui/**', 'build/**', '.svelte-kit/**', 'node_modules/**']`
- [x] добавить Prettier (с `prettier-plugin-svelte`, `prettier-plugin-tailwindcss`) и `.prettierignore` с `src/lib/components/ui/**`
- [x] добавить vitest + `@testing-library/svelte` (jsdom env)
- [x] прописать в `package.json` скрипты `lint`, `format`, `test`, `test:watch`
- [x] добавить в `Justfile` рецепты `frontend-dev`, `frontend-build`, `frontend-lint`, `frontend-test`
- [x] sanity-чек: `yarn check && yarn lint && yarn build` без ошибок (UI-каталог не должен попадать в lint-отчёт)

### Task 2: API-клиент с auth-flow и refresh-rotation

**Files:**
- Create: `frontend/src/lib/api/client.ts`, `frontend/src/lib/api/errors.ts`, `frontend/src/lib/api/types.ts`, `frontend/src/lib/api/endpoints/{auth,contexts,projects,sections,tasks,labels,views,config}.ts`
- Create: `frontend/src/lib/auth/store.svelte.ts` (runes-based), `frontend/src/lib/auth/guard.ts`
- Tests: `frontend/src/lib/api/client.test.ts`, `frontend/src/lib/auth/store.test.ts`

- [x] `client.ts`: `apiFetch<T>(path, init)` — добавляет `Authorization: Bearer`, обрабатывает 401 `auth_expired` → один параллельный `/auth/refresh` (singleflight через Promise) → retry; парсит error envelope в `ApiError(code, message, details, status)`
- [x] `endpoints/*` — типизированные обёртки над всеми эндпоинтами из `API.md` (config, contexts CRUD + tasks/projects, projects CRUD + actions complete/uncomplete/cancel/archive/unarchive/pin/unpin, sections CRUD + reorder, tasks CRUD + complete/uncomplete/cancel/move/plan/unplan/pin/unpin, labels CRUD, views inbox/today/tomorrow/week/backlog/overdue, search)
- [x] `types.ts` — DTO из `structs.md`/API.md (Context, Project, Section, Task, Label, View responses, Config); enums (Priority, Status, ColorToken, DayPart)
- [x] `auth/store.svelte.ts` — runes-state `{ user, accessToken, status: 'loading'|'guest'|'authenticated' }`; методы `bootstrap()` (вызов `/auth/setup-required` + попытка refresh), `login`, `setup`, `logout`, `logoutAll`
- [x] тесты: refresh-singleflight (два параллельных 401 → один `/auth/refresh`), error-envelope-парсинг, повтор после refresh, окончательный logout если refresh 401

### Task 3: App shell — auth-routes, layout с sidebar, тема

**Files:**
- Modify: `frontend/src/routes/+layout.svelte`, `frontend/src/app.html`
- Create: `frontend/src/routes/(auth)/+layout.svelte`, `frontend/src/routes/(auth)/login/+page.svelte`, `frontend/src/routes/(auth)/setup/+page.svelte`
- Create: `frontend/src/routes/(app)/+layout.svelte`, `frontend/src/routes/(app)/+layout.ts`
- Create: `frontend/src/lib/components/app/{Sidebar.svelte,SidebarSection.svelte,Topbar.svelte,UserMenu.svelte,ThemeToggle.svelte}`
- Create: `frontend/src/lib/stores/{contexts,projects,labels,config}.svelte.ts` — глобальные runes-stores, инициализируются один раз в `(app)/+layout`

- [x] root layout: bootstrap auth, маршрутизация — guest → `/login`/`/setup`, authenticated → `(app)`
- [x] `/login`, `/setup` (сценарий из `auth.md`: `/auth/setup-required` определяет что показать)
- [x] `(app)/+layout.svelte` — Sidebar + main; Sidebar содержит: Inbox, Today, Tomorrow, Week (с лимитом из config), Backlog, Overdue, Search, разделы Pinned Projects, Contexts → Projects (collapsible), Labels (favourites first)
- [x] загрузка `/api/v1/config`, contexts, projects, labels при входе в `(app)`
- [x] dark/light через `mode-watcher` (уже в deps), Topbar c quick-add и user-menu (logout / logout-all)
- [x] keyboard-shortcuts caption через `kbd` shadcn-компонент (Q — quick add, / — search)
- [x] тесты: smoke-рендер layout с моком store

### Task 4: Task primitives — TaskItem, TaskList, TaskTree, QuickAdd, TaskEditor

**Files:**
- Create: `frontend/src/lib/components/task/{TaskItem.svelte,TaskList.svelte,TaskTree.svelte,QuickAddDialog.svelte,TaskEditorSheet.svelte,PriorityPicker.svelte,DayPartPicker.svelte,LabelChips.svelte,DateBadge.svelte}`
- Create: `frontend/src/lib/utils/{taskTree.ts,format.ts,priority.ts}`
- Tests: `frontend/src/lib/utils/taskTree.test.ts`, `frontend/src/lib/utils/format.test.ts`

- [x] `taskTree.ts` — `buildTree(tasks)` из плоского списка по `parentId`, сохраняя порядок `position`
- [x] `format.ts` — формат дат (UTC `.000Z` ↔ локальная TZ из config), `formatDay`, `formatDayPart`
- [x] `TaskItem` — checkbox (complete/uncomplete), title, метки, project/section badge, priority dot, dueDate, day-part icon, pin, edit, delete, indent для подзадач
- [x] `QuickAddDialog` — natural-input (title, опционально `#project`, `@context`, `+label`, `!p1..p4`, дата) — на этой стадии минимально: title + project + priority + due + labels
- [x] `TaskEditorSheet` — полная форма (title, description, priority, dueDate, dayPart, plannedFor, labels[], removedAutoLabels[], parentId, sectionId, recurrence RRULE)
- [x] юнит-тесты на `buildTree` и `format`

### Task 5: Views — Inbox, Today, Tomorrow, Week, Backlog, Overdue

**Files:**
- Create: `frontend/src/routes/(app)/inbox/+page.svelte`
- Create: `frontend/src/routes/(app)/today/+page.svelte`
- Create: `frontend/src/routes/(app)/tomorrow/+page.svelte`
- Create: `frontend/src/routes/(app)/week/+page.svelte`
- Create: `frontend/src/routes/(app)/backlog/+page.svelte`
- Create: `frontend/src/routes/(app)/overdue/+page.svelte`
- Create: `frontend/src/lib/components/view/{ViewHeader.svelte,EmptyState.svelte,LimitBadge.svelte}`

- [x] каждая view использует соответствующий `views.*` endpoint и `TaskTree`
- [x] Today/Tomorrow — группировка по dayPart (morning/afternoon/evening/anytime), drag-n-drop отложен
- [x] Week — группировка по дням, бейдж лимита (config.weekly.limit), запрет добавлять при превышении (показ ошибки `limit_exceeded`)
- [x] Backlog — лимит из config, аналогично
- [x] Overdue — действия «перенести на сегодня/завтра/в backlog» через `/tasks/:id/plan`
- [x] Inbox — warnThreshold-баннер, кнопка quick-add по умолчанию в Inbox

### Task 6: Project / Context / Label / Search routes

**Files:**
- Create: `frontend/src/routes/(app)/project/[id]/+page.svelte`
- Create: `frontend/src/routes/(app)/context/[id]/+page.svelte`
- Create: `frontend/src/routes/(app)/label/[id]/+page.svelte`
- Create: `frontend/src/routes/(app)/search/+page.svelte`
- Create: `frontend/src/lib/components/project/{ProjectHeader.svelte,SectionList.svelte,SectionItem.svelte}`
- Create: `frontend/src/lib/components/context/ContextHeader.svelte`
- Create: `frontend/src/lib/components/label/LabelHeader.svelte`

- [x] Project: header с actions (complete/cancel/archive/pin), список секций + drag-reorder секций (минимум — кнопки вверх/вниз через `sections/:id/reorder`), задачи в секциях и без секции
- [x] Context: header + projects-табы + flat tasks
- [x] Label: tasks с этим лейблом
- [x] Search: input с debounce → `GET /api/v1/search?q=`, табы Tasks/Projects, подсветка совпадений (без подсветки на этом MVP)
- [x] confirmation-диалоги для DELETE контекста/проекта (показывают cascade-предупреждение)

### Task 7: CRUD-диалоги для контекстов, проектов, секций, меток

**Files:**
- Create: `frontend/src/lib/components/dialog/{ContextDialog.svelte,ProjectDialog.svelte,SectionDialog.svelte,LabelDialog.svelte,ConfirmDestructiveDialog.svelte}`
- Modify: Sidebar и соответствующие Header-компоненты (кнопки Add/Edit/Delete)

- [ ] Context: name, color (палитра colorToken из config), isFavourite
- [ ] Project: title, description, color, contextId, labels[]
- [ ] Section: title (только в проекте)
- [ ] Label: name, color, isFavourite
- [ ] всё через соответствующие endpoints, оптимистичные обновления stores; на ошибке валидации/конфликта — toast (svelte-sonner уже в deps)

### Task 8: Acceptance — полный прогон

- [ ] `just frontend-lint` — без ошибок, `src/lib/components/ui/**` исключён
- [ ] `just frontend-test` (vitest) — зелёный
- [ ] `yarn check` (svelte-check) — без ошибок типов
- [ ] `yarn build` — успешная статическая сборка, `build/` содержит `index.html` + ассеты
- [ ] ручной smoke: setup → login → создать context → project → section → task → complete → переключиться по всем views → logout

### Task 9: Документация

- [ ] обновить `frontend/README.md` (запуск, команды, структура)
- [ ] добавить секцию о frontend в `docs/architecture/` (новый `frontend.md`: SPA-режим, API-клиент, auth-flow, структура каталогов)
- [ ] переместить этот план в `docs/plans/completed/`

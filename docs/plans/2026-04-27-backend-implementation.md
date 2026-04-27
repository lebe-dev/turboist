# Реализация backend Turboist

## Overview
Полная реализация бэкенда single-user таск-менеджера Turboist по спецификации `files/files/*.md`. Стек: Go 1.26, Fiber v3, SQLite (modernc.org/sqlite, без CGO), goose-миграции, raw SQL без ORM, JWT + opaque refresh, argon2id. Реализуется весь публичный API из `API.md` (auth + `/api/v1/...`), бизнес-правила из `business-rules.md`, схема и инварианты из `database.md`.

## Context
- Files involved (spec): `files/files/README.md`, `api-requirements.md`, `API.md`, `auth.md`, `business-rules.md`, `config.md`, `conventions.md`, `database.md`, `structs.md`
- Files involved (code): `cmd/turboist/main.go` (расширяется), `go.mod` (новые зависимости), `Justfile` (без изменений в API), `config.example.yml` (приводится в соответствие со спекой)
- Создаётся пакетная структура `internal/...` с разделением слоёв (config, model, db, repo, auth, service, httpapi)
- Внешние библиотеки: `github.com/gofiber/fiber/v3`, `modernc.org/sqlite`, `github.com/pressly/goose/v3`, `github.com/golang-jwt/jwt/v5`, `golang.org/x/crypto/argon2`, `golang.org/x/time/rate`, `github.com/teambition/rrule-go`, `gopkg.in/yaml.v3`, `github.com/joho/godotenv`
- Конвенции: snake_case в БД, PascalCase в Go, camelCase в JSON, kebab-case в yaml, время `2006-01-02T15:04:05.000Z` UTC
- Тесты — против реального SQLite (in-memory или tmp file), без моков БД

## Development Approach
- **Testing approach**: Regular (код первый, тесты сразу после в той же задаче)
- Repository тесты — против реального SQLite с прогнанными миграциями
- HTTP-хендлеры — через `app.Test(req)` Fiber v3
- Каждая задача завершается прогоном `just test` и `just lint-backend` — оба должны быть зелёными перед переходом к следующей
- Покрытие — целевое 80%+ на пакеты `repo`, `service`, `auth`
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Скелет проекта, конфиг, логирование

**Files:**
- Modify: `cmd/turboist/main.go`, `go.mod`, `config.example.yml`
- Create: `internal/config/config.go`, `internal/config/config_test.go`, `internal/logging/logger.go`, `.env.example`

- [x] Добавить зависимости: fiber/v3, modernc.org/sqlite, goose/v3, jwt/v5, argon2 (x/crypto), x/time/rate, rrule-go, yaml.v3, godotenv
- [x] Реализовать загрузку env (BIND, LOG_LEVEL, BASE_URL, JWT_SECRET) с fail-fast и `.env`
- [x] Реализовать загрузку `config.yml` (yaml.v3) и валидацию: timezone через `time.LoadLocation`, day-parts покрытие/непересечение, priority overflow-task, лимиты > 0, auto-labels.label непустой
- [x] Привести `config.example.yml` к спеке (`weekly.limit`, `backlog.limit`, `inbox.warn-threshold`, `inbox.overflow-task`, `auto-labels` в kebab-case)
- [x] Настроить структурный логер (slog) с уровнем из LOG_LEVEL (case-insensitive)
- [x] `main.go`: load env → load config → init logger → log "starting"
- [x] Тесты config: валидный yaml; ошибки на пересекающихся day-parts; неверный timezone; неверный priority; пустой label
- [x] `just test ./internal/config/...` зелёный, `just lint-backend` зелёный

### Task 2: SQLite + миграции (goose)

**Files:**
- Create: `internal/db/db.go`, `internal/db/db_test.go`, `internal/db/migrations/001_schema.sql`, `internal/db/migrations/002_users_sessions.sql`, `internal/db/migrations.go`

- [x] Open SQLite через `modernc.org/sqlite` с DSN включающим `_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)`; выставить `SetMaxOpenConns(1)` для записей или sane defaults
- [x] Embed-миграции через `embed.FS`, прогон через goose на старте
- [x] Перенести миграции 001 и 002 из `database.md` дословно (включая INSERT inbox)
- [x] Helper для транзакций (`WithTx(ctx, fn)`)
- [x] Тесты: миграции up/down round-trip; PRAGMA foreign_keys=1 после Open; INSERT в `inbox(id=2)` падает; INSERT в `users(id=2)` падает
- [x] `just test ./internal/db/...` зелёный

### Task 3: Доменная модель

**Files:**
- Create: `internal/model/enums.go`, `internal/model/entities.go`, `internal/model/time.go`, `internal/model/model_test.go`

- [x] Перенести enums из `structs.md` (Priority, TaskStatus, ProjectStatus, DayPart, PlanState, ClientKind)
- [x] Структуры Context, Label, Project, ProjectSection, Task, User, Session с указателями на nullable
- [x] `Task.URL(baseURL)` метод
- [x] `model/time.go`: `FormatUTC(t time.Time) string` → `2006-01-02T15:04:05.000Z`; `ParseUTC(s string) (time.Time, error)`
- [x] Validators для enum-значений (IsValid методы) — используются хендлерами
- [x] Тесты: format/parse round-trip; IsValid на всех вариантах
- [x] `just test ./internal/model/...` зелёный

### Task 4: Репозитории — contexts, labels, projects, sections

**Files:**
- Create: `internal/repo/contexts.go`, `internal/repo/labels.go`, `internal/repo/projects.go`, `internal/repo/sections.go`, `internal/repo/project_labels.go`, `internal/repo/testdb_test.go`, `internal/repo/{contexts,labels,projects,sections}_test.go`

- [x] CRUD raw-SQL для каждой сущности; nullable → `sql.Null*` с конверсией в указатели
- [x] Listing с пагинацией (limit/offset) и `total` count'ом отдельным запросом
- [x] Projects: фильтры `?contextId=&status=`, сортировка `is_pinned DESC, pinned_at DESC, created_at DESC`
- [x] Hydrate `project_labels` отдельным запросом + сшивание в коде (без GROUP_CONCAT)
- [x] Helper `setupTestDB(t)` запускает миграции на in-memory SQLite
- [x] Тесты CRUD на каждое; конфликт UNIQUE → определённая ошибка; CASCADE при удалении контекста/проекта; pagination границы
- [x] `just test ./internal/repo/...` зелёный

### Task 5: Репозиторий tasks + task_labels + views + search

**Files:**
- Create: `internal/repo/tasks.go`, `internal/repo/task_labels.go`, `internal/repo/views.go`, `internal/repo/search.go`, тесты к каждому

- [ ] Insert/Update/Delete tasks с проверкой placement-инвариантов в коде (зеркалирует CHECK)
- [ ] Reject циклов: при `move` строится цепочка `parent_id` вверх, отказ если содержит target id
- [ ] `task_labels` linking + hydrate labels на чтение
- [ ] Запросы views: today/tomorrow (по `due_at` BETWEEN), overdue (`due_at < now AND status='open'`), week/backlog (по `plan_state`)
- [ ] Search: LIKE по title/description, минимум 2 символа (валидируется выше)
- [ ] Counters: `CountWeek`, `CountBacklog`, `CountInbox`, `CountPinnedTasks`, `CountPinnedProjects` для лимитов
- [ ] Single sort util (`ORDER BY is_pinned DESC, priority CASE..., pinned_at DESC, created_at DESC`)
- [ ] Тесты: placement матрица (inbox без подзадач, секция требует проект, ровно один из inbox/context); subtree move; FK CASCADE на удаление родителя
- [ ] `just test ./internal/repo/...` зелёный

### Task 6: Auth — argon2, JWT, sessions, rate-limit

**Files:**
- Create: `internal/auth/password.go`, `internal/auth/jwt.go`, `internal/auth/refresh.go`, `internal/auth/ratelimit.go`, `internal/repo/users.go`, `internal/repo/sessions.go`, `internal/auth/cleanup.go`, тесты

- [ ] argon2id hash/verify в PHC-формате (time=3, mem=64MB, threads=4, salt=16, key=32)
- [ ] JWT HS256: Issue(userID, sessionID) → 15min expiry; Verify → claims (sub, sid, iat, exp)
- [ ] Refresh: 32 random bytes, base64url; sha256 hash сохраняется в БД
- [ ] Sessions repo: create, getByTokenHash, rotate (update token_hash/expires_at/last_used_at), revoke, revokeAllForUser, enforceLimit5(client_kind), cleanup
- [ ] IPLimiter (token-bucket): rate.Every(6s), burst=10, ttl 10min, GC горутина
- [ ] StartSessionCleanup(ctx, db, log) — тикер 24h
- [ ] Тесты: hash/verify round-trip; jwt expiry; refresh ротация; session limit (6-я вытесняет старейшую); cleanup удаляет expired/revoked>7d; ratelimit allows N then blocks
- [ ] `just test ./internal/auth/...` зелёный

### Task 7: HTTP-сервер (Fiber v3) — каркас, middleware, error envelope

**Files:**
- Create: `internal/httpapi/server.go`, `internal/httpapi/errors.go`, `internal/httpapi/middleware.go`, `internal/httpapi/dto/common.go`, тесты

- [ ] Fiber v3 app с custom ErrorHandler, маппящим типизированные ошибки (`ErrValidation`, `ErrNotFound`, `ErrConflict`, `ErrLimitExceeded`, `ErrForbiddenPlacement`, `ErrAuthInvalid`, `ErrAuthExpired`, `ErrAuthRateLimited`, `ErrSetupAlreadyDone`, `ErrRecurrenceInvalid`) в envelope `{error: {code, message, details}}` с кодами из таблицы `API.md`
- [ ] Middleware: recover, request-id, structured access-log, AuthMiddleware (Bearer → claims в context)
- [ ] Pagination helper: парсит `limit/offset`, дефолт 50 / max 200, формирует envelope `{items, total, limit, offset}`
- [ ] DTO утилиты: nullable JSON (распознавание missing vs null vs value через `json.RawMessage` или `*pointer`+флаг); marshal time в формат `.000Z`
- [ ] Регистрация маршрутов в `RegisterRoutes(app, deps)` (заглушки)
- [ ] Тесты: ErrorHandler возвращает правильные коды и envelope; AuthMiddleware пропускает валидный bearer, отвергает битый/expired; pagination clamp
- [ ] `just test ./internal/httpapi/...` зелёный

### Task 8: Auth-эндпоинты

**Files:**
- Create: `internal/httpapi/handlers/auth.go`, `internal/httpapi/dto/auth.go`, тесты

- [ ] `GET /auth/setup-required`, `POST /auth/setup` (410 если уже создан), `POST /auth/login`, `POST /auth/refresh` (cookie ИЛИ body), `POST /auth/logout`, `POST /auth/logout-all`, `GET /auth/me`
- [ ] Login/setup с rate-limit per IP (10/min); `clientKind` enum-валидация; ответ `{access, refresh, user}`
- [ ] Web (по `clientKind=web`): `Set-Cookie refresh=...; HttpOnly; Secure; SameSite=Lax; Path=/auth/refresh; Max-Age=2592000`
- [ ] При login применять enforceLimit5 для client_kind
- [ ] Refresh-rotation с детекцией кражи (in-memory short-lived map старых hashes — best-effort)
- [ ] Тесты: end-to-end setup → login → refresh → me → logout; 410 на повторный setup; 401 на битый refresh; rate-limit срабатывает; cookie ставится только web
- [ ] `just test ./internal/httpapi/handlers/...` зелёный

### Task 9: Contexts, Labels, Sections, Inbox handlers + Health/Version/Config

**Files:**
- Create: `internal/httpapi/handlers/{contexts,labels,sections,inbox,meta}.go`, соответствующие dto-файлы и тесты

- [ ] CRUD для contexts: GET list, POST, GET/PATCH/DELETE/:id, GET /:id/projects, GET /:id/tasks (с фильтрами `status,priority,labelId,q`), POST /:id/tasks (создаёт задачу без проекта в контексте — делегирует tasks-сервису из task 11)
- [ ] CRUD для labels: GET list (?q=), POST, GET/PATCH/DELETE/:id, GET /:id/tasks, GET /:id/projects
- [ ] Sections: GET/PATCH/DELETE/:id, GET /:id/tasks, POST /:id/tasks; создание — `POST /projects/:id/sections`
- [ ] Inbox: `GET /api/v1/inbox` → `{count, warnThresholdExceeded, tasks: [...]}`; `POST /api/v1/inbox/tasks` (через tasks-сервис)
- [ ] Meta: `GET /healthz`, `GET /version` (без auth), `GET /api/v1/config` — фильтрованный публичный конфиг по схеме из API.md
- [ ] Тесты: 200/201/204/404 кейсы; UNIQUE conflict → `conflict` envelope; CASCADE проверка через GET после DELETE
- [ ] `just test ./...` зелёный

### Task 10: Projects handlers + actions

**Files:**
- Create: `internal/httpapi/handlers/projects.go`, `internal/service/pin.go`, тесты

- [ ] CRUD: `GET /api/v1/projects` (filter contextId/status), `POST /api/v1/contexts/:id/projects`, `GET/PATCH/DELETE /api/v1/projects/:id`
- [ ] Под-роуты: `/projects/:id/sections` (GET/POST), `/projects/:id/tasks` (GET/POST)
- [ ] Action-эндпоинты: complete/uncomplete/cancel/archive/unarchive/pin/unpin
- [ ] Pin-сервис: проверяет `max-pinned` для проектов отдельно от задач, ставит `pinned_at = now`
- [ ] Hydrate labels на ответе; на вход — `labels: [name]` с резолвом в id (неизвестные → validation_failed, кроме auto-labels — но auto-labels к проектам не применяются)
- [ ] Тесты: переходы статусов; pin > max → limit_exceeded; PATCH игнорирует placement/status/pin поля
- [ ] `just test ./...` зелёный

### Task 11: Tasks — CRUD, создание в контейнерах, subtasks, auto-labels

**Files:**
- Create: `internal/httpapi/handlers/tasks.go`, `internal/service/tasks_create.go`, `internal/service/auto_labels.go`, `internal/httpapi/dto/tasks.go`, тесты

- [ ] Create-эндпоинты per API.md: `POST /contexts/:id/tasks`, `POST /projects/:id/tasks`, `POST /sections/:id/tasks`, `POST /inbox/tasks`, `POST /tasks/:parentId/subtasks` (наследует placement)
- [ ] DTO `CreateTaskRequest` с теми же полями, partial; `min title required`
- [ ] `GET /tasks/:id`, `PATCH /tasks/:id` (только редактируемые поля, см. API.md), `DELETE /tasks/:id`
- [ ] Auto-labels сервис: применить правила к `title`, объединить с явными `labels`, вычесть `removedAutoLabels`; авто-создавать missing labels из `auto-labels` config
- [ ] Подключается на create и patch (если изменился title или labels)
- [ ] Hydrate labels + URL в ответе
- [ ] Тесты: создание во всех контейнерах; subtask наследует поля; auto-labels: mask matched/unmatched/case-sensitive; removedAutoLabels уважается; PATCH не трогает placement; неизвестная метка → validation_failed
- [ ] `just test ./...` зелёный

### Task 12: Tasks actions (complete/move/plan/pin), views, bulk, search

**Files:**
- Create: `internal/httpapi/handlers/{task_actions,task_views,task_bulk,search}.go`, `internal/service/{complete,move,plan}.go`, тесты

- [ ] complete: для recurring (`recurrence_rule != NULL`) парсить RRULE через rrule-go, считать next due_at от текущего due_at (или now если в прошлом); если итератор истощён → status=completed; иначе due_at=next, status=open
- [ ] uncomplete, cancel — простые UPDATE
- [ ] move: принимает один из inboxId / contextId{,projectId{,sectionId}} / parentId; одна транзакция: переместить задачу + всех потомков; запреты: подзадача в инбокс, цикл (target ∈ subtree)
- [ ] plan: проверяет `weekly.limit`/`backlog.limit` до UPDATE → `limit_exceeded`
- [ ] pin/unpin: проверяет `max-pinned` для задач (отдельно от проектов)
- [ ] Views: `/tasks/{today,tomorrow,overdue,week,backlog}` — окна в TZ конфига; today/tomorrow/overdue с пагинацией и фильтрами `contextId,projectId,labelId,priority`; week/backlog без пагинации
- [ ] Bulk: `/tasks/bulk/complete`, `/tasks/bulk/move` — независимые транзакции, ответ `{succeeded, failed}`
- [ ] `GET /api/v1/search?q=&type=tasks|projects|all&limit=&offset=` — min q≥2 → validation_failed; LIKE; envelope per type
- [ ] Тесты: recurring complete продвигает дату; cycle-detect в move; subtree-move; weekly limit; pin limit; today граница в not-UTC TZ; bulk частичный успех; search type фильтрация
- [ ] `just test ./...` зелёный

### Task 13: Сборка main, sessions cleanup, smoke-тест

**Files:**
- Modify: `cmd/turboist/main.go`
- Create: `internal/httpapi/server_smoke_test.go`

- [ ] В `main.go`: load config → open db → run migrations → build deps (repos, auth, services, handlers) → start session cleanup → запустить Fiber на BIND → graceful shutdown по SIGINT/SIGTERM
- [ ] Smoke-тест: поднять весь app, прогнать setup → login → создать context/project/task → complete → views; убедиться что 200/201
- [ ] `just test ./...` зелёный

### Task 14: Verify acceptance criteria

- [ ] `just test-all` зелёный
- [ ] `just lint` зелёный
- [ ] `just coverage` — `repo`, `service`, `auth` ≥ 80%
- [ ] `just build` собирает бинарь без ошибок

### Task 15: Update documentation

- [ ] README.md: дополнить разделом «Backend» — как запускать, env vars, миграции
- [ ] Создать (или обновить) `docs/architecture/backend.md` с layout `internal/...`
- [ ] Переместить этот план в `docs/plans/completed/`

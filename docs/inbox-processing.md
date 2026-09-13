# Inbox processing (LLM)

A background job files open Inbox tasks into projects with the help of a language model. For every
task it picks a project, adds labels and, when the wording makes it explicit, sets a priority and a
due date. The model can also decline — the task then stays in the Inbox. Every decision is written to
a journal, and a filed task can be returned to the Inbox with one click.

The feature is **off by default**. It talks to any OpenAI-compatible chat completions API; OpenRouter
is the default provider.

## What leaves the server

For each task the provider receives:

- the task's title, description, creation time and current labels;
- the names, descriptions, types, contexts and labels of all **open** projects;
- the names of all labels and contexts;
- the server's current time, timezone and your interface language.

Private projects, labels and tasks (`isPrivate`) are included: that flag controls the Public View
screen mode, not what is shared with a provider you configured yourself. The API key never leaves the
server — it is not logged, not returned by any endpoint and not shown in the UI.

## Enabling

Set the variables in the server environment (or `.env`) and restart:

| Variable | Required | Default | Description |
|---|---|---|---|
| `INBOX_PROCESSING_ENABLED` | — | `false` | Turns the background job and the manual run on |
| `INBOX_PROCESSING_INTERVAL` | — | `3m` | How often the Inbox is checked, Go duration, minimum `30s` |
| `INBOX_PROCESSING_API_URL` | — | `https://openrouter.ai/api/v1` | Base of the OpenAI-compatible API; `/chat/completions` is appended |
| `INBOX_PROCESSING_API_KEY` | with `ENABLED=true` | — | Sent as `Authorization: Bearer …` |
| `INBOX_PROCESSING_MODEL` | with `ENABLED=true` | — | Model id at the provider, e.g. `openai/gpt-4.1-mini` |
| `INBOX_PROCESSING_BATCH_LIMIT` | — | `10` | Maximum tasks sent to the model per run (1..100) |
| `INBOX_PROCESSING_TIMEOUT` | — | `60s` | Timeout of a single model request |

`INBOX_PROCESSING_ENABLED=true` without a key or a model is a startup error rather than a silent
no-op.

OpenRouter example:

```sh
INBOX_PROCESSING_ENABLED=true
INBOX_PROCESSING_API_KEY=sk-or-v1-...
INBOX_PROCESSING_MODEL=openai/gpt-4.1-mini
```

Any other OpenAI-compatible endpoint works the same way — set `INBOX_PROCESSING_API_URL` to its base
(for example `https://api.openai.com/v1`, or a local server such as `http://localhost:11434/v1`).

With the feature disabled, **Settings → Inbox** still shows the status card, the prompt editor and
the journal.

## Pausing

**Settings → Inbox → Pause automatic processing** stops the scheduled runs without touching the
environment or restarting the server. The pause is stored with the app settings, so it survives a
restart. While paused the background job does not read the Inbox or call the provider; **Process
now** still works, which lets you review decisions one batch at a time. Turning the switch off
resumes the schedule from the next tick.

## How a run works

1. A run starts right after the server boots, then every `INBOX_PROCESSING_INTERVAL`, or immediately
   from **Settings → Inbox → Process now**. Runs never overlap. Scheduled runs are skipped while
   processing is paused.
2. It picks up to `INBOX_PROCESSING_BATCH_LIMIT` open Inbox tasks, oldest first, that still need a
   decision: tasks never looked at, tasks edited since the last decision, and failed tasks whose
   retry time has come. When there are none, the run ends without touching the provider.
3. Each task is sent as its own request (one malformed answer never affects other tasks). The system
   message is your prompt followed by the fixed answer format; the user message is the task as JSON:

   ```json
   {"id": 42, "title": "Fix login redirect on Safari", "description": "", "createdAt": "2026-09-13T09:12:00+03:00", "labels": []}
   ```

4. The answer is validated and applied (see below), and the decision is journaled.
5. When at least one task was filed, every client is told to refresh its task lists.

### The answer

The server always appends this format to the prompt; it cannot be changed from the UI:

```json
{
  "action": "sort" | "keep",
  "projectId": <project id> | null,
  "labelIds": [<label ids>],
  "priority": "high" | "medium" | "low" | "no-priority" | null,
  "dueDate": "YYYY-MM-DD" | null,
  "confidence": <0.0 to 1.0>,
  "reason": "<one short sentence>"
}
```

The request asks for `response_format: json_object`; a model that rejects that field is retried once
without it. Markdown fences or a sentence around the JSON are tolerated.

### Validation and applying

- `keep`, or a `confidence` below `0.5`, leaves the task in the Inbox. It is not sent again until you
  edit its title or description.
- A `projectId` that is not an open project is a model error: the task is marked failed and retried
  later.
- Unknown label ids are dropped. Labels are only ever **added** — labels already on the task (for
  example from auto-label rules) stay.
- A priority outside the allowed values is ignored. A project that sits in a daily-plan (Troiki)
  bucket always gets that bucket's priority, whatever the model said.
- A due date is taken as a whole day in the server's timezone; a date in the past is ignored.
- A priority or due date you already set on the task is never overwritten — the model only fills
  gaps.
- If you edit or move the task while the model is thinking, your change wins and the answer is
  discarded.

The move goes through the same service as a manual move, so every placement rule holds. A filed task
carries the **Filed by AI** marker (a sparkle next to its title, `autoSortedAt` in the API). Moving
the task yourself removes the marker: from then on the placement is your decision.

### Failures and retries

- **One task fails** (unparseable answer, unknown project, a request the provider refused because of
  this task): the task is retried after `interval`, `2×interval`, `4×interval`, `8×interval`. After
  five failed attempts it waits until you edit it.
- **The provider fails** (rate limit, 5xx, network error, timeout, rejected key or unknown model): the
  run stops, no task is charged an attempt, and the scheduled runs pause for
  `interval × 2ⁿ` (at most 30 minutes). The error is shown as **Last error** in Settings. **Process now**
  ignores the pause.
- A prompt that no longer renders (for example after a manual database edit) stops the run and is
  reported as **Last error**.

## The prompt

**Settings → Inbox → Prompt** edits the instruction part of the system message. It is a Go
[text/template](https://pkg.go.dev/text/template); a typo in a field name is refused on save with the
exact error. **Preview** renders the text in the editor against your live projects and labels and the
oldest Inbox task (or a sample task when the Inbox is empty). **Reset to default** stores an empty
prompt, which means "use the built-in default" — improvements to the default reach you with new
releases for as long as you have not customised it.

Available data:

| Variable | Content |
|---|---|
| `.Now` | Current date and time in the server timezone (RFC 3339) |
| `.Timezone` | Server timezone name |
| `.Locale` | Your interface language (`en`, `ru`) |
| `.Contexts` | Contexts: `.ID`, `.Name` |
| `.Projects` | Open projects: `.ID`, `.Title`, `.Description`, `.Type` (`generic` / `software`), `.Context`, `.Labels` (names), `.TroikiCategory` (`important` / `medium` / `rest` or empty) |
| `.Labels` | Labels: `.ID`, `.Name` |
| `.Priorities` | `high`, `medium`, `low`, `no-priority` |
| `.Task` | The task: `.ID`, `.Title`, `.Description`, `.CreatedAt`, `.Labels` (names) |

Functions: `join` joins a list of strings — `{{join .Labels ", "}}`.

Example — list projects with their labels:

```text
{{range .Projects}}
- id={{.ID}} "{{.Title}}" ({{.Context}}){{if .Labels}}: {{join .Labels ", "}}{{end}}
{{end}}
```

Only ids that appear in the catalogue can be applied, so a prompt that omits the project list leaves
the model nothing it could file into.

## Journal and revert

**Settings → Inbox → Journal** lists the latest decisions: outcome (filed / kept / failed), the
project and the labels that were added, the model's reason, confidence and any error. Rows are kept
for 90 days; a deleted task keeps its rows with the title it had.

**Return to Inbox** on a filed task:

- moves it back to the Inbox and removes the marker;
- removes only the labels the model added, and restores priority and due date only if they still
  hold the values the model set — anything you changed afterwards stays;
- remembers the task, so it is not filed again until you edit it.

A task that has since become a subtask cannot be returned (`forbidden_placement`), and a decision can
be reverted only once.

## API

| Endpoint | Scope | Description |
|---|---|---|
| `GET /api/v1/inbox/processing` | `settings:read` | Status: `enabled`, `model`, `apiHost`, `interval`, `batchLimit`, `running`, `pendingCount`, `lastRunAt`, `lastRunSummary`, `lastError`, `backoffUntil`, `paused`, `defaultPrompt` |
| `POST /api/v1/inbox/processing/run` | `tasks:write` | Queue an immediate run: `202 {"running": true}`, or `409 inbox_processing_disabled` |
| `POST /api/v1/inbox/processing/preview` | `settings:read` | `{"prompt": "…"}` renders that text (empty string = the default); no body renders the saved prompt. `422` with `details.error` for a broken template |
| `GET /api/v1/inbox/processing/log?limit&offset` | `tasks:read` | Journal, newest first, standard paged envelope |
| `POST /api/v1/inbox/processing/log/:id/revert` | `tasks:write` | Return the task to the Inbox; answers with the task. `404`, `409 conflict`, `422 forbidden_placement` |
| `PUT /api/v1/app-settings/inbox-processing` | `settings:write` | `{"prompt": "…", "paused": true}` — either field or both; a field left out is kept. The prompt is validated by rendering it against the live catalogue (`422` on error); an empty prompt or the unchanged default stores the default |

The saved prompt and the pause are part of the app settings payload as `inboxProcessing.prompt` and
`inboxProcessing.paused`, and every task carries `autoSortedAt`.

## Diagnostics

- **Settings → Inbox** shows the last run, its summary, the last error and a provider pause.
- Log lines use plain messages such as `inbox processing run started`, `inbox processing filed a
  task`, `inbox processing kept a task in the inbox`, `inbox processing could not decide on a task`
  (WARN) and `inbox processing paused after a provider failure` (WARN). Set `LOG_LEVEL=debug` to see
  skipped runs and the `response_format` retry.
- A journal row with outcome **failed** carries the exact error; a model that keeps answering in
  prose shows up there as `no JSON object in the answer`.

## Storage

- `tasks.auto_sorted_at` — the marker. It travels to native replicas through the ordinary task change.
- `inbox_processing_state` — the processor's memory about tasks still in the Inbox (kept, failed with
  retry schedule, reverted). Rows of tasks that left the Inbox are pruned daily.
- `inbox_processing_log` — the journal, pruned after 90 days.

Neither table is replicated to native clients or included in backups; a backup does carry each task's
`autoSortedAt`.

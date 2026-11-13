# Labels

A label tags tasks across projects. Besides a name, a colour and the favourite /
private flags, a label can carry a **description**: plain text that says what the
label means and when to apply it.

## Descriptions

- Optional, plain text, up to **500 characters** (counted in characters, not bytes).
  Surrounding whitespace is trimmed on save; an empty string means "no description".
- Edited in the label editor: on the web, the label sheet opened from the sidebar
  ("New label") or from a label's own page ("Edit"); on Android, the editor sheet on
  the Labels screen ("+") and on a label's own screen.
- Exposed by the API as `description` on every label payload (`GET /api/v1/labels`,
  the labels embedded in tasks and projects, sync snapshots and change batches) and
  accepted by `POST /api/v1/labels` and `PATCH /api/v1/labels/:id`. A `PATCH` without
  the field leaves the stored description untouched. Longer text is rejected with
  `400 validation_failed`.
- Included in backups; a backup made before descriptions existed restores with empty
  ones.
- Read by the Android client's on-device label suggestions, together with the titles
  and descriptions of the projects that carry the label. A description that says what
  kind of work the label is for helps the right label rank higher in the label picker;
  see the label suggestions section of `docs/mobile.md`.

## Labels in a task's JSON copy

"Copy as JSON" on a task (web and Android) puts the task and its whole subtree on the
clipboard. Each task names its labels; the root carries a `labelGlossary` that
describes every label used anywhere in the tree exactly once, sorted by name
(case-insensitive). The glossary exists so an AI agent reading the copy learns what
each label means without the same text repeated on every task.

```json
{
  "title": "Fix login bug",
  "description": "Users on Safari cannot sign in",
  "labels": ["backend"],
  "subtasks": [
    {
      "title": "Reproduce",
      "description": "",
      "labels": ["backend", "urgent"]
    }
  ],
  "labelGlossary": [
    { "name": "backend", "description": "Server-side Go code" },
    { "name": "urgent", "description": "Must ship today" }
  ]
}
```

- `labels` is always present, possibly empty; `subtasks` appears only when there are
  subtasks; `labelGlossary` is always present on the root, possibly empty.
- A label without a description appears in the glossary with `"description": ""`.

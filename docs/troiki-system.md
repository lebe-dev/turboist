# Troiki System

Troiki is a strategic task management system based on the constraint of the number 3.

## Structure

The unit of work is a **project**. Each project may be assigned to one of three categories, each holding a maximum of 3 projects (all tasks of an assigned project, including subtasks, are shown together):

| Category  | Description                                          |
|-----------|------------------------------------------------------|
| Important | Highest priority projects. Always available.         |
| Medium    | Unlocked by completing tasks in Important projects.  |
| Rest      | Unlocked by completing tasks in Medium projects.     |

When a project is assigned to a category, the priority of all its open tasks (root + subtasks) is forced to the category's derived priority. Manual priority edits are rejected while a project has a category.

## Rules

1. **Important** — projects can be assigned at any time, as long as fewer than 3 are placed there.
2. **Medium** — each completion of a task inside an Important project opens +1 slot in Medium.
3. **Rest** — each completion of a task inside a Medium project opens +1 slot in Rest.
4. Important does **not** affect Rest directly. The chain is: Important → Medium → Rest.
5. Removing a project from a category frees the slot but does **not** open a slot in the next category — only completion does.
6. Earned capacity accumulates and never expires.
7. Capacity grants are idempotent per task (`tasks.troiki_capacity_granted`) — re-completing a task does not yield extra capacity.

## Initial State (initial fill)

Before pressing **Start the system**, all three categories accept projects for pre-fill, subject to the universal "≤ 3 projects per category" rule.

- Important: capacity = 3.
- Medium and Rest: capacity = 3 (initial-fill).

Once started, Medium/Rest switch to accumulation mode: capacity is snapshotted from the current project count in the category and grows further only through task completions in the higher category.

## Recommendations

- Phrase tasks as concrete actionable items following GTD principles.
- After completing a task, add the next one to keep the project moving.

## Reset

Pressing **Reset the system** (visible in place of Start once a cycle is running) returns Troiki to its initial state. With confirmation, it:

- Unassigns every project from its category (`projects.troiki_category = NULL`).
- Clears the per-task capacity-grant flag (`tasks.troiki_capacity_granted = 0`) so the same tasks may grant capacity again in a future cycle.
- Zeroes earned Medium and Rest capacity (`users.troiki_medium_capacity = 0`, `troiki_rest_capacity = 0`).
- Flips the cycle flag back (`users.troiki_started = 0`).

Task priorities are intentionally left untouched — a reset clears the *cycle*, not the user's backlog. The operation is idempotent: calling Reset on an already-reset system is a no-op.

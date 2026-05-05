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

## Initial State

- Important: available immediately (capacity = 3).
- Medium and Rest: locked (capacity = 0). Complete tasks in higher categories to unlock.

## Recommendations

- Phrase tasks as concrete actionable items following GTD principles.
- After completing a task, add the next one to keep the project moving.

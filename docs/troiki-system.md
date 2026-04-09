# Troiki System

Troiki is a strategic task management system based on the constraint of the number 3.

## Structure

Three categories, each holding a maximum of 3 root tasks (subtasks are unlimited):

| Category  | Description                                          |
|-----------|------------------------------------------------------|
| Important | Highest priority tasks. Always available.             |
| Medium    | Unlocked by completing tasks from Important.          |
| Rest      | Unlocked by completing tasks from Medium.             |

## Rules

1. **Important** — tasks can be added at any time, as long as there are fewer than 3.
2. **Medium** — each completion of a task in Important opens +1 slot in Medium.
3. **Rest** — each completion of a task in Medium opens +1 slot in Rest.
4. Important does **not** affect Rest directly. The chain is: Important → Medium → Rest.
5. Deleting a task frees the slot but does **not** open a slot in the next category — only completion does.
6. Earned capacity accumulates and never expires.

## Initial State

- Important: available immediately (capacity = 3).
- Medium and Rest: locked (capacity = 0). Complete tasks to unlock.

## Recommendations

- Phrase tasks as concrete actionable items following GTD principles.
- After completing a task, add the next one.

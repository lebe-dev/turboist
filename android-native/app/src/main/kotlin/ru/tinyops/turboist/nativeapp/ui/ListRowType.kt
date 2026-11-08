package ru.tinyops.turboist.nativeapp.ui

/**
 * The kinds of row a mixed lazy list holds, passed as its `contentType`.
 *
 * Without it a list recycles the composition of a row that scrolled away only
 * into a row that happens to be the same composable, found by trial: a task row
 * leaving the screen is thrown away if a heading comes in next, and the next task
 * row is then composed from nothing, mid-scroll. Saying what each row is lets the
 * list keep the right ones for the rows about to appear.
 */
internal enum class ListRowType {
    HEADING,
    DIVIDER,
    TASK,
    PROJECT,
    NOTE,
    EVENTS,
    TOGGLE,
    FOOTER,
    EMPTY_SLOT,
}

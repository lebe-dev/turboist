package ru.tinyops.turboist.core.model.view

/**
 * Which of the two things a drop over a subtask row means, given where inside
 * its height the pointer sits. The edges read as "relate to this row"
 * ([DEPENDENCY], marked by a padlock); the middle band reads as "go inside this
 * row" ([NEST], marked by an indent arrow) — the same split a file manager's
 * tree uses to tell "drop next to" from "drop into".
 */
enum class SubtaskDropMode {
    DEPENDENCY,
    NEST,
}

// The top/bottom fraction of a row's height that means DEPENDENCY rather than
// NEST. A quarter on each edge leaves the wide middle band for the nest that
// most drags are aiming for, while keeping edges easy to land on deliberately.
private const val DEPENDENCY_EDGE_FRACTION = 0.25f

/** [relativeY] is 0 at the row's top edge and 1 at its bottom edge. */
fun subtaskDropMode(relativeY: Float): SubtaskDropMode =
    if (relativeY < DEPENDENCY_EDGE_FRACTION || relativeY > 1f - DEPENDENCY_EDGE_FRACTION) {
        SubtaskDropMode.DEPENDENCY
    } else {
        SubtaskDropMode.NEST
    }

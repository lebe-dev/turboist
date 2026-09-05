package ru.tinyops.turboist.nativeapp.search

import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task

/**
 * The four things a search can find.
 *
 * The device indexes everything the user named — the work, the folders it is
 * filed in, and the two ways it is tagged — so a search answers across all four
 * at once rather than making the user choose a haystack first. The kinds stay
 * apart in the answer because they are read differently: a task is something to
 * open, a label is a way to narrow a list.
 */
enum class SearchKind {
    TASKS,
    PROJECTS,
    LABELS,
    CONTEXTS,
}

/**
 * The narrowing the search screen offers.
 *
 * Deliberately small, and the size is the result of looking at what the web
 * client actually offers rather than at what a search screen could offer: there,
 * a query is a query, and the only choice is whether to read the task matches or
 * the project ones. The endpoint behind it takes no status, project, label or
 * context filter at all. So this keeps that one choice — widened to the four
 * kinds the device indexes — and adds the single narrowing that only a local
 * index makes cheap: dropping finished work, which is a large share of what a
 * long-lived workspace holds and almost never what a search is looking for.
 *
 * @property kind the one kind to answer with, or `null` for all four.
 * @property openTasksOnly leaves completed and cancelled tasks out. Affects
 *   tasks alone — the other three kinds have no such state.
 */
data class SearchFilters(
    val kind: SearchKind? = null,
    val openTasksOnly: Boolean = false,
) {
    /** True when [kind] is answered by this search at all. */
    fun includes(kind: SearchKind): Boolean = this.kind == null || this.kind == kind
}

/**
 * A matching task, with the one thing a result row needs beside it.
 *
 * The project title is resolved here rather than in the row so that a result
 * list makes one pass over the workspace instead of one lookup per row drawn.
 * `null` means the task lives somewhere that is not a project — the inbox, or a
 * context directly.
 */
data class TaskHit(
    val task: Task,
    val projectTitle: String?,
)

/** Everything one search found, kind by kind, each already in rank order. */
data class SearchResults(
    val tasks: List<TaskHit> = emptyList(),
    val projects: List<Project> = emptyList(),
    val labels: List<Label> = emptyList(),
    val contexts: List<Context> = emptyList(),
) {
    /** How many rows the answer holds in total, across every kind. */
    val total: Int
        get() = tasks.size + projects.size + labels.size + contexts.size

    val isEmpty: Boolean
        get() = total == 0
}

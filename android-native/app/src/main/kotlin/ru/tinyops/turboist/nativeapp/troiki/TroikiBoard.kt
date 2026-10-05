package ru.tinyops.turboist.nativeapp.troiki

import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.model.view.splitByRootCompletion
import ru.tinyops.turboist.core.sync.write.ReplicaRules
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.tasks.taskListRows

/**
 * The three buckets of the daily plan, in the order they are worked through.
 *
 * The order is the method itself — the first bucket is what deserves attention
 * now, and each of the others is earned by finishing work in the one above it —
 * so it is written down once here rather than repeated by every screen that
 * draws the plan.
 */
val TROIKI_CATEGORIES: List<TroikiCategory> =
    listOf(TroikiCategory.IMPORTANT, TroikiCategory.MEDIUM, TroikiCategory.REST)

/**
 * One project standing in a bucket of the daily plan, with the work under it.
 *
 * Finished work is kept apart from open work rather than filtered away: the plan
 * is worked through by ticking things off, and a row that vanished on the tap
 * would take with it the only evidence that the tap did anything.
 */
data class TroikiProjectCard(
    val project: Project,
    val open: List<TaskListRow>,
    val done: List<TaskListRow>,
) {
    /** True once the project is known to hold no work at all. */
    val isEmpty: Boolean get() = open.isEmpty() && done.isEmpty()
}

/**
 * One bucket of the daily plan.
 *
 * [capacity] is the size every bucket starts at, which is the only size this
 * device can state as a fact. A bucket *earns* more room as work is finished in
 * the bucket above it, and that counter is the server's alone — it is not part
 * of the copied data, so the device does not guess at it. [freeSlots] is
 * therefore a floor rather than the truth: it is what is certainly still free,
 * and the server may allow more.
 */
data class TroikiSlot(
    val category: TroikiCategory,
    val projects: List<TroikiProjectCard>,
    val capacity: Int = ReplicaRules.TROIKI_SLOT_BASE_CAPACITY,
) {
    /** How many places are certainly still open in this bucket. */
    val freeSlots: Int get() = (capacity - projects.size).coerceAtLeast(0)
}

/**
 * Cuts the projects of the daily plan, and their work, into the three buckets.
 *
 * Only open projects take part. A finished project keeps the category it was
 * worked on under — the record of what it belonged to is worth keeping — but it
 * holds no place any more, which is the same rule the server counts by.
 *
 * The projects inside a bucket come out pinned first, then most recently pinned,
 * then newest, which is the order the server lists a bucket in. Two lists of the
 * same three projects in different orders would be two different plans to the
 * person reading them.
 */
fun troikiSlots(
    projects: List<Project>,
    tasks: List<Task>,
): List<TroikiSlot> {
    val byProject = tasks.groupBy { it.projectLocalId }
    val standing = projects.filter { it.status == ProjectStatus.OPEN && it.troikiCategory != null }
    return TROIKI_CATEGORIES.map { category ->
        TroikiSlot(
            category = category,
            projects =
                standing
                    .filter { it.troikiCategory == category }
                    .sortedWith(troikiProjectOrder)
                    .map { project -> card(project, byProject[project.localId].orEmpty()) },
        )
    }
}

/**
 * The order a bucket lists its projects in: pinned work first, the most recently
 * pinned of it leading, and the newest project ahead of older ones.
 */
private val troikiProjectOrder: Comparator<Project> =
    compareByDescending<Project> { it.isPinned }
        .thenByDescending { it.pinnedAt ?: Long.MIN_VALUE }
        .thenByDescending { it.createdAt }
        .thenByDescending { it.localId }

private fun card(
    project: Project,
    tasks: List<Task>,
): TroikiProjectCard {
    val split = splitByRootCompletion(tasks)
    // No project title on a row: every row under a card is in the project the
    // card is already named after.
    return TroikiProjectCard(
        project = project,
        open = taskListRows(split.open, emptyMap()),
        done = taskListRows(split.done, emptyMap()),
    )
}

package ru.tinyops.turboist.nativeapp.switcher

import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task

/** One row of the quick switcher's answer. */
sealed interface SwitcherRow {
    val ref: RecentRef

    /** True when the row is one the user opened lately. */
    val recent: Boolean

    data class TaskRow(
        val task: Task,
        /** The project the task sits in, or `null` for the inbox or a bare context. */
        val projectTitle: String?,
        override val recent: Boolean = false,
    ) : SwitcherRow {
        override val ref: RecentRef get() = RecentRef(SwitcherKind.TASK, task.localId)
    }

    data class ProjectRow(
        val project: Project,
        override val recent: Boolean = false,
    ) : SwitcherRow {
        override val ref: RecentRef get() = RecentRef(SwitcherKind.PROJECT, project.localId)
    }
}

/** A search match and where the words hit it: 0 title, 1 a label, 2 the description. */
data class RankedRow(
    val row: SwitcherRow,
    val rank: Int,
)

private fun SwitcherRow.markedRecent(): SwitcherRow =
    when (this) {
        is SwitcherRow.TaskRow -> copy(recent = true)
        is SwitcherRow.ProjectRow -> copy(recent = true)
    }

/**
 * What the quick switcher shows, in order.
 *
 * The same rules as the web client:
 * - with nothing typed, the recently opened rows alone, most recent first;
 * - otherwise the recently opened rows that match lead, most recent first, and
 *   everything else follows by where the words hit (title, label, description),
 *   projects ahead of tasks within one rank, each kind keeping the order its
 *   query returned (open before finished, then most recently touched).
 *
 * [recents] are rows already resolved against the replica, most recent first.
 * [tasks] and [projects] are the matches for [typed]; a recent row that is not
 * among them does not match and is left out.
 */
fun switcherRows(
    typed: String,
    recents: List<SwitcherRow>,
    tasks: List<RankedRow>,
    projects: List<RankedRow>,
    limit: Int = SWITCHER_ROW_LIMIT,
): List<SwitcherRow> {
    if (typed.isBlank()) return recents.take(RECENT_ITEMS_SHOWN).map { it.markedRecent() }

    val matched = LinkedHashMap<RecentRef, RankedRow>()
    for (hit in projects + tasks) matched.putIfAbsent(hit.row.ref, hit)

    val head = recents.mapNotNull { recent -> matched.remove(recent.ref)?.row?.markedRecent() }
    val tail =
        matched.values
            .withIndex()
            .sortedWith(
                compareBy<IndexedValue<RankedRow>> { it.value.rank }
                    .thenBy { if (it.value.row is SwitcherRow.ProjectRow) 0 else 1 }
                    .thenBy { it.index },
            ).map { it.value.row }
    return (head + tail).take(limit)
}

/** How many rows the switcher lists at most. It is a way to reach one thing, not a browser. */
const val SWITCHER_ROW_LIMIT: Int = 30

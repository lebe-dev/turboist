package ru.tinyops.turboist.nativeapp.tasks

import androidx.annotation.StringRes
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.calendar.CalendarEvent
import ru.tinyops.turboist.core.model.view.RelativeDay
import ru.tinyops.turboist.core.model.view.TaskNode
import ru.tinyops.turboist.core.model.view.buildTaskTree
import ru.tinyops.turboist.core.model.view.groupByDayPart
import ru.tinyops.turboist.core.model.view.groupTreeByDueDay
import ru.tinyops.turboist.core.model.view.relativeDay
import java.time.LocalDate
import java.time.ZoneId

/**
 * One line of a rendered list.
 *
 * @property depth how far the task sits below the topmost task of its own tree
 *   *within this list*. A subtask whose parent is out of frame is drawn as a
 *   root, because indenting it under nothing would only look like damage.
 * @property projectTitle the project the task lives in, already resolved, or
 *   `null` when it lives somewhere else — the inbox, or a context directly.
 */
data class TaskListRow(
    val task: Task,
    val depth: Int,
    val projectTitle: String?,
)

/** What a block of rows is headed with. */
sealed interface SectionHeading {
    /** No heading at all: the list is one undivided block. */
    data object None : SectionHeading

    /** Work whose date has gone by, called out above the rest of the day. */
    data object Overdue : SectionHeading

    /**
     * A phase of the day. [active] marks the phase the clock is in now, which is
     * the one the screen leads with visually.
     */
    data class Phase(val part: DayPart, val active: Boolean) : SectionHeading

    /**
     * A calendar day. [day] is `null` for the tasks with no date at all, and
     * [relative] says whether the day has a name of its own.
     */
    data class Day(val day: LocalDate?, val relative: RelativeDay?) : SectionHeading

    /** A heading with a fixed name, such as the backlog. */
    data class Named(
        @param:StringRes val titleRes: Int,
    ) : SectionHeading
}

/**
 * A titled block of a list.
 *
 * [key] is stable for as long as the block means the same thing, so a list
 * redrawn after a change keeps its scroll position instead of jumping.
 *
 * [emptyRes] is the wording the block shows in place of its rows when it holds
 * none. A block that carries one is a fixture of its screen and stays on it
 * whatever it holds; a block without one is only there because something fell
 * into it, and disappears with its last row.
 *
 * [events] is what the user's external calendar has on the same day or phase.
 * It rides on the block rather than in a band of its own because a day is read
 * as one thing — the meetings and the work are the same afternoon — and because
 * an appointment is not a task: it is shown, never ticked, never selected, and
 * nothing here can change it.
 */
data class TaskListSection(
    val key: String,
    val heading: SectionHeading,
    val rows: List<TaskListRow>,
    @param:StringRes val emptyRes: Int? = null,
    val events: List<CalendarEvent> = emptyList(),
)

/**
 * Flattens tasks into rows, nesting subtasks under the parents present in the
 * same list and preserving the order the query already decided.
 */
fun taskListRows(
    tasks: List<Task>,
    projectTitles: Map<Long, String>,
): List<TaskListRow> = rowsOf(buildTaskTree(tasks), projectTitles)

private fun rowsOf(
    nodes: List<TaskNode>,
    projectTitles: Map<Long, String>,
    depth: Int = 0,
): List<TaskListRow> {
    val out = mutableListOf<TaskListRow>()
    for (node in nodes) {
        out += TaskListRow(node.task, depth, node.task.projectLocalId?.let(projectTitles::get))
        out += rowsOf(node.children, projectTitles, depth + 1)
    }
    return out
}

/**
 * The overdue block of a day view, or `null` when nothing is overdue.
 *
 * It is a block of its own rather than rows mixed into the day, because overdue
 * work is not work planned for today — it is a decision the user still owes,
 * and burying it among today's phases hides exactly the thing they need to see.
 */
fun overdueSection(
    tasks: List<Task>,
    projectTitles: Map<Long, String>,
): TaskListSection? {
    if (tasks.isEmpty()) return null
    return TaskListSection("overdue", SectionHeading.Overdue, taskListRows(tasks, projectTitles))
}

/**
 * Cuts a day's tasks into the phases they are meant for. A phase nothing falls
 * into is left out rather than drawn as an empty heading.
 *
 * When the whole day fits in a single phase that phase is marked active
 * whatever the clock says: there is nothing else on screen for the highlight to
 * distinguish it from, and greying out the only block would say the day was
 * over.
 */
fun dayPartSections(
    tasks: List<Task>,
    projectTitles: Map<Long, String>,
    activePart: DayPart?,
): List<TaskListSection> {
    val groups = groupByDayPart(tasks)
    return groups.map { group ->
        TaskListSection(
            key = "phase-" + group.part.name,
            heading = SectionHeading.Phase(group.part, group.part == activePart || groups.size == 1),
            rows = taskListRows(group.tasks, projectTitles),
        )
    }
}

/**
 * Cuts a multi-day list into calendar days, earliest first, undated work last.
 *
 * A whole subtree is filed under the day of its topmost task: a subtask is in
 * such a list on its parent's behalf, and bucketing it on a date of its own
 * would scatter one piece of work across the screen.
 */
fun dueDaySections(
    tasks: List<Task>,
    projectTitles: Map<Long, String>,
    zone: ZoneId,
    today: LocalDate,
): List<TaskListSection> =
    groupTreeByDueDay(buildTaskTree(tasks), zone).map { group ->
        TaskListSection(
            key = "day-" + (group.day?.toString() ?: "undated"),
            heading = SectionHeading.Day(group.day, group.day?.let { relativeDay(it, today) }),
            rows = taskListRows(group.tasks, projectTitles),
        )
    }

/** A block with a fixed name. Returned even when empty, so its heading and its emptiness both show. */
fun namedSection(
    key: String,
    @StringRes titleRes: Int,
    tasks: List<Task>,
    projectTitles: Map<Long, String>,
    @StringRes emptyRes: Int? = null,
): TaskListSection = TaskListSection(key, SectionHeading.Named(titleRes), taskListRows(tasks, projectTitles), emptyRes)

/** A single undivided block, for a list that has nothing to cut itself on. */
fun plainSection(
    key: String,
    tasks: List<Task>,
    projectTitles: Map<Long, String>,
): TaskListSection = TaskListSection(key, SectionHeading.None, taskListRows(tasks, projectTitles))

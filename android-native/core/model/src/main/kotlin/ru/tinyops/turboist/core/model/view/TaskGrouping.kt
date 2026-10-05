package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Task
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

// How a rendered list is cut into sections.
//
// Grouping is not a query: a list query decides membership and order, and these
// functions only fold the result it returned into the headings a screen draws.
// That is why every one of them preserves the incoming order inside a group —
// re-sorting here would quietly overrule the ordering rules the queries exist to
// honour.
//
// Nothing here produces a heading's text. A group carries the value it was cut
// on — a phase of the day, a date, a column — and the screen turns that into
// words in the reader's language.

/** A phase of the day, with the tasks meant for it. */
data class DayPartGroup(
    val part: DayPart,
    val tasks: List<Task>,
)

// The order the day reads in. `NONE` is last because "any time today" is the
// bucket for work with no phase, not a fourth phase after the evening.
private val DAY_PART_ORDER = listOf(DayPart.MORNING, DayPart.AFTERNOON, DayPart.EVENING, DayPart.NONE)

/**
 * Buckets tasks by the phase of the day they are meant for, in the order the day
 * reads. A phase nothing falls into is left out entirely rather than drawn as an
 * empty heading.
 *
 * A phase this build does not recognise — a newer server naming a phase that did
 * not exist when the app was compiled — is folded into "any time today". A task
 * must never disappear from a screen because the name of its bucket is newer
 * than the code reading it.
 */
fun groupByDayPart(tasks: List<Task>): List<DayPartGroup> {
    val buckets = LinkedHashMap<DayPart, MutableList<Task>>()
    for (task in tasks) {
        val part = if (task.dayPart in DAY_PART_ORDER) task.dayPart else DayPart.NONE
        buckets.getOrPut(part) { mutableListOf() }.add(task)
    }
    return DAY_PART_ORDER.mapNotNull { part ->
        buckets[part]?.let { DayPartGroup(part, it) }
    }
}

/**
 * A calendar day, with the tasks that fall on it.
 *
 * [day] is `null` for the tasks that have no date at all. They are a real group —
 * a week list is mostly made of them — and they always come last, after every
 * dated one.
 */
data class DayGroup(
    val day: LocalDate?,
    val tasks: List<Task>,
)

/** Where a day sits relative to today, so a screen can name it in words. */
enum class RelativeDay {
    YESTERDAY,
    TODAY,
    TOMORROW,
    OTHER,
}

/** How [day] reads against [today]: the three days with names of their own, or a date. */
fun relativeDay(
    day: LocalDate,
    today: LocalDate,
): RelativeDay =
    when (day) {
        today -> RelativeDay.TODAY
        today.plusDays(1) -> RelativeDay.TOMORROW
        today.minusDays(1) -> RelativeDay.YESTERDAY
        else -> RelativeDay.OTHER
    }

/** The calendar day an instant falls on, as the user's zone sees it. */
fun dayOf(
    epochMillis: Long,
    zone: ZoneId,
): LocalDate = Instant.ofEpochMilli(epochMillis).atZone(zone).toLocalDate()

/** Buckets tasks by their due day, earliest first, undated tasks last. */
fun groupByDueDay(
    tasks: List<Task>,
    zone: ZoneId,
): List<DayGroup> = groupByDay(tasks.map { it to listOf(it) }, zone, { it.dueAt }, newestFirst = false)

/**
 * Buckets whole subtrees by the due day of their root, earliest first, subtrees
 * with an undated root last.
 *
 * A subtask is in such a list on its parent's behalf — the week list pulls in
 * the whole open subtree of anything it matched — so it belongs under the day
 * its parent was placed on. Bucketing each row on its own date would scatter a
 * tree across the screen, and would file every undated subtask under "no date"
 * while its parent sat on Tuesday.
 */
fun groupTreeByDueDay(
    roots: List<TaskNode>,
    zone: ZoneId,
): List<DayGroup> =
    groupByDay(
        roots.map {
            it.task to listOf(it).flattenTasks()
        },
        zone,
        { it.dueAt },
        newestFirst = false,
    )

/**
 * Buckets tasks by the day they were completed, most recent day first.
 *
 * A task with no completion time is left out: the completion history is a record
 * of what happened, and a row with nothing to record has no day to be filed
 * under.
 */
fun groupByCompletedDay(
    tasks: List<Task>,
    zone: ZoneId,
): List<DayGroup> =
    groupByDay(
        tasks.filter { it.completedAt != null }.map { it to listOf(it) },
        zone,
        { it.completedAt },
        newestFirst = true,
    )

private fun groupByDay(
    entries: List<Pair<Task, List<Task>>>,
    zone: ZoneId,
    instantOf: (Task) -> Long?,
    newestFirst: Boolean,
): List<DayGroup> {
    val dated = LinkedHashMap<LocalDate, MutableList<Task>>()
    val undated = mutableListOf<Task>()
    for ((carrier, members) in entries) {
        val at = instantOf(carrier)
        if (at == null) {
            undated.addAll(members)
            continue
        }
        dated.getOrPut(dayOf(at, zone)) { mutableListOf() }.addAll(members)
    }
    val ordered = if (newestFirst) dated.keys.sortedDescending() else dated.keys.sorted()
    val groups = ordered.mapTo(mutableListOf()) { DayGroup(it, dated.getValue(it)) }
    if (undated.isNotEmpty()) groups.add(DayGroup(null, undated))
    return groups
}

/**
 * A board column with the tasks filed under it. [sectionLocalId] is `null` for
 * the tasks that belong to the project but to none of its columns.
 */
data class SectionGroup(
    val sectionLocalId: Long?,
    val tasks: List<Task>,
)

/**
 * Lays a project's tasks out as a board: first the tasks filed under no column,
 * then one group per column in [sectionOrder].
 *
 * Every requested column is returned even when nothing is filed under it — an
 * empty column is a place to drop work, so it has to be drawn. A task pointing
 * at a column that is not in [sectionOrder] is treated as unfiled rather than
 * dropped, which is what a board shows for the moment between a column being
 * deleted and the pull that clears the tasks' pointer to it.
 */
fun groupBySection(
    tasks: List<Task>,
    sectionOrder: List<Long>,
): List<SectionGroup> {
    val known = sectionOrder.toHashSet()
    val bySection = tasks.groupBy { it.sectionLocalId?.takeIf { id -> id in known } }
    val groups = mutableListOf(SectionGroup(null, bySection[null].orEmpty()))
    sectionOrder.mapTo(groups) { SectionGroup(it, bySection[it].orEmpty()) }
    return groups
}

/** A project with the tasks of it that a list returned. */
data class ProjectGroup(
    val projectLocalId: Long,
    val tasks: List<Task>,
)

/**
 * Groups tasks by project, in the order the projects were asked for.
 *
 * A requested project with nothing in it still gets a group: the plan board
 * draws a slot per project, and a slot that disappeared when its last task was
 * ticked off would look like the project itself had gone.
 */
fun groupByProject(
    tasks: List<Task>,
    projectOrder: List<Long>,
): List<ProjectGroup> {
    val byProject = tasks.groupBy { it.projectLocalId }
    return projectOrder.map { ProjectGroup(it, byProject[it].orEmpty()) }
}

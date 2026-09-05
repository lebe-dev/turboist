package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.view.RelativeDay
import ru.tinyops.turboist.core.model.view.dayOf
import ru.tinyops.turboist.core.model.view.relativeDay
import java.time.LocalDate
import java.time.ZoneId

/** Which of the two places a line of the completion history was read from. */
enum class CompletedSource {
    /** The device's own copy: instant, and there whether or not the phone has a signal. */
    REPLICA,

    /**
     * Fetched from the server for this screen only, and held in memory.
     *
     * It is deliberately not written to the replica. The device keeps a bounded
     * stretch of finished work, and a row from beyond it would be removed again by
     * the very next tidy-up — writing it down would mean churning the database
     * every time somebody scrolled back through last year.
     */
    SERVER,
}

/**
 * One line of the completion history.
 *
 * [key] is what keeps a line in place while the list around it changes, and the
 * two sources are keyed differently because they are identified differently: the
 * replica names a task by the device's own id, while a line that only exists in
 * this screen's memory has no such id and is named the way the server names it.
 */
data class CompletedRow(
    val task: Task,
    val projectTitle: String?,
    val source: CompletedSource,
) {
    val key: String
        get() =
            when (source) {
                CompletedSource.REPLICA -> "replica-${task.localId}"
                CompletedSource.SERVER -> "server-${task.serverId}"
            }
}

/**
 * A day's worth of finished work.
 *
 * [day] is `null` for the rows the server reports as completed without saying
 * when, which is not supposed to happen and is shown at the end rather than
 * dropped: hiding a task because its date is missing would be a worse answer than
 * showing it under no date.
 */
data class CompletedDaySection(
    val day: LocalDate?,
    val relative: RelativeDay?,
    val rows: List<CompletedRow>,
) {
    val key: String
        get() = "day-" + (day?.toString() ?: "undated")
}

/**
 * What the screen has to say about the history below the last line on it.
 *
 * The device holds a bounded stretch of finished work and the server holds the
 * rest, so a completion history has an honest edge to it. This is that edge, and
 * every value of it is a statement rather than a spinner: the one thing a screen
 * must not do at the boundary is imply that something is on its way when nothing
 * is.
 */
enum class OlderHistory {
    /** The replica still holds lines below the ones drawn; showing them costs nothing. */
    IN_REPLICA,

    /** The device's copy is exhausted. Anything older lives on the server and has to be asked for. */
    ON_SERVER,

    /** A request for older history is on the wire. */
    LOADING,

    /** Older history needs a connection, and there is not one. Nothing is pending. */
    OFFLINE,

    /** The server was reached and the request did not succeed. Asking again is worth a try. */
    FAILED,

    /** There is nothing older to show. The history on screen is all of it. */
    EXHAUSTED,
}

/**
 * The lines of the history, the device's own first and the fetched ones after.
 *
 * A fetched row the replica also holds is dropped rather than drawn twice. The
 * two sources are read against the same boundary and so should not overlap at
 * all, but the boundary moves with the clock and the fetch happens a moment after
 * the read: the overlap is a millisecond wide and showing a task twice for it
 * would be plainly wrong.
 */
fun completedRows(
    replicated: List<Task>,
    older: List<Task>,
    projectTitles: Map<Long, String>,
): List<CompletedRow> {
    val known = replicated.mapNotNull { it.serverId }.toSet()
    val local = replicated.map { CompletedRow(it, it.projectTitle(projectTitles), CompletedSource.REPLICA) }
    val fetched =
        older
            .filter { it.serverId == null || it.serverId !in known }
            .map { CompletedRow(it, it.projectTitle(projectTitles), CompletedSource.SERVER) }
    return local + fetched
}

/**
 * Cuts the history into days, most recent first.
 *
 * The order is the one thing this list cannot be given by a query, because it
 * spans two sources. The rows inside a day keep the order they arrived in, which
 * is the order each source put them in — most recently finished first.
 *
 * Nothing is nested. A subtask finished on a different day from its parent cannot
 * be drawn underneath it without taking one of the two out of the day it belongs
 * to, and the day is what this screen is about.
 */
fun completedDaySections(
    rows: List<CompletedRow>,
    zone: ZoneId,
    today: LocalDate,
): List<CompletedDaySection> {
    val dated = LinkedHashMap<LocalDate, MutableList<CompletedRow>>()
    val undated = mutableListOf<CompletedRow>()
    for (row in rows) {
        val at = row.task.completedAt
        if (at == null) {
            undated += row
            continue
        }
        dated.getOrPut(dayOf(at, zone)) { mutableListOf() } += row
    }
    val days =
        dated.entries
            .sortedByDescending { it.key }
            .map { (day, members) -> CompletedDaySection(day, relativeDay(day, today), members) }
    return if (undated.isEmpty()) days else days + CompletedDaySection(null, null, undated)
}

private fun Task.projectTitle(titles: Map<Long, String>): String? =
    projectLocalId?.takeIf { it != NO_LOCAL_ID }?.let(titles::get)

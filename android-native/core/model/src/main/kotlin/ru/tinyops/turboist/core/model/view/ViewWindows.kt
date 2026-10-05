package ru.tinyops.turboist.core.model.view

import java.time.Instant
import java.time.ZoneId

/**
 * A half-open range of instants, `[from, untilExclusive)`, in epoch milliseconds.
 *
 * Half-open is not a detail: it is what makes the last millisecond of a day
 * belong to that day and the first millisecond of the next one belong to the
 * next, with no instant counted twice and none falling through the gap between
 * two adjacent windows.
 */
data class TimeWindow(
    val from: Long,
    val untilExclusive: Long,
) {
    init {
        require(untilExclusive >= from) { "a time window cannot end before it starts" }
    }

    operator fun contains(epochMillis: Long): Boolean = epochMillis >= from && epochMillis < untilExclusive
}

/**
 * The time boundaries every dated list is read against.
 *
 * The lists themselves take absolute instants rather than reading a clock, so a
 * screen renders the same rows no matter when the query object was built and a
 * test can pin a day without pretending to be a different machine. This is where
 * those instants come from.
 *
 * Two different notions of "a day later" live here on purpose, and they mirror
 * the ones the server uses:
 *
 * - a **day window** is a fixed 24 hours from midnight. Due dates are stored as
 *   absolute instants, and an all-day task sits exactly on midnight, so a fixed
 *   span is what puts it in one bucket and only one.
 * - a **week** is seven *calendar* days from Monday, resolved in the user's
 *   zone. Across a daylight-saving change that is 167 or 169 hours, not 168 —
 *   the week has to end on a Monday midnight, not an hour either side of one.
 */
object ViewWindows {
    /** A day window's length. Deliberately fixed; see the note on this object. */
    const val DAY_MILLIS: Long = 24L * 60L * 60L * 1000L

    /** Midnight of the day [now] falls on, in [zone]. */
    fun dayStart(
        now: Instant,
        zone: ZoneId,
    ): Long = now.atZone(zone).toLocalDate().atStartOfDay(zone).toInstant().toEpochMilli()

    /** The 24 hours that begin at [dayStart]. */
    fun day(dayStart: Long): TimeWindow = TimeWindow(dayStart, dayStart + DAY_MILLIS)

    /** The day [now] falls on. */
    fun today(
        now: Instant,
        zone: ZoneId,
    ): TimeWindow = day(dayStart(now, zone))

    /** The day after the one [now] falls on. */
    fun tomorrow(
        now: Instant,
        zone: ZoneId,
    ): TimeWindow = day(dayStart(now, zone) + DAY_MILLIS)

    /**
     * The current week: Monday midnight up to the following Monday midnight.
     * The week starts on Monday everywhere in the product, so this is not a
     * locale-dependent choice.
     */
    fun week(
        now: Instant,
        zone: ZoneId,
    ): TimeWindow {
        val today = now.atZone(zone).toLocalDate()
        val monday = today.minusDays(((today.dayOfWeek.value + 6) % 7).toLong())
        return TimeWindow(
            monday.atStartOfDay(zone).toInstant().toEpochMilli(),
            monday.plusDays(7).atStartOfDay(zone).toInstant().toEpochMilli(),
        )
    }

    /**
     * The trailing window the completion history is read over: [days] days
     * ending at the end of today, today included. A [days] of 1 is today alone.
     */
    fun completedHistory(
        now: Instant,
        zone: ZoneId,
        days: Int,
    ): TimeWindow {
        require(days >= 1) { "a history window must cover at least one day" }
        val start = dayStart(now, zone)
        return TimeWindow(start - (days - 1) * DAY_MILLIS, start + DAY_MILLIS)
    }
}

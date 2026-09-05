package ru.tinyops.turboist.nativeapp.calendar

import ru.tinyops.turboist.core.model.view.TimeWindow
import ru.tinyops.turboist.core.model.view.ViewWindows
import java.time.Instant
import java.time.ZoneId

/**
 * The span of days the app keeps calendar entries for.
 *
 * One span covers every screen that shows entries — today, tomorrow and the
 * current week — rather than one per screen, and it is deliberately a little
 * wider than the union of the three. Two reasons, both about the radio: a phone
 * pays for a request in wake-ups rather than in bytes, so three narrow requests
 * cost far more than one wide one; and a span that already reaches past
 * tomorrow survives midnight, so a device that goes offline in the evening still
 * has the next day's entries in the morning.
 *
 * The span always starts at today's midnight. Entries that have already gone by
 * are part of what today's screen shows — the day is not over — and a span that
 * began at "now" would lose them.
 */
object CalendarWindows {
    /** How far past the end of the current week the span reaches. */
    private const val TRAILING_DAYS = 2L

    /**
     * The span to fetch and keep at [now].
     *
     * It runs from today's midnight to whichever comes later: two days past the
     * end of the current week, or two days past tomorrow. The week is the widest
     * screen, and the extra days are the cushion that makes a day rolling over
     * not an immediate reason to go back to the server.
     */
    fun coverage(
        now: Instant,
        zone: ZoneId,
    ): TimeWindow {
        val dayStart = ViewWindows.dayStart(now, zone)
        val week = ViewWindows.week(now, zone)
        val trailing = TRAILING_DAYS * ViewWindows.DAY_MILLIS
        return TimeWindow(
            from = minOf(dayStart, week.from),
            untilExclusive = maxOf(dayStart + ViewWindows.DAY_MILLIS, week.untilExclusive) + trailing,
        )
    }
}

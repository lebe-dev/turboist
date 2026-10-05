package ru.tinyops.turboist.core.model.calendar

import ru.tinyops.turboist.core.model.view.TimeWindow
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

/**
 * One entry of the user's external calendar, as it is shown beside their tasks.
 *
 * Calendar entries are **not** part of the on-device copy of the user's data and
 * never become one. They are read from the server for the days currently on
 * screen, kept only so those days still read correctly with no network, and
 * never changed from this app: the record lives in the calendar provider, this
 * client only looks at it. Nothing here therefore carries a local id, a sync
 * position or a pending-change marker — the three things every replicated record
 * has — and there is no write path that takes one.
 *
 * @property id the server's own identifier for the entry, unique within a fetch
 *   and stable across fetches, which is all a list needs to key its rows on.
 * @property sourceColour the colour the calendar it came from is drawn in, as the
 *   provider's `#rrggbb` text. Empty when the provider named none.
 * @property startsAt when it begins, in epoch milliseconds. For an all-day entry
 *   this is whatever moment the provider chose to stand for the day, which is why
 *   the day itself is carried separately and is what the screens read.
 * @property startDay the first day an all-day entry covers, `null` for a timed
 *   one.
 * @property endDay the day an all-day entry ends **before** — exclusive, the way
 *   the provider reports it — `null` for a timed one.
 * @property link where the entry can be opened in the provider's own app. Empty
 *   when the provider offered none.
 */
data class CalendarEvent(
    val id: String,
    val title: String,
    val sourceName: String = "",
    val sourceColour: String = "",
    val startsAt: Long = 0,
    val endsAt: Long = 0,
    val startDay: LocalDate? = null,
    val endDay: LocalDate? = null,
    val allDay: Boolean = false,
    val link: String = "",
) {
    /**
     * The calendar day this entry is filed under, where the user is.
     *
     * An all-day entry is filed under the day the provider named rather than
     * under the day its instant falls on: the two disagree by a whole day
     * whenever the provider's idea of midnight and the user's zone differ, and
     * the day the provider named is the one the user's calendar shows.
     */
    fun dayIn(zone: ZoneId): LocalDate = startDay ?: Instant.ofEpochMilli(startsAt).atZone(zone).toLocalDate()

    /**
     * The span the entry occupies, resolved in [zone].
     *
     * An entry that ends before it starts — or one the provider gave no end for
     * — is treated as covering the single millisecond it begins on, so it still
     * lands inside exactly one day rather than falling out of every window.
     */
    fun spanIn(zone: ZoneId): TimeWindow {
        if (allDay) {
            val first = startDay ?: Instant.ofEpochMilli(startsAt).atZone(zone).toLocalDate()
            val afterLast = endDay?.takeIf { it.isAfter(first) } ?: first.plusDays(1)
            return TimeWindow(
                first.atStartOfDay(zone).toInstant().toEpochMilli(),
                afterLast.atStartOfDay(zone).toInstant().toEpochMilli(),
            )
        }
        return TimeWindow(startsAt, maxOf(endsAt, startsAt + 1))
    }

    /** Whether any part of the entry falls inside [window]. */
    fun overlaps(
        window: TimeWindow,
        zone: ZoneId,
    ): Boolean {
        val span = spanIn(zone)
        return span.from < window.untilExclusive && span.untilExclusive > window.from
    }

    /**
     * Whether the entry is already over at [now].
     *
     * An all-day entry counts as over only once the last day it covers has
     * passed: it has no time of day to be measured against, and calling it over
     * at any moment of the day it names would hide it while it is still the
     * answer to "what is on today".
     */
    fun hasEnded(
        now: Instant,
        zone: ZoneId,
    ): Boolean {
        if (!allDay) return endsAt in 1..now.toEpochMilli()
        val last = endDay ?: return false
        return !last.isAfter(now.atZone(zone).toLocalDate())
    }
}

/**
 * How the calendar entries a list shows are ordered: whole days first, then by
 * the moment they begin, then by title so a redraw never shuffles two entries
 * that start together.
 */
fun sortCalendarEvents(events: List<CalendarEvent>): List<CalendarEvent> =
    events.sortedWith(compareBy({ !it.allDay }, { it.startsAt }, { it.title }))

/**
 * Whether the server has a calendar to show entries from at all.
 *
 * Kept beside the entries because the answer decides two different things: that
 * asking for entries is worth a request, and that an empty day means "nothing
 * on" rather than "nothing connected".
 *
 * @property enabled the user's own preference, as the server reports it.
 * @property connectedAccounts how many calendar accounts are authorised.
 * @property selectedSources how many of their calendars the user chose to see.
 */
data class CalendarConnection(
    val enabled: Boolean = false,
    val connectedAccounts: Int = 0,
    val selectedSources: Int = 0,
) {
    /** True when a request for entries can return something. */
    val showsEvents: Boolean
        get() = enabled && connectedAccounts > 0 && selectedSources > 0
}

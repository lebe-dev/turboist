package ru.tinyops.turboist.core.model.calendar

import ru.tinyops.turboist.core.model.view.TimeWindow
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * How an appointment is placed on a day.
 *
 * The awkward case throughout is the whole-day entry. A provider reports one as
 * a pair of bare dates plus an instant chosen in whatever zone it liked, and
 * those two answers disagree by up to a day. The rule pinned here is that the
 * dates win: they are what the user's own calendar shows, and reading the
 * instant instead is how an entry ends up on the wrong side of midnight.
 */
class CalendarEventTest {
    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val day: LocalDate = LocalDate.of(2026, 3, 12)

    private fun at(hour: Int): Long = ZonedDateTime.of(day, LocalTime.of(hour, 0), zone).toInstant().toEpochMilli()

    private fun dayWindow(on: LocalDate = day): TimeWindow =
        TimeWindow(
            on.atStartOfDay(zone).toInstant().toEpochMilli(),
            on.plusDays(1).atStartOfDay(zone).toInstant().toEpochMilli(),
        )

    private fun timed(
        startHour: Int,
        endHour: Int,
    ) = CalendarEvent(id = "t", title = "t", startsAt = at(startHour), endsAt = at(endHour))

    @Test
    fun `a whole-day entry is filed under the day the provider named, not the day its instant falls on`() {
        // The instant sits on the evening of the 11th where the user is, while
        // the provider called it the 12th. The 12th is the answer.
        val event =
            CalendarEvent(
                id = "a",
                title = "Away",
                allDay = true,
                startsAt = ZonedDateTime.of(day.minusDays(1), LocalTime.of(23, 0), zone).toInstant().toEpochMilli(),
                startDay = day,
                endDay = day.plusDays(1),
            )

        assertEquals(day, event.dayIn(zone))
        assertTrue(event.overlaps(dayWindow(), zone))
        assertFalse(event.overlaps(dayWindow(day.minusDays(1)), zone))
    }

    @Test
    fun `a whole-day entry spanning several days is on every one of them`() {
        val event =
            CalendarEvent(
                id = "a",
                title = "Away",
                allDay = true,
                startDay = day,
                endDay = day.plusDays(3),
            )

        assertTrue(event.overlaps(dayWindow(day), zone))
        assertTrue(event.overlaps(dayWindow(day.plusDays(2)), zone))
        assertFalse(event.overlaps(dayWindow(day.plusDays(3)), zone), "the end day is the day it stops")
    }

    @Test
    fun `an entry with no usable end still lands on exactly one day`() {
        val event = CalendarEvent(id = "a", title = "a", startsAt = at(10), endsAt = 0)

        assertTrue(event.overlaps(dayWindow(), zone))
        assertFalse(event.overlaps(dayWindow(day.plusDays(1)), zone))
    }

    @Test
    fun `a timed entry is over once its end has passed`() {
        val event = timed(startHour = 9, endHour = 10)

        assertTrue(event.hasEnded(Instant.ofEpochMilli(at(11)), zone))
        assertFalse(event.hasEnded(Instant.ofEpochMilli(at(9)), zone))
    }

    @Test
    fun `a whole-day entry is over only once the days it covers are behind us`() {
        val event = CalendarEvent(id = "a", title = "a", allDay = true, startDay = day, endDay = day.plusDays(1))
        val duringTheDay = ZonedDateTime.of(day, LocalTime.of(23, 0), zone).toInstant()

        assertFalse(event.hasEnded(duringTheDay, zone), "it is still the answer to what is on today")
        assertTrue(event.hasEnded(duringTheDay.plusSeconds(2 * 3600), zone))
    }

    @Test
    fun `whole-day entries lead the day, then the rest by when they start`() {
        val allDay = CalendarEvent(id = "all", title = "Away", allDay = true, startDay = day)
        val late = timed(startHour = 16, endHour = 17)
        val early = timed(startHour = 9, endHour = 10).copy(id = "early")

        val ordered = sortCalendarEvents(listOf(late, early, allDay))

        assertEquals(listOf("all", "early", "t"), ordered.map { it.id })
    }
}

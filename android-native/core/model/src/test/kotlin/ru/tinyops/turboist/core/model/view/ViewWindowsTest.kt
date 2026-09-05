package ru.tinyops.turboist.core.model.view

import java.time.Instant
import java.time.ZoneId
import java.time.ZonedDateTime
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class ViewWindowsTest {
    private val berlin = ZoneId.of("Europe/Berlin")

    private fun at(text: String): Instant = Instant.parse(text)

    @Test
    fun `a day begins at midnight where the user is, not at midnight in UTC`() {
        // 00:30 UTC on the 15th is already 02:30 on the 15th in Berlin, so the
        // day the user is in started at 22:00 UTC on the 14th.
        val start = ViewWindows.dayStart(at("2026-06-15T00:30:00Z"), berlin)
        assertEquals(at("2026-06-14T22:00:00Z").toEpochMilli(), start)
    }

    @Test
    fun `a day window holds its last millisecond and not the next midnight`() {
        val day = ViewWindows.today(at("2026-06-15T09:00:00Z"), ZoneId.of("UTC"))
        assertTrue(at("2026-06-15T00:00:00Z").toEpochMilli() in day, "midnight opens the day")
        assertTrue(at("2026-06-15T23:59:59.999Z").toEpochMilli() in day, "the last millisecond is still today")
        assertFalse(at("2026-06-16T00:00:00Z").toEpochMilli() in day, "the next midnight belongs to the next day")
    }

    @Test
    fun `tomorrow picks up exactly where today stops`() {
        val now = at("2026-06-15T09:00:00Z")
        val today = ViewWindows.today(now, berlin)
        val tomorrow = ViewWindows.tomorrow(now, berlin)
        assertEquals(today.untilExclusive, tomorrow.from, "no instant may fall between the two lists")
    }

    @Test
    fun `the week runs from Monday to the following Monday`() {
        // A Wednesday.
        val week = ViewWindows.week(at("2026-04-15T09:30:00Z"), ZoneId.of("UTC"))
        assertEquals(at("2026-04-13T00:00:00Z").toEpochMilli(), week.from)
        assertEquals(at("2026-04-20T00:00:00Z").toEpochMilli(), week.untilExclusive)
    }

    @Test
    fun `a Sunday still belongs to the week that started six days earlier`() {
        val week = ViewWindows.week(at("2026-04-19T23:00:00Z"), ZoneId.of("UTC"))
        assertEquals(at("2026-04-13T00:00:00Z").toEpochMilli(), week.from)
    }

    @Test
    fun `a week across a clock change still ends on a Monday midnight`() {
        // Central European Summer Time begins on Sunday 29 March 2026, which
        // makes that week 167 hours long rather than 168.
        val week = ViewWindows.week(at("2026-03-25T12:00:00Z"), berlin)
        val end = ZonedDateTime.ofInstant(Instant.ofEpochMilli(week.untilExclusive), berlin)
        assertEquals(0, end.hour, "the week has to end at midnight, not an hour either side of one")
        assertEquals("MONDAY", end.dayOfWeek.name)
        assertEquals(167L * 60L * 60L * 1000L, week.untilExclusive - week.from)
    }

    @Test
    fun `the history window ends at the end of today and reaches back the days asked for`() {
        val now = at("2026-04-15T09:30:00Z")
        val zone = ZoneId.of("UTC")
        val window = ViewWindows.completedHistory(now, zone, days = 90)
        assertEquals(at("2026-01-16T00:00:00Z").toEpochMilli(), window.from)
        assertEquals(at("2026-04-16T00:00:00Z").toEpochMilli(), window.untilExclusive)
    }

    @Test
    fun `a one-day history is today alone`() {
        val now = at("2026-04-15T09:30:00Z")
        val zone = ZoneId.of("UTC")
        assertEquals(ViewWindows.today(now, zone), ViewWindows.completedHistory(now, zone, days = 1))
    }

    @Test
    fun `a window that ends before it starts is refused rather than silently empty`() {
        assertFailsWith<IllegalArgumentException> { TimeWindow(10L, 9L) }
    }
}

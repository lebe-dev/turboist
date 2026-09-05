package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.DayPart
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** Which phase of the day the clock is in. */
class DayPartScheduleTest {
    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val schedule = DayPartSchedule.Default

    /** Half past [hour], local time, which is unambiguously inside that hour. */
    private fun at(hour: Int): DayPart? =
        schedule.activeAt(LocalDate.of(2026, 3, 12).atTime(hour, 30).atZone(zone).toInstant(), zone)

    @Test
    fun `each phase claims its own hours`() {
        assertEquals(DayPart.MORNING, at(9))
        assertEquals(DayPart.MORNING, at(12))
        assertEquals(DayPart.AFTERNOON, at(13))
        assertEquals(DayPart.AFTERNOON, at(16))
        assertEquals(DayPart.EVENING, at(17))
        assertEquals(DayPart.EVENING, at(21))
    }

    @Test
    fun `the hours outside the day's phases belong to none of them`() {
        assertNull(at(3))
        assertNull(at(8))
        assertNull(at(22))
    }
}

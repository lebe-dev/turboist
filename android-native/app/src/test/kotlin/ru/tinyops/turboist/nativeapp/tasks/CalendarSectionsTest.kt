package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.calendar.CalendarEvent
import java.time.LocalDate
import java.time.ZoneId
import java.time.ZonedDateTime
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Where the user's appointments land among their work.
 *
 * The rule being pinned here is that the calendar is folded in *after* the list
 * has decided what it holds: entries may join a block or bring one into
 * existence, and may never touch the rows, the order or the blocks the tasks
 * produced. Everything below is one case of that.
 */
class CalendarSectionsTest {
    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val day: LocalDate = LocalDate.of(2026, 3, 12)
    private val schedule = DayPartSchedule.Default

    private fun at(
        hour: Int,
        minute: Int = 0,
    ): Long = ZonedDateTime.of(day, java.time.LocalTime.of(hour, minute), zone).toInstant().toEpochMilli()

    private fun event(
        id: String,
        hour: Int,
    ): CalendarEvent =
        CalendarEvent(
            id = id,
            title = "meeting $id",
            startsAt = at(hour),
            endsAt = at(hour + 1),
        )

    private fun allDay(
        id: String,
        on: LocalDate = day,
    ): CalendarEvent =
        CalendarEvent(
            id = id,
            title = "away $id",
            allDay = true,
            startDay = on,
            endDay = on.plusDays(1),
        )

    private fun phases(vararg parts: Pair<DayPart, List<Long>>): List<TaskListSection> =
        parts.map { (part, ids) ->
            TaskListSection(
                key = "phase-" + part.name,
                heading = SectionHeading.Phase(part, active = false),
                rows = taskListRows(ids.map { task(it) }, emptyMap()),
            )
        }

    @Test
    fun `an appointment joins the phase its start falls in`() {
        val sections = phases(DayPart.AFTERNOON to listOf(1L))

        val merged = withPhaseEvents(sections, listOf(event("a", hour = 14)), zone, schedule, DayPart.AFTERNOON)

        val afternoon = merged.single()
        assertEquals(listOf(1L), afternoon.rows.map { it.task.localId }, "the rows the query produced are untouched")
        assertEquals(listOf("a"), afternoon.events.map { it.id })
    }

    @Test
    fun `a phase with appointments and no tasks becomes a block of its own, in the order the day reads`() {
        val sections = phases(DayPart.EVENING to listOf(1L))

        val merged = withPhaseEvents(sections, listOf(event("a", hour = 10)), zone, schedule, DayPart.EVENING)

        assertEquals(
            listOf(DayPart.MORNING, DayPart.EVENING),
            merged.map { (it.heading as SectionHeading.Phase).part },
        )
        assertTrue(merged.first().rows.isEmpty(), "the morning holds no work, only the appointment")
        assertEquals(listOf("a"), merged.first().events.map { it.id })
    }

    @Test
    fun `a whole-day appointment goes to any time rather than to a phase`() {
        val merged = withPhaseEvents(emptyList(), listOf(allDay("holiday")), zone, schedule, null)

        assertEquals(DayPart.NONE, (merged.single().heading as SectionHeading.Phase).part)
    }

    @Test
    fun `an appointment outside every phase of the day still lands on the day`() {
        val merged = withPhaseEvents(emptyList(), listOf(event("dawn", hour = 6)), zone, schedule, null)

        assertEquals(DayPart.NONE, (merged.single().heading as SectionHeading.Phase).part)
        assertEquals(listOf("dawn"), merged.single().events.map { it.id })
    }

    @Test
    fun `the only block on a day is highlighted, however it came to be there`() {
        val merged = withPhaseEvents(emptyList(), listOf(event("a", hour = 10)), zone, schedule, activePart = null)

        assertTrue((merged.single().heading as SectionHeading.Phase).active)
    }

    @Test
    fun `the phase the clock is in is highlighted even when only appointments put it on screen`() {
        val sections = phases(DayPart.EVENING to listOf(1L))

        val merged = withPhaseEvents(sections, listOf(event("a", hour = 14)), zone, schedule, DayPart.AFTERNOON)

        val afternoon = merged.first { (it.heading as SectionHeading.Phase).part == DayPart.AFTERNOON }
        assertTrue((afternoon.heading as SectionHeading.Phase).active)
    }

    @Test
    fun `blocks that are not phases keep their place and take no appointments`() {
        val overdue = overdueSection(listOf(task(9)), emptyMap())!!
        val sections = listOf(overdue) + phases(DayPart.MORNING to listOf(1L))

        val merged = withPhaseEvents(sections, listOf(event("a", hour = 10)), zone, schedule, DayPart.MORNING)

        assertEquals("overdue", merged.first().key)
        assertTrue(merged.first().events.isEmpty(), "overdue work is not a time of day for an appointment to be in")
        assertEquals(listOf("a"), merged.last().events.map { it.id })
    }

    @Test
    fun `a day with no appointments is left exactly as the tasks made it`() {
        val sections = phases(DayPart.MORNING to listOf(1L))

        assertEquals(sections, withPhaseEvents(sections, emptyList(), zone, schedule, DayPart.MORNING))
    }

    @Test
    fun `on a multi-day list an appointment joins the day it falls on`() {
        val sections =
            listOf(
                TaskListSection(
                    key = "day-$day",
                    heading = SectionHeading.Day(day, null),
                    rows = taskListRows(listOf(task(1)), emptyMap()),
                ),
            )

        val merged = withDayEvents(sections, listOf(event("a", hour = 10)), zone, day)

        assertEquals(listOf(1L), merged.single().rows.map { it.task.localId })
        assertEquals(listOf("a"), merged.single().events.map { it.id })
    }

    @Test
    fun `a day with only appointments takes its place among the dated blocks`() {
        val later = day.plusDays(2)
        val sections =
            listOf(
                TaskListSection(
                    key = "day-$later",
                    heading = SectionHeading.Day(later, null),
                    rows = taskListRows(listOf(task(1)), emptyMap()),
                ),
            )

        val merged = withDayEvents(sections, listOf(allDay("holiday", on = day)), zone, day)

        assertEquals(listOf(day, later), merged.map { (it.heading as SectionHeading.Day).day })
        assertEquals(listOf("holiday"), merged.first().events.map { it.id })
    }

    @Test
    fun `the undated block stays last, and no appointment can reach it`() {
        val undated =
            TaskListSection(
                key = "day-undated",
                heading = SectionHeading.Day(null, null),
                rows = taskListRows(listOf(task(1)), emptyMap()),
            )

        val merged = withDayEvents(listOf(undated), listOf(event("a", hour = 10)), zone, day)

        assertEquals(listOf(day, null), merged.map { (it.heading as SectionHeading.Day).day })
        assertTrue(merged.last().events.isEmpty())
    }
}

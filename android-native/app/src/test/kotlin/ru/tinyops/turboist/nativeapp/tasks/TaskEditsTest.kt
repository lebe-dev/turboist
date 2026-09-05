package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.network.Clearable
import ru.tinyops.turboist.core.sync.write.TaskEdit
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The arithmetic behind the two dates, and the promise every editor here makes:
 * an edit carries the fields the user changed and nothing else.
 *
 * The promise is not a nicety. Two devices that changed different fields of one
 * task both keep their change only because neither write mentions the other's
 * field; a screen that sent the whole task would silently undo the other edit
 * the moment its queue drained.
 */
class TaskEditsTest {
    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val day: LocalDate = LocalDate.of(2026, 3, 12)

    private fun millisAt(
        date: LocalDate,
        time: LocalTime,
    ): Long = date.atTime(time).atZone(zone).toInstant().toEpochMilli()

    @Test
    fun `setting a due date on a task that had none touches only the date`() {
        val edit = dueDateEdit(task(1), day, zone)

        assertEquals(listOf("dueAt"), edit.touchedFields())
        assertEquals(Clearable.Set(millisAt(day, LocalTime.MIDNIGHT)), edit.dueAt)
    }

    @Test
    fun `moving a timed task to another day keeps the time of day`() {
        val at = millisAt(day, LocalTime.of(14, 30))
        val edit = dueDateEdit(task(1, dueAt = at, dueHasTime = true), day.plusDays(1), zone)

        assertEquals(listOf("dueAt"), edit.touchedFields())
        assertEquals(Clearable.Set(millisAt(day.plusDays(1), LocalTime.of(14, 30))), edit.dueAt)
    }

    @Test
    fun `picking the day the task is already due on sends nothing`() {
        val at = millisAt(day, LocalTime.MIDNIGHT)
        val edit = dueDateEdit(task(1, dueAt = at), day, zone)

        assertTrue(edit.touchedFields().isEmpty())
    }

    @Test
    fun `emptying a due date takes its time of day with it`() {
        val at = millisAt(day, LocalTime.of(9, 0))
        val edit = dueDateEdit(task(1, dueAt = at, dueHasTime = true), null, zone)

        assertEquals(listOf("dueAt", "dueHasTime"), edit.touchedFields())
        assertEquals(Clearable.Clear, edit.dueAt)
        assertEquals(false, edit.dueHasTime)
    }

    @Test
    fun `emptying an all-day date leaves the flag alone`() {
        val edit = dueDateEdit(task(1, dueAt = millisAt(day, LocalTime.MIDNIGHT)), null, zone)

        assertEquals(listOf("dueAt"), edit.touchedFields())
    }

    @Test
    fun `emptying a date that is already empty sends nothing`() {
        assertTrue(dueDateEdit(task(1), null, zone).touchedFields().isEmpty())
    }

    @Test
    fun `giving an all-day task a time moves the instant and raises the flag`() {
        val at = millisAt(day, LocalTime.MIDNIGHT)
        val edit = dueTimeEdit(task(1, dueAt = at), LocalTime.of(18, 15), zone)

        assertEquals(listOf("dueAt", "dueHasTime"), edit.touchedFields())
        assertEquals(Clearable.Set(millisAt(day, LocalTime.of(18, 15))), edit.dueAt)
        assertEquals(true, edit.dueHasTime)
    }

    @Test
    fun `taking the time off puts the task back on the whole day`() {
        val at = millisAt(day, LocalTime.of(18, 15))
        val edit = dueTimeEdit(task(1, dueAt = at, dueHasTime = true), null, zone)

        assertEquals(listOf("dueAt", "dueHasTime"), edit.touchedFields())
        assertEquals(Clearable.Set(millisAt(day, LocalTime.MIDNIGHT)), edit.dueAt)
        assertEquals(false, edit.dueHasTime)
    }

    @Test
    fun `a task with no due date has nothing to put a time on`() {
        assertTrue(dueTimeEdit(task(1), LocalTime.NOON, zone).touchedFields().isEmpty())
    }

    @Test
    fun `the deadline reads exactly as the due date does`() {
        val at = millisAt(day, LocalTime.of(12, 0))
        val task = task(1, deadlineAt = at, deadlineHasTime = true)

        assertEquals(day, task.deadlineDate(zone))
        assertEquals(LocalTime.of(12, 0), task.deadlineTime(zone))
        assertEquals(listOf("deadlineAt"), deadlineDateEdit(task, day.plusDays(2), zone).touchedFields())
        assertEquals(
            listOf("deadlineAt", "deadlineHasTime"),
            deadlineDateEdit(task, null, zone).touchedFields(),
        )
        assertEquals(
            listOf("deadlineAt", "deadlineHasTime"),
            deadlineTimeEdit(task, null, zone).touchedFields(),
        )
    }

    @Test
    fun `an all-day task reports no time of day even though its instant has one`() {
        val at = millisAt(day, LocalTime.MIDNIGHT)

        assertEquals(day, task(1, dueAt = at).dueDate(zone))
        assertEquals(null, task(1, dueAt = at).dueTime(zone))
    }

    @Test
    fun `setting how a task repeats carries the rule and nothing else`() {
        val edit = recurrenceEdit(task(1), "FREQ=WEEKLY;BYDAY=TU,TH")

        assertEquals(listOf("recurrenceRule"), edit.touchedFields())
        assertEquals(Clearable.Set("FREQ=WEEKLY;BYDAY=TU,TH"), edit.recurrenceRule)
    }

    @Test
    fun `stopping a task repeating clears the rule rather than emptying it`() {
        // An empty rule would come back from the server as a task that repeats by
        // nothing at all, which is a different thing from a task that does not.
        assertEquals(Clearable.Clear, recurrenceEdit(task(1, recurrenceRule = "FREQ=DAILY"), null).recurrenceRule)
        assertEquals(Clearable.Clear, recurrenceEdit(task(1, recurrenceRule = "FREQ=DAILY"), " ").recurrenceRule)
    }

    @Test
    fun `re-saving the rule a task already has changes nothing`() {
        assertTrue(recurrenceEdit(task(1, recurrenceRule = "FREQ=DAILY"), "FREQ=DAILY").touchedFields().isEmpty())
        assertTrue(recurrenceEdit(task(1, recurrenceRule = "FREQ=DAILY"), " FREQ=DAILY ").touchedFields().isEmpty())
        assertTrue(recurrenceEdit(task(1), null).touchedFields().isEmpty())
    }

    @Test
    fun `an untouched edit names no fields`() {
        assertTrue(TaskEdit().touchedFields().isEmpty())
    }
}

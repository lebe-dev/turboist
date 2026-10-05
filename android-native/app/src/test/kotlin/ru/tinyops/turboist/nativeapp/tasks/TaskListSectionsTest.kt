package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.view.RelativeDay
import ru.tinyops.turboist.nativeapp.R
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * How a list of tasks becomes the blocks and rows a screen draws.
 *
 * Nothing here touches Compose or the platform: what a screen shows is decided
 * before anything is drawn, which is what lets the decisions be checked one at a
 * time instead of by reading pixels.
 */
class TaskListSectionsTest {
    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)
    private val titles = mapOf(7L to "Kitchen")

    /** Midday on [day] in the zone the screens read, which is a due date with no time of its own. */
    private fun noon(day: LocalDate): Long = day.atTime(12, 0).atZone(zone).toInstant().toEpochMilli()

    @Test
    fun `a subtask is drawn one level under the parent it was listed with`() {
        val rows = taskListRows(listOf(task(1), task(2, parentLocalId = 1)), titles)

        assertEquals(listOf(1L, 2L), rows.map { it.task.localId })
        assertEquals(listOf(0, 1), rows.map { it.depth })
    }

    @Test
    fun `a subtask whose parent is out of frame is drawn as a row of its own`() {
        val rows = taskListRows(listOf(task(2, parentLocalId = 99)), titles)

        assertEquals(listOf(0), rows.map { it.depth })
    }

    @Test
    fun `a row names the project it lives in, and nothing when it lives elsewhere`() {
        val rows = taskListRows(listOf(task(1, projectLocalId = 7), task(2)), titles)

        assertEquals("Kitchen", rows[0].projectTitle)
        assertNull(rows[1].projectTitle)
    }

    @Test
    fun `a project the titles do not know is left unnamed rather than dropped`() {
        val rows = taskListRows(listOf(task(1, projectLocalId = 404)), titles)

        assertEquals(1, rows.size)
        assertNull(rows[0].projectTitle)
    }

    @Test
    fun `nothing overdue means no overdue block at all`() {
        assertNull(overdueSection(emptyList(), titles))
    }

    @Test
    fun `the overdue block carries what has slipped past its date`() {
        val section = overdueSection(listOf(task(1)), titles)

        assertEquals(SectionHeading.Overdue, section?.heading)
        assertEquals(listOf(1L), section?.rows?.map { it.task.localId })
    }

    @Test
    fun `a day is cut into the phases its tasks name, in the order the day reads`() {
        val sections =
            dayPartSections(
                listOf(
                    task(1, dayPart = DayPart.EVENING),
                    task(2, dayPart = DayPart.MORNING),
                    task(3),
                ),
                titles,
                activePart = DayPart.MORNING,
            )

        assertEquals(
            listOf(DayPart.MORNING, DayPart.EVENING, DayPart.NONE),
            sections.map { (it.heading as SectionHeading.Phase).part },
        )
        assertEquals(listOf(true, false, false), sections.map { (it.heading as SectionHeading.Phase).active })
    }

    @Test
    fun `a phase nothing falls into is not drawn`() {
        val sections = dayPartSections(listOf(task(1, dayPart = DayPart.MORNING)), titles, DayPart.EVENING)

        assertEquals(1, sections.size)
    }

    @Test
    fun `the only block of a day is highlighted whatever the clock says`() {
        val sections = dayPartSections(listOf(task(1, dayPart = DayPart.MORNING)), titles, activePart = null)

        assertTrue((sections.single().heading as SectionHeading.Phase).active)
    }

    @Test
    fun `a multi-day list is cut into days, earliest first and undated last`() {
        val sections =
            dueDaySections(
                listOf(
                    task(1, dueAt = noon(today.plusDays(2))),
                    task(2, dueAt = noon(today)),
                    task(3),
                ),
                titles,
                zone,
                today,
            )

        assertEquals(
            listOf(today, today.plusDays(2), null),
            sections.map { (it.heading as SectionHeading.Day).day },
        )
    }

    @Test
    fun `a day with a name of its own is marked so the screen can use the name`() {
        val sections =
            dueDaySections(
                listOf(task(1, dueAt = noon(today)), task(2, dueAt = noon(today.plusDays(1)))),
                titles,
                zone,
                today,
            )

        assertEquals(
            listOf(RelativeDay.TODAY, RelativeDay.TOMORROW),
            sections.map { (it.heading as SectionHeading.Day).relative },
        )
    }

    @Test
    fun `a subtask is filed under the day of the task it hangs from`() {
        val sections =
            dueDaySections(
                listOf(task(1, dueAt = noon(today)), task(2, parentLocalId = 1)),
                titles,
                zone,
                today,
            )

        val day = sections.single()
        assertEquals(today, (day.heading as SectionHeading.Day).day)
        assertEquals(listOf(1L, 2L), day.rows.map { it.task.localId })
    }

    @Test
    fun `a named block keeps its heading and its own wording when it holds nothing`() {
        val section =
            namedSection(
                key = "backlog",
                titleRes = R.string.page_nextWeek_backlogTitle,
                tasks = emptyList(),
                projectTitles = titles,
                emptyRes = R.string.page_nextWeek_backlogEmptyDesc,
            )

        assertEquals(R.string.page_nextWeek_backlogTitle, (section.heading as SectionHeading.Named).titleRes)
        assertEquals(emptyList(), section.rows)
        assertEquals(R.string.page_nextWeek_backlogEmptyDesc, section.emptyRes)
    }

    @Test
    fun `blocks keep stable keys so a redraw does not scroll the list`() {
        val first = dayPartSections(listOf(task(1, dayPart = DayPart.MORNING)), titles, DayPart.MORNING)
        val second = dayPartSections(listOf(task(2, dayPart = DayPart.MORNING)), titles, DayPart.EVENING)

        assertEquals(first.single().key, second.single().key)
    }
}

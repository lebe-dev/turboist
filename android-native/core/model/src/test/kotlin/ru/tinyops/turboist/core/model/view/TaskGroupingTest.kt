package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Task
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals

class TaskGroupingTest {
    private val berlin = ZoneId.of("Europe/Berlin")

    private fun at(text: String): Long = Instant.parse(text).toEpochMilli()

    private fun task(
        localId: Long,
        dayPart: DayPart = DayPart.NONE,
        dueAt: Long? = null,
        completedAt: Long? = null,
        parentLocalId: Long? = null,
        sectionLocalId: Long? = null,
        projectLocalId: Long? = null,
    ) = Task(
        localId = localId,
        title = "task $localId",
        dayPart = dayPart,
        dueAt = dueAt,
        completedAt = completedAt,
        parentLocalId = parentLocalId,
        sectionLocalId = sectionLocalId,
        projectLocalId = projectLocalId,
        createdAt = 0L,
        updatedAt = 0L,
    )

    @Test
    fun `the day reads morning, afternoon, evening, then anytime`() {
        val groups =
            groupByDayPart(
                listOf(
                    task(1, dayPart = DayPart.NONE),
                    task(2, dayPart = DayPart.EVENING),
                    task(3, dayPart = DayPart.MORNING),
                    task(4, dayPart = DayPart.AFTERNOON),
                ),
            )
        assertEquals(
            listOf(DayPart.MORNING, DayPart.AFTERNOON, DayPart.EVENING, DayPart.NONE),
            groups.map { it.part },
        )
    }

    @Test
    fun `a phase nothing falls into is not drawn`() {
        val groups = groupByDayPart(listOf(task(1, dayPart = DayPart.MORNING)))
        assertEquals(listOf(DayPart.MORNING), groups.map { it.part })
    }

    @Test
    fun `a phase newer than this build still shows its tasks`() {
        // A task must never disappear from a screen because the name of its
        // bucket arrived from a server this build has not heard of.
        val groups = groupByDayPart(listOf(task(1, dayPart = DayPart.UNKNOWN)))
        assertEquals(listOf(DayPart.NONE), groups.map { it.part })
        assertEquals(listOf(1L), groups.single().tasks.map { it.localId })
    }

    @Test
    fun `the order a list decided on survives grouping`() {
        val groups =
            groupByDayPart(
                listOf(task(9, dayPart = DayPart.MORNING), task(2, dayPart = DayPart.MORNING)),
            )
        assertEquals(listOf(9L, 2L), groups.single().tasks.map { it.localId })
    }

    @Test
    fun `due days run earliest first with the undated work last`() {
        val groups =
            groupByDueDay(
                listOf(
                    task(1, dueAt = at("2026-04-17T10:00:00Z")),
                    task(2),
                    task(3, dueAt = at("2026-04-15T10:00:00Z")),
                ),
                ZoneId.of("UTC"),
            )
        assertEquals(
            listOf(LocalDate.parse("2026-04-15"), LocalDate.parse("2026-04-17"), null),
            groups.map { it.day },
        )
    }

    @Test
    fun `a task is filed under the day the user is in, not the day in UTC`() {
        // 22:30 UTC on the 14th is already half past midnight on the 15th in Berlin.
        val groups = groupByDueDay(listOf(task(1, dueAt = at("2026-06-14T22:30:00Z"))), berlin)
        assertEquals(LocalDate.parse("2026-06-15"), groups.single().day)
    }

    @Test
    fun `a subtree is filed under the day of the task it belongs to`() {
        // A subtask is in a week list on its parent's behalf. Filing it on its
        // own date would scatter the tree across the screen and drop every
        // undated subtask into "no date" while its parent sat on Wednesday.
        val roots =
            buildTaskTree(
                listOf(
                    task(1, dueAt = at("2026-04-15T10:00:00Z")),
                    task(2, parentLocalId = 1),
                ),
            )
        val groups = groupTreeByDueDay(roots, ZoneId.of("UTC"))
        assertEquals(listOf(LocalDate.parse("2026-04-15")), groups.map { it.day })
        assertEquals(listOf(1L, 2L), groups.single().tasks.map { it.localId })
    }

    @Test
    fun `completions read most recent day first`() {
        val groups =
            groupByCompletedDay(
                listOf(
                    task(1, completedAt = at("2026-04-13T10:00:00Z")),
                    task(2, completedAt = at("2026-04-15T10:00:00Z")),
                ),
                ZoneId.of("UTC"),
            )
        assertEquals(listOf(LocalDate.parse("2026-04-15"), LocalDate.parse("2026-04-13")), groups.map { it.day })
    }

    @Test
    fun `a task with nothing to record is left out of the history`() {
        assertEquals(emptyList(), groupByCompletedDay(listOf(task(1)), ZoneId.of("UTC")))
    }

    @Test
    fun `the three days with names of their own are recognised`() {
        val today = LocalDate.parse("2026-04-15")
        assertEquals(RelativeDay.TODAY, relativeDay(today, today))
        assertEquals(RelativeDay.TOMORROW, relativeDay(today.plusDays(1), today))
        assertEquals(RelativeDay.YESTERDAY, relativeDay(today.minusDays(1), today))
        assertEquals(RelativeDay.OTHER, relativeDay(today.plusDays(2), today))
    }

    @Test
    fun `a board draws its unfiled tasks first and then every column it was given`() {
        val groups =
            groupBySection(
                listOf(task(1, sectionLocalId = 20L), task(2)),
                sectionOrder = listOf(10L, 20L),
            )
        assertEquals(listOf(null, 10L, 20L), groups.map { it.sectionLocalId })
        assertEquals(listOf(2L), groups[0].tasks.map { it.localId })
        assertEquals(emptyList(), groups[1].tasks.map { it.localId })
        assertEquals(listOf(1L), groups[2].tasks.map { it.localId })
    }

    @Test
    fun `a task pointing at a column the board does not have is drawn as unfiled`() {
        val groups = groupBySection(listOf(task(1, sectionLocalId = 99L)), sectionOrder = listOf(10L))
        assertEquals(listOf(1L), groups.first().tasks.map { it.localId })
    }

    @Test
    fun `a plan slot is drawn for every project asked about, in the order asked`() {
        val groups =
            groupByProject(
                listOf(task(1, projectLocalId = 7L)),
                projectOrder = listOf(7L, 8L),
            )
        assertEquals(listOf(7L, 8L), groups.map { it.projectLocalId })
        assertEquals(emptyList(), groups[1].tasks.map { it.localId })
    }
}

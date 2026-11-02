package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.Test
import kotlin.test.assertEquals

class WeekPlanStatsTest {
    private val work = label(1L, "work")
    private val home = label(2L, "home")
    private val admin = label(3L, "admin")

    @Test
    fun `labels are ordered by usage and then by name`() {
        val stats =
            weekPlanStats(
                listOf(
                    task(1L, labels = listOf(home, work)),
                    task(2L, labels = listOf(work)),
                    task(3L, labels = listOf(admin)),
                    task(4L, labels = listOf(home, work)),
                ),
                emptySet(),
            )

        assertEquals(
            listOf("work" to 3, "home" to 2, "admin" to 1),
            stats.labels.map { it.label.name to it.count },
        )
        assertEquals(4, stats.total)
    }

    @Test
    fun `tasks of projects standing in a bucket count as taken from troiki`() {
        val stats =
            weekPlanStats(
                listOf(
                    task(1L, project = 10L),
                    task(2L, project = 11L),
                    task(3L, project = 12L),
                    task(4L, project = null),
                ),
                setOf(10L, 12L),
            )

        assertEquals(2, stats.fromTroiki)
    }

    @Test
    fun `finished work is left out`() {
        val stats =
            weekPlanStats(
                listOf(
                    task(1L, project = 10L, labels = listOf(work)),
                    task(2L, project = 10L, labels = listOf(work), status = TaskStatus.COMPLETED),
                ),
                setOf(10L),
            )

        assertEquals(listOf(PlannedLabelCount(work, 1)), stats.labels)
        assertEquals(1, stats.fromTroiki)
        assertEquals(1, stats.total)
    }

    @Test
    fun `an empty plan has nothing to count`() {
        assertEquals(WeekPlanStats.EMPTY, weekPlanStats(emptyList(), emptySet()))
    }

    private fun label(
        id: Long,
        name: String,
    ) = Label(localId = id, name = name, createdAt = 0L, updatedAt = 0L)

    private fun task(
        id: Long,
        project: Long? = null,
        labels: List<Label> = emptyList(),
        status: TaskStatus = TaskStatus.OPEN,
    ) = Task(
        localId = id,
        title = "t$id",
        projectLocalId = project,
        status = status,
        labels = labels,
        createdAt = 0L,
        updatedAt = 0L,
    )
}

package ru.tinyops.turboist.nativeapp.troiki

import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.nativeapp.projects.project
import ru.tinyops.turboist.nativeapp.tasks.task
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * How the daily plan is cut into its three buckets.
 *
 * The rules checked here are the ones the plan is a plan because of: three
 * buckets always drawn in the same order, only open projects standing in them,
 * a bucket that starts with room for three, and the same listing order the
 * server uses so the plan reads the same on the phone as in a browser.
 */
class TroikiBoardTest {
    @Test
    fun `the three buckets are always drawn, in the order they are worked through`() {
        val slots = troikiSlots(projects = emptyList(), tasks = emptyList())

        assertEquals(
            listOf(TroikiCategory.IMPORTANT, TroikiCategory.MEDIUM, TroikiCategory.REST),
            slots.map { it.category },
        )
        assertEquals(listOf(3, 3, 3), slots.map { it.freeSlots })
    }

    @Test
    fun `a project stands in the bucket it was assigned to, with the work under it`() {
        val slots =
            troikiSlots(
                projects = listOf(project(7, troikiCategory = TroikiCategory.MEDIUM)),
                tasks = listOf(task(1, projectLocalId = 7), task(2, projectLocalId = 7, parentLocalId = 1)),
            )

        val medium = slots.single { it.category == TroikiCategory.MEDIUM }
        val card = medium.projects.single()
        assertEquals(7L, card.project.localId)
        assertEquals(listOf(1L, 2L), card.open.map { it.task.localId })
        assertEquals(listOf(0, 1), card.open.map { it.depth })
    }

    @Test
    fun `finished work stays under its project rather than disappearing from the plan`() {
        val slots =
            troikiSlots(
                projects = listOf(project(7, troikiCategory = TroikiCategory.IMPORTANT)),
                tasks =
                    listOf(
                        task(1, projectLocalId = 7),
                        task(2, projectLocalId = 7, status = TaskStatus.COMPLETED, completedAt = 5),
                    ),
            )

        val card = slots.first().projects.single()
        assertEquals(listOf(1L), card.open.map { it.task.localId })
        assertEquals(listOf(2L), card.done.map { it.task.localId })
    }

    @Test
    fun `a finished project holds no place, though it keeps the bucket it was worked on under`() {
        val slots =
            troikiSlots(
                projects =
                    listOf(
                        project(7, troikiCategory = TroikiCategory.REST, status = ProjectStatus.COMPLETED),
                        project(8, troikiCategory = TroikiCategory.REST),
                    ),
                tasks = emptyList(),
            )

        val rest = slots.single { it.category == TroikiCategory.REST }
        assertEquals(listOf(8L), rest.projects.map { it.project.localId })
        assertEquals(2, rest.freeSlots)
    }

    @Test
    fun `a bucket lists pinned projects first, then the newest`() {
        val slots =
            troikiSlots(
                projects =
                    listOf(
                        project(1, troikiCategory = TroikiCategory.IMPORTANT).copy(createdAt = 10),
                        project(2, troikiCategory = TroikiCategory.IMPORTANT).copy(createdAt = 30),
                        project(3, troikiCategory = TroikiCategory.IMPORTANT, isPinned = true, pinnedAt = 1)
                            .copy(createdAt = 20),
                    ),
                tasks = emptyList(),
            )

        assertEquals(listOf(3L, 2L, 1L), slots.first().projects.map { it.project.localId })
    }

    @Test
    fun `a full bucket offers no free place`() {
        val slots =
            troikiSlots(
                projects = (1L..3L).map { project(it, troikiCategory = TroikiCategory.IMPORTANT) },
                tasks = emptyList(),
            )

        assertEquals(0, slots.first().freeSlots)
    }

    @Test
    fun `work belonging to another project is not drawn under the plan's projects`() {
        val slots =
            troikiSlots(
                projects = listOf(project(7, troikiCategory = TroikiCategory.IMPORTANT)),
                tasks = listOf(task(1, projectLocalId = 8)),
            )

        assertEquals(emptyList(), slots.first().projects.single().open)
    }
}

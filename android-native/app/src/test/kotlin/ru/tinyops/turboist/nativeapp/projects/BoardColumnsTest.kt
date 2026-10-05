package ru.tinyops.turboist.nativeapp.projects

import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.tasks.task
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * How a project's work is cut into the columns of its board.
 *
 * The cut is the whole of what the board screen decides — the order of the rows
 * inside a column was decided by the query, and the order of the columns by the
 * server — so these checks are about membership and about the two things a
 * column has to get right: where unfiled work goes, and where finished work
 * goes.
 */
class BoardColumnsTest {
    @Test
    fun `the project's own column leads, then the board's columns in order`() {
        val columns =
            boardColumns(
                sections = listOf(section(10, "Doing", position = 0), section(11, "Done", position = 1)),
                tasks = emptyList(),
            )

        assertEquals(listOf(null, 10L, 11L), columns.map { it.sectionLocalId })
        assertEquals(listOf(null, 0, 1), columns.map { it.position })
    }

    @Test
    fun `work in no column falls into the project's own column`() {
        val columns =
            boardColumns(
                sections = listOf(section(10)),
                tasks = listOf(task(1), task(2, sectionLocalId = 10)),
            )

        assertEquals(listOf(1L), columns.first().open.map { it.task.localId })
        assertEquals(listOf(2L), columns.last().open.map { it.task.localId })
    }

    @Test
    fun `work naming a column the board no longer has is not lost`() {
        // Deleting a column keeps the tasks that were in it, on the server and
        // here alike, so a row pointing at one that is already gone has to land
        // somewhere the user can still see it.
        val columns = boardColumns(sections = emptyList(), tasks = listOf(task(1, sectionLocalId = 99)))

        assertEquals(1, columns.size)
        assertEquals(listOf(1L), columns.single().open.map { it.task.localId })
    }

    @Test
    fun `finished work is kept apart from open work rather than dropped`() {
        val columns =
            boardColumns(
                sections = emptyList(),
                tasks = listOf(task(1), task(2, status = TaskStatus.COMPLETED, completedAt = 5)),
            )

        assertEquals(listOf(1L), columns.single().open.map { it.task.localId })
        assertEquals(listOf(2L), columns.single().done.map { it.task.localId })
        assertFalse(columns.single().isEmpty)
    }

    @Test
    fun `a finished parent takes its open subtasks into the finished group with it`() {
        val columns =
            boardColumns(
                sections = emptyList(),
                tasks =
                    listOf(
                        task(1, status = TaskStatus.COMPLETED, completedAt = 5),
                        task(2, parentLocalId = 1),
                    ),
            )

        assertTrue(columns.single().open.isEmpty(), "the subtree moved as one, so nothing is left open")
        assertEquals(listOf(1L, 2L), columns.single().done.map { it.task.localId })
    }

    @Test
    fun `a column knows whether there is anywhere for it to move`() {
        val columns =
            boardColumns(
                sections = listOf(section(10, position = 0), section(11, position = 1), section(12, position = 2)),
                tasks = emptyList(),
            )

        assertEquals(listOf(false, true, true), columns.drop(1).map { it.canMoveEarlier })
        assertEquals(listOf(true, true, false), columns.drop(1).map { it.canMoveLater })
        assertFalse(columns.first().canMoveEarlier, "the project's own column is not part of the board")
        assertFalse(columns.first().canMoveLater)
    }

    @Test
    fun `a subtask is drawn under the parent it shares a column with`() {
        val columns =
            boardColumns(
                sections = listOf(section(10)),
                tasks = listOf(task(1, sectionLocalId = 10), task(2, sectionLocalId = 10, parentLocalId = 1)),
            )

        assertEquals(listOf(0, 1), columns.last().open.map { it.depth })
    }
}

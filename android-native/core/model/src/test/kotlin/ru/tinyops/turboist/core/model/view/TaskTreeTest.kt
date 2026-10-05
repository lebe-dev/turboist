package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.Test
import kotlin.test.assertEquals

class TaskTreeTest {
    private fun task(
        localId: Long,
        parentLocalId: Long? = null,
        status: TaskStatus = TaskStatus.OPEN,
    ) = Task(
        localId = localId,
        title = "task $localId",
        parentLocalId = parentLocalId,
        status = status,
        createdAt = 0L,
        updatedAt = 0L,
    )

    @Test
    fun `subtasks are nested under the parent they name`() {
        val tree = buildTaskTree(listOf(task(1), task(2, parentLocalId = 1), task(3, parentLocalId = 2)))
        assertEquals(listOf(1L), tree.map { it.task.localId })
        assertEquals(listOf(2L), tree.single().children.map { it.task.localId })
        assertEquals(listOf(3L), tree.single().children.single().children.map { it.task.localId })
    }

    @Test
    fun `the order a list decided on survives nesting`() {
        val tree = buildTaskTree(listOf(task(1), task(3, parentLocalId = 1), task(2, parentLocalId = 1)))
        assertEquals(listOf(3L, 2L), tree.single().children.map { it.task.localId })
    }

    @Test
    fun `a subtask whose parent is out of frame is still rendered`() {
        // The today list holds a subtask whose parent is due next month.
        val tree = buildTaskTree(listOf(task(7, parentLocalId = 99)))
        assertEquals(listOf(7L), tree.map { it.task.localId })
    }

    @Test
    fun `a corrupt parent link cannot make the walk loop forever`() {
        val flattened = buildTaskTree(listOf(task(1, parentLocalId = 2), task(2, parentLocalId = 1))).flattenTasks()
        assertEquals(2, flattened.size)
    }

    @Test
    fun `flattening walks parents before their own children`() {
        val tree = buildTaskTree(listOf(task(1), task(2, parentLocalId = 1), task(3)))
        assertEquals(listOf(1L, 2L, 3L), tree.flattenTasks().map { it.localId })
    }

    @Test
    fun `a completed parent takes its open subtasks into the done section`() {
        val split =
            splitByRootCompletion(
                listOf(task(1, status = TaskStatus.COMPLETED), task(2, parentLocalId = 1)),
            )
        assertEquals(listOf(1L, 2L), split.done.map { it.localId })
        assertEquals(emptyList<Long>(), split.open.map { it.localId })
    }

    @Test
    fun `a completed subtask stays under its open parent`() {
        val split =
            splitByRootCompletion(
                listOf(task(1), task(2, parentLocalId = 1, status = TaskStatus.COMPLETED)),
            )
        assertEquals(listOf(1L, 2L), split.open.map { it.localId })
        assertEquals(emptyList<Long>(), split.done.map { it.localId })
    }

    @Test
    fun `a cancelled task is not filed as done`() {
        // Cancelled work was dropped, not finished; the done section is a record
        // of what was achieved.
        val split = splitByRootCompletion(listOf(task(1, status = TaskStatus.CANCELLED)))
        assertEquals(listOf(1L), split.open.map { it.localId })
    }
}

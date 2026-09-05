package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * What a selection of tasks costs to change.
 *
 * The API answers a whole selection in one request for the three actions that
 * have a bulk endpoint, and a client that instead sent one request per task
 * would turn a twenty-task selection into twenty round trips — each with its own
 * chance of arriving out of order, and each replayed separately after a lost
 * answer. So "one action, one request" is a property worth pinning rather than a
 * detail of how the repository happens to be written today.
 *
 * It holds only where the API offers a bulk endpoint. Planning and deleting have
 * none, so a selection of those is honestly several requests; the checks below
 * say which is which so neither can drift into the other unnoticed.
 */
class BulkActionTest : WriteTest() {
    @Test
    fun `moving a selection asks once, naming every task it moved`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val selection = (11..15L).map { givenTask(title = "Task $it", serverId = it) }

            val written = tasks.bulkMove(selection, TaskDestination.InProject(projectLocalId))

            assertContentEquals(selection, written.accepted)
            val op = assertNotNull(queuedOps().single() as? BulkMoveTasksOp)
            assertContentEquals(selection, op.taskLocalIds)
            for (localId in selection) {
                assertEquals(projectLocalId, assertNotNull(db.tasks().byLocalId(localId)).projectLocalId)
            }
        }

    @Test
    fun `re-prioritising a selection asks once, naming every task it changed`() =
        runTest {
            val selection = (11..15L).map { givenTask(title = "Task $it", serverId = it) }

            val written = tasks.bulkPriority(selection, Priority.HIGH)

            assertContentEquals(selection, written.accepted)
            val op = assertNotNull(queuedOps().single() as? BulkTaskPriorityOp)
            assertContentEquals(selection, op.taskLocalIds)
            assertEquals("high", op.priority)
            for (localId in selection) {
                assertEquals(Priority.HIGH, assertNotNull(db.tasks().byLocalId(localId)).priority)
            }
        }

    @Test
    fun `re-prioritising leaves out the tasks whose project fixes their priority`() =
        runTest {
            val contextLocalId = givenContext()
            val planned =
                givenProject(contextLocalId, title = "In the plan", troikiCategory = TroikiCategory.IMPORTANT)
            val fixed =
                givenTask(
                    title = "Fixed by the plan",
                    serverId = 10,
                    projectLocalId = planned,
                    priority = Priority.HIGH,
                )
            val free = (11..13L).map { givenTask(title = "Task $it", serverId = it) }

            val written = tasks.bulkPriority(free + fixed, Priority.LOW)

            assertContentEquals(free, written.accepted)
            assertContentEquals(listOf(fixed), written.locked)
            val op = assertNotNull(queuedOps().single() as? BulkTaskPriorityOp)
            assertContentEquals(free, op.taskLocalIds)
            assertEquals(
                Priority.HIGH,
                assertNotNull(db.tasks().byLocalId(fixed)).priority,
                "the plan's own priority must survive a selection that tried to change it",
            )
        }

    @Test
    fun `a selection whose priority is fixed throughout asks nothing at all`() =
        runTest {
            val planned =
                givenProject(givenContext(), title = "In the plan", troikiCategory = TroikiCategory.REST)
            val fixed = (10..12L).map { givenTask(title = "Task $it", serverId = it, projectLocalId = planned) }

            val written = tasks.bulkPriority(fixed, Priority.HIGH)

            assertTrue(written.accepted.isEmpty())
            assertContentEquals(fixed, written.locked)
            assertTrue(queue().isEmpty(), "a request nothing would change must not be queued")
        }

    @Test
    fun `completing a selection asks once, and names only what it could finish`() =
        runTest {
            val blocker = givenTask(title = "Sign it off", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            givenBlocks(blocker, blocked)
            val free = (12..14L).map { givenTask(title = "Task $it", serverId = it) }

            val written = tasks.bulkComplete(free + blocked)

            assertContentEquals(free, written.accepted)
            assertEquals(mapOf(blocked to listOf(blocker)), written.refused)
            val op = assertNotNull(queuedOps().single() as? BulkCompleteTasksOp)
            assertContentEquals(free, op.taskLocalIds)
        }

    @Test
    fun `a selection nothing in it can be finished asks nothing at all`() =
        runTest {
            val blocker = givenTask(title = "Sign it off", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            givenBlocks(blocker, blocked)

            val written = tasks.bulkComplete(listOf(blocked))

            assertTrue(written.accepted.isEmpty())
            assertEquals(null, written.opId)
            assertTrue(queue().isEmpty(), "an empty request is a round trip that changes nothing")
            assertEquals(TaskStatus.OPEN, assertNotNull(db.tasks().byLocalId(blocked)).status)
        }

    @Test
    fun `planning and deleting a selection are one request per task, because the API offers no batch`() =
        runTest {
            val parked = (11..13L).map { givenTask(title = "Task $it", serverId = it) }

            for (localId in parked) tasks.plan(localId, PlanState.BACKLOG)

            assertEquals(parked.size, queue().size)
            assertTrue(queuedOps().all { it is PlanTaskOp })

            val doomed = (21..23L).map { givenTask(title = "Task $it", serverId = it) }
            for (localId in doomed) tasks.delete(localId)

            assertEquals(parked.size + doomed.size, queue().size)
            assertEquals(doomed.size, queuedOps().count { it is DeleteTaskOp })
        }
}

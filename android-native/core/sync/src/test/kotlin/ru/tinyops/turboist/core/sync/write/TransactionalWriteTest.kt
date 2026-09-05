package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.sync.OutboxState
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * One user action is one transaction.
 *
 * The change the user sees and the request that will report it are written
 * together or not at all. Split, they would sometimes be half-done: a screen
 * showing an edit nobody will ever hear about, or a request for an edit the
 * screen never made — and neither is recoverable afterwards, because nothing
 * would know it had happened.
 */
class TransactionalWriteTest : WriteTest() {
    @Test
    fun `a write leaves the change and the request that reports it`() =
        runTest {
            val projectLocalId = givenProject(givenContext())

            val written = tasks.create(TaskDestination.InProject(projectLocalId), NewTask(title = "Ship it"))

            val row = assertNotNull(db.tasks().byLocalId(written.entityLocalId))
            assertEquals("Ship it", row.title)
            assertEquals(projectLocalId, row.projectLocalId)

            val queued = queue().single()
            assertEquals(written.opId, queued.id)
            assertEquals(OpNames.TASK_CREATE, queued.op)
            assertEquals(ReplicaEntityKind.TASK, queued.entity)
            assertEquals(row.localId, queued.entityLocalId)
            assertEquals(OutboxState.PENDING, queued.state)
        }

    @Test
    fun `a write the queue refuses leaves nothing behind`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val first = tasks.create(TaskDestination.InProject(projectLocalId), NewTask(title = "First"))

            // The second write is queued under an identity the queue already
            // holds, so the queue rejects it — after the task row has been
            // written. If the two were not one transaction, the task would
            // survive as a change nothing will ever report.
            forcedOpId = first.opId
            assertFailsWith<Exception> {
                tasks.create(TaskDestination.InProject(projectLocalId), NewTask(title = "Second"))
            }

            assertEquals(1, db.tasks().count(), "the optimistic row outlived the write that made it")
            assertEquals(1, queue().size)
        }

    @Test
    fun `a refused write changes nothing`() =
        runTest {
            val blocker = givenTask(title = "Blocker", serverId = 10)
            val blocked = givenTask(title = "Blocked", serverId = 11)
            givenBlocks(blocker, blocked)

            assertFailsWith<WriteRefused.TaskBlocked> { tasks.complete(blocked) }

            assertNull(assertNotNull(db.tasks().byLocalId(blocked)).completedAt)
            assertTrue(queue().isEmpty(), "a refused write must not be queued")
        }

    @Test
    fun `the order writes were made in is the order they are queued in`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val taskLocalId = givenTask(projectLocalId = projectLocalId)

            tasks.pin(taskLocalId)
            tasks.patch(taskLocalId, TaskEdit(title = "Renamed"))
            projects.pin(projectLocalId)

            assertEquals(
                listOf(OpNames.TASK_PIN, OpNames.TASK_PATCH, OpNames.PROJECT_PIN),
                queue().map { it.op },
            )
            assertTrue(queue().zipWithNext().all { (a, b) -> a.seq < b.seq })
        }
}

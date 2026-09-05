package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.sync.OutboxState
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.isSynced
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * A row created here exists here first.
 *
 * It gets the device's own identity immediately and the server's later, which is
 * what lets a task written on a train be edited, completed and given subtasks
 * before the train reaches a signal. Until the server has heard of it the row
 * carries no server id, and that absence is the honest signal a screen uses to
 * hide the things that need one — a shareable link, most obviously.
 */
class OfflineIdentityTest : WriteTest() {
    @Test
    fun `a row created here has no server identity yet`() =
        runTest {
            val written = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))

            val row = assertNotNull(db.tasks().byLocalId(written.entityLocalId))
            assertNull(row.serverId)
            assertFalse(
                object : ru.tinyops.turboist.core.model.ReplicaEntity {
                    override val localId = row.localId
                    override val serverId = row.serverId
                }.isSynced,
            )
        }

    @Test
    fun `a write against a row created here names it by the identity it does have`() =
        runTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))

            tasks.complete(created.entityLocalId)

            val queued = queue().last()
            assertEquals(created.entityLocalId, queued.entityLocalId)
            assertEquals(ReplicaEntityKind.TASK, queued.entity)
            assertEquals(created.entityLocalId, (OutboxOpCodec.decode(queued.payload) as CompleteTaskOp).taskLocalId)
        }

    @Test
    fun `work created under work created here is queued behind it`() =
        runTest {
            val contextLocalId = givenContext()
            val project = projects.create(contextLocalId, NewProject(title = "New project"))
            val parent =
                tasks.create(TaskDestination.InProject(project.entityLocalId), NewTask(title = "Parent"))
            val child =
                tasks.create(TaskDestination.SubtaskOf(parent.entityLocalId), NewTask(title = "Child"))

            // Nothing here has a server id yet, and none is needed: the queue is
            // drained in order, so by the time the subtask is sent its parent —
            // and the project both of them live in — have been created and
            // answered for.
            assertEquals(
                listOf(OpNames.PROJECT_CREATE, OpNames.TASK_CREATE, OpNames.TASK_CREATE),
                queue().map { it.op },
            )
            val childOp = queuedOps().last() as CreateTaskOp
            assertEquals(
                TaskDestination.SubtaskOf(parent.entityLocalId),
                childOp.destination,
            )
            assertNull(assertNotNull(db.tasks().byLocalId(child.entityLocalId)).serverId)
        }

    @Test
    fun `a subtask takes the placement of the work it is part of`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val labelLocalId = givenLabel(name = "bug", serverId = 7)
            val parent = givenTask(title = "Parent", serverId = 5, projectLocalId = projectLocalId)
            db.tasks().setLabels(parent, listOf(labelLocalId), NOW)

            val child = tasks.create(TaskDestination.SubtaskOf(parent), NewTask(title = "Child"))

            val row = assertNotNull(db.tasks().byLocalId(child.entityLocalId))
            assertEquals(projectLocalId, row.projectLocalId)
            assertEquals(parent, row.parentLocalId)
            assertEquals(
                listOf(labelLocalId),
                db.tasks().labelsOf(child.entityLocalId).map { it.labelLocalId },
                "a subtask created with no labels of its own inherits its parent's",
            )
        }

    @Test
    fun `a task in the inbox cannot be given subtasks`() =
        runTest {
            val captured =
                givenTask(title = "Captured", serverId = 5, inboxId = ru.tinyops.turboist.core.model.INBOX_ID)

            kotlin.test.assertFailsWith<WriteRefused.Placement> {
                tasks.create(TaskDestination.SubtaskOf(captured), NewTask(title = "Child"))
            }
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `a row a write is queued against is not overwritten by an incoming change`() =
        runTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))

            // What makes the row protected is the queued op pointing at it, which
            // is the only thing that can tell "the user changed this and the
            // server has not been told" apart from "this is stale".
            assertTrue(db.outbox().isDirty(ReplicaEntityKind.TASK, created.entityLocalId))
            assertEquals(OutboxState.PENDING, queue().single().state)
        }
}

package ru.tinyops.turboist.core.sync.drain

import kotlinx.coroutines.runBlocking
import org.junit.Test
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.sync.SyncCycleResult
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskEdit
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * A batch of work done with no server, then a server again.
 *
 * This is the case the whole engine exists for, and the thing it must not do is
 * duplicate. A task written down on a plane, renamed twice and ticked off has to
 * arrive as one task in the state the user left it — not three tasks, not one
 * task in an earlier state, and not one task twice because an answer went
 * missing on the way back.
 */
class ConvergenceTest : DrainTest() {
    @Test
    fun `a batch made offline arrives as one copy of each change`() =
        runReplicaTest {
            val created =
                withServerUnreachable {
                    runBlocking {
                        val write = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
                        tasks.patch(write.entityLocalId, TaskEdit(title = "Buy oat milk"))
                        tasks.complete(write.entityLocalId)
                        tasks.create(TaskDestination.Inbox, NewTask(title = "Call the plumber"))
                        assertEquals(SyncCycleResult.Offline, cycle.runSyncCycle())
                        write
                    }
                }

            assertEquals(SyncCycleResult.Synced, cycle.runSyncCycle())

            assertEquals(
                listOf("Buy oat milk", "Call the plumber"),
                scripted.tasks().map { it.title },
            )
            assertEquals("completed", scripted.task(100)?.status)
            assertEquals(2, db.tasks().count())
            val local = assertNotNull(db.tasks().byLocalId(created.entityLocalId))
            assertEquals(100L, local.serverId)
            assertEquals(TaskStatus.COMPLETED, local.status)
            assertTrue(queue().isEmpty())
            assertTrue(setAside().isEmpty())
        }

    @Test
    fun `a second cycle sends nothing a second time`() =
        runReplicaTest {
            val write = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            tasks.complete(write.entityLocalId)
            cycle.runSyncCycle()
            val alreadySent = scripted.writes().size

            assertEquals(SyncCycleResult.Synced, cycle.runSyncCycle())

            assertEquals(alreadySent, scripted.writes().size)
            assertEquals(1, scripted.tasks().size)
            assertEquals(1, db.tasks().count())
        }

    @Test
    fun `a cycle sends everything queued before it reads anything`() =
        runReplicaTest {
            val write = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            tasks.complete(write.entityLocalId)

            cycle.runSyncCycle()

            val paths = scripted.seen.map { it.path }
            val lastWrite = paths.indexOfLast { !it.startsWith(ScriptedServer.SYNC_PREFIX) }
            val firstRead = paths.indexOfFirst { it.startsWith(ScriptedServer.SYNC_PREFIX) }
            assertTrue(lastWrite < firstRead, "the queue must be sent before the replica is read: $paths")
        }

    @Test
    fun `a cycle whose queue is stuck does not read over the writes it could not send`() =
        runReplicaTest {
            tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            scripted.faults += ScriptedServer.Fault.LoseAnswer()

            val stuck = cycle.runSyncCycle()

            assertTrue(stuck is SyncCycleResult.Refused)
            assertTrue(scripted.seen.none { it.path.startsWith(ScriptedServer.SYNC_PREFIX) })
            assertEquals(1, db.tasks().count())

            assertEquals(SyncCycleResult.Synced, cycle.runSyncCycle())

            assertEquals(1, scripted.tasks().size)
            assertEquals(1, db.tasks().count())
        }
}

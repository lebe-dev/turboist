package ru.tinyops.turboist.core.sync.drain

import kotlinx.coroutines.runBlocking
import org.junit.Test
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.OutboxOpCodec
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskEdit
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** Order, identity and repetition: the three things the queue exists to get right. */
class DrainOrderTest : DrainTest() {
    @Test
    fun `writes reach the server in the order the user made them`() =
        runReplicaTest {
            val first = givenSyncedTask(serverId = 5, title = "Older")
            val second = givenSyncedTask(serverId = 6, title = "Newer")
            scripted.givenTask(5)
            scripted.givenTask(6)
            tasks.patch(second, TaskEdit(title = "Renamed"))
            tasks.complete(first)
            tasks.complete(second)

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(3, result.sent)
            assertEquals(
                listOf("/api/v1/tasks/6", "/api/v1/tasks/5/complete", "/api/v1/tasks/6/complete"),
                scripted.writes().map { it.path },
            )
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `a task created here is named by the server before the writes that follow it go out`() =
        runReplicaTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            tasks.patch(created.entityLocalId, TaskEdit(title = "Buy oat milk"))
            tasks.complete(created.entityLocalId)
            assertNull(serverIdOfTask(created.entityLocalId))

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(3, result.sent)
            assertEquals(100L, serverIdOfTask(created.entityLocalId))
            assertEquals(
                listOf("/api/v1/inbox/tasks", "/api/v1/tasks/100", "/api/v1/tasks/100/complete"),
                scripted.writes().map { it.path },
            )
            val onServer = scripted.tasks().single()
            assertEquals("Buy oat milk", onServer.title)
            assertEquals("completed", onServer.status)
        }

    @Test
    fun `each write goes out under its own queued identity`() =
        runReplicaTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            tasks.complete(created.entityLocalId)

            drainer.drain()

            val keys = scripted.writes().map { it.key }
            assertEquals(listOf("op-1", "op-2"), keys)
        }

    @Test
    fun `a write whose answer was lost is repeated under the same key and lands once`() =
        runReplicaTest {
            tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            scripted.faults += ScriptedServer.Fault.LoseAnswer()

            val stalled = assertIs<DrainResult.Stalled>(drainer.drain())

            assertEquals(0, stalled.sent)
            assertEquals(1, stalled.remaining)
            // The write did land; only its answer did not come back.
            assertEquals(1, scripted.tasks().size)

            val second = drainer.drain()

            assertTrue(second.isDrained)
            assertEquals(1, second.sent)
            assertEquals(1, scripted.tasks().size)
            assertEquals(listOf("op-1", "op-1"), scripted.writes().map { it.key })
        }

    @Test
    fun `an answer the server replayed is read as the answer it stands in for`() =
        runReplicaTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            val row = queue().single()
            val op = OutboxOpCodec.decode(row.payload)
            val sender = OpSender(network, ids)

            val first = sender.send(op, row.id)
            val again = sender.send(op, row.id)

            assertFalse(first.replayed)
            assertTrue(again.replayed)
            assertEquals(first.assignments.map { it.serverId }, again.assignments.map { it.serverId })
            assertEquals(1, scripted.tasks().size)
            assertEquals(created.entityLocalId, again.assignments.single().ref.localId)
        }

    @Test
    fun `a queue with nothing in it is drained without asking the server anything`() =
        runReplicaTest {
            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(0, result.sent)
            assertTrue(scripted.seen.isEmpty())
        }

    @Test
    fun `a write in flight when the process died is sent again rather than abandoned`() =
        runReplicaTest {
            tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            withServerUnreachable { runBlocking { drainer.drain() } }
            assertEquals(1, queue().size)

            val afterRestart = newDrainer().drain()

            assertTrue(afterRestart.isDrained)
            assertEquals(1, scripted.tasks().size)
        }
}

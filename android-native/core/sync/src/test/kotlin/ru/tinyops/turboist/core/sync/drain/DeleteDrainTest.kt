package ru.tinyops.turboist.core.sync.drain

import kotlinx.coroutines.runBlocking
import org.junit.Test
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Removing something, which is the one write whose subject is gone before it is
 * sent.
 *
 * Every other write can be looked up in the replica at the moment it goes out.
 * A delete cannot: the row leaves in the same transaction that queues the write,
 * so unless the id the server knows the row by was written down at that moment
 * the request can never be built — and the thing the user threw away would live
 * on the server for good, with the queue quietly setting the write aside.
 */
class DeleteDrainTest : DrainTest() {
    @Test
    fun `deleting a replicated row names it to the server`() =
        runReplicaTest {
            val localId = givenSyncedTask(serverId = 7, title = "Done with this")
            scripted.givenTask(7)

            tasks.delete(localId)

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(1, result.sent)
            assertEquals(listOf("/api/v1/tasks/7"), scripted.writes().map { it.path })
            assertEquals(listOf("DELETE"), scripted.writes().map { it.method })
            assertTrue(scripted.tasks().isEmpty())
            assertTrue(queue().isEmpty())
            assertTrue(setAside().isEmpty(), "a delete must not end up in the refused pile")
        }

    @Test
    fun `a row written down and thrown away with no connection leaves nothing behind on either side`() =
        runReplicaTest {
            val created =
                withServerUnreachable {
                    runBlocking {
                        val write = tasks.create(TaskDestination.Inbox, NewTask(title = "Never mind"))
                        tasks.delete(write.entityLocalId)
                        write
                    }
                }

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(2, result.sent)
            assertTrue(scripted.tasks().isEmpty())
            assertNull(db.tasks().byLocalId(created.entityLocalId))
            assertTrue(queue().isEmpty())
            assertTrue(setAside().isEmpty())
        }
}

package ru.tinyops.turboist.core.sync.drain

import kotlinx.coroutines.runBlocking
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.blockerServerIds
import ru.tinyops.turboist.core.database.sync.OutboxState
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskEdit
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * What each way of failing does to the queue.
 *
 * The distinction the whole design rests on is between a write that cannot be
 * sent *yet* and one that can never be sent at all. The first must hold its
 * place, because the writes behind it were made after it and mean something
 * different in another order. The second must leave, because everything behind
 * it would otherwise wait forever on an answer that will not change.
 */
class DrainFailureTest : DrainTest() {
    @Test
    fun `a queue that cannot reach the server keeps its place and waits`() =
        runReplicaTest {
            val task = givenSyncedTask(serverId = 5)
            scripted.givenTask(5)
            tasks.patch(task, TaskEdit(title = "Renamed"))
            tasks.complete(task)

            val stalled = withServerUnreachable { runBlocking { assertIs<DrainResult.Stalled>(drainer.drain()) } }

            assertEquals(0, stalled.sent)
            assertEquals(0, stalled.quarantined)
            assertEquals(2, stalled.remaining)
            assertIs<ApiException.Network>(stalled.cause)
            assertEquals(DrainBackoff.BASE_DELAY_MILLIS, stalled.retryAfterMillis)
            assertEquals(listOf("op-1", "op-2"), queue().map { it.id })
            assertTrue(setAside().isEmpty())

            val afterNetworkReturned = drainer.drain()

            assertTrue(afterNetworkReturned.isDrained)
            assertEquals(2, afterNetworkReturned.sent)
        }

    @Test
    fun `waiting for a network does not count against a write`() =
        runReplicaTest {
            val task = givenSyncedTask(serverId = 5)
            scripted.givenTask(5)
            tasks.complete(task)

            withServerUnreachable {
                runBlocking {
                    drainer.drain()
                    drainer.drain()
                }
            }

            val waiting = queue().single()
            assertEquals(OutboxState.FAILED, waiting.state)
            assertEquals(0, waiting.attempts)
        }

    @Test
    fun `the wait between attempts grows while the queue stays stuck`() =
        runReplicaTest {
            tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))

            val waits =
                withServerUnreachable {
                    runBlocking {
                        (1..3).map { assertIs<DrainResult.Stalled>(drainer.drain()).retryAfterMillis }
                    }
                }

            assertEquals(waits.sorted(), waits)
            assertTrue(waits.first() < waits.last())
        }

    @Test
    fun `a refused write is set aside and the writes behind it carry on`() =
        runReplicaTest {
            val task = givenSyncedTask(serverId = 5)
            scripted.givenTask(5)
            tasks.complete(task)
            tasks.create(TaskDestination.Inbox, NewTask(title = "Unrelated"))
            scripted.faults += ScriptedServer.Fault.Refuse(409, ApiErrorCodes.TASK_BLOCKED, """{"blockerIds":[9]}""")

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(1, result.sent)
            assertEquals(1, result.quarantined)
            val refused = setAside().single()
            assertEquals(ApiErrorCodes.TASK_BLOCKED, refused.errorCode)
            assertEquals(409, refused.httpStatus)
            assertEquals(task, refused.entityLocalId)
            assertEquals(
                listOf(9L),
                refused.blockerServerIds,
                "which tasks were in the way is the only part of that refusal the user can act on",
            )
            assertTrue(queue().isEmpty())
            assertEquals(1, scripted.tasks().count { it.title == "Unrelated" })
        }

    @Test
    fun `a write against a row the server no longer has is filed as the row being gone`() =
        runReplicaTest {
            val vanished = givenSyncedTask(serverId = 77)
            tasks.complete(vanished)

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(1, result.quarantined)
            assertEquals(ApiErrorCodes.TARGET_GONE, setAside().single().errorCode)
        }

    @Test
    fun `a server that keeps breaking on one write eventually sets it aside`() =
        runReplicaTest {
            val patient = newDrainer(serverErrorLimit = 2)
            tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            repeat(2) { scripted.faults += ScriptedServer.Fault.Refuse(500, ApiErrorCodes.INTERNAL_ERROR) }

            val first = assertIs<DrainResult.Stalled>(patient.drain())
            assertIs<ApiException.Server>(first.cause)
            assertEquals(1, queue().single().attempts)

            val second = patient.drain()

            assertTrue(second.isDrained)
            assertEquals(1, second.quarantined)
            val given = setAside().single()
            assertEquals(ApiErrorCodes.INTERNAL_ERROR, given.errorCode)
            assertEquals(2, given.attempts)
            assertTrue(scripted.tasks().isEmpty())
        }

    @Test
    fun `credentials the server will not accept stop the drain with nothing lost`() =
        runReplicaTest {
            tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))
            scripted.faults += ScriptedServer.Fault.Refuse(401, ApiErrorCodes.AUTH_INVALID)

            val result = assertIs<DrainResult.NeedsCredentials>(drainer.drain())

            assertEquals(1, result.remaining)
            assertEquals(0, result.quarantined)
            assertEquals(OutboxState.PENDING, queue().single().state)
            assertTrue(setAside().isEmpty())
        }

    @Test
    fun `a change this version cannot read is set aside rather than sent`() =
        runReplicaTest {
            queueRaw(id = "op-x", op = "task.fromTheFuture", payload = """{"op":"task.fromTheFuture"}""", 1)

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(1, result.quarantined)
            assertEquals(ApiErrorCodes.WRITE_UNSENDABLE, setAside().single().errorCode)
            assertTrue(scripted.seen.isEmpty())
        }

    @Test
    fun `writes waiting on a creation that was refused are set aside too`() =
        runReplicaTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Ghost"))
            tasks.complete(created.entityLocalId)
            scripted.faults += ScriptedServer.Fault.Refuse(409, ApiErrorCodes.LIMIT_EXCEEDED)

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(2, result.quarantined)
            assertEquals(
                listOf(ApiErrorCodes.LIMIT_EXCEEDED, ApiErrorCodes.WRITE_UNSENDABLE),
                setAside().map { it.errorCode },
            )
            assertTrue(scripted.tasks().isEmpty())
        }

    @Test
    fun `a set-aside write can be discarded and stops being shown`() =
        runReplicaTest {
            val vanished = givenSyncedTask(serverId = 77)
            tasks.complete(vanished)
            drainer.drain()

            val discarded = unsent.discard(setAside().single().id)

            assertTrue(discarded)
            assertTrue(unsent.setAsideNow().isEmpty())
        }

    @Test
    fun `every set-aside write can be discarded at once`() =
        runReplicaTest {
            tasks.complete(givenSyncedTask(serverId = 77))
            tasks.complete(givenSyncedTask(serverId = 78))
            drainer.drain()
            assertEquals(2, unsent.setAsideNow().size)

            assertEquals(2, unsent.discardAll())
            assertTrue(unsent.setAsideNow().isEmpty())
        }

    @Test
    fun `discarding a refused creation takes the row only this device held with it`() =
        runReplicaTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Never landed"))
            scripted.faults += ScriptedServer.Fault.Refuse(HTTP_UNPROCESSABLE, ApiErrorCodes.LIMIT_EXCEEDED)
            drainer.drain()
            assertEquals(1, unsent.setAsideNow().size)

            assertTrue(unsent.discard(setAside().single().id))

            assertNull(
                db.tasks().byLocalId(created.entityLocalId),
                "a creation nobody will ever send must not leave a row on screen for good",
            )
        }

    @Test
    fun `discarding a refused change to a replicated row leaves that row alone`() =
        runReplicaTest {
            val task = givenSyncedTask(serverId = 77)
            scripted.givenTask(77)
            tasks.patch(task, TaskEdit(title = "Renamed"))
            scripted.faults += ScriptedServer.Fault.Refuse(HTTP_UNPROCESSABLE, ApiErrorCodes.LIMIT_EXCEEDED)
            drainer.drain()

            assertTrue(unsent.discard(setAside().single().id))

            assertNotNull(db.tasks().byLocalId(task), "the row is the server's too and a catch-up corrects it")
        }

    private companion object {
        /** What the server answers a write it understood and would not make. */
        const val HTTP_UNPROCESSABLE: Int = 422
    }
}

package ru.tinyops.turboist.core.sync.drain

import org.junit.Test
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.SettingsWriteRepo
import ru.tinyops.turboist.core.sync.write.TaskDestination
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The Today focus lock on its way to the server, and the server's own refusal
 * on the way back.
 */
class TodayLockDrainTest : DrainTest() {
    @Test
    fun `turning the lock on is sent once, under its own key`() =
        runReplicaTest {
            SettingsWriteRepo(db, writer).lockToday()

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(1, result.sent)
            val sent = scripted.seen.single { !it.path.startsWith(ScriptedServer.SYNC_PREFIX) }
            assertEquals("POST", sent.method)
            assertEquals(ScriptedServer.TODAY_LOCK, sent.path)
            assertEquals("op-1", sent.key, "the write must replay under the key it was queued with")
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `a write the server's lock refuses is set aside under that reason`() =
        runReplicaTest {
            // The device did not know about the lock — another device turned it
            // on — so the write went out and came back refused.
            tasks.create(TaskDestination.Inbox, NewTask(title = "Today, please", dueAt = NOW))
            tasks.create(TaskDestination.Inbox, NewTask(title = "Unrelated"))
            scripted.faults +=
                ScriptedServer.Fault.Refuse(
                    409,
                    ApiErrorCodes.TODAY_LOCKED,
                    """{"lockedUntil":"2024-03-10T00:00:00.000Z","projectId":null}""",
                )

            val result = drainer.drain()

            assertTrue(result.isDrained)
            assertEquals(1, result.quarantined)
            assertEquals(1, result.sent)
            assertEquals(ApiErrorCodes.TODAY_LOCKED, setAside().single().errorCode)
        }
}

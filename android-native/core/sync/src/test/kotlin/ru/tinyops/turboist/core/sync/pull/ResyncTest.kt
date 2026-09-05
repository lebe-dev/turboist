package ru.tinyops.turboist.core.sync.pull

import kotlinx.coroutines.runBlocking
import org.junit.Test
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.ApiErrorCodes
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * The two ways a catch-up can be refused, and the two ways it can simply fail.
 *
 * Being told "you cannot continue from there" is not an error the user should
 * ever hear about: the device takes a complete copy and carries on. What it must
 * not do while carrying on is lose the writes it has not sent — those belong to
 * the user, not to the copy of the server's data.
 */
class ResyncTest : SyncTest() {
    private suspend fun seeded(): Long {
        enqueueJson(
            snapshotJson(
                cursor = 100,
                contexts = listOf(contextJson(7)),
                projects = listOf(projectJson(11, contextId = 7)),
                tasks = listOf(taskJson(31, "Original", projectId = 11)),
            ),
        )
        assertTrue(puller.pull().isApplied)
        return assertNotNull(db.tasks().byServerId(31)).localId
    }

    @Test
    fun `a history that was replaced sends the device to a complete copy and the unsent queue survives`() =
        runReplicaTest {
            val localId = seeded()
            val queued = queueWrite(ReplicaEntityKind.TASK, localId)

            enqueueError(409, ApiErrorCodes.SYNC_EPOCH_MISMATCH, """{"epoch":5}""")
            enqueueJson(
                snapshotJson(
                    epoch = 5,
                    cursor = 900,
                    contexts = listOf(contextJson(7)),
                    projects = listOf(projectJson(11, contextId = 7)),
                    tasks = listOf(taskJson(31, "Original", projectId = 11)),
                ),
            )

            val result = puller.pull()

            assertTrue(result is PullResult.Applied && result.fromSnapshot)
            val position = assertNotNull(db.syncState().get())
            assertEquals(5L, position.epoch)
            assertEquals(900L, position.cursor)
            assertEquals(
                listOf(queued.id),
                db.outbox().all().map { it.id },
                "a restore does not discard the user's writes",
            )
            assertEquals(emptyList(), db.outbox().quarantined())
        }

    @Test
    fun `a position the server no longer keeps does the same`() =
        runReplicaTest {
            val localId = seeded()
            queueWrite(ReplicaEntityKind.TASK, localId)

            enqueueError(410, ApiErrorCodes.SYNC_CURSOR_EXPIRED, """{"epoch":1,"oldestRetained":500}""")
            enqueueJson(
                snapshotJson(
                    cursor = 900,
                    contexts = listOf(contextJson(7)),
                    projects = listOf(projectJson(11, contextId = 7)),
                    tasks = listOf(taskJson(31, "Original", projectId = 11)),
                ),
            )

            val result = puller.pull()

            assertTrue(result is PullResult.Applied && result.fromSnapshot)
            assertEquals(900L, assertNotNull(db.syncState().get()).cursor)
            assertEquals(1, db.outbox().all().size)
        }

    @Test
    fun `losing the network leaves the replica and its position exactly as they were`() =
        runReplicaTest {
            seeded()

            val result = withServerUnreachable { runBlocking { puller.pull() } }

            assertTrue(result is PullResult.Offline)
            assertEquals(100L, assertNotNull(db.syncState().get()).cursor)
            assertEquals("Original", assertNotNull(db.tasks().byServerId(31)).title)
        }

    @Test
    fun `a refusal the device cannot act on is reported rather than thrown`() =
        runReplicaTest {
            seeded()
            enqueueError(403, ApiErrorCodes.FORBIDDEN)

            val result = puller.pull()

            assertTrue(result is PullResult.Refused)
            assertEquals(ApiErrorCodes.FORBIDDEN, (result as PullResult.Refused).cause.code)
            assertEquals(100L, assertNotNull(db.syncState().get()).cursor)
        }
}

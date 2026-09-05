package ru.tinyops.turboist.core.sync.pull

import org.junit.Test
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.network.ApiErrorCodes
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * What happens to a row the device has changed and not yet told the server about.
 *
 * The rule has two halves and they point opposite ways on purpose: an incoming
 * *record* loses to the unsent change, and an incoming *deletion* beats it.
 * Anything else either makes the user's edit blink out and back, or leaves a
 * write queued forever against a row that no longer exists.
 */
class DirtyRowTest : SyncTest() {
    /** A replica holding one project and one task, at position 100. */
    private suspend fun seeded() {
        enqueueJson(
            snapshotJson(
                cursor = 100,
                contexts = listOf(contextJson(7)),
                projects = listOf(projectJson(11, contextId = 7)),
                tasks = listOf(taskJson(31, "Original", projectId = 11)),
            ),
        )
        assertTrue(puller.pull().isApplied)
    }

    @Test
    fun `an incoming record leaves a row with an unsent change alone`() =
        runReplicaTest {
            seeded()
            val task = assertNotNull(db.tasks().byServerId(31))
            db.tasks().update(task.copy(title = "Renamed here"))
            queueWrite(ReplicaEntityKind.TASK, task.localId)

            enqueueJson(
                changesJson(
                    listOf(upsert("task", 101, 31, taskJson(31, "Renamed there", projectId = 11))),
                    cursor = 150,
                ),
            )

            assertTrue(puller.pull().isApplied)

            assertEquals("Renamed here", assertNotNull(db.tasks().byServerId(31)).title)
            assertEquals(150L, assertNotNull(db.syncState().get()).cursor, "the position still moves past the page")
        }

    @Test
    fun `a row with nothing queued against it is written over in place`() =
        runReplicaTest {
            seeded()
            val localId = assertNotNull(db.tasks().byServerId(31)).localId

            enqueueJson(
                changesJson(
                    listOf(upsert("task", 101, 31, taskJson(31, "Renamed there", projectId = 11))),
                    cursor = 150,
                ),
            )

            assertTrue(puller.pull().isApplied)

            val updated = assertNotNull(db.tasks().byServerId(31))
            assertEquals("Renamed there", updated.title)
            assertEquals(localId, updated.localId, "an update keeps the id every open screen is holding")
        }

    @Test
    fun `a deletion beats an unsent change and the change is kept where the user can see it`() =
        runReplicaTest {
            seeded()
            val task = assertNotNull(db.tasks().byServerId(31))
            val queued = queueWrite(ReplicaEntityKind.TASK, task.localId, op = "task.complete")

            enqueueJson(changesJson(listOf(tombstone("task", 101, 31)), cursor = 150))

            assertTrue(puller.pull().isApplied)

            assertNull(db.tasks().byServerId(31), "a deletion is the server's final word about the row")
            assertEquals(emptyList(), db.outbox().all(), "a write against a record that is gone cannot stall the queue")
            val setAside = db.outbox().quarantined().single()
            assertEquals(queued.id, setAside.id)
            assertEquals(ApiErrorCodes.TARGET_GONE, setAside.errorCode)
            assertEquals("task.complete", setAside.op)
        }

    @Test
    fun `the capacity bucket of the daily plan survives a catch-up`() =
        runReplicaTest {
            seeded()
            val task = assertNotNull(db.tasks().byServerId(31))
            db.tasks().update(task.copy(troikiCategory = TroikiCategory.IMPORTANT))

            enqueueJson(
                changesJson(listOf(upsert("task", 101, 31, taskJson(31, "Original", projectId = 11))), cursor = 150),
            )

            assertTrue(puller.pull().isApplied)

            assertEquals(
                TroikiCategory.IMPORTANT,
                assertNotNull(db.tasks().byServerId(31)).troikiCategory,
                "task payloads do not carry the bucket, so a catch-up must not erase it",
            )
        }
}

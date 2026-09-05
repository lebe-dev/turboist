package ru.tinyops.turboist.core.database

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.SyncStateRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Emptying the replica, which is what signing out does to the device.
 *
 * Worth asking the real engine rather than trusting it: the search indexes are
 * content-backed and maintained by triggers, so "delete everything" has to
 * satisfy both the tables and the indexes hanging off them. A wipe that threw
 * here would fail at the worst possible moment — after the token has already
 * been thrown away — and leave a signed-out device with somebody's data on it.
 */
class ReplicaWipeTest : ReplicaTest() {
    @Test
    fun `a wipe removes the records, the queue and the sync position together`() =
        runTest {
            val contextId = db.contexts().insert(context())
            val projectId = db.projects().insert(project(contextLocalId = contextId))
            val taskId = db.tasks().insert(task(title = "Findable", projectLocalId = projectId))
            db.outbox().enqueue(
                OutboxOpRow(
                    id = "op-1",
                    op = "task.complete",
                    payload = """{"id":$taskId}""",
                    entity = ReplicaEntityKind.TASK,
                    entityLocalId = taskId,
                    createdAt = NOW,
                    updatedAt = NOW,
                ),
            )
            db.syncState().save(SyncStateRow(epoch = 1, cursor = 42))
            assertTrue(db.searchIndex().matchingTaskLocalIds("Findable").isNotEmpty())

            // Off the calling thread, the way the app does it: Room refuses to be
            // emptied on the main thread, and the wipe is not a small statement.
            withContext(Dispatchers.IO) { db.clearAllTables() }

            assertEquals(0, db.tasks().count())
            assertEquals(0, db.projects().count())
            assertEquals(0, db.contexts().count())
            assertTrue(db.outbox().all().isEmpty())
            assertNull(db.syncState().get())
            // The index is derived from the tables, so emptying them has to empty
            // it too: a surviving hit would point at a row that no longer exists.
            assertTrue(db.searchIndex().matchingTaskLocalIds("Findable").isEmpty())
        }
}

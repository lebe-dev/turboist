package ru.tinyops.turboist.core.database

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The rule that everything else in the replica leans on: applying the same
 * record twice updates one row, and that row keeps the identity the device gave
 * it.
 */
class UpsertIdentityTest : ReplicaTest() {
    @Test
    fun `applying a record twice keeps one row at the same local id`() =
        runTest {
            val first = db.tasks().upsertByServerId(task(title = "Buy milk", serverId = 77))
            val second =
                db.tasks().upsertByServerId(
                    task(title = "Buy oat milk", serverId = 77).copy(
                        priority = Priority.HIGH,
                        status = TaskStatus.COMPLETED,
                    ),
                )

            assertEquals(first, second, "the second application must land on the same row")
            assertEquals(1, db.tasks().count())

            val stored = assertNotNull(db.tasks().byLocalId(first))
            assertEquals("Buy oat milk", stored.title, "the record's fields must be the server's latest")
            assertEquals(Priority.HIGH, stored.priority)
            assertEquals(TaskStatus.COMPLETED, stored.status)
            assertEquals(77L, stored.serverId)
        }

    @Test
    fun `a record the device has never seen is inserted`() =
        runTest {
            val one = db.tasks().upsertByServerId(task(title = "One", serverId = 1))
            val two = db.tasks().upsertByServerId(task(title = "Two", serverId = 2))

            assertTrue(one != two)
            assertEquals(2, db.tasks().count())
        }

    @Test
    fun `a row created on this device is inserted rather than matched`() =
        runTest {
            // Two offline creations are two separate tasks even though neither
            // has a server id to tell them apart yet.
            val one = db.tasks().upsertByServerId(task(title = "Offline one"))
            val two = db.tasks().upsertByServerId(task(title = "Offline two"))

            assertTrue(one != two)
            assertEquals(2, db.tasks().count())
            assertNull(assertNotNull(db.tasks().byLocalId(one)).serverId)
        }

    @Test
    fun `a record carrying a stale local id does not claim an occupied row`() =
        runTest {
            val occupied = db.tasks().upsertByServerId(task(title = "Already here", serverId = 5))

            // A payload decoded on another device, or replayed from a snapshot,
            // can carry a local id that means nothing here. It must not be able
            // to overwrite the row that happens to sit at that key.
            val arriving = task(title = "Newcomer", serverId = 6).copy(localId = occupied)
            val landed = db.tasks().upsertByServerId(arriving)

            assertTrue(landed != occupied, "the newcomer must get a row of its own")
            assertEquals("Already here", assertNotNull(db.tasks().byLocalId(occupied)).title)
            assertEquals(2, db.tasks().count())
        }

    @Test
    fun `every replicated table resolves records the same way`() =
        runTest {
            val contextLocalId = db.contexts().upsertByServerId(context(serverId = 3))
            assertEquals(contextLocalId, db.contexts().upsertByServerId(context(name = "Renamed", serverId = 3)))
            assertEquals("Renamed", assertNotNull(db.contexts().byLocalId(contextLocalId)).name)

            val labelLocalId = db.labels().upsertByServerId(label(serverId = 4))
            assertEquals(labelLocalId, db.labels().upsertByServerId(label(name = "defect", serverId = 4)))

            val projectLocalId = db.projects().upsertByServerId(project(contextLocalId, serverId = 5))
            assertEquals(
                projectLocalId,
                db.projects().upsertByServerId(project(contextLocalId, title = "Renamed", serverId = 5)),
            )

            val sectionLocalId = db.sections().upsertByServerId(section(projectLocalId, serverId = 6))
            assertEquals(
                sectionLocalId,
                db.sections().upsertByServerId(section(projectLocalId, title = "Done", serverId = 6)),
            )
        }

    @Test
    fun `a deleted record takes exactly its own row`() =
        runTest {
            val kept = db.tasks().upsertByServerId(task(title = "Kept", serverId = 10))
            db.tasks().upsertByServerId(task(title = "Gone", serverId = 11))

            assertEquals(1, db.tasks().deleteByServerId(11))
            assertEquals(0, db.tasks().deleteByServerId(11), "a repeated tombstone changes nothing")
            assertEquals(1, db.tasks().count())
            assertNotNull(db.tasks().byLocalId(kept))
        }

    @Test
    fun `a locally created row becomes addressable without changing identity`() =
        runTest {
            val localId = db.tasks().insert(task(title = "Created offline"))
            assertTrue(localId != NO_LOCAL_ID)

            db.tasks().assignServerId(localId = localId, serverId = 4242, updatedAt = NOW + 1)

            val stored = assertNotNull(db.tasks().byLocalId(localId))
            assertEquals(4242L, stored.serverId)
            assertEquals(localId, stored.localId, "the screen holding this task must keep pointing at it")

            // And the record now resolves the way any other server record does.
            assertEquals(
                localId,
                db.tasks().upsertByServerId(task(title = "Echoed back", serverId = 4242)),
            )
            assertEquals(1, db.tasks().count())
        }
}

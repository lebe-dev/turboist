package ru.tinyops.turboist.core.database

import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * What a screen showing one task can ask about its links.
 *
 * A link is only useful when the task at the other end is named, and which end
 * that is depends on which task is being looked at: the same stored row is
 * something this task waits for, or something waiting on it. Both readings come
 * out of one query so a screen never has to work out which end it is holding.
 */
class TaskRelationReadTest : ReplicaTest() {
    private fun relation(
        source: Long,
        target: Long,
        type: RelationType = RelationType.BLOCKS,
        createdAt: Long = NOW,
    ) = TaskRelationRow(sourceTaskLocalId = source, targetTaskLocalId = target, type = type, createdAt = createdAt)

    @Test
    fun `a link names the task at the other end and which way it runs`() =
        runTest {
            val blocker = db.tasks().insert(task(title = "Sign the contract", serverId = 10))
            val blocked = db.tasks().insert(task(title = "Start the work", serverId = 11))
            db.taskRelations().insert(relation(blocker, blocked))

            val waiting = db.taskRelations().observePeersForTask(blocked).first().single()
            assertEquals("Sign the contract", waiting.peerTitle)
            assertEquals(10L, waiting.peerServerId)
            assertEquals(TaskStatus.OPEN, waiting.peerStatus)
            assertTrue(!waiting.outgoing, "the peer is the one holding this task up")

            val holding = db.taskRelations().observePeersForTask(blocker).first().single()
            assertEquals("Start the work", holding.peerTitle)
            assertTrue(holding.outgoing, "read from the other end, the same row says the opposite")
            assertEquals(waiting.relationLocalId, holding.relationLocalId)
        }

    @Test
    fun `the state of the peer travels with the link`() =
        runTest {
            val blocker = db.tasks().insert(task(title = "Sign the contract", serverId = 10))
            val blocked = db.tasks().insert(task(title = "Start the work", serverId = 11))
            db.taskRelations().insert(relation(blocker, blocked))
            val row = assertNotNull(db.tasks().byLocalId(blocker))

            db.tasks().update(row.copy(status = TaskStatus.COMPLETED))

            // The link stays; what changed is that it no longer stands in the way.
            val link = db.taskRelations().observePeersForTask(blocked).first().single()
            assertEquals(TaskStatus.COMPLETED, link.peerStatus)
        }

    @Test
    fun `links are listed in the order they were made`() =
        runTest {
            val task = db.tasks().insert(task(title = "The work", serverId = 10))
            val first = db.tasks().insert(task(title = "First", serverId = 11))
            val second = db.tasks().insert(task(title = "Second", serverId = 12))
            db.taskRelations().insert(relation(second, task, createdAt = NOW + 1))
            db.taskRelations().insert(relation(first, task, createdAt = NOW))

            assertEquals(
                listOf("First", "Second"),
                db.taskRelations().observePeersForTask(task).first().map { it.peerTitle },
            )
        }

    @Test
    fun `a task with no links is answered with nothing rather than with a row`() =
        runTest {
            val task = db.tasks().insert(task(title = "The work", serverId = 10))

            assertTrue(db.taskRelations().observePeersForTask(task).first().isEmpty())
        }

    @Test
    fun `only the edges that still hold something up are open ones`() =
        runTest {
            val open = db.tasks().insert(task(title = "Sign the contract", serverId = 10))
            val done = db.tasks().insert(task(title = "Book the room", serverId = 11))
            val blocked = db.tasks().insert(task(title = "Start the work", serverId = 12))
            db.taskRelations().insert(relation(open, blocked))
            db.taskRelations().insert(relation(done, blocked))
            db.tasks().update(assertNotNull(db.tasks().byLocalId(done)).copy(status = TaskStatus.CANCELLED))

            assertEquals(2, db.taskRelations().blockEdges().size, "the graph the loop check reads keeps both")
            assertEquals(
                listOf(open),
                db.taskRelations().openBlockEdges().map { it.blockerLocalId },
                "a task that is no longer going to happen stops holding anything up",
            )
        }

    @Test
    fun `an informational link is not part of the waiting graph`() =
        runTest {
            val first = db.tasks().insert(task(title = "Read the spec", serverId = 10))
            val second = db.tasks().insert(task(title = "Write the notes", serverId = 11))
            db.taskRelations().insert(relation(first, second, type = RelationType.RELATED))

            assertTrue(db.taskRelations().blockEdges().isEmpty())
            assertEquals(1, db.taskRelations().observePeersForTask(first).first().size)
        }

    @Test
    fun `a pair is looked up by kind as well as by the two ends`() =
        runTest {
            val first = db.tasks().insert(task(title = "Draft it", serverId = 10))
            val second = db.tasks().insert(task(title = "Publish it", serverId = 11))
            val localId = db.taskRelations().insert(relation(first, second))

            assertEquals(localId, db.taskRelations().localIdForEdge(first, second, RelationType.BLOCKS))
            assertNull(
                db.taskRelations().localIdForEdge(first, second, RelationType.RELATED),
                "a different kind of link between the same two tasks is a different statement",
            )
            assertNull(
                db.taskRelations().localIdForEdge(second, first, RelationType.BLOCKS),
                "the same kind the other way round is a different row, and a loop",
            )
        }
}

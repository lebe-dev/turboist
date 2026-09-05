package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.RelationDirection
import ru.tinyops.turboist.core.model.RelationType
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * Which links the device makes, and which it turns down on the spot.
 *
 * A link is not a field edit: the server keeps one row per pair and kind and
 * refuses a wait that closes a loop, and both refusals are permanent. A device
 * that accepted such a link would draw it, queue it, and have it thrown out
 * whenever the queue next drained — by which time there is nothing on screen
 * left to connect the failure to. So the same three refusals are made here,
 * before anything is written and before anything is queued.
 */
class RelationGuardTest : WriteTest() {
    @Test
    fun `a task cannot be linked to itself`() =
        runTest {
            val task = givenTask(title = "The work", serverId = 10)

            assertFailsWith<WriteRefused.Invalid> {
                tasks.addRelation(task, task, RelationType.BLOCKS, RelationDirection.INCOMING)
            }
            assertEquals(0, db.taskRelations().count())
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `a blocking link is stored in the direction the user named`() =
        runTest {
            val blocked = givenTask(title = "Start the work", serverId = 10)
            val blocker = givenTask(title = "Sign the contract", serverId = 11)

            val written = tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)

            val row = assertNotNull(db.taskRelations().byLocalId(written.entityLocalId))
            assertEquals(blocker, row.sourceTaskLocalId, "the task that has to happen first is the source")
            assertEquals(blocked, row.targetTaskLocalId)
            assertEquals(listOf(blocker), rules.openBlockerLocalIds(blocked))
            assertTrue(rules.openBlockerLocalIds(blocker).isEmpty(), "the blocker itself waits for nothing")
        }

    @Test
    fun `an informational link added from either end is one link`() =
        runTest {
            val first = givenTask(title = "Read the spec", serverId = 10)
            val second = givenTask(title = "Write the notes", serverId = 11)
            tasks.addRelation(second, first, RelationType.RELATED)

            // The same pair from the other side, with the opposite direction hint,
            // is the same statement — and a symmetric link has no second copy.
            assertFailsWith<WriteRefused.RelationExists> {
                tasks.addRelation(first, second, RelationType.RELATED, RelationDirection.INCOMING)
            }
            assertEquals(1, db.taskRelations().count())
            val row = assertNotNull(db.taskRelations().forTask(first).singleOrNull())
            assertEquals(minOf(first, second), row.sourceTaskLocalId)
            assertEquals(maxOf(first, second), row.targetTaskLocalId)
        }

    @Test
    fun `the same wait cannot be recorded twice`() =
        runTest {
            val blocker = givenTask(title = "Sign the contract", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            val first = tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)

            val refused =
                assertFailsWith<WriteRefused.RelationExists> {
                    tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)
                }

            assertEquals(
                first.entityLocalId,
                refused.relationLocalId,
                "the refusal names the link that is already there",
            )
            assertEquals(1, db.taskRelations().count())
        }

    @Test
    fun `the same pair may wait one way and be merely related as well`() =
        runTest {
            val first = givenTask(title = "Draft it", serverId = 10)
            val second = givenTask(title = "Publish it", serverId = 11)
            tasks.addRelation(first, second, RelationType.BLOCKS, RelationDirection.OUTGOING)

            // A different kind of link between the same two tasks is a different
            // statement, and the store keeps one row per pair *and kind*.
            tasks.addRelation(first, second, RelationType.RELATED)

            assertEquals(2, db.taskRelations().count())
        }

    @Test
    fun `a wait that closes a loop is refused`() =
        runTest {
            val first = givenTask(title = "One", serverId = 10)
            val second = givenTask(title = "Two", serverId = 11)
            val third = givenTask(title = "Three", serverId = 12)
            tasks.addRelation(first, second, RelationType.BLOCKS, RelationDirection.OUTGOING)
            tasks.addRelation(second, third, RelationType.BLOCKS, RelationDirection.OUTGOING)

            // Three now waits for one at a remove, so saying one waits for three
            // would leave all three permanently unfinishable.
            val refused =
                assertFailsWith<WriteRefused.RelationCycle> {
                    tasks.addRelation(third, first, RelationType.BLOCKS, RelationDirection.OUTGOING)
                }

            assertEquals(third, refused.blockerLocalId)
            assertEquals(first, refused.blockedLocalId)
            assertEquals(2, db.taskRelations().count())
        }

    @Test
    fun `an informational link between the same two tasks is still allowed`() =
        runTest {
            val first = givenTask(title = "One", serverId = 10)
            val second = givenTask(title = "Two", serverId = 11)
            tasks.addRelation(first, second, RelationType.BLOCKS, RelationDirection.OUTGOING)

            // Nothing waits for anything, so an informational link can never
            // deadlock a pair however they are already linked.
            tasks.addRelation(second, first, RelationType.RELATED)

            assertEquals(2, db.taskRelations().count())
        }

    @Test
    fun `the shortest loop of all is refused`() =
        runTest {
            val first = givenTask(title = "One", serverId = 10)
            val second = givenTask(title = "Two", serverId = 11)
            tasks.addRelation(first, second, RelationType.BLOCKS, RelationDirection.OUTGOING)

            assertFailsWith<WriteRefused.RelationCycle> {
                tasks.addRelation(first, second, RelationType.BLOCKS, RelationDirection.INCOMING)
            }
        }

    @Test
    fun `a link the device turns down is never queued for the server`() =
        runTest {
            val blocker = givenTask(title = "Sign the contract", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)
            val queuedBefore = queue().size

            assertFailsWith<WriteRefused.RelationExists> {
                tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)
            }
            assertFailsWith<WriteRefused.RelationCycle> {
                tasks.addRelation(blocker, blocked, RelationType.BLOCKS, RelationDirection.INCOMING)
            }

            // A refused link that reached the queue would be sent, refused again
            // by the server and put aside as something the user has to deal with —
            // for a link that was never made.
            assertEquals(queuedBefore, queue().size)
        }

    @Test
    fun `removing a link takes it out of the replica and asks the server to do the same`() =
        runTest {
            val blocker = givenTask(title = "Sign the contract", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            val added = tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)

            tasks.removeRelation(blocked, added.entityLocalId)

            assertEquals(0, db.taskRelations().count())
            assertTrue(rules.openBlockerLocalIds(blocked).isEmpty(), "the task is no longer waiting for anything")
            val op = assertNotNull(queuedOps().filterIsInstance<RemoveTaskRelationOp>().singleOrNull())
            assertEquals(added.entityLocalId, op.relationLocalId)
            assertEquals(blocked, op.taskLocalId)
        }

    @Test
    fun `a link is queued naming the pair as this device knows them`() =
        runTest {
            val blocked = givenTask(title = "Start the work", serverId = 10)
            val blocker = givenTask(title = "Sign the contract", serverId = 11)

            val written = tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)

            val op = assertNotNull(queuedOps().filterIsInstance<AddTaskRelationOp>().singleOrNull())
            assertEquals(blocked, op.taskLocalId, "the request is made against the task the user was looking at")
            assertEquals(blocker, op.targetTaskLocalId)
            assertEquals(written.entityLocalId, op.relationLocalId)
            assertEquals(RelationType.BLOCKS.wire, op.type)
            assertEquals(
                RelationDirection.INCOMING.wire,
                op.direction,
                "the direction is read relative to the task in the path, so it has to travel with the request",
            )
        }

    @Test
    fun `an informational link carries no direction at all`() =
        runTest {
            val first = givenTask(title = "Read the spec", serverId = 10)
            val second = givenTask(title = "Write the notes", serverId = 11)

            tasks.addRelation(first, second, RelationType.RELATED, RelationDirection.INCOMING)

            val op = assertNotNull(queuedOps().filterIsInstance<AddTaskRelationOp>().singleOrNull())
            assertEquals(null, op.direction, "a symmetric link reads the same from both ends")
        }
}

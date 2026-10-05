package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.RelationDirection
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.view.BlockEdge
import ru.tinyops.turboist.core.model.view.blockedByOpenCounts
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Finishing a blocker with no network releases what it was holding up, at once.
 *
 * This is the whole reason the relation graph is kept on the device rather than
 * being asked about: the padlock a list draws and the refusal the completion
 * guard makes are two readings of the same rule over the same rows, so a blocker
 * ticked off on a plane clears both in the same instant. If they were two rules,
 * or one rule over two sets of rows, a user would meet a padlock on a task the
 * screen had already let them finish.
 */
class ReleaseOnCompleteTest : WriteTest() {
    @Test
    fun `finishing a blocker clears the padlock and the refusal together`() =
        runTest {
            val blocker = givenTask(title = "Sign the contract", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)

            assertEquals(mapOf(blocked to 1), padlocks(blocked))
            assertEquals(listOf(blocker), rules.openBlockerLocalIds(blocked))

            tasks.complete(blocker)

            assertTrue(padlocks(blocked).isEmpty(), "the list must stop drawing a padlock the guard would not make")
            assertTrue(rules.openBlockerLocalIds(blocked).isEmpty())
            tasks.complete(blocked)
        }

    @Test
    fun `abandoning a blocker releases it just as finally`() =
        runTest {
            val blocker = givenTask(title = "Wait for the old vendor", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)

            tasks.cancel(blocker)

            assertTrue(padlocks(blocked).isEmpty())
            assertTrue(rules.openBlockerLocalIds(blocked).isEmpty())
        }

    @Test
    fun `taking the link off releases the task as well`() =
        runTest {
            val blocker = givenTask(title = "Sign the contract", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            val link = tasks.addRelation(blocked, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)

            tasks.removeRelation(blocked, link.entityLocalId)

            assertTrue(padlocks(blocked).isEmpty())
            assertTrue(rules.openBlockerLocalIds(blocked).isEmpty())
        }

    @Test
    fun `a subtask of released work is released with it`() =
        runTest {
            val blocker = givenTask(title = "Get approval", serverId = 10)
            val parent = givenTask(title = "Build it", serverId = 11)
            val child = givenTask(title = "Write the code", serverId = 12, parentLocalId = parent)
            tasks.addRelation(parent, blocker, RelationType.BLOCKS, RelationDirection.INCOMING)

            assertEquals(mapOf(parent to 1, child to 1), padlocks(parent, child))

            tasks.complete(blocker)

            assertTrue(padlocks(parent, child).isEmpty(), "the inherited padlock goes when the blocker does")
            assertTrue(rules.openBlockerLocalIds(child).isEmpty())
        }

    /**
     * The padlock a list would draw for these tasks, built the way a list builds
     * it: from the standing queries over the replica, through the shared rule.
     */
    private suspend fun padlocks(vararg taskLocalIds: Long): Map<Long, Int> {
        val edges =
            db.taskHydration().observeRelationEdges().first()
                .filter { it.type == RelationType.BLOCKS && it.sourceOpen }
                .map { BlockEdge(blockerLocalId = it.sourceTaskLocalId, blockedLocalId = it.targetTaskLocalId) }
        val parents = db.taskHydration().observeParentLinks().first().associate { it.taskLocalId to it.parentLocalId }
        return blockedByOpenCounts(taskLocalIds.toList(), edges, parents)
    }
}

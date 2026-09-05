package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * A task nothing is waiting on can be finished; one that something is waiting on
 * cannot.
 *
 * The device repeats this rule rather than leaving it to the server because the
 * alternative is a tick that lands on screen, sits in the queue for a day, and is
 * quietly undone by an answer the user stopped waiting for. With the whole
 * relation graph on the device, finishing a blocker offline genuinely releases
 * what it was holding up.
 */
class BlockerGuardTest : WriteTest() {
    @Test
    fun `an open blocker refuses the completion and names itself`() =
        runTest {
            val blocker = givenTask(title = "Sign the contract", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            givenBlocks(blocker, blocked)

            val refused = assertFailsWith<WriteRefused.TaskBlocked> { tasks.complete(blocked) }

            assertEquals(listOf(blocker), refused.blockerLocalIds)
        }

    @Test
    fun `a finished blocker releases what it was holding up`() =
        runTest {
            val blocker = givenTask(title = "Sign the contract", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            givenBlocks(blocker, blocked)

            tasks.complete(blocker)
            tasks.complete(blocked)

            assertEquals(TaskStatus.COMPLETED, assertNotNull(db.tasks().byLocalId(blocked)).status)
        }

    @Test
    fun `an abandoned blocker releases what it was holding up`() =
        runTest {
            // Cancelling is not completing, but it does mean the task is no
            // longer going to happen. Treating it as still blocking would leave
            // everything behind it permanently unfinishable.
            val blocker = givenTask(title = "Wait for the old vendor", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            givenBlocks(blocker, blocked)

            tasks.cancel(blocker)
            tasks.complete(blocked)

            assertEquals(TaskStatus.COMPLETED, assertNotNull(db.tasks().byLocalId(blocked)).status)
        }

    @Test
    fun `a blocker on a parent blocks the work underneath it`() =
        runTest {
            val blocker = givenTask(title = "Get approval", serverId = 10)
            val parent = givenTask(title = "Build it", serverId = 11)
            val child = givenTask(title = "Write the code", serverId = 12, parentLocalId = parent)
            val grandchild = givenTask(title = "Write a test", serverId = 13, parentLocalId = child)
            givenBlocks(blocker, parent)

            // Finishing a piece of work that is not allowed to start is not
            // progress, so the block is inherited all the way down.
            assertEquals(listOf(blocker), rules.openBlockerLocalIds(child))
            assertEquals(listOf(blocker), rules.openBlockerLocalIds(grandchild))
            assertFailsWith<WriteRefused.TaskBlocked> { tasks.complete(grandchild) }
        }

    @Test
    fun `a task does not inherit itself as its own blocker`() =
        runTest {
            val parent = givenTask(title = "Build it", serverId = 11)
            val child = givenTask(title = "Design", serverId = 12, parentLocalId = parent)
            givenBlocks(child, parent)

            // The parent waits for its own child, which is a legitimate thing to
            // say. What must not happen is the child inheriting that same
            // blocker through the parent: the pair would then be unfinishable
            // from either end.
            assertEquals(listOf(child), rules.openBlockerLocalIds(parent))
            assertTrue(rules.openBlockerLocalIds(child).isEmpty())
        }

    @Test
    fun `a blocker of the parent still holds up its other children`() =
        runTest {
            val parent = givenTask(title = "Build it", serverId = 11)
            val first = givenTask(title = "Design", serverId = 12, parentLocalId = parent)
            val second = givenTask(title = "Implement", serverId = 13, parentLocalId = parent)
            givenBlocks(first, parent)

            // The exclusion is narrow on purpose: only the task being asked
            // about, and the work under it, are exempt. A sibling really is held
            // up — its parent cannot proceed, so neither can it.
            assertEquals(listOf(first), rules.openBlockerLocalIds(second))
        }

    @Test
    fun `an informational link never blocks anything`() =
        runTest {
            val other = givenTask(title = "Related work", serverId = 10)
            val task = givenTask(title = "The work", serverId = 11)
            tasks.addRelation(task, other, ru.tinyops.turboist.core.model.RelationType.RELATED)

            assertTrue(rules.openBlockerLocalIds(task).isEmpty())
            tasks.complete(task)
            assertEquals(TaskStatus.COMPLETED, assertNotNull(db.tasks().byLocalId(task)).status)
        }

    @Test
    fun `finishing a parent leaves a blocked subtask open`() =
        runTest {
            val parent = givenTask(title = "Release", serverId = 11)
            val ready = givenTask(title = "Tag it", serverId = 12, parentLocalId = parent)
            val waiting = givenTask(title = "Announce it", serverId = 13, parentLocalId = parent)
            val outsider = givenTask(title = "Legal review", serverId = 14)
            givenBlocks(outsider, waiting)

            tasks.complete(parent)

            assertEquals(TaskStatus.COMPLETED, assertNotNull(db.tasks().byLocalId(ready)).status)
            assertEquals(
                TaskStatus.OPEN,
                assertNotNull(db.tasks().byLocalId(waiting)).status,
                "finishing the parent must not push past something the user said was in the way",
            )
        }

    @Test
    fun `a bulk completion leaves out only what is blocked`() =
        runTest {
            val blocker = givenTask(title = "Sign the contract", serverId = 10)
            val blocked = givenTask(title = "Start the work", serverId = 11)
            val free = givenTask(title = "Tidy the desk", serverId = 12)
            givenBlocks(blocker, blocked)

            val written = tasks.bulkComplete(listOf(blocked, free))

            assertEquals(listOf(free), written.accepted)
            assertEquals(mapOf(blocked to listOf(blocker)), written.refused)
            assertEquals(TaskStatus.OPEN, assertNotNull(db.tasks().byLocalId(blocked)).status)
            assertEquals(TaskStatus.COMPLETED, assertNotNull(db.tasks().byLocalId(free)).status)
            assertEquals(
                listOf(free),
                (queuedOps().single() as BulkCompleteTasksOp).taskLocalIds,
                "the request must not ask for what the device already knows is refused",
            )
        }
}

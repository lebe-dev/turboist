package ru.tinyops.turboist.core.sync.maintenance

import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.ViewWindows
import ru.tinyops.turboist.core.sync.write.TaskEdit
import ru.tinyops.turboist.core.sync.write.WriteTest
import java.time.Clock
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

/**
 * What the device keeps of its finished work, and what it lets go of.
 *
 * The whole subject is a boundary in time and a set of exceptions to it, and both
 * are claims about rows in a database: whether a task is still there afterwards,
 * and whether the things hanging off it went with it. So this runs against real
 * SQLite, with a fixed clock — a window measured back from "whenever the test
 * happened to run" would be a different window on every run.
 */
class CompletedHistoryPruneTest : WriteTest() {
    private val now: Instant = Instant.parse("2026-03-01T09:00:00Z")

    /** The first instant the device still keeps a copy of finished work from. */
    private val windowStart: Long =
        ViewWindows.completedHistory(now, ZONE, REPLICATED_COMPLETED_HISTORY_DAYS).from

    private val prune = { CompletedHistoryPrune(db, Clock.fixed(now, ZONE)) }

    @Test
    fun `a completion from before the window goes, one from inside it stays`() =
        runTest {
            val old = givenCompletion(completedAt = windowStart - 1)
            val recent = givenCompletion(completedAt = windowStart + 1)

            assertEquals(1, prune().prune())

            assertNull(db.tasks().byLocalId(old), "a completion older than the window is not kept")
            assertNotNull(db.tasks().byLocalId(recent), "a completion inside the window is kept")
        }

    @Test
    fun `the first instant of the window is inside it`() =
        runTest {
            val onTheEdge = givenCompletion(completedAt = windowStart)

            assertEquals(0, prune().prune())

            assertNotNull(db.tasks().byLocalId(onTheEdge), "the window includes the instant it starts at")
        }

    @Test
    fun `a task with a write still queued against it is never removed`() =
        runTest {
            val dirty = givenCompletion(completedAt = windowStart - 1)
            val clean = givenCompletion(completedAt = windowStart - 1)
            tasks.patch(dirty, TaskEdit(title = "renamed while offline"))

            assertEquals(1, prune().prune())

            assertNotNull(db.tasks().byLocalId(dirty), "a row the server has not been told about is kept")
            assertNull(db.tasks().byLocalId(clean))
        }

    @Test
    fun `an aged task is left alone while anything under it is staying`() =
        runTest {
            val parent = givenCompletion(completedAt = windowStart - 1)
            val openSubtask =
                db.tasks().insert(
                    TaskRow(
                        serverId = 91,
                        title = "still to do",
                        parentLocalId = parent,
                        createdAt = NOW,
                        updatedAt = NOW,
                    ),
                )

            assertEquals(0, prune().prune())

            assertNotNull(db.tasks().byLocalId(parent), "removing it would take the open subtask with it")
            assertNotNull(db.tasks().byLocalId(openSubtask))
        }

    @Test
    fun `a whole aged branch goes at once`() =
        runTest {
            val parent = givenCompletion(completedAt = windowStart - 1)
            val child = givenCompletion(completedAt = windowStart - 2, parentLocalId = parent)
            val grandchild = givenCompletion(completedAt = windowStart - 3, parentLocalId = child)

            assertEquals(3, prune().prune(), "the whole branch is counted, not just the row at the top of it")

            assertNull(db.tasks().byLocalId(parent))
            assertNull(db.tasks().byLocalId(child))
            assertNull(db.tasks().byLocalId(grandchild))
        }

    @Test
    fun `one dirty leaf keeps its whole line of ancestors`() =
        runTest {
            val parent = givenCompletion(completedAt = windowStart - 1)
            val child = givenCompletion(completedAt = windowStart - 2, parentLocalId = parent)
            val grandchild = givenCompletion(completedAt = windowStart - 3, parentLocalId = child)
            tasks.patch(grandchild, TaskEdit(title = "renamed while offline"))

            assertEquals(0, prune().prune())

            assertNotNull(db.tasks().byLocalId(parent), "its branch holds a row that has to stay")
            assertNotNull(db.tasks().byLocalId(child))
            assertNotNull(db.tasks().byLocalId(grandchild))
        }

    @Test
    fun `an open task is never history, however old`() =
        runTest {
            val ancient =
                db.tasks().insert(
                    TaskRow(serverId = 92, title = "never finished", createdAt = NOW, updatedAt = NOW),
                )

            assertEquals(0, prune().prune())

            assertNotNull(db.tasks().byLocalId(ancient))
        }

    private var nextServerId = 1_000L

    private suspend fun givenCompletion(
        completedAt: Long,
        parentLocalId: Long? = null,
    ): Long =
        db.tasks().insert(
            TaskRow(
                serverId = nextServerId++,
                title = "finished at $completedAt",
                parentLocalId = parentLocalId,
                status = TaskStatus.COMPLETED,
                completedAt = completedAt,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )
}

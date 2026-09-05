package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Splitting one task into the several it turned out to be.
 *
 * The split happens on the device, so the questions the server answers about it
 * — what a piece inherits, what becomes of the task it came from, and when the
 * whole thing is refused — are answered here first and are checked against a
 * real replica.
 */
class DecomposeTest : WriteTest() {
    @Test
    fun `each piece inherits everything about the task except its wording`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val urgent = givenLabel(name = "urgent", serverId = 21)
            val sourceLocalId = givenTask(title = "Draft outline", serverId = 31, projectLocalId = projectLocalId)
            db.tasks().setLabels(sourceLocalId, listOf(urgent), NOW)
            db.tasks().update(
                assertNotNull(db.tasks().byLocalId(sourceLocalId)).copy(
                    description = "Everything the launch needs",
                    priority = Priority.HIGH,
                    dueAt = NOW,
                    dueHasTime = true,
                    dayPart = DayPart.MORNING,
                    planState = PlanState.WEEK,
                    isPrivate = true,
                ),
            )

            tasks.decompose(sourceLocalId, listOf("Review with team", "Publish"))

            val pieces = piecesOf(sourceLocalId)
            assertContentEquals(listOf("Review with team", "Publish"), pieces.map { it.title })
            assertNull(pieces.last().serverId, "a piece written here has no name on the server yet")
            for (piece in pieces) {
                assertEquals(projectLocalId, piece.projectLocalId, "a piece stays where the work was")
                assertEquals("Everything the launch needs", piece.description)
                assertEquals(Priority.HIGH, piece.priority)
                assertEquals(NOW, piece.dueAt)
                assertTrue(piece.dueHasTime)
                assertEquals(DayPart.MORNING, piece.dayPart)
                assertEquals(PlanState.WEEK, piece.planState)
                assertTrue(piece.isPrivate, "a private task does not become public by being split")
                assertContentEquals(listOf(urgent), db.tasks().labelsOf(piece.localId).map { it.labelLocalId })
            }
        }

    @Test
    fun `the task that was split is no longer there under its own wording`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val sourceLocalId = givenTask(title = "Draft outline", serverId = 31, projectLocalId = projectLocalId)

            val write = tasks.decompose(sourceLocalId, listOf("Review with team", "Publish"))

            // The row the task was in becomes the first piece rather than being
            // deleted: that is what still names the task in the write that has
            // yet to be sent, and what a screen showing it goes on showing.
            assertEquals(sourceLocalId, write.entityLocalId)
            assertEquals("Review with team", assertNotNull(db.tasks().byLocalId(sourceLocalId)).title)
            assertEquals(31L, db.tasks().byLocalId(sourceLocalId)?.serverId, "until the answer renames it")
        }

    @Test
    fun `the links the task had go with it, because the task they belonged to does`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val sourceLocalId = givenTask(title = "Draft outline", serverId = 31, projectLocalId = projectLocalId)
            val blocker = givenTask(title = "Agree the brief", serverId = 32, projectLocalId = projectLocalId)
            givenBlocks(blocker = blocker, blocked = sourceLocalId)

            tasks.decompose(sourceLocalId, listOf("Review with team", "Publish"))

            assertEquals(0, db.taskRelations().count(), "the pieces are new work, and nothing waits on them yet")
        }

    @Test
    fun `the split is one queued write, naming the pieces in the order it wrote them`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val sourceLocalId = givenTask(title = "Draft outline", serverId = 31, projectLocalId = projectLocalId)

            tasks.decompose(sourceLocalId, listOf("Review with team", "Publish"))

            val op = queuedOps().filterIsInstance<DecomposeTaskOp>().single()
            assertEquals(sourceLocalId, op.taskLocalId)
            assertContentEquals(listOf("Review with team", "Publish"), op.titles)
            assertContentEquals(
                listOf("Review with team", "Publish"),
                op.createdTaskLocalIds.map { assertNotNull(db.tasks().byLocalId(it)).title },
                "the pieces are named in the order the titles were sent in",
            )
        }

    @Test
    fun `blank lines are dropped and the rest trimmed, as the server does`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val sourceLocalId = givenTask(title = "Draft outline", serverId = 31, projectLocalId = projectLocalId)

            tasks.decompose(sourceLocalId, listOf("  Review with team  ", "   ", "Publish", ""))

            val op = queuedOps().filterIsInstance<DecomposeTaskOp>().single()
            assertContentEquals(listOf("Review with team", "Publish"), op.titles)
            assertContentEquals(listOf("Review with team", "Publish"), piecesOf(sourceLocalId).map { it.title })
        }

    @Test
    fun `an outline with nothing in it is refused, and the task is left alone`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val sourceLocalId = givenTask(title = "Draft outline", serverId = 31, projectLocalId = projectLocalId)

            assertFailsWith<WriteRefused.Invalid> { tasks.decompose(sourceLocalId, listOf(" ", "")) }

            assertNotNull(db.tasks().byLocalId(sourceLocalId), "a refused split changes nothing")
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `a task with work under it cannot be split, because that work would have nowhere to go`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val sourceLocalId = givenTask(title = "Draft outline", serverId = 31, projectLocalId = projectLocalId)
            givenTask(
                title = "Gather notes",
                serverId = 32,
                projectLocalId = projectLocalId,
                parentLocalId = sourceLocalId,
            )

            assertFailsWith<WriteRefused.Invalid> { tasks.decompose(sourceLocalId, listOf("Review with team")) }

            assertNotNull(db.tasks().byLocalId(sourceLocalId))
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `splitting a task that is not here is refused`() =
        runTest {
            assertFailsWith<WriteRefused.RowMissing> { tasks.decompose(404L, listOf("Review with team")) }
            assertTrue(queue().isEmpty())
        }

    /** The rows the split produced, read back through the write it queued. */
    private suspend fun piecesOf(sourceLocalId: Long) =
        queuedOps()
            .filterIsInstance<DecomposeTaskOp>()
            .single { it.taskLocalId == sourceLocalId }
            .createdTaskLocalIds
            .map { assertNotNull(db.tasks().byLocalId(it)) }
}

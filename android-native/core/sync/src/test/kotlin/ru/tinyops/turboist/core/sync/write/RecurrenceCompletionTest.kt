package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.WireTime
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

/**
 * Ticking off a task that repeats.
 *
 * A repeating task is not finished when it is ticked off: it moves to its next
 * occurrence, and the run just done is written down separately so the history
 * has something to show. Both halves have to happen on the device, because the
 * alternative is a screen that says nothing happened until the queue drains.
 *
 * The dates themselves are not re-derived here — they come from the worked
 * examples shared with the server. What these cases pin is everything around
 * them: what is written down, what is left alone, and what a second tap does.
 */
class RecurrenceCompletionTest : WriteTest() {
    @Test
    fun `a repeating task moves to its next occurrence instead of being finished`() =
        runTest {
            val taskLocalId = givenTask(dueAt = DUE, recurrenceRule = "FREQ=DAILY")

            tasks.complete(taskLocalId)

            val task = assertNotNull(db.tasks().byLocalId(taskLocalId))
            assertEquals(TaskStatus.OPEN, task.status)
            assertEquals(NEXT_DUE, task.dueAt)
            assertNull(task.completedAt)
        }

    @Test
    fun `moving on clears the week's plan and the count of times it was put off`() =
        runTest {
            val taskLocalId = givenTask(dueAt = DUE, recurrenceRule = "FREQ=DAILY")
            val planned = assertNotNull(db.tasks().byLocalId(taskLocalId))
            db.tasks().update(planned.copy(planState = PlanState.WEEK, postponeCount = 4))

            tasks.complete(taskLocalId)

            val task = assertNotNull(db.tasks().byLocalId(taskLocalId))
            assertEquals(PlanState.NONE, task.planState)
            assertEquals(0, task.postponeCount)
        }

    @Test
    fun `the run that was finished is written down and points back at the task`() =
        runTest {
            val taskLocalId = givenTask(dueAt = DUE, recurrenceRule = "FREQ=DAILY")

            tasks.complete(taskLocalId)

            val run = assertNotNull(runRecordedAt(taskLocalId, NOW))
            assertEquals(TaskStatus.COMPLETED, run.status)
            assertEquals(taskLocalId, run.sourceTaskLocalId)
            assertNull(run.serverId, "the server has not been told about it yet")
        }

    @Test
    fun `the run carries what was done and where, and nothing that is still pending`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val labelLocalId = givenLabel(name = "chore")
            val taskLocalId =
                givenTask(
                    title = "Water the plants",
                    projectLocalId = projectLocalId,
                    dueAt = DUE,
                    recurrenceRule = "FREQ=DAILY",
                )
            db.tasks().setLabels(taskLocalId, listOf(labelLocalId), NOW)

            tasks.complete(taskLocalId)

            val run = assertNotNull(runRecordedAt(taskLocalId, NOW))
            assertEquals("Water the plants", run.title)
            assertEquals(projectLocalId, run.projectLocalId)
            assertEquals(listOf(labelLocalId), db.tasks().labelsOf(run.localId).map { it.labelLocalId })
            assertNull(run.recurrenceRule, "a recorded run must not repeat on its own")
            assertNull(run.dueAt)
            assertEquals(PlanState.NONE, run.planState)
            assertEquals(false, run.isPinned)
        }

    @Test
    fun `ticking the same repeating task off twice in one day is one run`() =
        runTest {
            val taskLocalId = givenTask(dueAt = DUE, recurrenceRule = "FREQ=DAILY")

            tasks.complete(taskLocalId)
            clock = NOW + FIVE_HOURS
            tasks.complete(taskLocalId)

            // The server answers the second one the same way, so nothing here has
            // to be taken back when the queue drains.
            assertNotNull(runRecordedAt(taskLocalId, NOW))
            assertNull(runRecordedAt(taskLocalId, NOW + FIVE_HOURS), "one day of a repeating task is one run")
            assertEquals(NEXT_DUE, assertNotNull(db.tasks().byLocalId(taskLocalId)).dueAt)
        }

    @Test
    fun `ticking it off again the next day is a second run`() =
        runTest {
            val taskLocalId = givenTask(dueAt = DUE, recurrenceRule = "FREQ=DAILY")

            tasks.complete(taskLocalId)
            clock = NOW + ONE_DAY
            tasks.complete(taskLocalId)

            assertNotNull(runRecordedAt(taskLocalId, NOW))
            assertNotNull(runRecordedAt(taskLocalId, NOW + ONE_DAY))
            assertEquals(DAY_AFTER_NEXT_DUE, assertNotNull(db.tasks().byLocalId(taskLocalId)).dueAt)
        }

    @Test
    fun `a series with nothing left closes the task for good`() =
        runTest {
            val taskLocalId = givenTask(dueAt = DUE, recurrenceRule = "FREQ=DAILY;COUNT=1")

            tasks.complete(taskLocalId)

            val task = assertNotNull(db.tasks().byLocalId(taskLocalId))
            assertEquals(TaskStatus.COMPLETED, task.status)
            assertEquals(NOW, task.completedAt)
            // The task itself is now the finished row; a second one would be the
            // same work counted twice in the history.
            assertNull(runRecordedAt(taskLocalId, NOW))
            assertEquals(1, db.tasks().count())
        }

    @Test
    fun `a rule this build cannot read leaves the task exactly as it was`() =
        runTest {
            val taskLocalId = givenTask(dueAt = DUE, recurrenceRule = "every other tuesday")

            tasks.complete(taskLocalId)

            val task = assertNotNull(db.tasks().byLocalId(taskLocalId))
            assertEquals(TaskStatus.OPEN, task.status)
            assertEquals(DUE, task.dueAt, "a confident wrong date is worse than the server's answer a moment later")
            assertNull(runRecordedAt(taskLocalId, NOW))
            assertEquals(1, db.tasks().count())
            assertEquals(1, queue().size, "the server still has to be told")
        }

    @Test
    fun `moving on does not finish the work underneath`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val parentLocalId =
                givenTask(projectLocalId = projectLocalId, dueAt = DUE, recurrenceRule = "FREQ=DAILY")
            val childLocalId =
                givenTask(title = "Buy the feed", serverId = 2, parentLocalId = parentLocalId)

            tasks.complete(parentLocalId)

            assertEquals(TaskStatus.OPEN, assertNotNull(db.tasks().byLocalId(childLocalId)).status)
        }

    @Test
    fun `the last run of a series does finish the work underneath`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val parentLocalId =
                givenTask(projectLocalId = projectLocalId, dueAt = DUE, recurrenceRule = "FREQ=DAILY;COUNT=1")
            val childLocalId =
                givenTask(title = "Buy the feed", serverId = 2, parentLocalId = parentLocalId)

            tasks.complete(parentLocalId)

            assertEquals(TaskStatus.COMPLETED, assertNotNull(db.tasks().byLocalId(childLocalId)).status)
        }

    @Test
    fun `moving a repeating task on is still one thing to tell the server`() =
        runTest {
            val taskLocalId = givenTask(dueAt = DUE, recurrenceRule = "FREQ=DAILY")

            tasks.complete(taskLocalId)

            // The advance and the recorded run are both consequences of the
            // completion, and the server works them out again from the same one
            // request. Sending them separately would be sending the same fact twice.
            assertEquals(listOf(OpNames.TASK_COMPLETE), queue().map { it.op })
        }

    @Test
    fun `ticking off a batch leaves a repeating task for the server to move on`() =
        runTest {
            val repeatingLocalId = givenTask(dueAt = DUE, recurrenceRule = "FREQ=DAILY")
            val plainLocalId = givenTask(title = "Take the bins out", serverId = 2)

            tasks.bulkComplete(listOf(repeatingLocalId, plainLocalId))

            // A batch names the tasks and nothing else, so the server times the
            // completion itself and no run written down here could ever be
            // recognised as the one it records. Waiting for its answer costs a
            // moment; writing a run down would cost a permanent second entry in
            // the history.
            val repeating = assertNotNull(db.tasks().byLocalId(repeatingLocalId))
            assertEquals(TaskStatus.OPEN, repeating.status)
            assertEquals(DUE, repeating.dueAt)
            assertNull(runRecordedAt(repeatingLocalId, NOW))
            assertEquals(2, db.tasks().count())

            // What is not repeating is still ticked off at once.
            assertEquals(TaskStatus.COMPLETED, assertNotNull(db.tasks().byLocalId(plainLocalId)).status)
        }

    @Test
    fun `a batch still tells the server about the repeating task`() =
        runTest {
            val repeatingLocalId = givenTask(dueAt = DUE, recurrenceRule = "FREQ=DAILY")

            val write = tasks.bulkComplete(listOf(repeatingLocalId))

            assertEquals(listOf(repeatingLocalId), write.accepted)
            assertEquals(listOf(OpNames.TASK_BULK_COMPLETE), queue().map { it.op })
        }

    @Test
    fun `a batch does not finish the work under a repeating task either`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val parentLocalId =
                givenTask(projectLocalId = projectLocalId, dueAt = DUE, recurrenceRule = "FREQ=DAILY")
            val childLocalId =
                givenTask(title = "Buy the feed", serverId = 2, parentLocalId = parentLocalId)

            tasks.bulkComplete(listOf(parentLocalId))

            assertEquals(TaskStatus.OPEN, assertNotNull(db.tasks().byLocalId(childLocalId)).status)
        }

    /**
     * The run of [taskLocalId] written down as finished at [moment], if there is
     * one. Asked through the same query the catch-up uses to recognise it, so a
     * case here and the real reconciliation cannot disagree about what was written.
     */
    private suspend fun runRecordedAt(
        taskLocalId: Long,
        moment: Long,
    ): TaskRow? = db.tasks().unnamedRecurrenceCompletion(taskLocalId, moment)

    private companion object {
        /** 2024-03-12T09:00:00.000Z — where the task sits, still ahead of [NOW]. */
        val DUE: Long = WireTime.parse("2024-03-12T09:00:00.000Z")

        /** What the shared worked examples say a daily rule moves it to. */
        val NEXT_DUE: Long = WireTime.parse("2024-03-13T09:00:00.000Z")
        val DAY_AFTER_NEXT_DUE: Long = WireTime.parse("2024-03-14T09:00:00.000Z")

        const val FIVE_HOURS: Long = 5 * 60 * 60 * 1000L
        const val ONE_DAY: Long = 24 * 60 * 60 * 1000L
    }
}

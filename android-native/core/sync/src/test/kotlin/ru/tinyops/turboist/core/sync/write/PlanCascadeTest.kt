package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.model.INBOX_ID
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

/**
 * Planning a task plans the work inside it.
 *
 * Both decisions carry downwards, at any depth, and the device applies them
 * itself: a subtask left in yesterday's plan until the queue drained would
 * contradict, on screen, the choice the user just made.
 *
 * The two cascades differ in one deliberate way. Parking clears the day a task
 * was scheduled for, because a parked task has no day at all; committing to the
 * week keeps it, because a subtask scheduled for a concrete day inside that week
 * is still valid planning.
 */
class PlanCascadeTest : WriteTest() {
    @Test
    fun `parking a task parks the work under it and clears its day`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val parent = givenTask(title = "Release", serverId = 11, projectLocalId = projectLocalId)
            val child = scheduled("Tag it", parent, projectLocalId)
            val grandchild = scheduled("Sign the tag", child, projectLocalId)

            tasks.plan(parent, PlanState.BACKLOG)

            for (localId in listOf(parent, child, grandchild)) {
                val row = assertNotNull(db.tasks().byLocalId(localId))
                assertEquals(PlanState.BACKLOG, row.planState)
                assertNull(row.dueAt, "a parked task has no day")
                assertEquals(false, row.dueHasTime)
            }
        }

    @Test
    fun `committing a task to the week keeps the days its subtasks are scheduled for`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val parent = givenTask(title = "Release", serverId = 11, projectLocalId = projectLocalId)
            val child = scheduled("Tag it", parent, projectLocalId)

            tasks.plan(parent, PlanState.WEEK)

            val row = assertNotNull(db.tasks().byLocalId(child))
            assertEquals(PlanState.WEEK, row.planState)
            assertEquals(NOW, row.dueAt, "a day inside the week is still valid planning")
        }

    @Test
    fun `a cascade leaves finished work and the inbox alone`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val parent = givenTask(title = "Release", serverId = 11, projectLocalId = projectLocalId)
            val done =
                db.tasks().insert(
                    TaskRow(
                        serverId = 12,
                        title = "Already done",
                        projectLocalId = projectLocalId,
                        parentLocalId = parent,
                        status = TaskStatus.COMPLETED,
                        completedAt = NOW,
                        createdAt = NOW,
                        updatedAt = NOW,
                    ),
                )
            val captured =
                db.tasks().insert(
                    TaskRow(
                        serverId = 13,
                        title = "Still in the inbox",
                        inboxId = INBOX_ID,
                        parentLocalId = parent,
                        createdAt = NOW,
                        updatedAt = NOW,
                    ),
                )

            tasks.plan(parent, PlanState.BACKLOG)

            assertEquals(
                PlanState.NONE,
                assertNotNull(db.tasks().byLocalId(done)).planState,
                "finished work is history, not something to re-plan",
            )
            assertEquals(
                PlanState.NONE,
                assertNotNull(db.tasks().byLocalId(captured)).planState,
                "the backlog lives inside contexts; moving something out of the inbox is a separate decision",
            )
        }

    @Test
    fun `setting the plan while editing cascades the same way as planning does`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val parent = givenTask(title = "Release", serverId = 11, projectLocalId = projectLocalId)
            val child = scheduled("Tag it", parent, projectLocalId)

            tasks.patch(parent, TaskEdit(planState = PlanState.BACKLOG))

            assertEquals(PlanState.BACKLOG, assertNotNull(db.tasks().byLocalId(child)).planState)
        }

    @Test
    fun `planning queues one request and lets the server cascade on its side`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val parent = givenTask(title = "Release", serverId = 11, projectLocalId = projectLocalId)
            scheduled("Tag it", parent, projectLocalId)

            tasks.plan(parent, PlanState.WEEK)

            val op = queuedOps().single() as PlanTaskOp
            assertEquals(parent, op.taskLocalId)
            assertEquals("week", op.state)
        }

    /** An open subtask with a day of its own, which is what the two cascades treat differently. */
    private suspend fun scheduled(
        title: String,
        parentLocalId: Long,
        projectLocalId: Long,
    ): Long =
        db.tasks().insert(
            TaskRow(
                title = title,
                projectLocalId = projectLocalId,
                parentLocalId = parentLocalId,
                dueAt = NOW,
                dueHasTime = true,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )
}

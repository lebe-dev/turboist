package ru.tinyops.turboist.core.database

import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.dao.WeekProgressRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.assertEquals

/**
 * The drawer's "done out of all" for the week.
 *
 * Only top-level tasks planned for the week count, and the finished ones stay in
 * the total, or the number would shrink as the user gets through the week instead
 * of the first half of it growing.
 */
class WeekProgressQueryTest : ReplicaTest() {
    private val from = NOW
    private val until = NOW + 7 * DAY
    private val inside = NOW + DAY

    private suspend fun progress(): WeekProgressRow = db.taskViews().observeWeekProgress(from, until).first()

    private fun planned() = task().copy(planState = PlanState.WEEK)

    private fun done(row: TaskRow = task()) = row.copy(status = TaskStatus.COMPLETED, completedAt = inside)

    @Test
    fun `an untouched week is nothing out of nothing`() =
        runTest {
            db.tasks().insert(task())

            assertEquals(WeekProgressRow(done = 0, total = 0), progress())
        }

    @Test
    fun `planned work counts, finished or not`() =
        runTest {
            db.tasks().insert(planned())
            db.tasks().insert(planned())
            db.tasks().insert(done(planned()))

            assertEquals(WeekProgressRow(done = 1, total = 3), progress())
        }

    @Test
    fun `work merely due inside the week was not planned for it`() =
        runTest {
            db.tasks().insert(task().copy(dueAt = inside))
            db.tasks().insert(done(task().copy(dueAt = inside)))

            assertEquals(WeekProgressRow(done = 0, total = 0), progress())
        }

    @Test
    fun `work finished in an earlier week has left this one`() =
        runTest {
            db.tasks().insert(planned().copy(status = TaskStatus.COMPLETED, completedAt = from - 1))

            assertEquals(WeekProgressRow(done = 0, total = 0), progress())
        }

    @Test
    fun `work finished this week without being planned is not counted`() =
        runTest {
            db.tasks().insert(done())

            assertEquals(WeekProgressRow(done = 0, total = 0), progress())
        }

    @Test
    fun `a planned subtask is left out, whatever its parent is`() =
        runTest {
            val plannedParent = db.tasks().insert(planned())
            val otherParent = db.tasks().insert(task())
            db.tasks().insert(task(parentLocalId = plannedParent).copy(planState = PlanState.WEEK))
            db.tasks().insert(task(parentLocalId = otherParent).copy(planState = PlanState.WEEK))
            db.tasks().insert(done(task(parentLocalId = plannedParent).copy(planState = PlanState.WEEK)))

            assertEquals(WeekProgressRow(done = 0, total = 1), progress())
        }

    @Test
    fun `cancelled work counts for nothing`() =
        runTest {
            db.tasks().insert(planned().copy(status = TaskStatus.CANCELLED))

            assertEquals(WeekProgressRow(done = 0, total = 0), progress())
        }

    @Test
    fun `the week is half open`() =
        runTest {
            db.tasks().insert(done(planned()).copy(completedAt = from))
            db.tasks().insert(done(planned()).copy(completedAt = until))

            assertEquals(WeekProgressRow(done = 1, total = 1), progress())
        }

    private companion object {
        const val DAY = 24L * 60L * 60L * 1000L
    }
}

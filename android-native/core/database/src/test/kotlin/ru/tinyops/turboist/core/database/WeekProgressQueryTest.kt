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
 * The total has to be the work the week screen is made of plus what has already
 * left it by being finished, or the number would shrink as the user gets through
 * the week instead of the first half of it growing.
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
    fun `planned and due work both count, finished or not`() =
        runTest {
            db.tasks().insert(planned())
            db.tasks().insert(task().copy(dueAt = inside))
            db.tasks().insert(done(planned()))
            db.tasks().insert(done(task().copy(dueAt = inside)))

            assertEquals(WeekProgressRow(done = 2, total = 4), progress())
        }

    @Test
    fun `work finished in an earlier week has left this one`() =
        runTest {
            db.tasks().insert(planned().copy(status = TaskStatus.COMPLETED, completedAt = from - 1))

            assertEquals(WeekProgressRow(done = 0, total = 0), progress())
        }

    @Test
    fun `work finished this week without belonging to it is not counted`() =
        runTest {
            db.tasks().insert(done())

            assertEquals(WeekProgressRow(done = 0, total = 0), progress())
        }

    @Test
    fun `a finished run of a repeating task counts although its plan and date are gone`() =
        runTest {
            val source = db.tasks().insert(task().copy(dueAt = inside, recurrenceRule = "FREQ=DAILY"))
            db.tasks().insert(done(task(sourceTaskLocalId = source)))

            assertEquals(WeekProgressRow(done = 1, total = 2), progress())
        }

    @Test
    fun `subtasks travel with their parent and cancelled work counts for nothing`() =
        runTest {
            val parent = db.tasks().insert(planned())
            db.tasks().insert(task(parentLocalId = parent).copy(planState = PlanState.WEEK))
            db.tasks().insert(planned().copy(status = TaskStatus.CANCELLED))

            assertEquals(WeekProgressRow(done = 0, total = 1), progress())
        }

    @Test
    fun `the week is half open`() =
        runTest {
            db.tasks().insert(task().copy(dueAt = from))
            db.tasks().insert(task().copy(dueAt = until))

            assertEquals(WeekProgressRow(done = 0, total = 1), progress())
        }

    private companion object {
        const val DAY = 24L * 60L * 60L * 1000L
    }
}

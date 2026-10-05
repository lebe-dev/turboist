package ru.tinyops.turboist.core.database

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

/**
 * How the enumerated columns survive contact with a server that moved on.
 *
 * They are stored as the server's own spellings, so a row read back from the
 * device says the same thing as the same row read from the server — and a value
 * added after this build shipped costs one attribute of one row rather than the
 * whole replica.
 */
class EnumStorageTest : ReplicaTest() {
    @Test
    fun `a task keeps every enumerated attribute it was given`() =
        runTest {
            val localId =
                db.tasks().insert(
                    task(title = "Ship it").copy(
                        priority = Priority.HIGH,
                        status = TaskStatus.CANCELLED,
                        dayPart = DayPart.EVENING,
                        planState = PlanState.BACKLOG,
                        troikiCategory = TroikiCategory.REST,
                    ),
                )

            val stored = assertNotNull(db.tasks().byLocalId(localId))
            assertEquals(Priority.HIGH, stored.priority)
            assertEquals(TaskStatus.CANCELLED, stored.status)
            assertEquals(DayPart.EVENING, stored.dayPart)
            assertEquals(PlanState.BACKLOG, stored.planState)
            assertEquals(TroikiCategory.REST, stored.troikiCategory)
        }

    @Test
    fun `columns hold the spelling the server uses, not a position in an enum`() =
        runTest {
            db.tasks().insert(task(title = "Ship it").copy(priority = Priority.NONE, dayPart = DayPart.MORNING))

            val row =
                assertNotNull(
                    db.openHelper.readableDatabase.query("SELECT priority, dayPart, status FROM tasks"),
                ).use { cursor ->
                    cursor.moveToFirst()
                    Triple(cursor.getString(0), cursor.getString(1), cursor.getString(2))
                }

            // Inserting a constant in the middle of an enum would renumber every
            // stored ordinal; a spelling cannot be renumbered.
            assertEquals("no-priority", row.first)
            assertEquals("morning", row.second)
            assertEquals("open", row.third)
        }

    @Test
    fun `a value this build has never heard of costs one attribute, not the row`() =
        runTest {
            db.openHelper.writableDatabase.execSQL(
                """
                INSERT INTO tasks (localId, serverId, title, description, priority, status, dueHasTime,
                                   deadlineHasTime, dayPart, planState, isPinned, isPrivate, isComplex,
                                   postponeCount, createdAt, updatedAt)
                VALUES (1, 9, 'From a newer server', '', 'critical', 'open', 0, 0, 'none', 'none',
                        0, 0, 0, 0, $NOW, $NOW)
                """.trimIndent(),
            )

            val stored = assertNotNull(db.tasks().byLocalId(1))
            assertEquals("From a newer server", stored.title, "the row still reads")
            assertEquals(Priority.UNKNOWN, stored.priority, "only the attribute is lost")
        }

    @Test
    fun `being outside the daily plan is not the same as a bucket this build cannot read`() =
        runTest {
            val localId = db.tasks().insert(task(title = "Unplanned"))

            assertNull(assertNotNull(db.tasks().byLocalId(localId)).troikiCategory)
        }

    @Test
    fun `a project keeps its status and its kind`() =
        runTest {
            val contextLocalId = db.contexts().insert(context())
            val localId =
                db.projects().insert(
                    project(contextLocalId).copy(
                        status = ProjectStatus.ARCHIVED,
                        type = ProjectType.SOFTWARE,
                    ),
                )

            val stored = assertNotNull(db.projects().byLocalId(localId))
            assertEquals(ProjectStatus.ARCHIVED, stored.status)
            assertEquals(ProjectType.SOFTWARE, stored.type)
        }
}

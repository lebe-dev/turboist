package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.Clearable
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The Today focus lock, repeated on the device.
 *
 * While it runs, nothing may be taken into Today from a project that has no open
 * work there, nor from the inbox. The device says so at the moment of the tap:
 * a refused write changes nothing and queues nothing, instead of landing on
 * screen and coming back undone hours later.
 */
class TodayLockGuardTest : WriteTest() {
    private val today = NOW - 60_000
    private val overdue = NOW - DAY
    private val tomorrow = MIDNIGHT + HOUR

    // NOW is a Saturday in UTC, so the week ends at the next midnight but one.
    private val nextWeek = MIDNIGHT + DAY + HOUR

    private suspend fun givenLock(until: Long = MIDNIGHT) =
        givenUserSettings("""{"locale":"ru","todayLockUntil":"${WireTime.format(until)}"}""")

    /** A workspace with "Focus" on Today and "Other" not, and the lock on. */
    private suspend fun givenFocusDay(): Pair<Long, Long> {
        val ctx = givenContext()
        val focus = givenProject(ctx, title = "Focus", serverId = 1)
        val other = givenProject(ctx, title = "Other", serverId = 2)
        givenTask(title = "Today's work", serverId = 10, projectLocalId = focus, dueAt = today)
        givenLock()
        return focus to other
    }

    // --- turning it on -------------------------------------------------------

    @Test
    fun `locking runs until the next midnight and is queued once`() =
        runTest {
            val lock = SettingsWriteRepo(db, writer, ZONE)

            lock.lockToday()

            assertEquals(MIDNIGHT, rules.userSettings().todayLockUntil)
            assertEquals(listOf<OutboxOp>(LockTodayOp), queuedOps())
            assertEquals(ReplicaEntityKind.USER_SETTINGS, queue().single().entity)
        }

    @Test
    fun `locking keeps the rest of the preferences`() =
        runTest {
            givenUserSettings("""{"locale":"ru","newer":{"x":1}}""")

            SettingsWriteRepo(db, writer, ZONE).lockToday()

            val stored = assertNotNull(db.settings().userSettings()).payload
            assertTrue("\"locale\":\"ru\"" in stored)
            assertTrue("\"newer\"" in stored, "a key from a newer server was destroyed")
        }

    @Test
    fun `locking again keeps the running lock's end`() =
        runTest {
            val later = MIDNIGHT + 3 * HOUR
            givenLock(later)

            SettingsWriteRepo(db, writer, ZONE).lockToday()

            assertEquals(later, rules.userSettings().todayLockUntil)
        }

    @Test
    fun `a lock that ran out is replaced by a fresh one`() =
        runTest {
            givenLock(NOW - HOUR)

            SettingsWriteRepo(db, writer, ZONE).lockToday()

            assertEquals(MIDNIGHT, rules.userSettings().todayLockUntil)
        }

    @Test
    fun `the lock ends at the device's own midnight until the server says otherwise`() =
        runTest {
            val moscow = java.time.ZoneId.of("Europe/Moscow")

            SettingsWriteRepo(db, writer, moscow).lockToday()

            // 12:34Z is 15:34 in Moscow; its midnight is 21:00Z.
            assertEquals(WireTime.parse("2024-03-09T21:00:00.000Z"), rules.userSettings().todayLockUntil)
        }

    // --- creating ------------------------------------------------------------

    @Test
    fun `work for today goes into one of today's projects`() =
        runTest {
            val (focus, _) = givenFocusDay()

            tasks.create(TaskDestination.InProject(focus), NewTask(title = "More", dueAt = today))

            assertEquals(1, queue().size)
        }

    @Test
    fun `work for today in another project is refused and nothing is written`() =
        runTest {
            val (_, other) = givenFocusDay()

            val refused =
                assertFailsWith<WriteRefused.TodayLocked> {
                    tasks.create(TaskDestination.InProject(other), NewTask(title = "Sneak in", dueAt = today))
                }

            assertEquals(MIDNIGHT, refused.untilMillis)
            assertTrue(db.tasks().openInProject(other).isEmpty())
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `a project marked as exempt takes work for today with no open work there`() =
        runTest {
            val (_, other) = givenFocusDay()
            db.projects().update(checkNotNull(db.projects().byLocalId(other)).copy(todayLockExempt = true))

            tasks.create(TaskDestination.InProject(other), NewTask(title = "Spontaneous", dueAt = today))

            assertEquals(1, db.tasks().openInProject(other).size)
        }

    @Test
    fun `an overdue date is refused like today`() =
        runTest {
            val (_, other) = givenFocusDay()

            assertFailsWith<WriteRefused.TodayLocked> {
                tasks.create(TaskDestination.InProject(other), NewTask(title = "Late", dueAt = overdue))
            }
        }

    @Test
    fun `the inbox takes nothing for today`() =
        runTest {
            givenFocusDay()

            assertFailsWith<WriteRefused.TodayLocked> {
                tasks.create(TaskDestination.Inbox, NewTask(title = "Capture", dueAt = today))
            }
            tasks.create(TaskDestination.Inbox, NewTask(title = "Capture"))
            assertEquals(1, queue().size, "an undated capture is always fine")
        }

    @Test
    fun `work for another day or with no date goes anywhere`() =
        runTest {
            val (_, other) = givenFocusDay()

            tasks.create(TaskDestination.InProject(other), NewTask(title = "Later", dueAt = tomorrow))
            tasks.create(TaskDestination.InProject(other), NewTask(title = "Someday"))
            tasks.create(TaskDestination.InProject(other), NewTask(title = "At midnight", dueAt = MIDNIGHT))

            assertEquals(3, queue().size)
        }

    @Test
    fun `a project whose work for today is done is closed for the rest of the day`() =
        runTest {
            val ctx = givenContext()
            val focus = givenProject(ctx, title = "Focus", serverId = 1)
            val only = givenTask(title = "Today's work", serverId = 10, projectLocalId = focus, dueAt = today)
            givenLock()

            tasks.complete(only)

            assertFailsWith<WriteRefused.TodayLocked> {
                tasks.create(TaskDestination.InProject(focus), NewTask(title = "More", dueAt = today))
            }
        }

    // --- editing -------------------------------------------------------------

    @Test
    fun `scheduling another project's task for today is refused and the task is untouched`() =
        runTest {
            val (_, other) = givenFocusDay()
            val later = givenTask(title = "Later", serverId = 11, projectLocalId = other, dueAt = nextWeek)

            assertFailsWith<WriteRefused.TodayLocked> {
                tasks.patch(later, TaskEdit(title = "Now", dueAt = Clearable.Set(today)))
            }

            val row = assertNotNull(db.tasks().byLocalId(later))
            assertEquals("Later", row.title)
            assertEquals(nextWeek, row.dueAt)
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `a task dated later this week may be taken into today from another project`() =
        runTest {
            val (_, other) = givenFocusDay()
            val later = givenTask(title = "Sunday", serverId = 11, projectLocalId = other, dueAt = tomorrow)

            // The task qualifies, not its project.
            assertFailsWith<WriteRefused.TodayLocked> {
                tasks.create(TaskDestination.InProject(other), NewTask(title = "Sneak in", dueAt = today))
            }

            tasks.patch(later, TaskEdit(dueAt = Clearable.Set(today)))

            assertEquals(today, assertNotNull(db.tasks().byLocalId(later)).dueAt)
        }

    @Test
    fun `a task planned for the week may be taken into today from another project`() =
        runTest {
            val (_, other) = givenFocusDay()
            val planned = givenTask(title = "This week", serverId = 11, projectLocalId = other)
            db.tasks().update(assertNotNull(db.tasks().byLocalId(planned)).copy(planState = PlanState.WEEK))

            tasks.patch(planned, TaskEdit(dueAt = Clearable.Set(today)))

            assertEquals(today, assertNotNull(db.tasks().byLocalId(planned)).dueAt)
        }

    @Test
    fun `a task already on today may be edited and rescheduled within the day`() =
        runTest {
            val ctx = givenContext()
            val other = givenProject(ctx, title = "Other", serverId = 2)
            // On Today before the lock went on: the lock does not throw it out.
            val task = givenTask(title = "Already here", serverId = 11, projectLocalId = other, dueAt = today)
            givenLock()

            tasks.patch(task, TaskEdit(title = "Renamed", dueAt = Clearable.Set(today + 1_000)))

            assertEquals("Renamed", assertNotNull(db.tasks().byLocalId(task)).title)
        }

    @Test
    fun `taking a task off today is always allowed`() =
        runTest {
            val ctx = givenContext()
            val focus = givenProject(ctx, title = "Focus", serverId = 1)
            val task = givenTask(title = "Today's work", serverId = 10, projectLocalId = focus, dueAt = today)
            givenLock()

            tasks.patch(task, TaskEdit(dueAt = Clearable.Set(tomorrow)))
            tasks.patch(task, TaskEdit(dueAt = Clearable.Clear))

            assertNull(assertNotNull(db.tasks().byLocalId(task)).dueAt)
        }

    @Test
    fun `fields that do not touch the day are always editable`() =
        runTest {
            val (_, other) = givenFocusDay()
            val later = givenTask(title = "Later", serverId = 11, projectLocalId = other)

            tasks.patch(later, TaskEdit(title = "Renamed"))

            assertEquals("Renamed", assertNotNull(db.tasks().byLocalId(later)).title)
        }

    // --- moving --------------------------------------------------------------

    @Test
    fun `moving a today task into another project is refused`() =
        runTest {
            val (focus, other) = givenFocusDay()
            val task = givenTask(title = "Today's too", serverId = 11, projectLocalId = focus, dueAt = today)

            assertFailsWith<WriteRefused.TodayLocked> { tasks.move(task, TaskDestination.InProject(other)) }

            assertEquals(focus, assertNotNull(db.tasks().byLocalId(task)).projectLocalId)
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `moving a today inbox task into a project is refused`() =
        runTest {
            val (_, other) = givenFocusDay()
            val task = givenTask(title = "Captured", serverId = 11, inboxId = 1, dueAt = today)

            assertFailsWith<WriteRefused.TodayLocked> { tasks.move(task, TaskDestination.InProject(other)) }
        }

    @Test
    fun `a bulk move leaves out only the tasks the lock keeps out`() =
        runTest {
            val (focus, other) = givenFocusDay()
            val onToday = givenTask(title = "Today's too", serverId = 11, projectLocalId = focus, dueAt = today)
            val undated = givenTask(title = "Someday", serverId = 12, projectLocalId = focus)

            val result = tasks.bulkMove(listOf(onToday, undated), TaskDestination.InProject(other))

            assertEquals(listOf(undated), result.accepted)
            assertEquals(listOf(onToday), result.todayLocked)
            assertEquals(
                listOf<OutboxOp>(BulkMoveTasksOp(listOf(undated), TaskDestination.InProject(other))),
                queuedOps(),
            )
            assertEquals(focus, assertNotNull(db.tasks().byLocalId(onToday)).projectLocalId)
        }

    @Test
    fun `a bulk move the lock keeps out entirely queues nothing`() =
        runTest {
            val (focus, other) = givenFocusDay()
            val onToday = givenTask(title = "Today's too", serverId = 11, projectLocalId = focus, dueAt = today)

            val result = tasks.bulkMove(listOf(onToday), TaskDestination.InProject(other))

            assertNull(result.opId)
            assertEquals(listOf(onToday), result.todayLocked)
            assertTrue(queue().isEmpty())
        }

    // --- reopening and copying -----------------------------------------------

    @Test
    fun `reopening another project's task for today is refused`() =
        runTest {
            val (_, other) = givenFocusDay()
            val done =
                givenTask(
                    title = "Done",
                    serverId = 11,
                    projectLocalId = other,
                    dueAt = today,
                    status = TaskStatus.COMPLETED,
                )

            assertFailsWith<WriteRefused.TodayLocked> { tasks.uncomplete(done) }

            assertEquals(TaskStatus.COMPLETED, assertNotNull(db.tasks().byLocalId(done)).status)
        }

    @Test
    fun `reopening work for another day is allowed`() =
        runTest {
            val (_, other) = givenFocusDay()
            val done =
                givenTask(
                    title = "Done",
                    serverId = 11,
                    projectLocalId = other,
                    dueAt = tomorrow,
                    status = TaskStatus.COMPLETED,
                )

            tasks.uncomplete(done)

            assertEquals(TaskStatus.OPEN, assertNotNull(db.tasks().byLocalId(done)).status)
        }

    @Test
    fun `a copy for today in another project is refused`() =
        runTest {
            val (_, other) = givenFocusDay()
            val source = givenTask(title = "Template-ish", serverId = 11, projectLocalId = other, dueAt = tomorrow)

            assertFailsWith<WriteRefused.TodayLocked> { tasks.duplicate(source, dueAt = today) }
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `a copy of a task on today stays in today's project`() =
        runTest {
            val (focus, _) = givenFocusDay()
            val source = givenTask(title = "Also today", serverId = 11, projectLocalId = focus, dueAt = today)

            tasks.duplicate(source)

            assertEquals(1, queue().size)
        }

    // --- when no lock runs ---------------------------------------------------

    @Test
    fun `without a lock anything goes`() =
        runTest {
            val ctx = givenContext()
            val other = givenProject(ctx, title = "Other", serverId = 2)

            tasks.create(TaskDestination.InProject(other), NewTask(title = "Now", dueAt = today))
            tasks.create(TaskDestination.Inbox, NewTask(title = "Capture", dueAt = today))

            assertEquals(2, queue().size)
        }

    @Test
    fun `a lock that ran out refuses nothing`() =
        runTest {
            val ctx = givenContext()
            val other = givenProject(ctx, title = "Other", serverId = 2)
            givenLock(NOW - HOUR)

            tasks.create(TaskDestination.InProject(other), NewTask(title = "Now", dueAt = today))

            assertEquals(1, queue().size)
        }

    private companion object {
        const val HOUR: Long = 3_600_000L
        const val DAY: Long = 24 * HOUR

        /** The UTC midnight after [NOW]: 2024-03-10T00:00:00.000Z. */
        val MIDNIGHT: Long = WireTime.parse("2024-03-10T00:00:00.000Z")
    }
}

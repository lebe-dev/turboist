package ru.tinyops.turboist.nativeapp.sync

import android.app.Application
import androidx.room.Room
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.SyncStateRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.sync.SyncCycleResult
import ru.tinyops.turboist.core.sync.drain.UnsentChanges
import ru.tinyops.turboist.core.sync.write.OutboxOpKind
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull

/**
 * The five things the strip says, each read from the place that records it.
 *
 * Every part of the status is already written down for the engine's own reasons
 * — how far the replica has caught up is a row of the replica, what is queued is
 * the queue, what was refused is the set-aside pile — so the only thing that can
 * go wrong here is the wiring: a count taken from the wrong query reads as a
 * perfectly ordinary number and is wrong every time. The counts below are
 * therefore all different from one another, so a crossed wire cannot pass.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], application = Application::class)
class ReplicaSyncStatusSourceTest {
    private lateinit var db: TurboistDatabase
    private lateinit var activity: SyncActivity

    @Before
    fun openReplica() {
        db =
            Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), TurboistDatabase::class.java)
                .build()
        activity = SyncActivity()
    }

    @After
    fun closeReplica() {
        db.close()
    }

    @Test
    fun `a device that has never synced says nothing has happened yet`() =
        runTest {
            val status = source().status.first()

            assertFalse(status.syncing)
            assertEquals(SyncOutcome.UNKNOWN, status.outcome)
            assertNull(status.lastSyncAt, "a replica that has never caught up has no time to report")
            assertEquals(0, status.waiting)
            assertEquals(0, status.setAside)
        }

    @Test
    fun `the time, the queue and the refused pile each come from their own record`() =
        runTest {
            db.syncState().save(SyncStateRow(epoch = 1, cursor = 12, lastSyncAt = LAST_SYNC_AT))
            queue("op-a")
            queue("op-b")
            queue("op-c")
            refuse("op-d")
            refuse("op-e")

            val status = source().status.first { it.waiting > 0 }

            assertEquals(LAST_SYNC_AT, status.lastSyncAt)
            assertEquals(3, status.waiting, "the queue holds three")
            assertEquals(2, status.setAside, "and two were refused")
        }

    @Test
    fun `a refusal leaves the queue, so it is counted once and on the pile that needs a person`() =
        runTest {
            queue("op-a")
            refuse("op-b")

            val status = source().status.first { it.setAside > 0 }

            assertEquals(1, status.waiting)
            assertEquals(1, status.setAside)
        }

    @Test
    fun `a change made while the screen is open reaches it`() =
        runTest {
            val source = source()
            assertEquals(0, source.status.first().waiting)

            queue("op-a")

            assertEquals(1, source.status.first { it.waiting > 0 }.waiting)
        }

    @Test
    fun `a cycle on the wire is reported as running, and its answer as the outcome`() =
        runTest {
            val source = source()
            val release = CompletableDeferred<Unit>()
            val cycle = activity.watching { release.await().let { SyncCycleResult.Offline } }

            val running = launch { cycle.runSyncCycle() }
            assertEquals(true, source.status.first { it.syncing }.syncing)

            release.complete(Unit)
            running.join()

            val settled = source.status.first { !it.syncing }
            assertEquals(SyncOutcome.UNREACHABLE, settled.outcome, "the cycle's own answer, unaltered")
        }

    /**
     * A source over this replica, collected for as long as the test runs.
     *
     * The scope is the test's background scope rather than a scope of its own so
     * the five queries are torn down with the test instead of outliving it.
     */
    private fun TestScope.source(): ReplicaSyncStatusSource =
        ReplicaSyncStatusSource(
            database = db,
            unsent = UnsentChanges(db),
            activity = activity,
            scope = backgroundScope,
        )

    private suspend fun queue(id: String) {
        db.outbox().enqueue(
            OutboxOpRow(
                id = id,
                op = OutboxOpKind.TASK_COMPLETE.stored,
                payload = "{}",
                entity = ReplicaEntityKind.TASK,
                entityLocalId = 1,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )
    }

    private suspend fun refuse(id: String) {
        queue(id)
        db.outbox().quarantine(
            op = requireNotNull(db.outbox().byId(id)),
            errorCode = ApiErrorCodes.CONFLICT,
            errorMessage = "",
            httpStatus = null,
            quarantinedAt = NOW,
        )
    }

    private companion object {
        /** 2024-03-09T12:34:56.789Z, the instant the rest of the replica's tests are written against. */
        const val NOW: Long = 1_709_987_696_789L

        /** A moment distinct from every other number here, so it cannot be mistaken for a count. */
        const val LAST_SYNC_AT: Long = 1_709_900_000_000L
    }
}

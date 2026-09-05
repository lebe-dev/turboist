package ru.tinyops.turboist.core.sync.maintenance

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncCycleResult
import java.io.IOException
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertSame

/**
 * When housekeeping happens, and what it is allowed to cost.
 *
 * This is the seam anything that needs tidying on the back of a sync uses, so the
 * two things every such step relies on are worth pinning: it never runs on a turn
 * that did not reach the server — tidying a copy known to be behind means acting
 * on a guess — and it never changes what that turn reports, however badly it goes
 * itself. A caller asked whether the device and the server agree; the answer to
 * that question cannot depend on whether some cleanup succeeded.
 */
@RunWith(RobolectricTestRunner::class)
class ReplicaMaintenanceTest {
    private var runs = 0

    private val counting = ReplicaMaintenance { runs++ }

    @Test
    fun `a turn that agreed with the server is followed by housekeeping`() =
        runTest {
            val result = cycle(SyncCycleResult.Synced).withMaintenance(counting).runSyncCycle()

            assertEquals(1, runs, "the replica has just been made current, which is when it can be tidied")
            assertEquals(SyncCycleResult.Synced, result)
        }

    @Test
    fun `a turn that never reached the server is not followed by housekeeping`() =
        runTest {
            val result = cycle(SyncCycleResult.Offline).withMaintenance(counting).runSyncCycle()

            assertEquals(0, runs, "the replica is known to be behind, so anything it says about age is a guess")
            assertEquals(SyncCycleResult.Offline, result)
        }

    @Test
    fun `a refused turn is not followed by housekeeping`() =
        runTest {
            val refusal = SyncCycleResult.Refused(IllegalStateException("no"))

            val result = cycle(refusal).withMaintenance(counting).runSyncCycle()

            assertEquals(0, runs, "the server was reached and would not answer, so nothing was made current")
            assertSame(refusal, result, "the refusal reaches the caller as it was, cause and all")
        }

    @Test
    fun `housekeeping that fails does not make a successful turn look failed`() =
        runTest {
            val failing = ReplicaMaintenance { throw IOException("the database went away mid-tidy") }

            val result = cycle(SyncCycleResult.Synced).withMaintenance(failing).runSyncCycle()

            assertEquals(
                SyncCycleResult.Synced,
                result,
                "the device and the server did agree; tidying up is a separate matter",
            )
        }

    @Test
    fun `a cancelled scope is passed on rather than swallowed`() =
        runTest {
            val cancelled = ReplicaMaintenance { throw CancellationException("the scope is going away") }

            assertFailsWith<CancellationException> {
                cycle(SyncCycleResult.Synced).withMaintenance(cancelled).runSyncCycle()
            }
        }

    @Test
    fun `every turn gets its own housekeeping`() =
        runTest {
            val cycle = cycle(SyncCycleResult.Synced).withMaintenance(counting)

            repeat(3) { cycle.runSyncCycle() }

            assertEquals(3, runs, "the window moves with the day, so tidying is per turn and not once ever")
        }

    private fun cycle(result: SyncCycleResult): SyncCycle = SyncCycle { result }
}

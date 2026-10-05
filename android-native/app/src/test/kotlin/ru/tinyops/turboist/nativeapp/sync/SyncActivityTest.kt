package ru.tinyops.turboist.nativeapp.sync

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.yield
import org.junit.Test
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncCycleResult
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What the screen learns from cycles nobody on screen asked for.
 *
 * The engine answers its caller and keeps nothing, which leaves the status strip
 * with nothing to read: the cycles it has to report on are the automatic ones.
 * So the cycle is wrapped, and these are the four things the wrapping has to get
 * right — it is running, it finished, how it finished, and that a cycle blowing
 * up is still reported rather than swallowed or left as "in progress" forever.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class SyncActivityTest {
    private val activity = SyncActivity()

    @Test
    fun `a cycle that has not run yet has said nothing`() =
        runTest {
            assertEquals(SyncOutcome.UNKNOWN, activity.outcome.value)
            assertFalse(activity.syncing.first())
        }

    @Test
    fun `a cycle is reported as running for exactly as long as it runs`() =
        runTest {
            val release = CompletableDeferred<Unit>()
            val cycle =
                activity.watching {
                    release.await()
                    SyncCycleResult.Synced
                }

            val running = launch { cycle.runSyncCycle() }
            yield()
            assertTrue(activity.syncing.first(), "the strip has to be up while the cycle is on the wire")

            release.complete(Unit)
            running.join()
            assertFalse(activity.syncing.first())
        }

    @Test
    fun `the three answers a cycle can give each reach the screen`() =
        runTest {
            assertEquals(SyncOutcome.SYNCED, outcomeOf(SyncCycleResult.Synced))
            assertEquals(SyncOutcome.UNREACHABLE, outcomeOf(SyncCycleResult.Offline))
            assertEquals(SyncOutcome.REFUSED, outcomeOf(SyncCycleResult.Refused(IllegalStateException("no"))))
        }

    @Test
    fun `a cycle that blows up is reported and passed on`() =
        runTest {
            val cycle = activity.watching { error("the replica could not be written") }

            // Passed on, because the caller above still has to decide what to do
            // with a defect; swallowing it here would dress one up as an
            // ordinary refusal everywhere above.
            assertFailsWith<IllegalStateException> { cycle.runSyncCycle() }

            assertEquals(SyncOutcome.REFUSED, activity.outcome.value)
            assertFalse(activity.syncing.first(), "a cycle that threw is not still running")
        }

    @Test
    fun `two cycles at once leave the strip up until the last one is done`() =
        runTest {
            val first = CompletableDeferred<Unit>()
            val second = CompletableDeferred<Unit>()
            val one = launch { activity.watching { first.await().let { SyncCycleResult.Synced } }.runSyncCycle() }
            val two = launch { activity.watching { second.await().let { SyncCycleResult.Synced } }.runSyncCycle() }
            yield()

            first.complete(Unit)
            one.join()
            assertTrue(activity.syncing.first(), "the other cycle is still on the wire")

            second.complete(Unit)
            two.join()
            assertFalse(activity.syncing.first())
        }

    private suspend fun outcomeOf(result: SyncCycleResult): SyncOutcome {
        val cycle: SyncCycle = activity.watching { result }
        cycle.runSyncCycle()
        return activity.outcome.value
    }
}

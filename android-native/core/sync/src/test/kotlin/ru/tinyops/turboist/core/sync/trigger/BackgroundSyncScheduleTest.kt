package ru.tinyops.turboist.core.sync.trigger

import android.content.Context
import androidx.work.Configuration
import androidx.work.NetworkType
import androidx.work.WorkInfo
import androidx.work.WorkManager
import androidx.work.testing.SynchronousExecutor
import androidx.work.testing.WorkManagerTestInitHelper
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import java.util.concurrent.TimeUnit
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * The jobs the system runs on the app's behalf, against the real scheduler.
 *
 * The property under test is uniqueness, and it is not a tidiness one. A trigger
 * that enqueues rather than replaces leaves a job behind every time the app is
 * opened or the network flickers, and a day of that is a queue of hundreds, each
 * one waking the radio to ask a question the previous one already answered.
 */
@RunWith(RobolectricTestRunner::class)
class BackgroundSyncScheduleTest {
    private lateinit var context: Context
    private lateinit var workManager: WorkManager
    private lateinit var schedule: BackgroundSyncSchedule

    @Before
    fun startScheduler() {
        context = RuntimeEnvironment.getApplication()
        WorkManagerTestInitHelper.initializeTestWorkManager(
            context,
            Configuration.Builder().setExecutor(SynchronousExecutor()).build(),
        )
        workManager = WorkManager.getInstance(context)
        schedule = WorkManagerSyncSchedule(workManager)
    }

    @Test
    fun `asking to keep syncing twice leaves one repeating job`() {
        schedule.keepSyncing()
        schedule.keepSyncing()
        schedule.keepSyncing()

        val jobs = pendingJobs(WorkManagerSyncSchedule.PERIODIC_WORK)
        assertEquals(1, jobs.size, "every launch asks for this, and every launch must find the one already there")
    }

    @Test
    fun `several reasons to sync soon leave one job`() {
        schedule.syncSoon(SyncReason.APP_FOREGROUND)
        schedule.syncSoon(SyncReason.NETWORK_REGAINED)

        val jobs = pendingJobs(WorkManagerSyncSchedule.IMMEDIATE_WORK)
        assertEquals(1, jobs.size, "one pending catch-up answers every reason waiting for one")
    }

    @Test
    fun `both jobs wait for a network`() {
        schedule.keepSyncing()
        schedule.syncSoon(SyncReason.APP_FOREGROUND)

        val jobs =
            pendingJobs(WorkManagerSyncSchedule.PERIODIC_WORK) + pendingJobs(WorkManagerSyncSchedule.IMMEDIATE_WORK)
        assertEquals(2, jobs.size)
        assertTrue(
            jobs.all { it.constraints.requiredNetworkType == NetworkType.CONNECTED },
            "waking a device that cannot reach the server spends battery to learn nothing",
        )
    }

    @Test
    fun `a device told not to spend mobile data waits for a connection it is not paying for`() {
        // The system, not this app, knows which connections the user considers
        // expensive, so the choice is expressed as the platform's own
        // "unmetered" rather than by inspecting the current network here.
        val frugal = WorkManagerSyncSchedule(workManager, meteredAllowed = { false })

        frugal.keepSyncing()
        frugal.syncSoon(SyncReason.APP_FOREGROUND)

        val jobs =
            pendingJobs(WorkManagerSyncSchedule.PERIODIC_WORK) + pendingJobs(WorkManagerSyncSchedule.IMMEDIATE_WORK)
        assertEquals(2, jobs.size)
        assertTrue(jobs.all { it.constraints.requiredNetworkType == NetworkType.UNMETERED })
    }

    @Test
    fun `the repeating job waits a quarter of an hour between runs`() {
        schedule.keepSyncing()

        val job = pendingJobs(WorkManagerSyncSchedule.PERIODIC_WORK).single()

        // What a backgrounded app costs is one wake-up per interval, so the
        // interval is the whole of the battery budget for an app nobody is
        // looking at. It is also the shortest the platform accepts: asking for
        // less would be silently rounded up to this and would only make the code
        // claim something untrue.
        assertEquals(
            TimeUnit.MINUTES.toMillis(WorkManagerSyncSchedule.PERIOD_MINUTES),
            assertNotNull(job.periodicityInfo).repeatIntervalMillis,
        )
    }

    @Test
    fun `stopping takes both jobs away`() {
        schedule.keepSyncing()
        schedule.syncSoon(SyncReason.APP_FOREGROUND)

        schedule.stopSyncing()

        assertEquals(0, pendingJobs(WorkManagerSyncSchedule.PERIODIC_WORK).size)
        assertEquals(0, pendingJobs(WorkManagerSyncSchedule.IMMEDIATE_WORK).size)
    }

    private fun pendingJobs(name: String): List<WorkInfo> =
        workManager.getWorkInfosForUniqueWork(name).get().filterNot { it.state.isFinished }
}

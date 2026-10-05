package ru.tinyops.turboist.core.sync.trigger

import android.content.Context
import android.os.Build
import androidx.work.Constraints
import androidx.work.Data
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequest
import androidx.work.OutOfQuotaPolicy
import androidx.work.PeriodicWorkRequest
import androidx.work.WorkManager
import java.util.concurrent.TimeUnit

/**
 * Syncing that outlives the app's process.
 *
 * Two jobs and no more. One repeats, so a phone left in a pocket does not open
 * on yesterday's tasks; one runs as soon as it can, for the moments when
 * something just changed the odds that a sync is worth doing — the network came
 * back, the user opened the app.
 */
interface BackgroundSyncSchedule {
    /** Makes sure the repeating job exists. Calling it again does not add a second one. */
    fun keepSyncing()

    /** Asks for one sync as soon as the system will allow it. */
    fun syncSoon(reason: SyncReason)

    /** Drops both jobs. What signing out means to the background. */
    fun stopSyncing()
}

/**
 * The [BackgroundSyncSchedule] as the platform's own job scheduler runs it.
 *
 * @param meteredAllowed whether a job may run on a connection the user pays by
 *   the megabyte for. Asked each time a job is built rather than fixed at
 *   construction, because the answer is a setting the user can change; a job
 *   already handed to the platform keeps the constraint it was enqueued with,
 *   so a change is only picked up by a job enqueued afterwards.
 */
class WorkManagerSyncSchedule(
    private val workManager: WorkManager,
    private val meteredAllowed: () -> Boolean = { true },
) : BackgroundSyncSchedule {
    override fun keepSyncing() {
        workManager.enqueueUniquePeriodicWork(
            PERIODIC_WORK,
            // Keeping the existing job rather than replacing it is what makes this
            // safe to call on every launch: replacing would restart the interval
            // each time, so an app opened every ten minutes would never reach the
            // end of one and would never sync in the background at all.
            ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequest.Builder(SyncWorker::class.java, PERIOD_MINUTES, TimeUnit.MINUTES)
                .setConstraints(networkRequired)
                .setInputData(reasonData(SyncReason.PERIODIC))
                .build(),
        )
    }

    override fun syncSoon(reason: SyncReason) {
        workManager.enqueueUniqueWork(
            IMMEDIATE_WORK,
            // One pending catch-up is as good as three: they would all ask the
            // server the same question, and the first answer settles it.
            ExistingWorkPolicy.KEEP,
            immediateRequest(reason),
        )
    }

    override fun stopSyncing() {
        workManager.cancelUniqueWork(PERIODIC_WORK)
        workManager.cancelUniqueWork(IMMEDIATE_WORK)
    }

    private fun immediateRequest(reason: SyncReason): OneTimeWorkRequest {
        val request =
            OneTimeWorkRequest.Builder(SyncWorker::class.java)
                .setConstraints(networkRequired)
                .setInputData(reasonData(reason))
        // Asking for priority is only free from the version where the system
        // grants it as a job. Below that it is granted as a foreground service,
        // which owes the user a permanent notification — far too much ceremony to
        // announce that a task list is being refreshed. Those devices get an
        // ordinary job, which the system still runs within moments while the app
        // is the one in front of the user.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            request.setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
        }
        return request.build()
    }

    private fun reasonData(reason: SyncReason): Data = Data.Builder().putString(SyncWorker.REASON, reason.name).build()

    /**
     * Any network, or only one the user is not paying for by the megabyte.
     *
     * `UNMETERED` is the platform's way of saying "wi-fi, or something the user
     * has told the system is as good as wi-fi", which is the honest reading of a
     * switch labelled "not on mobile data": the system, not this app, knows which
     * connections the user considers expensive.
     */
    private val networkRequired: Constraints
        get() =
            Constraints.Builder()
                .setRequiredNetworkType(if (meteredAllowed()) NetworkType.CONNECTED else NetworkType.UNMETERED)
                .build()

    companion object {
        /**
         * The name both jobs are enqueued under, one each. A unique name is what
         * turns "ask for a sync" into "make sure a sync is pending": without it,
         * every foreground and every reconnect would leave another job behind, and
         * a day of commuting would end in a queue of hundreds.
         */
        const val PERIODIC_WORK: String = "turboist-sync-periodic"
        const val IMMEDIATE_WORK: String = "turboist-sync-now"

        /**
         * How often a backgrounded app catches up. It is also the shortest period
         * the platform accepts, so asking for less would silently become this.
         */
        const val PERIOD_MINUTES: Long = 15

        /** Builds the schedule against the process-wide job scheduler. */
        fun of(
            context: Context,
            meteredAllowed: () -> Boolean = { true },
        ): BackgroundSyncSchedule = WorkManagerSyncSchedule(WorkManager.getInstance(context), meteredAllowed)
    }
}

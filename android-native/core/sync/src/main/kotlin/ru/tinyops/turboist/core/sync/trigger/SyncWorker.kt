package ru.tinyops.turboist.core.sync.trigger

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import ru.tinyops.turboist.core.sync.SyncCycleResult

/**
 * A sync cycle run by the system rather than by the app.
 *
 * It exists because a process is not a promise: the app the user last opened may
 * be gone by the time a sync is due, and the job that runs this can start a new
 * one. Everything it needs is therefore built from the dependency graph at the
 * moment it runs, never captured beforehand — which is what a worker factory is
 * for, and why this class takes what it needs as a constructor argument instead
 * of reaching for a singleton.
 *
 * The work itself is one call. Deciding what a cycle is belongs to the engine;
 * deciding what a failed one means to the system belongs here.
 */
class SyncWorker(
    appContext: Context,
    parameters: WorkerParameters,
    private val scheduler: SyncScheduler,
) : CoroutineWorker(appContext, parameters) {
    override suspend fun doWork(): Result {
        val reason =
            inputData.getString(REASON)
                ?.let { name -> SyncReason.entries.firstOrNull { it.name == name } }
                ?: SyncReason.PERIODIC
        return when (scheduler.requestSyncNow(reason)) {
            SyncCycleResult.Synced -> Result.success()
            // The network went away between the job's constraint being met and
            // the request going out. Worth another attempt, which the system will
            // back off and place when there is a network again.
            SyncCycleResult.Offline -> Result.retry()
            // The server was reached and said no. Repeating the identical call
            // repeats the identical answer, so this run is over; whatever the
            // refusal was about is settled by the user or by the next scheduled
            // run, not by trying harder right now.
            is SyncCycleResult.Refused -> Result.success()
        }
    }

    companion object {
        /** Input key carrying the [SyncReason] name, so the log says what woke the device. */
        const val REASON: String = "reason"
    }
}

package ru.tinyops.turboist.nativeapp.sync

import android.content.Context
import androidx.work.ListenableWorker
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters
import ru.tinyops.turboist.core.sync.trigger.SyncScheduler
import ru.tinyops.turboist.core.sync.trigger.SyncWorker
import javax.inject.Inject
import javax.inject.Provider
import javax.inject.Singleton

/**
 * Builds the background sync worker from the dependency graph.
 *
 * A scheduled job can outlive the process that scheduled it: the system may
 * start a fresh one to run work enqueued hours ago by an app the user has since
 * closed. Whatever the worker needs therefore cannot be handed to it at enqueue
 * time — it has to be built when the worker is, from a graph that is itself
 * freshly built. That is the whole reason this class exists, and why the
 * dependency is taken as a [Provider]: resolving it at run time, not at
 * factory-construction time, keeps the graph from being touched until a worker
 * actually needs it.
 */
@Singleton
class SyncWorkerFactory
    @Inject
    constructor(
        private val scheduler: Provider<SyncScheduler>,
    ) : WorkerFactory() {
        override fun createWorker(
            appContext: Context,
            workerClassName: String,
            workerParameters: WorkerParameters,
        ): ListenableWorker? {
            // Returning null for anything else is how a factory says "not mine",
            // which leaves the default one free to build workers this app does not
            // own — a library's, for instance.
            if (workerClassName != SyncWorker::class.java.name) return null
            return SyncWorker(appContext, workerParameters, scheduler.get())
        }
    }

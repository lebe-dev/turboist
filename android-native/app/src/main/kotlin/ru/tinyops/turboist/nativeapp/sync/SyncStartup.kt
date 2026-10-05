package ru.tinyops.turboist.nativeapp.sync

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.sync.trigger.SyncTriggers
import ru.tinyops.turboist.nativeapp.di.ApplicationScope
import java.util.concurrent.atomic.AtomicBoolean
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Arms the sync triggers, once per process.
 *
 * Called from the application object rather than from a screen, for the same
 * reason the session is resolved there: syncing has to start before anything is
 * displayed and keep going after the user navigates away from whatever screen
 * happened to be open.
 */
@Singleton
class SyncStartup
    @Inject
    constructor(
        private val triggers: SyncTriggers,
        private val metered: MeteredSyncPolicy,
        @param:ApplicationScope private val scope: CoroutineScope,
    ) {
        private val started = AtomicBoolean(false)

        fun start() {
            if (!started.compareAndSet(false, true)) return
            scope.launch {
                // Read before anything is scheduled. Arming the triggers first
                // would enqueue the repeating job under whatever the flag happened
                // to hold, and a job's constraints are fixed when it is enqueued —
                // for this setting that means one run over mobile data the user
                // had asked the app not to make.
                metered.load()
                triggers.start(scope)
            }
        }
    }

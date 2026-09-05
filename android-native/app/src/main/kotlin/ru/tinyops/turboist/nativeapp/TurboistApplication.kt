package ru.tinyops.turboist.nativeapp

import android.app.Application
import androidx.work.Configuration
import dagger.hilt.android.HiltAndroidApp
import ru.tinyops.turboist.nativeapp.auth.SessionManager
import ru.tinyops.turboist.nativeapp.sync.SyncStartup
import ru.tinyops.turboist.nativeapp.sync.SyncWorkerFactory
import javax.inject.Inject
import javax.inject.Provider

/**
 * Process entry point. Hilt generates the application component from here, so
 * every injected graph in the app hangs off this class.
 *
 * It also supplies the configuration for the platform's job scheduler, because a
 * scheduled sync may be run by a process that has just been created for it: the
 * worker is built from this graph at that moment, which is only possible if the
 * scheduler is told where the graph is before it builds anything.
 *
 * That last duty is why the background triggers are asked for through a
 * [Provider] rather than injected outright. Reaching the job scheduler makes it
 * ask this object for its configuration, which reads [workerFactory]; building
 * anything that reaches it *while this object's own fields are still being
 * filled* therefore reads a field that does not exist yet, and the process dies
 * on launch. Deferring to [onCreate] — by which point every field is assigned —
 * is what keeps that loop from closing. The durable rule: nothing constructed
 * during this object's injection may touch the job scheduler.
 */
@HiltAndroidApp
class TurboistApplication :
    Application(),
    Configuration.Provider {
    @Inject
    lateinit var session: SessionManager

    @Inject
    lateinit var sync: Provider<SyncStartup>

    @Inject
    lateinit var workerFactory: SyncWorkerFactory

    override val workManagerConfiguration: Configuration
        get() = Configuration.Builder().setWorkerFactory(workerFactory).build()

    override fun onCreate() {
        super.onCreate()
        // Resolving the stored session decides what the first screen *is*, so it
        // cannot be started by a screen. It runs in the background and publishes
        // its answer; until then the shell shows a neutral splash.
        session.start()
        // The triggers watch that same session and stay dormant until there is
        // one, so arming them here costs a signed-out device nothing. Built only
        // now, for the reason in the class comment.
        sync.get().start()
    }
}

package ru.tinyops.turboist.nativeapp.e2e

import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import ru.tinyops.turboist.core.database.entity.QuarantinedOpRow
import ru.tinyops.turboist.core.sync.SyncCycleResult
import ru.tinyops.turboist.nativeapp.auth.AuthFailure
import ru.tinyops.turboist.nativeapp.auth.AuthResult
import ru.tinyops.turboist.nativeapp.auth.AuthStep
import ru.tinyops.turboist.nativeapp.devtools.EngineAccess
import ru.tinyops.turboist.nativeapp.session.SessionState

/**
 * The installed app, driven from inside its own process.
 *
 * Everything here goes through the objects the app itself uses — its session
 * manager, its write path, its sync engine, its replica. Nothing is
 * reconstructed for the test, because a reconstruction is a second wiring that
 * can pass while the real one is broken.
 *
 * The screens are deliberately not driven. What these scenarios are about — a
 * queue that keeps its order across a lost network, an id assigned by a server
 * landing on a row created before there was one, a refusal that has to be kept
 * rather than retried — happens underneath the screens, and a walk through the
 * interface would test the layout instead, slowly and flakily.
 */
class DeviceUnderTest(private val settings: HarnessSettings) {
    private val engine = EngineAccess.of(InstrumentationRegistry.getInstrumentation().targetContext)

    val database = engine.database()
    val taskWrites = engine.taskWrites()
    val syncCycle = engine.syncCycle()

    private val sessions = engine.sessions()
    private val unsentChanges = engine.unsentChanges()

    /**
     * Puts the device back to what a first launch finds: no copied data, no
     * queue, no session, and no idea which server it belongs to.
     *
     * The suite is installed once and runs several scenarios in one process, so
     * this — rather than the install — is what makes each of them start from a
     * clean device. The install is clean too; this keeps the later scenarios as
     * honest as the first.
     */
    suspend fun resetToFirstLaunch() {
        // The launch sequence is already running when the process starts. Letting
        // it finish first keeps it from writing a session back over this wipe.
        withTimeout(BOOT_TIMEOUT_MILLIS) { sessions.state.first { it != SessionState.Connecting } }
        withContext(Dispatchers.IO) { database.clearAllTables() }
        engine.refreshTokens().clear()
        engine.accessTokens().set(null)
        engine.serverAddresses().clear()
        engine.serverUrl().clear()
    }

    /**
     * Points the app at the server and gets it a session, taking whichever way in
     * the server offers.
     *
     * A server nobody has claimed yet has exactly one way in — creating the
     * single account it hosts — and once it is claimed that way is closed. Which
     * of the two applies is the server's answer, not the test's assumption, so
     * the flow is followed rather than predicted.
     *
     * @return the step the server sent the app to, which is how a scenario can
     *   say whether this was a virgin instance.
     */
    suspend fun connectAndAuthenticate(): AuthStep {
        val connected = connectPatiently()
        check(connected is AuthResult.MovedTo) {
            "connecting to the server did not lead to a sign-in screen: $connected"
        }
        val step = connected.step
        val signedIn =
            when (step) {
                AuthStep.Setup -> sessions.createAccount(settings.username, settings.password)
                AuthStep.SignIn -> sessions.signIn(settings.username, settings.password)
                AuthStep.Connect -> error("the server address was accepted and then asked for again")
            }
        check(signedIn is AuthResult.Done) { "signing in did not produce a session: $signedIn" }
        check(sessions.state.value == SessionState.LoggedIn) { "the session did not settle as signed in" }
        return step
    }

    /**
     * Proves the address, retrying while the only thing wrong is that nothing
     * answered.
     *
     * Radios that have just been switched back on are reported as a usable
     * network before the route to anything is actually up, and a scenario that
     * begins the moment the previous one restored them lands in that gap. Every
     * other refusal — a wrong password, an address that is not a server — is
     * returned at once, because retrying those changes nothing.
     */
    private suspend fun connectPatiently(): AuthResult {
        var attempt = 0
        while (true) {
            val outcome = sessions.connect(settings.appBaseUrl)
            val unreachable = outcome is AuthResult.Failed && outcome.failure == AuthFailure.Unreachable
            if (!unreachable || attempt >= CONNECT_ATTEMPTS) return outcome
            attempt++
            delay(CONNECT_RETRY_MILLIS)
        }
    }

    /** One turn of the engine: send what is queued, then read what changed. */
    suspend fun sync(): SyncCycleResult = syncCycle.runSyncCycle()

    /**
     * Turns the engine until it reports that the two sides agree.
     *
     * A network that has just come back is not usable the instant the platform
     * says so — a route may still be settling — so a turn that reports itself
     * offline is retried rather than failed on.
     */
    suspend fun syncUntilCurrent() {
        withTimeout(SYNC_TIMEOUT_MILLIS) {
            while (true) {
                when (val outcome = sync()) {
                    is SyncCycleResult.Synced -> return@withTimeout
                    is SyncCycleResult.Refused -> error("the server refused the sync cycle: ${outcome.cause}")
                    is SyncCycleResult.Offline -> delay(SYNC_RETRY_MILLIS)
                }
            }
        }
    }

    /** How many writes are still queued for the server. */
    suspend fun queueDepth(): Int = database.outbox().count()

    /** The writes the server refused, kept for the user to look at. */
    suspend fun setAside(): List<QuarantinedOpRow> = unsentChanges.setAsideNow()

    private companion object {
        const val BOOT_TIMEOUT_MILLIS: Long = 30_000
        const val SYNC_TIMEOUT_MILLIS: Long = 120_000
        const val SYNC_RETRY_MILLIS: Long = 1_000
        const val CONNECT_ATTEMPTS: Int = 30
        const val CONNECT_RETRY_MILLIS: Long = 1_000
    }
}

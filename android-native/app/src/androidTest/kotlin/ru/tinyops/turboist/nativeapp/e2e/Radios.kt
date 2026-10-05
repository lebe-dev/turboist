package ru.tinyops.turboist.nativeapp.e2e

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.ParcelFileDescriptor
import android.os.SystemClock
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.rules.TestRule
import org.junit.runner.Description
import org.junit.runners.model.Statement

/**
 * Takes the device's network away, and gives it back whatever the test did.
 *
 * Offline behaviour cannot honestly be tested by pointing the app at a dead
 * address or by swapping in a stand-in that throws: what is being checked is how
 * the app behaves when the platform reports no network, including the parts —
 * the connectivity watcher, the trigger that fires when a network returns — that
 * exist only to react to that report. So the radios really are switched off.
 *
 * Restoring them is a rule rather than a line at the end of a test because a
 * failed assertion would otherwise leave the emulator with no network, and every
 * test after it would fail for a reason that has nothing to do with it. The
 * radios come back on the way in as well as the way out, so a run started after
 * an interrupted one begins from a known state.
 */
class Radios : TestRule {
    private val context: Context
        get() = InstrumentationRegistry.getInstrumentation().targetContext

    override fun apply(
        base: Statement,
        description: Description,
    ): Statement =
        object : Statement() {
            override fun evaluate() {
                restore()
                try {
                    base.evaluate()
                } finally {
                    restore()
                }
            }
        }

    /** Switches every radio off and waits until the platform agrees there is no network. */
    fun cut() {
        shell("svc wifi disable")
        shell("svc data disable")
        awaitConnectivity(expected = false, timeoutMillis = CUT_TIMEOUT_MILLIS)
    }

    /** Switches them back on and waits until the platform reports a usable network. */
    fun restore() {
        shell("svc wifi enable")
        shell("svc data enable")
        awaitConnectivity(expected = true, timeoutMillis = RESTORE_TIMEOUT_MILLIS)
    }

    private fun awaitConnectivity(
        expected: Boolean,
        timeoutMillis: Long,
    ) {
        val deadline = SystemClock.uptimeMillis() + timeoutMillis
        while (SystemClock.uptimeMillis() < deadline) {
            if (hasNetwork() == expected) return
            Thread.sleep(POLL_MILLIS)
        }
        val wanted = if (expected) "a usable network" else "no network at all"
        error("the device still did not report $wanted ${timeoutMillis}ms after the radios were switched")
    }

    private fun hasNetwork(): Boolean {
        val manager = context.getSystemService(ConnectivityManager::class.java) ?: return false
        val active = manager.activeNetwork ?: return false
        val capabilities = manager.getNetworkCapabilities(active) ?: return false
        return capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
    }

    /**
     * Runs one command as the shell user.
     *
     * The stream has to be read to the end and closed: the command is not
     * finished until it is, and a switch left half-applied would make the wait
     * that follows time out for the wrong reason.
     */
    private fun shell(command: String): String {
        val descriptor = InstrumentationRegistry.getInstrumentation().uiAutomation.executeShellCommand(command)
        return ParcelFileDescriptor.AutoCloseInputStream(descriptor).use { it.readBytes().decodeToString() }
    }

    private companion object {
        const val CUT_TIMEOUT_MILLIS: Long = 20_000
        const val RESTORE_TIMEOUT_MILLIS: Long = 60_000
        const val POLL_MILLIS: Long = 250
    }
}

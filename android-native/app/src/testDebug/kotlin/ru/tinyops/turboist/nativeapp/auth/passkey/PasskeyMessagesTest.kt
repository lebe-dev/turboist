package ru.tinyops.turboist.nativeapp.auth.passkey

import android.app.Application
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.nativeapp.auth.passkey.ui.messageRes
import kotlin.test.assertTrue

/**
 * Every way a passkey can fail has a sentence, and every sentence is a resource.
 *
 * The mapping is the last step before the user, so a way of failing that reaches
 * it without wording of its own is a screen that either says nothing or crashes
 * looking for a string that was never written. The wording itself comes half
 * from the shared translations and half from the device-specific set, and this
 * resolves both the same way the screen does.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], application = Application::class)
class PasskeyMessagesTest {
    private fun text(problem: PasskeyProblem): String =
        RuntimeEnvironment.getApplication().getString(problem.messageRes())

    @Test
    fun `every way a ceremony can fail resolves to something to say`() {
        for (problem in PasskeyProblem.entries) {
            assertTrue(text(problem).isNotBlank(), "$problem has no wording")
        }
    }

    @Test
    fun `the device-side refusals all name the password as the way in`() {
        // A user told only that something failed taps the same button again. The
        // password is always available — a passkey is an additional method, never
        // the only one — so every device-side refusal says so.
        val deviceSide =
            listOf(
                PasskeyProblem.Cancelled,
                PasskeyProblem.NotEnrolled,
                PasskeyProblem.Untrusted,
                PasskeyProblem.Unsupported,
                PasskeyProblem.Refused,
            )
        for (problem in deviceSide) {
            assertTrue(
                text(problem).contains("password", ignoreCase = true),
                "$problem does not point the user at the password",
            )
        }
    }
}

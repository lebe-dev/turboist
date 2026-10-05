package ru.tinyops.turboist.nativeapp.auth.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyAuthenticator
import ru.tinyops.turboist.nativeapp.auth.passkey.PasskeyProblem
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertNotNull

/**
 * What the sign-in screen shows about passkeys, and what it never stops showing.
 *
 * The offer is not a decision this screen makes: an instance that never brought
 * WebAuthn up does not route the ceremony at all, so a button for it would open
 * a sheet that can only end in an error. It is drawn when the server says it has
 * something to offer, and left out otherwise.
 *
 * The password form is the constant. It is still there while a ceremony is
 * running and still there after one failed, because it is the recovery path for
 * every case a passkey cannot cover — a new phone, a revoked credential, a
 * server whose association files are not in place yet.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class SignInPasskeyOfferTest {
    @get:Rule
    val compose = createComposeRule()

    private var offered: PasskeyAuthenticator? = null

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun show(
        state: AuthUiState = AuthUiState(),
        passkeyOffered: Boolean,
    ) {
        compose.setContent {
            TurboistTheme {
                SignInScreen(
                    state = state,
                    passkeyOffered = passkeyOffered,
                    onSignIn = { _, _ -> },
                    onPasskeySignIn = { offered = it },
                    onVerifyOtp = {},
                    onCancelOtp = {},
                    onUseRecoveryCode = {},
                    onEdit = {},
                    onChangeServer = {},
                )
            }
        }
    }

    @Test
    fun `a server with nothing to offer gets no passkey button`() {
        show(passkeyOffered = false)

        compose.onNodeWithText(text(R.string.auth_password)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.auth_passkeySignIn)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.auth_passkeyOr)).assertDoesNotExist()
    }

    @Test
    fun `a server that offers one draws the button, and it hands over an authenticator`() {
        show(passkeyOffered = true)

        compose.onNodeWithText(text(R.string.auth_passkeySignIn)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.auth_passkeySignIn)).performClick()

        // The screen owns the ceremony's authenticator: the platform sheet is
        // drawn over this activity, so nothing further up can supply it.
        assertNotNull(offered)
    }

    @Test
    fun `while the device is deciding, the button says so and the password form stays`() {
        show(state = AuthUiState(busy = true, awaitingDevice = true), passkeyOffered = true)

        compose.onNodeWithText(text(R.string.auth_passkeySigningIn)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.auth_passkeySigningIn)).assertIsNotEnabled()
        compose.onNodeWithText(text(R.string.auth_username)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.auth_password)).assertIsDisplayed()
    }

    @Test
    fun `a failed ceremony is explained next to the button, and never replaces the form`() {
        show(state = AuthUiState(passkeyProblem = PasskeyProblem.NotEnrolled), passkeyOffered = true)

        compose.onNodeWithText(text(R.string.native_passkey_not_enrolled)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.auth_username)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.auth_password)).assertIsDisplayed()
    }

    @Test
    fun `the second factor step belongs to the password path only`() {
        // An assertion is already two factors, so a passkey never leads here; the
        // step exists for a password sign-in, and the passkey offer is gone while
        // it is up.
        show(state = AuthUiState(awaitingOtp = true), passkeyOffered = true)

        compose.onNodeWithText(text(R.string.auth_passkeySignIn)).assertDoesNotExist()
    }
}

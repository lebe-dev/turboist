package ru.tinyops.turboist.nativeapp.account.ui

import android.app.Application
import androidx.compose.runtime.Composable
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasNoClickAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isDialog
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.network.dto.ActiveSessionDto
import ru.tinyops.turboist.core.network.dto.ApiTokenDto
import ru.tinyops.turboist.core.network.dto.CreatedApiTokenDto
import ru.tinyops.turboist.core.network.dto.TotpEnrolmentDto
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.account.AccountProblem
import ru.tinyops.turboist.nativeapp.account.ApiTokensUiState
import ru.tinyops.turboist.nativeapp.account.SessionsConfirmation
import ru.tinyops.turboist.nativeapp.account.SessionsUiState
import ru.tinyops.turboist.nativeapp.account.TwoFactorStep
import ru.tinyops.turboist.nativeapp.account.TwoFactorUiState
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals

/**
 * The three account screens as they are actually drawn.
 *
 * Their behaviour is checked without Compose elsewhere; what is left here is
 * what only the drawing can get wrong.
 *
 * All three admit the same thing in the same words when the server cannot be
 * reached — they keep nothing on the device, so there is genuinely nothing to
 * show — and an empty list is never allowed to stand in for that admission.
 *
 * The secrets are shown where the wording that governs them is: the warning that
 * a token is readable only once sits with the token, and the same for the
 * recovery codes. A screen that showed either without its sentence would be a
 * screen that quietly lost the user a credential.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class AccountScreensTest {
    @get:Rule
    val compose = createComposeRule()

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun show(content: @Composable () -> Unit) {
        compose.setContent { TurboistTheme(dynamicColor = false) { content() } }
        compose.waitForIdle()
    }

    @Test
    fun `an unreachable session list says so instead of showing an empty account`() {
        show {
            SessionsContent(
                state = SessionsUiState(loading = false, unreachable = true, problem = AccountProblem.OFFLINE),
                onAsk = {},
                onCancel = {},
                onConfirm = {},
                onRetry = {},
            )
        }

        compose.onNodeWithText(text(R.string.native_account_needsConnection)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.offline_retry)).assertIsDisplayed()
        // "No active sessions" here would say nothing can reach the account,
        // which nobody has established.
        compose.onNodeWithText(text(R.string.settings_sessions_empty)).assertDoesNotExist()
    }

    @Test
    fun `an unreachable token list says so instead of showing an account with no tokens`() {
        show {
            ApiTokensContent(
                state = ApiTokensUiState(loading = false, unreachable = true, problem = AccountProblem.OFFLINE),
                callbacks = ApiTokenCallbacks(),
            )
        }

        compose.onNodeWithText(text(R.string.native_account_needsConnection)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.settings_api_empty)).assertDoesNotExist()
        // The form goes with the list: minting a token against a server that
        // cannot be reached is a request that fails.
        compose.onNodeWithText(text(R.string.settings_api_generate)).assertDoesNotExist()
    }

    @Test
    fun `an unreadable second-factor state says so instead of claiming it is off`() {
        show {
            TwoFactorContent(
                state = TwoFactorUiState(loading = false, unreachable = true, problem = AccountProblem.OFFLINE),
                callbacks = TwoFactorCallbacks(),
            )
        }

        compose.onNodeWithText(text(R.string.native_account_needsConnection)).assertIsDisplayed()
        // Offering to enrol would invite the user to add a factor they may
        // already have, on an account nobody could check.
        compose.onNodeWithText(text(R.string.settings_twofa_enableButton)).assertDoesNotExist()
    }

    @Test
    fun `a deployment without a second factor says why and offers nothing`() {
        show {
            TwoFactorContent(
                state = TwoFactorUiState(loading = false, available = false),
                callbacks = TwoFactorCallbacks(),
            )
        }

        compose.onNodeWithText(text(R.string.settings_twofa_unavailable)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.settings_twofa_enableButton)).assertDoesNotExist()
        // Nothing to retry either: no tap brings a route back that was never
        // registered.
        compose.onNodeWithText(text(R.string.offline_retry)).assertDoesNotExist()
    }

    @Test
    fun `the session this request is made with cannot be revoked from its own row`() {
        show {
            SessionsContent(
                state =
                    SessionsUiState(
                        loading = false,
                        sessions = listOf(sessionOf(1, current = true)),
                    ),
                onAsk = {},
                onCancel = {},
                onConfirm = {},
                onRetry = {},
            )
        }

        compose.onNodeWithText(text(R.string.settings_sessions_current)).assertIsDisplayed()
        // Ending it from the row would sign this device out sideways, without
        // the question the sign-out below asks about the work still queued.
        compose.onNodeWithText(text(R.string.settings_sessions_revoke)).assertDoesNotExist()
    }

    @Test
    fun `signing out everywhere says how much unsent work goes with it`() {
        var confirmed = 0
        show {
            SessionsContent(
                state =
                    SessionsUiState(
                        loading = false,
                        sessions = listOf(sessionOf(1, current = true)),
                        confirming = SessionsConfirmation.SignOutEverywhere,
                        unsentChangeCount = 3,
                    ),
                onAsk = {},
                onCancel = {},
                onConfirm = { confirmed++ },
                onRetry = {},
            )
        }

        compose.onNodeWithText(text(R.string.settings_sessions_confirmLogoutAllTitle)).assertIsDisplayed()
        compose.onNodeWithText(unsentText(3), substring = true).assertIsDisplayed()

        // The dialog's own button, not the one on the screen behind it — they
        // carry the same wording, and only one of them is the confirmation.
        compose.onNode(
            hasText(text(R.string.settings_session_logoutAll)) and hasAnyAncestor(isDialog()),
        ).performClick()
        compose.waitForIdle()
        assertEquals(1, confirmed)
    }

    @Test
    fun `a freshly minted token is shown with the warning that it is the only time`() {
        show {
            ApiTokensContent(
                state =
                    ApiTokensUiState(
                        loading = false,
                        tokens =
                            listOf(
                                ApiTokenDto(
                                    id = 1,
                                    name = "n8n",
                                    scopes = listOf("tasks:read"),
                                    createdAt = "2026-08-18T10:00:00.000Z",
                                ),
                            ),
                        created =
                            CreatedApiTokenDto(
                                id = 1,
                                name = "n8n",
                                scopes = listOf("tasks:read"),
                                token = "secret-value",
                                createdAt = "2026-08-18T10:00:00.000Z",
                            ),
                    ),
                callbacks = ApiTokenCallbacks(),
            )
        }

        compose.onNodeWithText(text(R.string.settings_api_warningOnce)).assertIsDisplayed()
        compose.onNodeWithText("secret-value").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.settings_api_copy)).assertIsDisplayed()
    }

    @Test
    fun `a token granted everything reads as that rather than as a list of scopes`() {
        show {
            ApiTokensContent(
                state =
                    ApiTokensUiState(
                        loading = false,
                        tokens =
                            listOf(
                                ApiTokenDto(
                                    id = 1,
                                    name = "admin",
                                    scopes = listOf("*"),
                                    createdAt = "2026-08-18T10:00:00.000Z",
                                ),
                            ),
                    ),
                callbacks = ApiTokenCallbacks(),
            )
        }

        // The badge on the row, not the preset button above it: the two share
        // their wording, and only one of them describes the stored token.
        compose.onNode(
            hasText(text(R.string.settings_api_scopes_fullBadge)) and hasNoClickAction(),
        ).performScrollTo().assertIsDisplayed()
        // And the wildcard is never shown as the symbol it is on the wire.
        compose.onNodeWithText("*").assertDoesNotExist()
    }

    @Test
    fun `the recovery codes come with the sentence saying they will not be shown again`() {
        show {
            TwoFactorContent(
                state =
                    TwoFactorUiState(
                        loading = false,
                        enabled = true,
                        step = TwoFactorStep.RECOVERY,
                        recoveryCodes = listOf("AAAA1111", "BBBB2222"),
                    ),
                callbacks = TwoFactorCallbacks(),
            )
        }

        compose.onNodeWithText(text(R.string.settings_twofa_recoveryHint)).assertIsDisplayed()
        compose.onNodeWithText("AAAA1111").assertIsDisplayed()
        compose.onNodeWithText("BBBB2222").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.settings_twofa_finishRecovery)).assertIsDisplayed()
    }

    @Test
    fun `an enrolment shows the secret in a form that can be typed as well as scanned`() {
        show {
            TwoFactorContent(
                state =
                    TwoFactorUiState(
                        loading = false,
                        step = TwoFactorStep.ENROLLING,
                        enrolment = TotpEnrolmentDto(secret = "JBSWY3DPEHPK3PXP"),
                    ),
                callbacks = TwoFactorCallbacks(),
            )
        }

        compose.onNodeWithText(text(R.string.settings_twofa_setupHint)).assertIsDisplayed()
        // A server that sent no picture, or one this device cannot decode, still
        // leaves an enrolment that can be completed.
        compose.onNodeWithText("JBSWY3DPEHPK3PXP").assertIsDisplayed()
    }

    private fun sessionOf(
        id: Long,
        current: Boolean = false,
    ) = ActiveSessionDto(
        id = id,
        clientKind = "android",
        displayName = "Device $id",
        createdAt = "2026-08-18T10:00:00.000Z",
        lastUsedAt = "2026-08-20T09:12:00.000Z",
        isCurrent = current,
    )

    private fun unsentText(count: Int): String =
        RuntimeEnvironment.getApplication().resources
            .getQuantityString(R.plurals.native_settings_confirmUnsent, count, count)
}

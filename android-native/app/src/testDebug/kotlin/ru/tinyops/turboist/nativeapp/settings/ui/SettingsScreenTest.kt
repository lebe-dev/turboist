package ru.tinyops.turboist.nativeapp.settings.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
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
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.model.UserSettings
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.DeviceOptions
import ru.tinyops.turboist.nativeapp.settings.SettingsConfirmation
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals

/**
 * The settings screen as it is actually drawn.
 *
 * The presenter is checked on its own elsewhere, so what is left here is what
 * the screen is for: that the three stores it sits over read as three different
 * things, that the two rule lists say which of them acts and which only offers,
 * and that the actions which empty the device ask first with the number at
 * stake rather than after the fact.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class SettingsScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `the two rule lists say which one acts and which one only offers`() {
        show(
            SettingsUiState(
                loading = false,
                app =
                    AppSettings(
                        autoLabels = listOf(AutoLabelRule("buy", listOf(3))),
                        projectSuggestions = listOf(ProjectSuggestionRule("deploy", listOf(7))),
                    ),
            ),
        )

        compose.onNodeWithText(text(R.string.native_settings_ruleAppliedAutomatically))
            .performScrollTo()
            .assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_settings_ruleOnlySuggested))
            .performScrollTo()
            .assertIsDisplayed()
    }

    @Test
    fun `the choices that stay on this phone say so, so they are not mistaken for account settings`() {
        show(SettingsUiState(loading = false))

        compose.onNodeWithText(text(R.string.native_settings_deviceDescription))
            .performScrollTo()
            .assertIsDisplayed()
    }

    @Test
    fun `changing the server asks first, and says how much work would be lost`() {
        var confirmed = 0
        show(
            state =
                SettingsUiState(
                    loading = false,
                    confirming = SettingsConfirmation.CHANGE_SERVER,
                    unsentChangeCount = 3,
                ),
            callbacks = SettingsCallbacks(onConfirm = { confirmed++ }),
        )

        compose.onNodeWithText(text(R.string.native_settings_confirmChangeServerTitle)).assertIsDisplayed()
        compose.onNodeWithText(unsentText(3), substring = true).assertIsDisplayed()
        assertEquals(0, confirmed, "the question alone must not disconnect anything")

        compose.onNodeWithText(text(R.string.native_settings_confirmChangeServerAction)).performClick()

        assertEquals(1, confirmed)
    }

    @Test
    fun `clearing the copy on this device is a different question from changing servers`() {
        show(
            state =
                SettingsUiState(loading = false, confirming = SettingsConfirmation.CLEAR_LOCAL_DATA),
            callbacks = SettingsCallbacks(),
        )

        compose.onNodeWithText(text(R.string.native_settings_confirmClearLocalDataTitle)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_settings_confirmClearLocalDataBody), substring = true)
            .assertIsDisplayed()
    }

    @Test
    fun `a preference switch is toggled by the whole row, and reports the value it would become`() {
        val flipped = mutableListOf<Boolean>()
        show(
            state = SettingsUiState(loading = false, user = UserSettings(troikiEnabled = false)),
            callbacks = SettingsCallbacks(onSetTroikiEnabled = { flipped += it }),
        )

        compose.onNodeWithText(text(R.string.settings_troiki_toggle)).performScrollTo().performClick()

        assertEquals(listOf(true), flipped)
    }

    @Test
    fun `the build says which version it is, so a report about it identifies the code`() {
        show(SettingsUiState(loading = false, version = "1.2.3+abc1234"))

        compose.onNodeWithText("1.2.3+abc1234").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the legal documents are not offered before there is a server to open them on`() {
        // They are the connected installation's own documents rather than a copy
        // shipped in the package, so a device that has never connected has none
        // to show — and a link with nowhere to go is worse than no link.
        show(SettingsUiState(loading = false))

        compose.onNodeWithText(text(R.string.legal_tos_title)).assertDoesNotExist()
    }

    @Test
    fun `the legal documents appear once the device is pointed at a server`() {
        show(SettingsUiState(loading = false, serverAddress = "https://turboist.example/"))

        compose.onNodeWithText(text(R.string.legal_tos_title)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.legal_privacy_title)).performScrollTo().assertIsDisplayed()
    }

    private fun show(
        state: SettingsUiState,
        callbacks: SettingsCallbacks = SettingsCallbacks(),
        device: DeviceOptions = DeviceOptions(),
    ) {
        compose.setContent {
            TurboistTheme {
                SettingsScreen(
                    state = state.copy(device = device),
                    callbacks = callbacks,
                    destinations = SettingsDestinations(),
                )
            }
        }
        compose.waitForIdle()
    }

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)

    private fun unsentText(count: Int): String =
        RuntimeEnvironment.getApplication().resources
            .getQuantityString(R.plurals.native_settings_confirmUnsent, count, count)
}

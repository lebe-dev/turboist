package ru.tinyops.turboist.nativeapp.settings.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.flow.flowOf
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
import ru.tinyops.turboist.nativeapp.settings.DeletedRule
import ru.tinyops.turboist.nativeapp.settings.SettingsConfirmation
import ru.tinyops.turboist.nativeapp.settings.SettingsMessage
import ru.tinyops.turboist.nativeapp.settings.SettingsSection
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals

/**
 * The settings screens as they are actually drawn.
 *
 * The presenter is checked on its own elsewhere, so what is left here is what
 * the screens are for: that the top level is a map of sections rather than a
 * wall of controls, that the stores underneath read as different things, that
 * the two rule lists say which of them acts and which only offers, and that the
 * actions which empty the device ask first with the number at stake.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class SettingsScreenTest {
    @get:Rule
    val compose = createComposeRule()

    // --- the top level ---------------------------------------------------------

    @Test
    fun `the top level leads to every section`() {
        val opened = mutableListOf<SettingsSection>()
        showRoot(destinations = SettingsDestinations(openSection = { opened += it }))

        for (section in SettingsSection.entries) {
            compose.onNode(hasText(text(section.titleRes)) and hasClickAction()).performScrollTo().performClick()
        }

        assertEquals(SettingsSection.entries.toList(), opened)
    }

    @Test
    fun `changes the server refused are announced at the top, and lead to the pile`() {
        var opened = 0
        showRoot(
            state = SettingsUiState(loading = false, refusedChangeCount = 3),
            destinations = SettingsDestinations(openUnsentChanges = { opened++ }),
        )

        compose.onNodeWithText(refusedText(3)).assertIsDisplayed().performClick()

        assertEquals(1, opened)
    }

    @Test
    fun `nothing is announced while the server has refused nothing`() {
        showRoot(state = SettingsUiState(loading = false, refusedChangeCount = 0))

        compose.onNodeWithText(refusedText(1)).assertDoesNotExist()
    }

    @Test
    fun `a setting is found by name and the result opens the section it lives in`() {
        val opened = mutableListOf<SettingsSection>()
        showRoot(destinations = SettingsDestinations(openSection = { opened += it }))

        compose.onNode(hasSetTextAction()).performTextInput(text(R.string.settings_troiki_heading))
        compose
            .onNode(hasText(text(R.string.settings_troiki_heading)) and hasClickAction() and !hasSetTextAction())
            .performClick()

        assertEquals(listOf(SettingsSection.WORKSPACE), opened)
    }

    // --- the sections ------------------------------------------------------------

    @Test
    fun `the two rule lists say which one acts and which one only offers`() {
        showSection(
            SettingsSection.RULES,
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
    fun `a deleted rule can be taken back from the message that says it went`() {
        val restored = mutableListOf<DeletedRule>()
        val deleted = DeletedRule.AutoLabel(0, AutoLabelRule("buy", listOf(3)))
        showSection(
            SettingsSection.RULES,
            SettingsUiState(loading = false),
            callbacks = SettingsCallbacks(onRestoreRule = { restored += it }),
            messages = flowOf(SettingsMessage.RuleDeleted(deleted)),
        )

        compose.onNodeWithText(text(R.string.native_settings_ruleUndo)).performClick()
        compose.waitForIdle()

        assertEquals(listOf<DeletedRule>(deleted), restored)
    }

    @Test
    fun `the theme says it stays on this phone, unlike the language beside it`() {
        showSection(SettingsSection.GENERAL, SettingsUiState(loading = false))

        compose.onNodeWithText(text(R.string.native_settings_scopeDevice)).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the choices that stay on this phone say so, so they are not mistaken for account settings`() {
        showSection(SettingsSection.DEVICE, SettingsUiState(loading = false))

        compose.onNodeWithText(text(R.string.native_settings_deviceDescription))
            .performScrollTo()
            .assertIsDisplayed()
    }

    @Test
    fun `a pinning cap is stepped rather than typed`() {
        val typed = mutableListOf<String>()
        showSection(
            SettingsSection.WORKSPACE,
            SettingsUiState(loading = false, user = UserSettings(maxPinnedTasks = 5)),
            callbacks = SettingsCallbacks(onEditMaxPinnedTasks = { typed += it }),
        )

        compose.onNodeWithContentDescription(text(R.string.native_settings_pinnedTasksMore))
            .performScrollTo()
            .performClick()

        assertEquals(listOf("6"), typed)
    }

    @Test
    fun `hiding past events cannot be chosen while calendars are off`() {
        showSection(
            SettingsSection.WORKSPACE,
            SettingsUiState(loading = false, user = UserSettings(calendarEnabled = false)),
        )

        compose.onNode(hasText(text(R.string.settings_calendars_hidePastEvents)) and hasClickAction())
            .performScrollTo()
            .assertIsNotEnabled()
    }

    @Test
    fun `a preference switch is toggled by the whole row, and reports the value it would become`() {
        val flipped = mutableListOf<Boolean>()
        showSection(
            SettingsSection.WORKSPACE,
            SettingsUiState(loading = false, user = UserSettings(troikiEnabled = false)),
            callbacks = SettingsCallbacks(onSetTroikiEnabled = { flipped += it }),
        )

        compose.onNodeWithText(text(R.string.settings_troiki_toggle)).performScrollTo().performClick()

        assertEquals(listOf(true), flipped)
    }

    @Test
    fun `changing the server asks first, and says how much work would be lost`() {
        var confirmed = 0
        showSection(
            SettingsSection.DEVICE,
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
        showSection(
            SettingsSection.DEVICE,
            SettingsUiState(loading = false, confirming = SettingsConfirmation.CLEAR_LOCAL_DATA),
        )

        compose.onNodeWithText(text(R.string.native_settings_confirmClearLocalDataTitle)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_settings_confirmClearLocalDataBody), substring = true)
            .assertIsDisplayed()
    }

    @Test
    fun `the build says which version it is, so a report about it identifies the code`() {
        showSection(SettingsSection.ABOUT, SettingsUiState(loading = false, version = "1.2.3+abc1234"))

        compose.onNodeWithText("1.2.3+abc1234").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the legal documents are not offered before there is a server to open them on`() {
        // They are the connected installation's own documents rather than a copy
        // shipped in the package, so a device that has never connected has none
        // to show — and a link with nowhere to go is worse than no link.
        showSection(SettingsSection.ABOUT, SettingsUiState(loading = false))

        compose.onNodeWithText(text(R.string.legal_tos_title)).assertDoesNotExist()
    }

    @Test
    fun `the legal documents appear once the device is pointed at a server`() {
        showSection(
            SettingsSection.ABOUT,
            SettingsUiState(loading = false, serverAddress = "https://turboist.example/"),
        )

        compose.onNodeWithText(text(R.string.legal_tos_title)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.legal_privacy_title)).performScrollTo().assertIsDisplayed()
    }

    private fun showRoot(
        state: SettingsUiState = SettingsUiState(loading = false),
        destinations: SettingsDestinations = SettingsDestinations(),
    ) {
        compose.setContent {
            TurboistTheme {
                SettingsScreen(state = state, destinations = destinations)
            }
        }
        compose.waitForIdle()
    }

    private fun showSection(
        section: SettingsSection,
        state: SettingsUiState,
        callbacks: SettingsCallbacks = SettingsCallbacks(),
        messages: Flow<SettingsMessage> = emptyFlow(),
    ) {
        compose.setContent {
            TurboistTheme {
                SettingsSectionScreen(
                    section = section,
                    state = state,
                    callbacks = callbacks,
                    destinations = SettingsDestinations(),
                    messages = messages,
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

    private fun refusedText(count: Int): String =
        RuntimeEnvironment.getApplication().resources
            .getQuantityString(R.plurals.native_settings_refusedBanner, count, count)
}

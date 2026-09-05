package ru.tinyops.turboist.nativeapp.settings

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.DEFAULT_MAX_PINNED
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.model.UserSettings
import ru.tinyops.turboist.core.model.normalizeMaxPinned
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * What the settings screen does with the three stores it sits over.
 *
 * All of it runs without Compose, without a database and without a server: what
 * a settings edit *is* — which document it belongs to, and how little of that
 * document it says — is a decision about values.
 *
 * Three claims are worth naming, because each of them is a bug that would only
 * be noticed much later:
 *
 * - a change carries **only what changed**. A full snapshot would hand the server
 *   this build's idea of every preference, and a key it has never heard of would
 *   come back absent — that is, destroyed;
 * - the user's own preferences and the installation's rules **never reach each
 *   other's store**. They are different documents with different owners, and one
 *   written into the other is silent, permanent damage;
 * - a pinning cap out of range is **refused** rather than corrected. A stored one
 *   is a different matter and falls back to the default, because a blob written
 *   before the setting existed is a gap rather than a mistake.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class SettingsPresenterTest {
    private val userSettings = MutableStateFlow(UserSettings())
    private val appSettings = MutableStateFlow(AppSettings())
    private val labels = MutableStateFlow(listOf(label(3, 30, "chore"), label(4, 40, "bug")))
    private val projects = MutableStateFlow(listOf(project(7, 70, "Website")))
    private val device = MutableStateFlow(DeviceOptions())
    private val actions = RecordingSettingsActions()
    private val deviceActions = RecordingDeviceOptionActions()
    private val connection = RecordingServerConnection()

    private fun TestScope.presenter(): SettingsPresenter {
        val presenter =
            SettingsPresenter(
                scope = backgroundScope,
                userSettings = userSettings,
                appSettings = appSettings,
                labels = labels,
                projects = projects,
                deviceOptions = device,
                actions = actions,
                device = deviceActions,
                connection = connection,
                release = AppRelease { "1.2.3+abc1234" },
            )
        backgroundScope.launch { presenter.state.collect {} }
        return presenter
    }

    // --- what a change says --------------------------------------------------

    @Test
    fun `a change carries the one key it changed and nothing else`() =
        runTest {
            val presenter = presenter()

            presenter.setLocale("ru")
            runCurrent()

            assertEquals("""{"locale":"ru"}""", encoded(actions.userPatches.single()))
        }

    @Test
    fun `a switch that is genuinely off is still written, because absent means untouched`() =
        runTest {
            userSettings.value = UserSettings(troikiEnabled = true)
            val presenter = presenter()
            runCurrent()

            presenter.setTroikiEnabled(false)
            runCurrent()

            assertEquals("""{"troikiEnabled":false}""", encoded(actions.userPatches.single()))
        }

    @Test
    fun `letting the banner run all day is said out loud, because it is not the none phase`() =
        runTest {
            val presenter = presenter()

            presenter.setBannerDayPart(null)
            presenter.setBannerDayPart(DayPart.MORNING)
            runCurrent()

            assertEquals("""{"bannerDayPart":""}""", encoded(actions.userPatches.first()))
            assertEquals("""{"bannerDayPart":"morning"}""", encoded(actions.userPatches.last()))
        }

    @Test
    fun `putting a label on a preference list sends the whole list and only that key`() =
        runTest {
            userSettings.value = UserSettings(weeklyUnplannedExcludedLabelIds = listOf(30))
            val presenter = presenter()
            runCurrent()

            presenter.toggleWeeklyExcludedLabel(40)
            runCurrent()

            assertEquals(
                """{"weeklyUnplannedExcludedLabelIds":[30,40]}""",
                encoded(actions.userPatches.single()),
            )
        }

    @Test
    fun `the banner text is written when it is saved rather than on every keystroke`() =
        runTest {
            val presenter = presenter()

            presenter.editBannerText("Standup at ten")
            runCurrent()
            assertTrue(actions.userPatches.isEmpty(), "typing is not a decision")

            presenter.saveBannerText()
            runCurrent()

            assertEquals("""{"bannerText":"Standup at ten"}""", encoded(actions.userPatches.single()))
        }

    // --- the two documents stay apart ---------------------------------------

    @Test
    fun `changing the user's own preferences never touches the installation's rules`() =
        runTest {
            val presenter = presenter()

            presenter.setPublicView(true)
            presenter.setLocale("en")
            presenter.setCalendarEnabled(true)
            runCurrent()

            assertEquals(3, actions.userPatches.size)
            assertTrue(
                actions.autoLabelWrites.isEmpty() && actions.suggestionWrites.isEmpty(),
                "a preference write reached the installation's rules",
            )
        }

    @Test
    fun `changing an installation rule never touches the user's own preferences`() =
        runTest {
            val presenter = presenter()

            presenter.startNewRule(RuleKind.AUTO_LABEL)
            presenter.setRuleMask("buy")
            presenter.toggleRuleTarget(30)
            presenter.saveRule()
            runCurrent()

            assertEquals(listOf(AutoLabelRule("buy", listOf(30))), actions.autoLabelWrites.single())
            assertTrue(actions.userPatches.isEmpty(), "a rule write reached the user's own preferences")
        }

    @Test
    fun `the two rule lists have separate write paths, so one is never replaced by the other`() =
        runTest {
            appSettings.value =
                AppSettings(
                    autoLabels = listOf(AutoLabelRule("buy", listOf(30))),
                    projectSuggestions = listOf(ProjectSuggestionRule("deploy", listOf(70))),
                )
            val presenter = presenter()
            runCurrent()

            presenter.startNewRule(RuleKind.PROJECT_SUGGESTION)
            presenter.setRuleMask("ship")
            presenter.toggleRuleTarget(70)
            presenter.saveRule()
            runCurrent()

            assertContentEquals(
                listOf(ProjectSuggestionRule("deploy", listOf(70)), ProjectSuggestionRule("ship", listOf(70))),
                actions.suggestionWrites.single(),
            )
            assertTrue(actions.autoLabelWrites.isEmpty(), "the labelling rules were rewritten by a suggestion edit")
        }

    @Test
    fun `editing a rule replaces it in place rather than adding a second one`() =
        runTest {
            appSettings.value =
                AppSettings(autoLabels = listOf(AutoLabelRule("buy", listOf(30)), AutoLabelRule("call", listOf(40))))
            val presenter = presenter()
            runCurrent()

            presenter.editRule(RuleKind.AUTO_LABEL, 1)
            presenter.setRuleMask("phone")
            presenter.saveRule()
            runCurrent()

            assertContentEquals(
                listOf(AutoLabelRule("buy", listOf(30)), AutoLabelRule("phone", listOf(40))),
                actions.autoLabelWrites.single(),
            )
        }

    @Test
    fun `a rule with no mask or nothing to apply is not written`() =
        runTest {
            val presenter = presenter()
            val said = mutableListOf<SettingsMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }

            presenter.startNewRule(RuleKind.AUTO_LABEL)
            presenter.setRuleMask("   ")
            presenter.saveRule()
            runCurrent()

            assertTrue(actions.autoLabelWrites.isEmpty())
            assertEquals(listOf<SettingsMessage>(SettingsMessage.RuleIncomplete), said)
            assertNotNull(presenter.state.value.editor, "the editor stays open on what the user was writing")
        }

    // --- the pinning caps ----------------------------------------------------

    @Test
    fun `a stored cap out of range reads as the default rather than the nearest bound`() {
        // A blob written before the caps existed is a gap to fill in, not a wish
        // for "one" — which is why this is a fallback and not a clamp.
        assertEquals(DEFAULT_MAX_PINNED, normalizeMaxPinned(0))
        assertEquals(DEFAULT_MAX_PINNED, normalizeMaxPinned(51))
        assertEquals(DEFAULT_MAX_PINNED, normalizeMaxPinned(null))
        assertEquals(1, normalizeMaxPinned(1))
        assertEquals(50, normalizeMaxPinned(50))
    }

    @Test
    fun `a cap the user typed out of range is refused rather than silently corrected`() =
        runTest {
            val presenter = presenter()
            val said = mutableListOf<SettingsMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }

            presenter.editMaxPinnedTasks("0")
            presenter.savePinnedCaps()
            runCurrent()

            assertTrue(actions.userPatches.isEmpty(), "nothing is written when a typed value is out of range")
            assertEquals(listOf<SettingsMessage>(SettingsMessage.PinnedCapOutOfRange), said)
            assertEquals("0", presenter.state.value.maxPinnedTasks, "what was typed stays on screen to be corrected")
        }

    @Test
    fun `both caps are sent together, because the field the user did not touch still has a value`() =
        runTest {
            userSettings.value = UserSettings(maxPinnedTasks = 10, maxPinnedProjects = 4)
            val presenter = presenter()
            runCurrent()

            presenter.editMaxPinnedTasks("25")
            presenter.savePinnedCaps()
            runCurrent()

            assertEquals(
                """{"maxPinnedTasks":25,"maxPinnedProjects":4}""",
                encoded(actions.userPatches.single()),
            )
        }

    @Test
    fun `a saved cap lets go of its draft, so a change made elsewhere is shown`() =
        runTest {
            val presenter = presenter()

            presenter.editMaxPinnedTasks("25")
            presenter.savePinnedCaps()
            runCurrent()
            userSettings.value = UserSettings(maxPinnedTasks = 25)
            runCurrent()

            assertEquals("25", presenter.state.value.maxPinnedTasks)
            userSettings.value = UserSettings(maxPinnedTasks = 12)
            runCurrent()
            assertEquals("12", presenter.state.value.maxPinnedTasks, "the field follows the document again")
        }

    @Test
    fun `a refused write leaves what the user typed where they can still save it`() =
        runTest {
            actions.refuse = true
            val presenter = presenter()
            val said = mutableListOf<SettingsMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }

            presenter.editBannerText("Standup at ten")
            presenter.saveBannerText()
            runCurrent()

            assertEquals(listOf<SettingsMessage>(SettingsMessage.SaveFailed), said)
            assertEquals("Standup at ten", presenter.state.value.bannerText)
        }

    // --- what only names a server id ----------------------------------------

    @Test
    fun `a label the server has never seen is not offered, because these lists are server ids`() =
        runTest {
            labels.value = labels.value + label(localId = 9, serverId = null, name = "written on the train")
            val presenter = presenter()
            runCurrent()

            assertContentEquals(
                listOf("chore", "bug"),
                presenter.state.value.nameableLabels.map { it.name },
            )
        }

    // --- this device ---------------------------------------------------------

    @Test
    fun `the device's own choices are stored on the device and never sent`() =
        runTest {
            val presenter = presenter()

            presenter.setTheme(ThemeChoice.DARK)
            presenter.setSyncOnMetered(false)
            runCurrent()

            assertEquals(ThemeChoice.DARK, deviceActions.theme)
            assertEquals(false, deviceActions.syncOnMetered)
            assertTrue(
                actions.userPatches.isEmpty() && actions.autoLabelWrites.isEmpty(),
                "a device choice was sent to the server",
            )
        }

    @Test
    fun `nothing destructive happens until it has been asked about, with the number at stake`() =
        runTest {
            connection.unsent = 3
            val presenter = presenter()

            presenter.ask(SettingsConfirmation.CHANGE_SERVER)
            runCurrent()

            assertEquals(SettingsConfirmation.CHANGE_SERVER, presenter.state.value.confirming)
            assertEquals(3, presenter.state.value.unsentChangeCount)
            assertEquals(0, connection.forgotten, "the question alone must not disconnect anything")
        }

    @Test
    fun `answering no leaves the device connected and its copy intact`() =
        runTest {
            val presenter = presenter()

            presenter.ask(SettingsConfirmation.CHANGE_SERVER)
            runCurrent()
            presenter.cancelConfirmation()
            runCurrent()

            assertNull(presenter.state.value.confirming)
            assertEquals(0, connection.forgotten)
        }

    @Test
    fun `changing the server forgets it, which is what takes the app back to the address screen`() =
        runTest {
            val presenter = presenter()

            presenter.ask(SettingsConfirmation.CHANGE_SERVER)
            runCurrent()
            presenter.confirm()
            runCurrent()

            assertEquals(1, connection.forgotten)
            assertEquals(0, connection.cleared, "changing servers is not the same as repairing a copy")
            assertNull(presenter.state.value.confirming)
        }

    @Test
    fun `clearing the copy keeps the session, and says so once it is done`() =
        runTest {
            val presenter = presenter()
            val said = mutableListOf<SettingsMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }

            presenter.ask(SettingsConfirmation.CLEAR_LOCAL_DATA)
            runCurrent()
            presenter.confirm()
            runCurrent()

            assertEquals(1, connection.cleared)
            assertEquals(0, connection.forgotten, "repairing a copy must not sign the device out")
            assertEquals(listOf<SettingsMessage>(SettingsMessage.LocalDataCleared), said)
        }

    // --- what the screen says about itself ----------------------------------

    @Test
    fun `the build names itself and points the legal documents at the connected server`() =
        runTest {
            val presenter = presenter()
            runCurrent()

            val state = presenter.state.value
            assertEquals("1.2.3+abc1234", state.version)
            assertEquals("https://turboist.example/terms-of-service", state.termsUrl)
            assertEquals("https://turboist.example/privacy-policy", state.privacyUrl)
        }

    private fun encoded(edit: PatchUserSettingsRequest): String =
        TurboistJson.encodeToString(PatchUserSettingsRequest.serializer(), edit)

    private fun label(
        localId: Long,
        serverId: Long?,
        name: String,
    ): Label = Label(localId = localId, serverId = serverId, name = name, createdAt = 0, updatedAt = 0)

    private fun project(
        localId: Long,
        serverId: Long?,
        title: String,
    ): Project =
        Project(
            localId = localId,
            serverId = serverId,
            contextLocalId = 1,
            title = title,
            createdAt = 0,
            updatedAt = 0,
        )
}

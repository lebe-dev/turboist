package ru.tinyops.turboist.nativeapp.quickadd.ui

import android.app.Application
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.quickadd.FakeRecentProjects
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddPresenter
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddWorkspace
import ru.tinyops.turboist.nativeapp.quickadd.RecordingQuickAddActions
import ru.tinyops.turboist.nativeapp.quickadd.label
import ru.tinyops.turboist.nativeapp.quickadd.project
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals

/**
 * The capture sheet composed for real, driven by its own presenter.
 *
 * The replica behind it is a stand-in, because these cases are about the sheet:
 * that a line typed into it is written down where it says it will be, that the
 * labels a title earns are shown while they can still be refused, and that a
 * suggested project stays a suggestion until somebody taps it. Where the tasks
 * then go is settled by the write path, which has checks of its own.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class QuickAddSheetTest {
    @get:Rule
    val compose = createComposeRule()

    private val actions = RecordingQuickAddActions()
    private val recent = FakeRecentProjects(listOf(4L, 2L))
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private val workspace =
        MutableStateFlow(
            QuickAddWorkspace(
                projects =
                    listOf(
                        project(1, "Home"),
                        project(2, "Work"),
                        project(3, "Garden"),
                        project(4, "Music"),
                    ),
                labels = listOf(label(1, "errand")),
            ),
        )

    private lateinit var presenter: QuickAddPresenter

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)

    private fun launch() {
        compose.setContent {
            val scope = rememberCoroutineScope()
            val live =
                remember(scope) {
                    QuickAddPresenter(
                        scope = scope,
                        workspace = workspace,
                        recent = recent,
                        actions = actions,
                        zone = ZoneId.of("Europe/Berlin"),
                    ).also { it.open() }
                }
            presenter = live
            val state by live.state.collectAsState()
            TurboistTheme {
                QuickAddSheetContent(state = state, today = today, callbacks = quickAddCallbacks(live))
            }
        }
        compose.waitForIdle()
    }

    /** How many nodes on screen carry this wording. */
    private fun shown(resId: Int): Int = compose.onAllNodesWithText(text(resId)).fetchSemanticsNodes().size

    private fun type(title: String) {
        compose.onNodeWithContentDescription(text(R.string.dialog_quickAdd_titleAriaLabel)).performTextInput(title)
        compose.waitForIdle()
    }

    @Test
    fun `a line typed in and saved is written down in the inbox`() {
        launch()

        type("Renew the passport")
        compose.onNodeWithText(text(R.string.dialog_quickAdd_submitSingle)).performClick()
        compose.waitForIdle()

        assertEquals(1, actions.calls.size)
        assertEquals(TaskDestination.Inbox, actions.calls.single().destination)
        assertEquals("Renew the passport", actions.calls.single().task.title)
    }

    @Test
    fun `several lines are offered as several tasks`() {
        launch()

        type("Buy milk\nCall the dentist")

        compose.onNodeWithText(text(R.string.dialog_quickAdd_submitMulti, 2)).assertIsDisplayed()
    }

    @Test
    fun `the labels a title earns are shown while they can still be refused`() {
        workspace.value =
            workspace.value.copy(
                appSettings = AppSettings(autoLabels = listOf(AutoLabelRule("buy", listOf(1), ignoreCase = true))),
            )
        launch()

        type("Buy milk")
        compose.onNodeWithText(text(R.string.dialog_quickAdd_autoLabelHint)).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.dialog_quickAdd_rejectAutoLabel, "errand")).performClick()
        compose.waitForIdle()

        assertEquals(0, shown(R.string.dialog_quickAdd_autoLabelHint))
        assertEquals(listOf("errand"), presenter.state.value.draft.rejectedAutoLabels)
    }

    @Test
    fun `a suggested project stays a suggestion until it is tapped`() {
        workspace.value =
            workspace.value.copy(
                appSettings =
                    AppSettings(projectSuggestions = listOf(ProjectSuggestionRule("garden", listOf(3), true))),
            )
        launch()

        type("garden shed")
        compose.onNodeWithText(text(R.string.dialog_quickAdd_projectSuggestionHint)).assertIsDisplayed()
        assertEquals(null, presenter.state.value.projectTitle)

        compose.onAllNodesWithContentDescription("Garden").onFirst().performClick()
        compose.waitForIdle()

        assertEquals("Garden", presenter.state.value.projectTitle)
    }

    @Test
    fun `the picker leads with the inbox and the places this device files things`() {
        launch()

        compose.onNodeWithContentDescription(text(R.string.nav_inbox)).performClick()
        compose.waitForIdle()

        compose.onNodeWithText(text(R.string.native_quickAdd_recentProjects)).assertIsDisplayed()
        // The inbox leads the list, and the two remembered projects are lifted
        // out of it rather than repeated inside it.
        assertEquals(listOf("Music", "Work"), presenter.state.value.recentProjects.map { it.title })
        compose.onAllNodesWithContentDescription("Music").onFirst().performClick()
        compose.waitForIdle()

        assertEquals("Music", presenter.state.value.projectTitle)
    }

    /**
     * A date is a plan and the inbox holds what has not been planned, so the day
     * shortcuts appear only once the task has somewhere to live.
     */
    @Test
    fun `there is nothing to schedule while the task is headed for the inbox`() {
        launch()

        assertEquals(0, shown(R.string.common_today))

        compose.onNodeWithContentDescription(text(R.string.nav_inbox)).performClick()
        compose.waitForIdle()
        compose.onAllNodesWithContentDescription("Home").onFirst().performClick()
        compose.waitForIdle()

        compose.onNodeWithText(text(R.string.common_today)).assertIsDisplayed()
    }
}

package ru.tinyops.turboist.nativeapp.quickadd

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.nativeapp.labelsuggest.FakeLabelSuggestions
import ru.tinyops.turboist.nativeapp.labelsuggest.SuggestionSurface
import java.time.Instant
import java.time.ZoneId
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** Labels the on-device model offers while a title is typed. */
@OptIn(ExperimentalCoroutinesApi::class)
class QuickAddLabelSuggestionsTest {
    private val suggestions = FakeLabelSuggestions()

    private val workspace =
        MutableStateFlow(
            QuickAddWorkspace(
                projects = listOf(project(1, "Home")),
                labels = listOf(label(1, "errand"), label(2, "money"), label(3, "health"), label(4, "work")),
            ),
        )

    private fun TestScope.presenter(): QuickAddPresenter =
        QuickAddPresenter(
            scope = CoroutineScope(StandardTestDispatcher(testScheduler)),
            workspace = workspace,
            recent = FakeRecentProjects(),
            actions = RecordingQuickAddActions(),
            zone = ZoneId.of("Europe/Berlin"),
            recentLabels = FakeRecentLabels(),
            now = { Instant.parse("2026-05-20T10:00:00Z") },
            labelSuggestions = suggestions,
        ).also {
            backgroundScope.launch { it.state.collect { } }
            runCurrent()
        }

    @Test
    fun `the title is asked about once typing pauses, and the answer is offered`() =
        runTest {
            suggestions.answer = listOf(2L, 3L)
            val presenter = presenter()

            presenter.open()
            presenter.setTitles("Pay")
            advanceTimeBy(100)
            presenter.setTitles("Pay the rent")
            runCurrent()
            assertTrue(suggestions.asked.isEmpty(), "asked before typing paused")

            advanceTimeBy(400)
            runCurrent()

            assertEquals(listOf("Pay the rent"), suggestions.asked.map { it.title })
            assertEquals(SuggestionSurface.CAPTURE, suggestions.asked.single().surface)
            assertEquals(listOf("money", "health"), presenter.state.value.suggestedLabels.map { it.name })
        }

    @Test
    fun `only the first line and the destination are asked about`() =
        runTest {
            val presenter = presenter()

            presenter.open()
            presenter.chooseProject(1)
            presenter.setTitles("Pay the rent\nCall the landlord")
            advanceTimeBy(400)
            runCurrent()

            assertEquals("Pay the rent", suggestions.asked.last().title)
            assertEquals(1L, suggestions.asked.last().projectLocalId)
        }

    @Test
    fun `a suggestion is never a label the sheet already shows another way`() =
        runTest {
            workspace.value =
                workspace.value.copy(
                    appSettings = AppSettings(autoLabels = listOf(AutoLabelRule("rent", listOf(1), ignoreCase = true))),
                )
            suggestions.answer = listOf(1L, 2L, 3L, 4L)
            val presenter = presenter()

            presenter.open()
            presenter.setTitles("Pay the rent")
            presenter.setLabels(listOf("health"))
            advanceTimeBy(400)
            runCurrent()

            // errand is earned by the rule, health was picked by hand.
            assertEquals(listOf("money", "work"), presenter.state.value.suggestedLabels.map { it.name })
        }

    @Test
    fun `tapping a suggestion puts it on the draft`() =
        runTest {
            suggestions.answer = listOf(2L)
            val presenter = presenter()

            presenter.open()
            presenter.setTitles("Pay the rent")
            advanceTimeBy(400)
            runCurrent()
            val offered = presenter.state.value.suggestedLabels.single()
            presenter.setLabels(presenter.state.value.draft.labelNames + offered.name)
            runCurrent()

            assertEquals(listOf("money"), presenter.state.value.draft.labelNames)
            assertTrue(presenter.state.value.suggestedLabels.isEmpty())
        }

    @Test
    fun `a closed sheet and a blank title ask nothing`() =
        runTest {
            suggestions.answer = listOf(2L)
            val presenter = presenter()

            presenter.open()
            presenter.setTitles("   ")
            advanceTimeBy(400)
            runCurrent()

            assertTrue(suggestions.asked.isEmpty())
            assertTrue(presenter.state.value.suggestedLabels.isEmpty())
        }
}

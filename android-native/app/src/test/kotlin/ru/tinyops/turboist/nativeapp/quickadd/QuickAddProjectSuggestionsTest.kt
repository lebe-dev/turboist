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
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.nativeapp.labelsuggest.FakeProjectSuggestions
import java.time.Instant
import java.time.ZoneId
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** Projects the on-device model offers while a title headed for the inbox is typed. */
@OptIn(ExperimentalCoroutinesApi::class)
class QuickAddProjectSuggestionsTest {
    private val suggestions = FakeProjectSuggestions()

    private val workspace =
        MutableStateFlow(
            QuickAddWorkspace(
                projects =
                    listOf(
                        project(1, "Home"),
                        project(2, "Work"),
                        project(3, "Health"),
                        project(4, "Money"),
                        project(5, "Garden"),
                    ),
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
            projectSuggestions = suggestions,
        ).also {
            backgroundScope.launch { it.state.collect { } }
            runCurrent()
        }

    @Test
    fun `the first line is asked about once typing pauses, and the answer is offered`() =
        runTest {
            suggestions.answer = listOf(4L, 3L)
            val presenter = presenter()

            presenter.open()
            presenter.setTitles("Pay")
            advanceTimeBy(100)
            presenter.setTitles("Pay the rent\nCall the landlord")
            runCurrent()
            assertTrue(suggestions.asked.isEmpty(), "asked before typing paused")

            advanceTimeBy(400)
            runCurrent()

            assertEquals(listOf("Pay the rent"), suggestions.asked.map { it.title })
            assertEquals(listOf("Money", "Health"), presenter.state.value.suggestedProjects.map { it.title })
        }

    @Test
    fun `the model's projects follow the rules' ones, each once, at most three`() =
        runTest {
            workspace.value =
                workspace.value.copy(
                    appSettings =
                        AppSettings(projectSuggestions = listOf(ProjectSuggestionRule("rent", listOf(4), true))),
                )
            suggestions.answer = listOf(4L, 1L, 2L, 3L, 5L)
            val presenter = presenter()

            presenter.open()
            presenter.setTitles("Pay the rent")
            advanceTimeBy(400)
            runCurrent()

            assertEquals(
                listOf("Money", "Home", "Work", "Health"),
                presenter.state.value.suggestedProjects.map { it.title },
            )
        }

    @Test
    fun `nothing is asked while the draft is headed for a project`() =
        runTest {
            suggestions.answer = listOf(4L)
            val presenter = presenter()

            presenter.open(QuickAddRequest(projectLocalId = 1))
            presenter.setTitles("Pay the rent")
            advanceTimeBy(400)
            runCurrent()

            assertTrue(suggestions.asked.isEmpty())
            assertTrue(presenter.state.value.suggestedProjects.isEmpty())
        }

    @Test
    fun `choosing a project takes the model's suggestions away, and the inbox brings them back`() =
        runTest {
            suggestions.answer = listOf(4L)
            val presenter = presenter()

            presenter.open()
            presenter.setTitles("Pay the rent")
            advanceTimeBy(400)
            runCurrent()
            presenter.chooseProject(4)
            advanceTimeBy(400)
            runCurrent()

            assertEquals(4L, presenter.state.value.draft.projectLocalId)
            assertTrue(presenter.state.value.suggestedProjects.isEmpty())

            presenter.chooseProject(null)
            advanceTimeBy(400)
            runCurrent()

            assertEquals(listOf("Money"), presenter.state.value.suggestedProjects.map { it.title })
        }

    @Test
    fun `a blank title asks nothing`() =
        runTest {
            suggestions.answer = listOf(4L)
            val presenter = presenter()

            presenter.open()
            presenter.setTitles("   ")
            advanceTimeBy(400)
            runCurrent()

            assertTrue(suggestions.asked.isEmpty())
            assertTrue(presenter.state.value.suggestedProjects.isEmpty())
        }
}

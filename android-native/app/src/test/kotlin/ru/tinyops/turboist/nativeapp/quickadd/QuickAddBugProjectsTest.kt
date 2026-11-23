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
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.nativeapp.labelsuggest.FakeProjectSuggestions
import ru.tinyops.turboist.nativeapp.labelsuggest.ProjectSuggestions
import java.time.Instant
import java.time.ZoneId
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * What the bug shortcut does to the places the capture sheet offers.
 *
 * A bug belongs to something written in software, so the shortcut narrows every
 * way a project is offered — the picker, the model's suggestions and today's row —
 * while a destination already chosen stays the user's own decision.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class QuickAddBugProjectsTest {
    private val zone: ZoneId = ZoneId.of("Europe/Berlin")

    private val workspace =
        MutableStateFlow(
            QuickAddWorkspace(
                projects =
                    listOf(
                        project(1, "Toolbox", type = ProjectType.SOFTWARE),
                        project(2, "Garden", type = ProjectType.GENERIC),
                        project(3, "App", type = ProjectType.SOFTWARE),
                        project(4, "Books", type = ProjectType.GENERIC),
                    ),
                labels = listOf(label(1, "bug")),
                bugLabelNames = listOf("bug"),
            ),
        )

    private fun TestScope.presenter(
        recent: RecentProjects = FakeRecentProjects(),
        projectSuggestions: ProjectSuggestions = ProjectSuggestions.None,
    ): QuickAddPresenter =
        QuickAddPresenter(
            scope = CoroutineScope(StandardTestDispatcher(testScheduler)),
            workspace = workspace,
            recent = recent,
            actions = RecordingQuickAddActions(),
            zone = zone,
            recentLabels = FakeRecentLabels(),
            now = { Instant.parse("2026-05-20T10:00:00Z") },
            projectSuggestions = projectSuggestions,
        ).also {
            backgroundScope.launch { it.state.collect { } }
            runCurrent()
        }

    @Test
    fun `the shortcut narrows the picker to software projects`() =
        runTest {
            val presenter = presenter(recent = FakeRecentProjects(listOf(2, 1)))
            presenter.open()
            runCurrent()

            assertEquals(
                listOf("Garden", "Toolbox", "App", "Books"),
                (presenter.state.value.recentProjects + presenter.state.value.otherProjects).map { it.title },
            )

            presenter.toggleBug()
            runCurrent()

            // Two candidates fit the whole row, so nothing is lifted as recent.
            assertTrue(presenter.state.value.recentProjects.isEmpty())
            assertEquals(listOf("Toolbox", "App"), presenter.state.value.otherProjects.map { it.title })
        }

    @Test
    fun `turning the shortcut off offers every project again`() =
        runTest {
            val presenter = presenter()
            presenter.open()
            presenter.toggleBug()
            runCurrent()

            presenter.toggleBug()
            runCurrent()

            assertEquals(
                listOf("App", "Books", "Garden", "Toolbox"),
                (presenter.state.value.recentProjects + presenter.state.value.otherProjects).map { it.title }.sorted(),
            )
        }

    @Test
    fun `today's row offers only software projects while the shortcut is on`() =
        runTest {
            workspace.value = workspace.value.copy(todayProjectLocalIds = listOf(1, 2))
            val presenter = presenter()
            presenter.open(QuickAddRequest(todayProjects = true))
            runCurrent()

            assertEquals(
                listOf("Garden", "Toolbox"),
                presenter.state.value.todayProjects.map { it.title }.sorted(),
            )

            presenter.toggleBug()
            runCurrent()

            assertEquals(listOf("Toolbox"), presenter.state.value.todayProjects.map { it.title })
        }

    @Test
    fun `the model's project is offered only while it is software`() =
        runTest {
            val suggestions = FakeProjectSuggestions()
            suggestions.answer = listOf(4L, 3L)
            val presenter = presenter(projectSuggestions = suggestions)
            presenter.open()
            presenter.setTitles("Crash on save")
            advanceTimeBy(400)
            runCurrent()

            assertEquals(listOf("Books", "App"), presenter.state.value.suggestedProjects.map { it.title })

            presenter.toggleBug()
            advanceTimeBy(400)
            runCurrent()

            assertEquals(listOf("App"), presenter.state.value.suggestedProjects.map { it.title })
        }

    @Test
    fun `a project already chosen stays the destination`() =
        runTest {
            val presenter = presenter()
            presenter.open(QuickAddRequest(projectLocalId = 2))
            runCurrent()

            presenter.toggleBug()
            runCurrent()

            assertEquals(2L, presenter.state.value.draft.projectLocalId)
            assertEquals("Garden", presenter.state.value.projectTitle)
        }

    @Test
    fun `the shortcut is still on with no software projects around`() =
        runTest {
            workspace.value =
                workspace.value.copy(projects = listOf(project(2, "Garden", type = ProjectType.GENERIC)))
            val presenter = presenter()
            presenter.open()
            presenter.setLabels(listOf("bug"))
            runCurrent()

            assertTrue(presenter.state.value.bugActive)
            assertTrue(presenter.state.value.otherProjects.isEmpty())
            assertTrue(presenter.state.value.recentProjects.isEmpty())
        }
}

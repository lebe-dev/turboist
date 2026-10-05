package ru.tinyops.turboist.nativeapp.projects

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.TroikiCategory
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What the projects screen does with the workspace it is given.
 *
 * Every check here runs without Compose, without a database and without a
 * server: narrowing, searching and ordering are decisions about a list already
 * in memory, and none of them is a drawing concern.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class ProjectsPresenterTest {
    private val groups = MutableStateFlow<List<ProjectGroup>>(emptyList())
    private val contexts = MutableStateFlow(listOf(context(1), context(2)))
    private val dailyPlanEnabled = MutableStateFlow(true)
    private val actions = RecordingProjectActions()
    private val sync = CountingScheduler()

    private fun kotlinx.coroutines.test.TestScope.presenter(): ProjectsPresenter {
        val presenter = ProjectsPresenter(backgroundScope, groups, contexts, dailyPlanEnabled, actions, sync)
        backgroundScope.launch { presenter.state.collect {} }
        return presenter
    }

    @Test
    fun `a workspace not read yet is not the same as an empty one`() =
        runTest {
            val presenter = presenter()

            assertTrue(presenter.state.value.loading)
            assertFalse(presenter.state.value.isEmpty, "nothing has been read, so nothing is known to be empty")
        }

    @Test
    fun `projects are grouped under the context they are filed in`() =
        runTest {
            val presenter = presenter()
            groups.value =
                listOf(
                    ProjectGroup(context(1, "Work"), listOf(project(10, "Website", contextLocalId = 1))),
                    ProjectGroup(context(2, "Home"), emptyList()),
                )
            runCurrent()

            assertEquals(listOf("Work", "Home"), presenter.state.value.groups.map { it.context.name })
            assertEquals(listOf("Website"), presenter.state.value.groups.first().projects.map { it.title })
            assertTrue(
                presenter.state.value.groups.last().projects.isEmpty(),
                "a context with no projects is still where the next one is filed",
            )
        }

    @Test
    fun `open work leads, then the daily plan in its own order, then the alphabet`() =
        runTest {
            val presenter = presenter()
            groups.value =
                listOf(
                    ProjectGroup(
                        context(1),
                        listOf(
                            project(10, "Zebra"),
                            project(11, "Archived one", status = ProjectStatus.ARCHIVED),
                            project(12, "Rest work", troikiCategory = TroikiCategory.REST),
                            project(13, "Important work", troikiCategory = TroikiCategory.IMPORTANT),
                            project(14, "Alpha"),
                        ),
                    ),
                )
            runCurrent()

            assertEquals(
                listOf("Important work", "Rest work", "Alpha", "Zebra", "Archived one"),
                presenter.state.value.groups.single().projects.map { it.title },
            )
        }

    @Test
    fun `a narrowing shows one slice and counts them all`() =
        runTest {
            val presenter = presenter()
            groups.value =
                listOf(
                    ProjectGroup(
                        context(1),
                        listOf(
                            project(10, "Website", type = ProjectType.SOFTWARE),
                            project(11, "Move house"),
                            project(12, "Old one", status = ProjectStatus.ARCHIVED),
                        ),
                    ),
                )
            presenter.narrowTo(ProjectFilter.SOFTWARE)
            runCurrent()

            assertEquals(listOf("Website"), presenter.state.value.groups.single().projects.map { it.title })
            assertEquals(3, presenter.state.value.counts[ProjectFilter.ALL])
            assertEquals(1, presenter.state.value.counts[ProjectFilter.SOFTWARE])
            assertEquals(1, presenter.state.value.counts[ProjectFilter.ARCHIVED])
        }

    @Test
    fun `a search narrows by title without regard to case`() =
        runTest {
            val presenter = presenter()
            groups.value =
                listOf(ProjectGroup(context(1), listOf(project(10, "Website"), project(11, "Move house"))))
            presenter.search("WEB")
            runCurrent()

            assertEquals(listOf("Website"), presenter.state.value.groups.single().projects.map { it.title })
            assertTrue(presenter.state.value.isEmpty.not())
        }

    @Test
    fun `pinned projects are surfaced whichever context they live in, oldest pin first`() =
        runTest {
            val presenter = presenter()
            groups.value =
                listOf(
                    ProjectGroup(context(1), listOf(project(10, "Website", isPinned = true, pinnedAt = 20))),
                    ProjectGroup(
                        context(2),
                        listOf(project(11, "Move house", contextLocalId = 2, isPinned = true, pinnedAt = 10)),
                    ),
                )
            runCurrent()

            assertEquals(listOf("Move house", "Website"), presenter.state.value.pinned.map { it.title })
        }

    @Test
    fun `a narrowing never hides a pinned project from the pinned row`() =
        runTest {
            val presenter = presenter()
            groups.value =
                listOf(
                    ProjectGroup(
                        context(1),
                        listOf(project(10, "Old one", status = ProjectStatus.ARCHIVED, isPinned = true)),
                    ),
                )
            presenter.narrowTo(ProjectFilter.SOFTWARE)
            runCurrent()

            assertTrue(presenter.state.value.groups.single().projects.isEmpty())
            assertEquals(listOf("Old one"), presenter.state.value.pinned.map { it.title })
        }

    @Test
    fun `starting a project files it in the context it was started from`() =
        runTest {
            val presenter = presenter()

            presenter.createProject(contextLocalId = 2, title = "  Website  ")
            runCurrent()

            assertEquals(listOf("createProject 2 Website"), actions.calls)
        }

    @Test
    fun `a project with no name is not started at all`() =
        runTest {
            val presenter = presenter()

            presenter.createProject(contextLocalId = 2, title = "   ")
            runCurrent()

            assertEquals(emptyList(), actions.calls)
        }

    @Test
    fun `pulling the screen down asks the sync engine to catch up`() =
        runTest {
            val presenter = presenter()

            presenter.refresh()
            runCurrent()

            assertEquals(1, sync.requests)
            assertFalse(presenter.state.value.refreshing, "the cycle is over, so the spinner is down")
        }
}

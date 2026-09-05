package ru.tinyops.turboist.nativeapp.search.ui

import android.app.Application
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.search.FakeRecentSearches
import ru.tinyops.turboist.nativeapp.search.RecordingSearchRepository
import ru.tinyops.turboist.nativeapp.search.SearchNavigation
import ru.tinyops.turboist.nativeapp.search.SearchPresenter
import ru.tinyops.turboist.nativeapp.search.SearchResults
import ru.tinyops.turboist.nativeapp.search.TaskHit
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals

/**
 * The search screen composed for real, driven by its own presenter.
 *
 * The replica behind it is a stand-in, because these cases are about the screen:
 * that an answer is drawn as four separate groups, that a result leads somewhere,
 * and that a screen with nothing typed into it offers the last few searches
 * instead of an explanation nobody needs twice.
 *
 * The presenter is given no typing pause here. Waiting for the next keystroke is
 * a decision with a test of its own; repeating it in a screen test would only
 * make every case depend on a clock.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class SearchScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val repository = RecordingSearchRepository()
    private var recent = FakeRecentSearches()
    private val opened = mutableListOf<String>()

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private val navigation =
        SearchNavigation(
            onOpenTask = { opened += "task:" + it.title },
            onOpenProject = { opened += "project:" + it.title },
            onOpenLabel = { opened += "label:" + it.name },
            onOpenContext = { opened += "context:" + it.name },
        )

    private fun launch() {
        compose.setContent {
            val scope = rememberCoroutineScope()
            val presenter = remember(scope) { SearchPresenter(scope, repository, recent, debounceMillis = 0) }
            val state by presenter.state.collectAsStateWithLifecycle()
            TurboistTheme {
                SearchScreen(
                    state = state,
                    callbacks =
                        SearchCallbacks(
                            onType = presenter::type,
                            onSubmit = presenter::submit,
                            onNarrowTo = presenter::narrowTo,
                            onToggleOpenOnly = presenter::toggleOpenTasksOnly,
                            onRerun = presenter::rerun,
                            onForgetRecent = presenter::forgetRecent,
                            navigation = navigation,
                        ),
                )
            }
        }
        compose.waitForIdle()
    }

    private fun project(
        localId: Long,
        title: String,
    ) = Project(localId = localId, serverId = localId, contextLocalId = 1, title = title, createdAt = 0, updatedAt = 0)

    private fun label(
        localId: Long,
        name: String,
    ) = Label(localId = localId, serverId = localId, name = name, createdAt = 0, updatedAt = 0)

    private fun workspaceContext(
        localId: Long,
        name: String,
    ) = Context(localId = localId, serverId = localId, name = name, createdAt = 0, updatedAt = 0)

    private fun taskHit(
        localId: Long,
        title: String,
        status: TaskStatus = TaskStatus.OPEN,
        projectTitle: String? = null,
    ): TaskHit = TaskHit(task(localId, title = title, status = status), projectTitle)

    private fun search(typed: String) {
        compose.onNode(hasSetTextAction()).performTextInput(typed)
        compose.waitForIdle()
    }

    @Test
    fun `a screen with nothing typed into it explains what the field is for`() {
        launch()

        compose.onNodeWithText(text(R.string.page_search_emptyTitle)).assertIsDisplayed()
    }

    @Test
    fun `an answer is drawn as one block per kind`() {
        repository.answer =
            SearchResults(
                tasks = listOf(taskHit(1, "Renew passport", projectTitle = "Travel")),
                projects = listOf(project(2, "Passport paperwork")),
                labels = listOf(label(3, "passport-run")),
                contexts = listOf(workspaceContext(4, "Passport office")),
            )
        launch()

        search("passport")

        // Each kind names itself twice once it has results: once as the chip that
        // narrows to it, once as the heading over its block.
        compose.onAllNodesWithText(text(R.string.page_search_tasksTab)).assertCountEquals(2)
        compose.onAllNodesWithText(text(R.string.page_search_projectsTab)).assertCountEquals(2)
        compose.onAllNodesWithText(text(R.string.nav_labels)).assertCountEquals(2)
        compose.onAllNodesWithText(text(R.string.native_search_contextsTab)).assertCountEquals(2)

        compose.onNodeWithText("Renew passport").assertIsDisplayed()
        compose.onNodeWithText("Travel").assertIsDisplayed()
        compose.onNodeWithText("Passport paperwork").assertIsDisplayed()
    }

    @Test
    fun `a result leads to the record it names`() {
        repository.answer =
            SearchResults(
                tasks = listOf(taskHit(1, "Renew passport")),
                projects = listOf(project(2, "Passport paperwork")),
            )
        launch()
        search("passport")

        compose.onNodeWithText("Renew passport").performClick()
        compose.onNodeWithText("Passport paperwork").performClick()

        assertEquals(listOf("task:Renew passport", "project:Passport paperwork"), opened)
    }

    @Test
    fun `a search that matches nothing says so rather than looking unanswered`() {
        launch()

        search("passport")

        compose.onNodeWithText(text(R.string.native_search_noMatches)).assertIsDisplayed()
    }

    @Test
    fun `the last few searches are offered before anything is typed, and re-run on a tap`() {
        recent = FakeRecentSearches(listOf("renew passport"))
        repository.answer = SearchResults(tasks = listOf(taskHit(1, "Renew passport")))
        launch()

        compose.onNodeWithText(text(R.string.native_search_recent)).assertIsDisplayed()

        compose.onNodeWithText("renew passport").performClick()
        compose.waitForIdle()

        compose.onNodeWithText("Renew passport").assertIsDisplayed()
    }
}

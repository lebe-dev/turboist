package ru.tinyops.turboist.nativeapp.projects.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import kotlinx.coroutines.flow.emptyFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.sync.write.ProjectStatusAction
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.projects.BoardColumn
import ru.tinyops.turboist.nativeapp.projects.ProjectBoardUiState
import ru.tinyops.turboist.nativeapp.projects.ProjectGroup
import ru.tinyops.turboist.nativeapp.projects.ProjectsUiState
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals

/**
 * The project screens as they are actually drawn.
 *
 * The presenters are checked on their own elsewhere, so what is left to a screen
 * is what a screen is for: that the things on it are visible and that touching
 * one asks for what it says it will. Both screens are driven in their stateless
 * form, which is the same one the app composes.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class ProjectScreensTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)

    private fun context(
        localId: Long,
        name: String,
    ) = Context(localId = localId, serverId = localId, name = name, createdAt = 0, updatedAt = 0)

    private fun project(
        localId: Long,
        title: String,
        status: ProjectStatus = ProjectStatus.OPEN,
        type: ProjectType = ProjectType.GENERIC,
    ) = Project(
        localId = localId,
        serverId = localId,
        contextLocalId = 1,
        title = title,
        status = status,
        type = type,
        createdAt = 0,
        updatedAt = 0,
    )

    private fun task(
        localId: Long,
        title: String,
    ) = Task(localId = localId, serverId = localId, title = title, createdAt = 0, updatedAt = 0)

    private fun projectsCallbacks(record: (String) -> Unit) =
        ProjectsCallbacks(
            onNarrowTo = { record("narrow ${it.name}") },
            onSearch = { record("search $it") },
            onRefresh = { record("refresh") },
            onCreateProject = { contextLocalId, title -> record("create $contextLocalId $title") },
            onOpenProject = { record("openProject ${it.localId}") },
            onOpenContext = { record("openContext ${it.localId}") },
        )

    private fun boardCallbacks(record: (String) -> Unit) =
        ProjectBoardCallbacks(
            onRefresh = { record("refresh") },
            onOpenTask = { record("openTask ${it.localId}") },
            onToggleComplete = { record("toggle ${it.localId}") },
            onMoveTask = { taskLocalId, sectionLocalId -> record("moveTask $taskLocalId $sectionLocalId") },
            onAddTask = { sectionLocalId, title -> record("addTask $sectionLocalId $title") },
            onCreateSection = { record("createSection $it") },
            onRenameSection = { sectionLocalId, title -> record("renameSection $sectionLocalId $title") },
            onDeleteSection = { record("deleteSection $it") },
            onMoveSectionEarlier = { record("moveEarlier $it") },
            onMoveSectionLater = { record("moveLater $it") },
            onRename = { record("rename $it") },
            onSetStatus = { record("status ${it.segment}") },
            onTogglePin = { record("togglePin") },
            onSetTroiki = { record("troiki ${it?.wire}") },
            onSetPrivate = { record("private $it") },
            onDelete = { record("delete") },
        )

    @Test
    fun `the projects screen groups projects under their context and opens one`() {
        val done = mutableListOf<String>()
        compose.setContent {
            TurboistTheme {
                ProjectsScreen(
                    state =
                        ProjectsUiState(
                            loading = false,
                            groups = listOf(ProjectGroup(context(1, "Work"), listOf(project(10, "Website")))),
                        ),
                    messages = emptyFlow(),
                    callbacks = projectsCallbacks { done += it },
                )
            }
        }

        compose.onNodeWithText("Work").assertIsDisplayed()
        compose.onNodeWithText("Website").assertIsDisplayed()
        compose.onNodeWithText("Website").performClick()

        assertEquals(listOf("openProject 10"), done)
    }

    @Test
    fun `a context heading leads to the context itself`() {
        val done = mutableListOf<String>()
        compose.setContent {
            TurboistTheme {
                ProjectsScreen(
                    state =
                        ProjectsUiState(
                            loading = false,
                            groups = listOf(ProjectGroup(context(1, "Work"), emptyList())),
                        ),
                    messages = emptyFlow(),
                    callbacks = projectsCallbacks { done += it },
                )
            }
        }

        compose.onNodeWithText("Work").performClick()

        assertEquals(listOf("openContext 1"), done)
    }

    @Test
    fun `a workspace known to hold nothing says what the screen is for`() {
        compose.setContent {
            TurboistTheme {
                ProjectsScreen(
                    state = ProjectsUiState(loading = false),
                    messages = emptyFlow(),
                    callbacks = projectsCallbacks { },
                )
            }
        }

        compose.onNodeWithText(text(R.string.page_projects_emptyTitle)).assertIsDisplayed()
    }

    @Test
    fun `a context with no projects still offers somewhere to file the next one`() {
        compose.setContent {
            TurboistTheme {
                ProjectsScreen(
                    state =
                        ProjectsUiState(
                            loading = false,
                            groups = listOf(ProjectGroup(context(4, "Work"), emptyList())),
                        ),
                    messages = emptyFlow(),
                    callbacks = projectsCallbacks { },
                )
            }
        }

        // The heading stays and keeps its own way to add a project: replacing it
        // with "nothing here" would leave the user with a message and nowhere to
        // act on it. The button names the context it files into, which is what an
        // assistive reader announces.
        compose.onNodeWithText("Work").assertIsDisplayed()
        compose
            .onNodeWithContentDescription(text(R.string.sidebar_addToAriaLabel, "Work"))
            .assertIsDisplayed()
    }

    @Test
    fun `the board draws the project's own column first and then its columns`() {
        compose.setContent {
            TurboistTheme {
                ProjectScreen(
                    state =
                        ProjectBoardUiState(
                            loading = false,
                            project = project(1, "Website"),
                            contextName = "Work",
                            columns =
                                listOf(
                                    BoardColumn("root", null, null, null, emptyList(), emptyList()),
                                    BoardColumn(
                                        key = "doing",
                                        sectionLocalId = 10,
                                        title = "Doing",
                                        position = 0,
                                        open = listOf(TaskListRow(task(5, "Ship it"), depth = 0, projectTitle = null)),
                                        done = emptyList(),
                                    ),
                                ),
                        ),
                    zone = zone,
                    today = today,
                    messages = emptyFlow(),
                    callbacks = boardCallbacks { },
                )
            }
        }

        compose.onNodeWithText(text(R.string.dialog_moveSection_noSection)).assertIsDisplayed()
        compose.onNodeWithText("Doing").assertIsDisplayed()
        compose.onNodeWithText("Ship it").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.section_noTasks)).assertIsDisplayed()
    }

    @Test
    fun `a project that is gone says so instead of drawing an empty board`() {
        compose.setContent {
            TurboistTheme {
                ProjectScreen(
                    state = ProjectBoardUiState(loading = false, project = null),
                    zone = zone,
                    today = today,
                    messages = emptyFlow(),
                    callbacks = boardCallbacks { },
                )
            }
        }

        compose.onNodeWithText(text(R.string.page_project_notFound)).assertIsDisplayed()
    }

    @Test
    fun `finishing the project is asked for from its actions`() {
        val done = mutableListOf<String>()
        compose.setContent {
            TurboistTheme {
                ProjectScreen(
                    state = ProjectBoardUiState(loading = false, project = project(1, "Website")),
                    zone = zone,
                    today = today,
                    messages = emptyFlow(),
                    callbacks = boardCallbacks { done += it },
                )
            }
        }

        compose.onNodeWithContentDescription(text(R.string.project_actionsAriaLabel)).performClick()
        compose.onNodeWithText(text(R.string.project_complete)).performClick()

        assertEquals(listOf("status " + ProjectStatusAction.COMPLETE.segment), done)
    }

    @Test
    fun `a finished project is offered no place in the daily plan`() {
        compose.setContent {
            TurboistTheme {
                ProjectScreen(
                    state =
                        ProjectBoardUiState(
                            loading = false,
                            project = project(1, "Website", status = ProjectStatus.COMPLETED),
                        ),
                    zone = zone,
                    today = today,
                    messages = emptyFlow(),
                    callbacks = boardCallbacks { },
                )
            }
        }

        compose.onNodeWithContentDescription(text(R.string.project_actionsAriaLabel)).performClick()
        compose.onNodeWithText(text(R.string.project_reopen)).assertIsDisplayed()
    }
}

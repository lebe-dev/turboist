package ru.tinyops.turboist.nativeapp.troiki.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onFirst
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
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.troiki.TroikiUiState
import ru.tinyops.turboist.nativeapp.troiki.troikiSlots
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals

/**
 * The daily plan as it is actually drawn.
 *
 * The cut into buckets and the presenter are checked on their own elsewhere, so
 * what is left to the screen is what a screen is for: that the three buckets and
 * the work in them are visible, that a free place leads somewhere a project can
 * be picked, and that ending a cycle asks before it empties the plan.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class TroikiScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)

    private fun project(
        localId: Long,
        title: String,
        category: TroikiCategory?,
    ) = Project(
        localId = localId,
        serverId = localId,
        contextLocalId = 1,
        title = title,
        troikiCategory = category,
        createdAt = 0,
        updatedAt = 0,
    )

    private fun task(
        localId: Long,
        title: String,
        projectLocalId: Long,
    ) = Task(
        localId = localId,
        serverId = localId,
        title = title,
        projectLocalId = projectLocalId,
        createdAt = 0,
        updatedAt = 0,
    )

    private fun callbacks(record: (String) -> Unit) =
        TroikiCallbacks(
            onRefresh = { record("refresh") },
            onOpenTask = { record("openTask ${it.localId}") },
            onOpenProject = { record("openProject ${it.localId}") },
            onToggleComplete = { record("toggle ${it.localId}") },
            onAddTask = { projectLocalId, title -> record("addTask $projectLocalId $title") },
            onAssign = { projectLocalId, category -> record("assign $projectLocalId ${category.wire}") },
            onRemove = { record("remove $it") },
            onStart = { record("start") },
            onReset = { record("reset") },
        )

    private fun plan(
        projects: List<Project> = emptyList(),
        tasks: List<Task> = emptyList(),
        assignable: List<Project> = emptyList(),
    ) = TroikiUiState(loading = false, slots = troikiSlots(projects, tasks), assignable = assignable)

    private fun show(
        state: TroikiUiState,
        record: (String) -> Unit = {},
    ) {
        compose.setContent {
            TurboistTheme(dynamicColor = false) {
                TroikiScreen(
                    state = state,
                    zone = zone,
                    today = today,
                    messages = emptyFlow(),
                    callbacks = callbacks(record),
                )
            }
        }
    }

    @Test
    fun `the plan draws its three buckets with the work standing in them`() {
        show(
            plan(
                projects = listOf(project(7, "Website", TroikiCategory.IMPORTANT)),
                tasks = listOf(task(1, "Ship it", projectLocalId = 7)),
            ),
        )

        compose.onNodeWithText(text(R.string.troiki_section_important)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.troiki_section_medium)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.troiki_section_rest)).assertIsDisplayed()
        compose.onNodeWithText("Website").assertIsDisplayed()
        compose.onNodeWithText("Ship it").assertIsDisplayed()
    }

    @Test
    fun `a bucket says how full it is against the room it starts with`() {
        show(plan(projects = listOf(project(7, "Website", TroikiCategory.IMPORTANT))))

        compose.onNodeWithText(text(R.string.page_weekSummary_troikiSlots, "1", "3")).assertIsDisplayed()
    }

    @Test
    fun `a free place leads to the projects that can take it`() {
        val done = mutableListOf<String>()
        show(plan(assignable = listOf(project(9, "Kitchen", category = null))), record = { done += it })

        compose.onAllNodesWithText(text(R.string.troiki_emptySlot)).onFirst().performClick()
        compose.onNodeWithText("Kitchen").performClick()

        assertEquals(listOf("assign 9 important"), done)
    }

    @Test
    fun `a project can be taken out of the plan from its own menu`() {
        val done = mutableListOf<String>()
        show(plan(projects = listOf(project(7, "Website", TroikiCategory.IMPORTANT))), record = { done += it })

        compose.onNodeWithContentDescription(text(R.string.project_actionsAriaLabel)).performClick()
        compose.onNodeWithText(text(R.string.project_removeFromTroiki)).performClick()

        assertEquals(listOf("remove 7"), done)
    }

    @Test
    fun `beginning a cycle is one tap and ending one is asked about first`() {
        val done = mutableListOf<String>()
        show(plan(), record = { done += it })

        compose.onNodeWithText(text(R.string.troiki_start)).performClick()
        assertEquals(listOf("start"), done)

        compose.onNodeWithText(text(R.string.troiki_reset)).performClick()
        compose.onNodeWithText(text(R.string.troiki_resetDescription)).assertIsDisplayed()
        assertEquals(listOf("start"), done, "the plan was emptied before the question was answered")

        compose.onNodeWithText(text(R.string.troiki_resetConfirm)).performClick()
        assertEquals(listOf("start", "reset"), done)
    }

    @Test
    fun `each project of the plan offers a way to write a task into it, named after the project`() {
        show(plan(projects = listOf(project(7, "Website", TroikiCategory.IMPORTANT))))

        // Named after the project rather than "add", because that is what an
        // assistive reader announces and a plan holds several of these at once.
        compose.onNodeWithContentDescription(text(R.string.troiki_addTaskAria, "Website")).assertIsDisplayed()
    }
}

package ru.tinyops.turboist.nativeapp.templates.ui

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
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.TaskTemplate
import ru.tinyops.turboist.core.model.TaskTemplateSubtask
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.templates.TemplatesUiState
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals

/**
 * The templates screen as it is actually drawn.
 *
 * The presenter is checked on its own elsewhere, so what is left here is what a
 * screen is for: that a template says how much work it carries, that the picker
 * appears where it should, and that touching something asks for what it says it
 * will.
 *
 * The editor is deliberately not driven from here. It is a dialog full of text
 * fields, and a text field in a dialog window never reports itself settled under
 * this test runner, so what the editor does is checked where it is decided —
 * against the presenter, which is also where it can be checked properly.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class TemplatesScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `a workspace with no templates says so rather than showing an empty list`() {
        show(TemplatesUiState(loading = false))

        compose.onNodeWithText(text(R.string.settings_templates_empty)).assertIsDisplayed()
    }

    @Test
    fun `a template says how many lines its checklist carries`() {
        show(TemplatesUiState(loading = false, templates = listOf(template("Release", listOf("Tag", "Notes")))))

        compose.onNodeWithText("Release").assertIsDisplayed()
        compose
            .onNodeWithText(
                RuntimeEnvironment.getApplication().resources
                    .getQuantityString(R.plurals.settings_templates_subtaskCount, 2, 2),
            ).assertIsDisplayed()
    }

    @Test
    fun `using a template asks where the work should go`() {
        val asked = mutableListOf<TaskTemplate>()
        show(
            state = TemplatesUiState(loading = false, templates = listOf(template("Release", listOf("Tag")))),
            onStartInstantiate = { asked += it },
        )

        compose.onNodeWithContentDescription(text(R.string.topbar_fromTemplate)).performClick()

        assertEquals(listOf("Release"), asked.map { it.name })
    }

    @Test
    fun `the project picker offers the projects the work can go into`() {
        val picked = mutableListOf<Long>()
        show(
            state =
                TemplatesUiState(
                    loading = false,
                    templates = listOf(template("Release")),
                    projects = listOf(project(7, "Website")),
                    choosingProjectFor = template("Release"),
                ),
            onInstantiate = { picked += it },
        )

        compose.onNodeWithText(text(R.string.template_pickProject_title)).assertIsDisplayed()
        compose.onNodeWithText("Website").performClick()

        assertEquals(listOf(7L), picked)
    }

    // --- the harness ----------------------------------------------------------

    private fun show(
        state: TemplatesUiState,
        onStartInstantiate: (TaskTemplate) -> Unit = {},
        onInstantiate: (Long) -> Unit = {},
    ) {
        compose.setContent {
            TurboistTheme {
                TemplatesScreen(
                    state = state,
                    messages = emptyFlow(),
                    callbacks =
                        TemplatesCallbacks(
                            onRefresh = {},
                            onStartNew = {},
                            onEdit = {},
                            onDelete = {},
                            onRename = {},
                            onDescribe = {},
                            onAddLine = {},
                            onRetitleLine = { _, _ -> },
                            onRemoveLine = {},
                            onSave = {},
                            onCancelEdit = {},
                            onStartInstantiate = onStartInstantiate,
                            onInstantiate = onInstantiate,
                            onCancelInstantiate = {},
                        ),
                )
            }
        }
    }

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)

    private fun template(
        name: String,
        lines: List<String> = emptyList(),
    ): TaskTemplate =
        TaskTemplate(
            localId = 1,
            serverId = 1,
            name = name,
            subtasks =
                lines.mapIndexed { index, title ->
                    TaskTemplateSubtask(
                        localId = index + 1L,
                        templateLocalId = 1,
                        position = index,
                        title = title,
                    )
                },
            createdAt = 0,
            updatedAt = 0,
        )

    private fun project(
        localId: Long,
        title: String,
    ): Project =
        Project(
            localId = localId,
            serverId = localId,
            contextLocalId = 1,
            title = title,
            createdAt = 0,
            updatedAt = 0,
        )
}

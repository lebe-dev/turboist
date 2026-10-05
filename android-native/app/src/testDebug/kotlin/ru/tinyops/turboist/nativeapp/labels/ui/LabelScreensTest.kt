package ru.tinyops.turboist.nativeapp.labels.ui

import android.app.Application
import androidx.compose.material3.Text
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import kotlinx.coroutines.flow.emptyFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.view.LabelPeriodUsage
import ru.tinyops.turboist.core.model.view.LabelUsage
import ru.tinyops.turboist.core.model.view.LabelUsagePeriod
import ru.tinyops.turboist.core.model.view.LabelUsageTotals
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.labels.LabelDetailUiState
import ru.tinyops.turboist.nativeapp.labels.LabelStatsRow
import ru.tinyops.turboist.nativeapp.labels.LabelStatsUiState
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals

/**
 * The label screens as they are actually drawn.
 *
 * The presenters are checked on their own elsewhere, so what is left to a screen
 * is what a screen is for: that the things on it are visible and that touching
 * one asks for what it says it will. Both screens are driven in their stateless
 * form, which is the same one the app composes.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class LabelScreensTest {
    @get:Rule
    val compose = createComposeRule()

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)

    private fun label(
        localId: Long,
        name: String,
        isFavourite: Boolean = false,
        isPrivate: Boolean = false,
    ) = Label(
        localId = localId,
        serverId = localId,
        name = name,
        isFavourite = isFavourite,
        isPrivate = isPrivate,
        createdAt = 0,
        updatedAt = 0,
    )

    private fun row(
        localId: Long,
        name: String,
        applied: Int = 0,
        previousApplied: Int = 0,
        completed: Int = 0,
        openTasks: Int = 0,
        overdue: Int = 0,
        totalTasks: Int = 0,
        projects: Int = 0,
        lastUsedDaysAgo: Long? = null,
    ) = LabelStatsRow(
        usage =
            LabelUsage(
                label = label(localId, name),
                totalTasks = totalTasks,
                openTasks = openTasks,
                overdue = overdue,
                projects = projects,
                periods =
                    mapOf(
                        LabelUsagePeriod.WEEK to LabelPeriodUsage(applied, previousApplied, completed),
                    ),
            ),
        lastUsedDaysAgo = lastUsedDaysAgo,
    )

    private fun labelsCallbacks(record: (String) -> Unit) =
        LabelsCallbacks(
            onChoosePeriod = { record("period ${it.name}") },
            onRefresh = { record("refresh") },
            onCreateLabel = { name, color -> record("create $name $color") },
            onOpenLabel = { record("open ${it.localId}") },
        )

    private fun detailCallbacks(record: (String) -> Unit) =
        LabelDetailCallbacks(
            onEdit = { name, color -> record("edit $name $color") },
            onToggleFavourite = { record("favourite") },
            onTogglePrivate = { record("private") },
            onDelete = { record("delete") },
        )

    @Test
    fun `a workspace with no labels says so instead of drawing an empty report`() {
        compose.setContent {
            TurboistTheme {
                LabelsScreen(
                    state = LabelStatsUiState(loading = false),
                    messages = emptyFlow(),
                    callbacks = labelsCallbacks {},
                )
            }
        }

        compose.onNodeWithText(text(R.string.page_labels_emptyTitle)).assertIsDisplayed()
    }

    @Test
    fun `a label in use shows what its work is doing`() {
        compose.setContent {
            TurboistTheme {
                LabelsScreen(
                    state =
                        LabelStatsUiState(
                            loading = false,
                            active =
                                listOf(
                                    row(
                                        localId = 1,
                                        name = "urgent",
                                        applied = 4,
                                        completed = 2,
                                        openTasks = 3,
                                        overdue = 1,
                                        totalTasks = 9,
                                        projects = 2,
                                    ),
                                ),
                            totals = LabelUsageTotals(applied = 4, completed = 2, overdue = 1),
                            labelCount = 1,
                            mostApplied = 4,
                        ),
                    messages = emptyFlow(),
                    callbacks = labelsCallbacks {},
                )
            }
        }

        compose.onNodeWithText("urgent").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.page_labels_stats_applied)).assertIsDisplayed()
        val facts =
            listOf(
                text(R.string.page_labels_row_completed, "2"),
                text(R.string.page_labels_row_open, "3"),
                text(R.string.page_labels_row_overdue, "1"),
                text(R.string.page_labels_row_total, "9"),
                text(R.string.page_labels_row_projects, "2"),
            ).joinToString(" · ")
        compose.onNodeWithText(facts).assertIsDisplayed()
    }

    @Test
    fun `an unused label is offered for cleanup with how long ago it was used`() {
        compose.setContent {
            TurboistTheme {
                LabelsScreen(
                    state =
                        LabelStatsUiState(
                            loading = false,
                            idle = listOf(row(localId = 2, name = "someday", lastUsedDaysAgo = 40)),
                            labelCount = 1,
                        ),
                    messages = emptyFlow(),
                    callbacks = labelsCallbacks {},
                )
            }
        }

        compose.onNodeWithText(text(R.string.page_labels_idleTitle, "1")).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.page_labels_lastUsed_daysAgo, "40")).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.page_labels_noActivity)).assertIsDisplayed()
    }

    @Test
    fun `choosing a window asks for that window`() {
        val asked = mutableListOf<String>()
        compose.setContent {
            TurboistTheme {
                LabelsScreen(
                    state = LabelStatsUiState(loading = false, labelCount = 1),
                    messages = emptyFlow(),
                    callbacks = labelsCallbacks { asked += it },
                )
            }
        }

        compose.onNodeWithText(text(R.string.page_labels_period_quarter)).performClick()

        assertEquals(listOf("period QUARTER"), asked)
    }

    @Test
    fun `tapping a label opens the work carrying it`() {
        val asked = mutableListOf<String>()
        compose.setContent {
            TurboistTheme {
                LabelsScreen(
                    state =
                        LabelStatsUiState(
                            loading = false,
                            active = listOf(row(localId = 5, name = "urgent", applied = 1)),
                            labelCount = 1,
                            mostApplied = 1,
                        ),
                    messages = emptyFlow(),
                    callbacks = labelsCallbacks { asked += it },
                )
            }
        }

        compose.onNodeWithText("urgent").performClick()

        assertEquals(listOf("open 5"), asked)
    }

    @Test
    fun `the report offers a way to name a new label`() {
        compose.setContent {
            TurboistTheme {
                LabelsScreen(
                    state = LabelStatsUiState(loading = false, labelCount = 1),
                    messages = emptyFlow(),
                    callbacks = labelsCallbacks {},
                )
            }
        }

        compose.onNodeWithContentDescription(text(R.string.dialog_label_newTitle)).assertIsDisplayed()
    }

    // --- the editor ---

    @Test
    fun `naming a label and picking a colour writes both down`() {
        val saved = mutableListOf<LabelDraft>()
        compose.setContent {
            TurboistTheme {
                LabelEditorContent(initial = null, onConfirm = { saved += it }, onDismiss = {})
            }
        }

        compose.onNodeWithContentDescription(text(R.string.common_name)).performTextInput("  urgent  ")
        compose.onNodeWithContentDescription("blue").performClick()
        compose.onNodeWithText(text(R.string.common_create)).performClick()

        assertEquals(listOf(LabelDraft("urgent", "blue")), saved)
    }

    @Test
    fun `an editor opened on a label starts from what the label already says`() {
        val saved = mutableListOf<LabelDraft>()
        compose.setContent {
            TurboistTheme {
                LabelEditorContent(
                    initial = LabelDraft("urgent", "red"),
                    onConfirm = { saved += it },
                    onDismiss = {},
                )
            }
        }

        compose.onNodeWithText("urgent").assertIsDisplayed()
        compose.onNodeWithContentDescription("green").performClick()
        compose.onNodeWithText(text(R.string.common_save)).performClick()

        assertEquals(listOf(LabelDraft("urgent", "green")), saved)
    }

    @Test
    fun `a label with no name cannot be saved`() {
        val saved = mutableListOf<LabelDraft>()
        compose.setContent {
            TurboistTheme {
                LabelEditorContent(initial = null, onConfirm = { saved += it }, onDismiss = {})
            }
        }

        compose.onNodeWithText(text(R.string.common_create)).performClick()

        assertEquals(emptyList(), saved)
    }

    // --- one label's own screen ---

    @Test
    fun `a label that is gone says so rather than showing the last thing it held`() {
        compose.setContent {
            TurboistTheme {
                LabelTasksScreen(
                    state = LabelDetailUiState(loading = false, label = null),
                    messages = emptyFlow(),
                    callbacks = detailCallbacks {},
                ) { Text("the list") }
            }
        }

        compose.onNodeWithText(text(R.string.page_label_notFound)).assertIsDisplayed()
    }

    @Test
    fun `the header names the label and keeps the work under it`() {
        compose.setContent {
            TurboistTheme {
                LabelTasksScreen(
                    state = LabelDetailUiState(loading = false, label = label(7, "urgent")),
                    messages = emptyFlow(),
                    callbacks = detailCallbacks {},
                ) { Text("the list") }
            }
        }

        compose.onNodeWithText("urgent").assertIsDisplayed()
        compose.onNodeWithText("the list").assertIsDisplayed()
    }

    @Test
    fun `the star asks for the opposite of what the label says now`() {
        val asked = mutableListOf<String>()
        compose.setContent {
            TurboistTheme {
                LabelTasksScreen(
                    state = LabelDetailUiState(loading = false, label = label(7, "urgent", isFavourite = true)),
                    messages = emptyFlow(),
                    callbacks = detailCallbacks { asked += it },
                ) { Text("the list") }
            }
        }

        compose.onNodeWithContentDescription(text(R.string.common_unfavourite)).performClick()

        assertEquals(listOf("favourite"), asked)
    }

    @Test
    fun `the overflow menu offers the label's own actions`() {
        val asked = mutableListOf<String>()
        compose.setContent {
            TurboistTheme {
                LabelTasksScreen(
                    state = LabelDetailUiState(loading = false, label = label(7, "urgent")),
                    messages = emptyFlow(),
                    callbacks = detailCallbacks { asked += it },
                ) { Text("the list") }
            }
        }

        compose.onNodeWithContentDescription(text(R.string.label_actionsAriaLabel)).performClick()
        compose.onNodeWithText(text(R.string.common_edit)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.common_markPrivate)).performClick()

        assertEquals(listOf("private"), asked)
    }

    @Test
    fun `deleting a label warns how many tasks are about to lose it`() {
        val asked = mutableListOf<String>()
        compose.setContent {
            TurboistTheme {
                LabelTasksScreen(
                    state = LabelDetailUiState(loading = false, label = label(7, "urgent"), taggedTasks = 40),
                    messages = emptyFlow(),
                    callbacks = detailCallbacks { asked += it },
                ) { Text("the list") }
            }
        }

        compose.onNodeWithContentDescription(text(R.string.label_actionsAriaLabel)).performClick()
        compose.onNodeWithText(text(R.string.common_delete)).performClick()
        compose.onNodeWithText(text(R.string.page_label_confirmDeleteDesc)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.page_labels_row_total, "40")).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.common_delete)).performClick()

        assertEquals(listOf("delete"), asked)
    }
}

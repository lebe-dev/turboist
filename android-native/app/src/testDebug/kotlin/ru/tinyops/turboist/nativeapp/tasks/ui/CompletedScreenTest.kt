package ru.tinyops.turboist.nativeapp.tasks.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import kotlinx.coroutines.flow.emptyFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.RelativeDay
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.CompletedDaySection
import ru.tinyops.turboist.nativeapp.tasks.CompletedRow
import ru.tinyops.turboist.nativeapp.tasks.CompletedSource
import ru.tinyops.turboist.nativeapp.tasks.CompletedUiState
import ru.tinyops.turboist.nativeapp.tasks.OlderHistory
import ru.tinyops.turboist.nativeapp.tasks.task
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals

/**
 * What the bottom of the completion history says.
 *
 * The list itself is the same rows every other screen draws; what is only true
 * here is the edge, where the device's copy of the history ends. The claim under
 * test is that the edge never lies: with no connection it says so in words, and
 * there is no spinner anywhere on the screen suggesting that something is on its
 * way.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class CompletedScreenTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    @Test
    fun `the history is drawn under the day each task was finished on`() {
        show(state(OlderHistory.EXHAUSTED))

        compose.onNodeWithText(text(R.string.common_today)).assertIsDisplayed()
        compose.onNodeWithText("finished today").assertIsDisplayed()
    }

    @Test
    fun `with no connection the edge says so and nothing on screen is loading`() {
        show(state(OlderHistory.OFFLINE))

        compose.onNodeWithText(text(R.string.native_completed_olderNeedsConnection)).assertIsDisplayed()
        assertEquals(
            0,
            compose
                .onAllNodesWithContentDescription(text(R.string.native_completed_loadingOlder))
                .fetchSemanticsNodes()
                .size,
            "nothing is on its way, so nothing may look like it is",
        )
    }

    @Test
    fun `a history that has all been shown says so`() {
        show(state(OlderHistory.EXHAUSTED))

        compose.onNodeWithText(text(R.string.native_completed_allShown)).assertIsDisplayed()
    }

    @Test
    fun `a refusal by the server can be tried again`() {
        var retries = 0
        show(state(OlderHistory.FAILED), onShowMore = { retries++ })

        compose.onNodeWithText(text(R.string.native_completed_olderUnavailable)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.app_retry)).performClick()

        assertEquals(1, retries)
    }

    @Test
    fun `ticking a fetched line off hands back the line, not just a task id`() {
        var reopened: CompletedRow? = null
        show(state(OlderHistory.EXHAUSTED), onUncomplete = { reopened = it })

        compose.onAllNodesWithContentDescription(text(R.string.task_markIncomplete))[1].performClick()

        assertEquals(CompletedSource.SERVER, reopened?.source)
        assertEquals(90L, reopened?.task?.serverId)
    }

    @Test
    fun `an empty history says what the screen is for`() {
        show(CompletedUiState(loading = false, sections = emptyList(), older = OlderHistory.EXHAUSTED))

        compose.onNodeWithText(text(R.string.page_completed_emptyTitle)).assertIsDisplayed()
    }

    private fun show(
        state: CompletedUiState,
        onShowMore: () -> Unit = {},
        onUncomplete: (CompletedRow) -> Unit = {},
    ) {
        compose.setContent {
            TurboistTheme {
                CompletedScreen(
                    state = state,
                    zone = zone,
                    today = today,
                    messages = emptyFlow(),
                    callbacks =
                        CompletedCallbacks(
                            onUncomplete = onUncomplete,
                            onOpen = { },
                            onShowMore = onShowMore,
                            onRefresh = { },
                        ),
                )
            }
        }
    }

    /** One day of the device's own history, and one line fetched from beyond it. */
    private fun state(older: OlderHistory) =
        CompletedUiState(
            loading = false,
            sections =
                listOf(
                    CompletedDaySection(
                        day = today,
                        relative = RelativeDay.TODAY,
                        rows =
                            listOf(
                                CompletedRow(
                                    task = finished(1, 1, "2026-03-12T09:00:00Z", "finished today"),
                                    projectTitle = null,
                                    source = CompletedSource.REPLICA,
                                ),
                            ),
                    ),
                    CompletedDaySection(
                        day = LocalDate.of(2024, 5, 6),
                        relative = RelativeDay.OTHER,
                        rows =
                            listOf(
                                CompletedRow(
                                    task = finished(NO_LOCAL_ID, 90, "2024-05-06T09:00:00Z", "finished long ago"),
                                    projectTitle = null,
                                    source = CompletedSource.SERVER,
                                ),
                            ),
                    ),
                ),
            older = older,
        )

    private fun finished(
        localId: Long,
        serverId: Long,
        at: String,
        title: String,
    ): Task =
        task(
            localId = localId,
            serverId = serverId,
            title = title,
            status = TaskStatus.COMPLETED,
            completedAt = Instant.parse(at).toEpochMilli(),
        )
}

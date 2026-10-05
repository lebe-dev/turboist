package ru.tinyops.turboist.nativeapp.unsent.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.sync.write.OutboxOpKind
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import ru.tinyops.turboist.nativeapp.unsent.UnsentChange
import ru.tinyops.turboist.nativeapp.unsent.UnsentChangesUiState
import ru.tinyops.turboist.nativeapp.unsent.UnsentReason
import kotlin.test.assertEquals

/**
 * The unsent-changes screen as it is actually drawn.
 *
 * The presenter is checked on its own elsewhere, so what is left here is what
 * the screen is for: that a refused change reads as something a person can
 * understand rather than as an op name and a status code, that clearing the
 * whole pile asks first, and that a change still on its way out carries nothing
 * that would throw it away.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class UnsentChangesScreenTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun `a device holding nothing back says so rather than showing an empty list`() {
        show(UnsentChangesUiState(loading = false))

        compose.onNodeWithText(text(R.string.offline_unsentEmpty)).assertIsDisplayed()
    }

    @Test
    fun `a screen that does not know yet says nothing rather than saying nothing is unsent`() {
        show(UnsentChangesUiState())

        // "Everything has been sent" is a claim, and the queue has not been read
        // yet. Saying it now and taking it back a frame later is worse than the
        // blank moment it would fill.
        compose.onNodeWithText(text(R.string.offline_unsentEmpty)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.native_unsent_setAsideHeading)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.native_unsent_waitingHeading)).assertDoesNotExist()
    }

    @Test
    fun `a refused change reads as what it was, what it was for, and why it was refused`() {
        show(
            UnsentChangesUiState(
                loading = false,
                setAside =
                    listOf(
                        UnsentChange(
                            id = "a",
                            kind = OutboxOpKind.TASK_COMPLETE,
                            target = "Renew the domain",
                            at = 1_000,
                            reason = UnsentReason.BLOCKED,
                            blockers = listOf("Pay the invoice"),
                        ),
                    ),
            ),
        )

        compose.onNodeWithText(text(R.string.offline_unsentOpComplete)).assertIsDisplayed()
        compose.onNodeWithText("Renew the domain").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_unsent_reason_blocked)).assertIsDisplayed()
        // The ids the server named are the only part of the refusal that says
        // which work to go and look at, so they are resolved into names.
        compose.onNodeWithText(text(R.string.native_unsent_blockedBy, "Pay the invoice")).assertIsDisplayed()
    }

    @Test
    fun `a change aimed at something already deleted still says what the change was`() {
        show(
            UnsentChangesUiState(
                loading = false,
                setAside =
                    listOf(
                        UnsentChange(
                            id = "a",
                            kind = OutboxOpKind.TASK_PATCH,
                            target = null,
                            at = 1_000,
                            reason = UnsentReason.TARGET_GONE,
                        ),
                    ),
            ),
        )

        compose.onNodeWithText(text(R.string.native_unsent_op_taskPatch)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_unsent_reason_targetGone)).assertIsDisplayed()
    }

    @Test
    fun `a change queued by a build this one does not know is still listed`() {
        show(
            UnsentChangesUiState(
                loading = false,
                setAside =
                    listOf(
                        UnsentChange(
                            id = "a",
                            kind = null,
                            target = null,
                            at = 1,
                            reason = UnsentReason.UNSENDABLE,
                        ),
                    ),
            ),
        )

        compose.onNodeWithText(text(R.string.native_unsent_op_unknown)).assertIsDisplayed()
    }

    @Test
    fun `discarding one names the change it was next to`() {
        val discarded = mutableListOf<String>()
        show(
            state = UnsentChangesUiState(loading = false, setAside = listOf(refused("a"))),
            onDiscard = { discarded += it.id },
        )

        compose.onNodeWithText(text(R.string.native_unsent_discard)).performClick()

        assertEquals(listOf("a"), discarded)
    }

    @Test
    fun `clearing the whole pile asks first`() {
        var asked = 0
        show(
            state = UnsentChangesUiState(loading = false, setAside = listOf(refused("a"), refused("b"))),
            onAskToDiscardAll = { asked++ },
        )

        compose.onNodeWithText(text(R.string.native_unsent_discardAll)).performClick()

        assertEquals(1, asked)
    }

    @Test
    fun `the confirmation says how many notes are about to go`() {
        var confirmed = 0
        show(
            state =
                UnsentChangesUiState(
                    loading = false,
                    setAside = listOf(refused("a"), refused("b")),
                    confirmingDiscardAll = true,
                ),
            onDiscardAll = { confirmed++ },
        )

        compose.onNodeWithText(text(R.string.native_unsent_discardAllTitle)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_unsent_discardAllBody, 2)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_unsent_discardAllConfirm)).performClick()

        assertEquals(1, confirmed)
    }

    @Test
    fun `a change still on its way out is shown and carries nothing that would throw it away`() {
        show(
            UnsentChangesUiState(
                loading = false,
                waiting =
                    listOf(UnsentChange(id = "b", kind = OutboxOpKind.TASK_CREATE, target = "Buy milk", at = 2)),
            ),
        )

        compose.onNodeWithText(text(R.string.native_unsent_waitingHeading)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.native_unsent_op_taskCreate)).assertIsDisplayed()
        compose.onNodeWithText("Buy milk").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.offline_awaitingSend)).assertIsDisplayed()
        // Discarding a write that is still going to land would be exactly the
        // silent loss this screen exists to prevent.
        compose.onNodeWithText(text(R.string.native_unsent_discard)).assertDoesNotExist()
    }

    private fun refused(id: String): UnsentChange =
        UnsentChange(
            id = id,
            kind = OutboxOpKind.TASK_COMPLETE,
            target = "Renew the domain",
            at = 1_000,
            reason = UnsentReason.CONFLICT,
        )

    private fun show(
        state: UnsentChangesUiState,
        onDiscard: (UnsentChange) -> Unit = {},
        onAskToDiscardAll: () -> Unit = {},
        onDiscardAll: () -> Unit = {},
    ) {
        compose.setContent {
            TurboistTheme {
                UnsentChangesScreen(
                    state = state,
                    callbacks =
                        UnsentChangesCallbacks(
                            onDiscard = onDiscard,
                            onAskToDiscardAll = onAskToDiscardAll,
                            onCancelDiscardAll = {},
                            onDiscardAll = onDiscardAll,
                        ),
                )
            }
        }
    }

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)
}

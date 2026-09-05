package ru.tinyops.turboist.nativeapp.tasks.ui

import android.app.Application
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import kotlin.test.assertEquals

/**
 * Choosing how a task repeats, composed for real.
 *
 * The rule is stored in calendar notation, and everything worth checking is
 * about the gap between that notation and a person: the named choices have to
 * write the notation for the user, the date the rule leads to has to be on
 * screen before the choice is made, a rule typed by hand has to be refused
 * before it is stored rather than ignored afterwards, and a rule written on
 * another client has to survive being opened here.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class RecurrenceSheetTest {
    @get:Rule
    val compose = createComposeRule()

    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val today: LocalDate = LocalDate.of(2026, 3, 12)
    private val saved = mutableListOf<String?>()

    private fun text(resId: Int): String = RuntimeEnvironment.getApplication().getString(resId)

    private fun show(
        rule: String?,
        dueAt: Long? = null,
    ) {
        compose.setContent {
            TurboistTheme {
                RecurrenceSheet(
                    rule = rule,
                    dueAt = dueAt,
                    from = today.atStartOfDay(zone).toInstant().toEpochMilli(),
                    zone = zone,
                    onChange = { saved += it },
                    onDismiss = {},
                )
            }
        }
        compose.waitForIdle()
    }

    @Test
    fun `choosing a named repeat writes the rule behind it`() {
        show(null)

        compose.onNodeWithText(text(R.string.task_recurrence_daily)).performClick()

        assertEquals(listOf<String?>("FREQ=DAILY"), saved)
    }

    @Test
    fun `the weekday choice writes the five working days`() {
        show(null)

        compose.onNodeWithText(text(R.string.native_recurrence_weekdays)).performClick()

        assertEquals(listOf<String?>("FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"), saved)
    }

    @Test
    fun `stopping a task repeating saves no rule at all`() {
        show("FREQ=DAILY")

        compose.onNodeWithText(text(R.string.task_recurrence_noRepeat)).performClick()

        assertEquals(listOf<String?>(null), saved)
    }

    @Test
    fun `the date the task would move to next is on screen`() {
        val dueAt = today.atTime(LocalTime.of(9, 0)).atZone(zone).toInstant().toEpochMilli()

        show("FREQ=DAILY", dueAt = dueAt)

        val expected =
            RuntimeEnvironment.getApplication().getString(
                R.string.native_recurrence_next,
                today.plusDays(1).format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)),
            )
        compose.onNodeWithText(expected).assertIsDisplayed()
    }

    @Test
    fun `a task that does not repeat is offered no next date`() {
        show(null)

        val prefix =
            RuntimeEnvironment.getApplication().getString(R.string.native_recurrence_next, "").trim()
        compose.onAllNodesWithText(prefix, substring = true).assertCountEquals(0)
    }

    @Test
    fun `a rule typed by hand that is not a rule cannot be stored`() {
        // The field opens on the rule the task already carries, so the refusal is
        // on screen before anything is typed — and a rule the device cannot read
        // is one it could not have acted on either.
        show("every other tuesday")

        compose.onNodeWithText(text(R.string.native_recurrence_ruleUnreadable)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.common_save)).assertIsNotEnabled()
        assertEquals(emptyList<String?>(), saved)
    }

    @Test
    fun `a rule with no sentence of its own opens as the rule it is`() {
        // Written elsewhere, honoured in full, and offered back for editing
        // rather than described wrongly or thrown away.
        show("FREQ=MONTHLY;BYDAY=3FR")

        compose.onNodeWithText(text(R.string.native_recurrence_ruleLabel)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.common_save)).assertIsEnabled()
    }

    @Test
    fun `a rule the choices do not cover is spelled out in words`() {
        show("FREQ=DAILY;INTERVAL=3")

        compose
            .onNodeWithText(
                RuntimeEnvironment.getApplication().getString(R.string.task_recurrence_intervalDays, "3"),
            ).assertIsDisplayed()
    }

    @Test
    fun `a rule written by hand is stored on a deliberate tap`() {
        show("FREQ=MONTHLY;BYDAY=3FR")

        compose.onNodeWithText(text(R.string.common_save)).performClick()

        assertEquals(listOf<String?>("FREQ=MONTHLY;BYDAY=3FR"), saved)
    }
}

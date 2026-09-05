package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.sync.write.RecurrenceAdvancer
import ru.tinyops.turboist.core.sync.write.RecurrenceOutcome
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.RecurrencePreset
import ru.tinyops.turboist.nativeapp.tasks.RecurrenceWeekday
import ru.tinyops.turboist.nativeapp.tasks.RecurrenceWording
import ru.tinyops.turboist.nativeapp.tasks.recurrenceWording
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * How often the task comes back.
 *
 * Five named choices cover what people actually ask for; the sixth is the
 * calendar notation itself, because a task manager that offers only five is one
 * a user with a sixth need has to leave — and because a rule written on another
 * client has to survive being looked at here. A named choice is applied on the
 * tap, like every other choice on this screen. A rule written by hand is applied
 * deliberately, because half a rule is not a rule.
 *
 * The seven choices are a list in a sheet rather than a run of chips on the
 * screen: exactly one of them is true at a time, which is what a list of radio
 * buttons says and what a wrapping row of chips does not — and the row was three
 * lines tall on a phone, for a field most tasks leave at "never".
 *
 * Underneath sits the date the rule would move the task to next, worked out by
 * the same calculator the completion path uses. That is the answer people are
 * really after, and having it before the choice is made is what makes the
 * notation usable at all.
 */
@Composable
fun RecurrenceSheet(
    rule: String?,
    dueAt: Long?,
    from: Long,
    zone: ZoneId,
    onChange: (String?) -> Unit,
    onDismiss: () -> Unit,
) {
    var draft by remember(rule) { mutableStateOf(rule.orEmpty()) }
    var writingByHand by remember(rule) {
        mutableStateOf(rule != null && rule.isNotBlank() && RecurrencePreset.of(rule) == null)
    }
    val wording = recurrenceWording(draft)
    val unreadable = wording == RecurrenceWording.Unreadable

    FieldSheet(title = stringResource(R.string.page_task_repeat), onDismiss = onDismiss) {
        SheetChoiceRow(
            text = stringResource(R.string.task_recurrence_noRepeat),
            selected = !writingByHand && draft.isBlank(),
            onSelect = {
                draft = ""
                writingByHand = false
                onChange(null)
                onDismiss()
            },
        )
        for (preset in RecurrencePreset.entries) {
            SheetChoiceRow(
                text = presetLabel(preset),
                selected = !writingByHand && RecurrencePreset.of(draft) == preset,
                onSelect = {
                    draft = preset.rule
                    writingByHand = false
                    onChange(preset.rule)
                    onDismiss()
                },
            )
        }
        HorizontalDivider(modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp))
        SheetChoiceRow(
            text = stringResource(R.string.native_recurrence_custom),
            selected = writingByHand,
            supporting =
                recurrenceLabel(wording).takeIf { writingByHand && draft.isNotBlank() && !unreadable },
            onSelect = { writingByHand = true },
        )
        if (writingByHand) {
            HandWrittenRule(
                draft = draft,
                unreadable = unreadable,
                onType = { draft = it },
                onApply = {
                    onChange(draft.trim().takeIf { it.isNotEmpty() })
                    onDismiss()
                },
            )
        }
        if (!unreadable && draft.isNotBlank()) {
            Text(
                text = nextOccurrenceLabel(draft, dueAt, from, zone),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp),
            )
        }
    }
}

/**
 * The rule as notation, with the verdict on it underneath.
 *
 * Applied on a deliberate tap rather than as it is typed: every prefix of a good
 * rule is a bad rule, and queuing each of them would fill the outbox with
 * half-written instructions. The tap is refused outright while the rule is not
 * one, so a rule that cannot be acted on cannot be stored either.
 */
@Composable
private fun HandWrittenRule(
    draft: String,
    unreadable: Boolean,
    onType: (String) -> Unit,
    onApply: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp)) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            OutlinedTextField(
                value = draft,
                onValueChange = onType,
                label = { Text(stringResource(R.string.native_recurrence_ruleLabel)) },
                singleLine = true,
                isError = unreadable,
                modifier = Modifier.weight(1f),
            )
            TextButton(enabled = !unreadable && draft.isNotBlank(), onClick = onApply) {
                Text(stringResource(R.string.common_save))
            }
        }
        Text(
            text =
                stringResource(
                    if (unreadable) R.string.native_recurrence_ruleUnreadable else R.string.native_recurrence_ruleHelp,
                ),
            style = MaterialTheme.typography.bodySmall,
            color = if (unreadable) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** The date the rule would move the task to, or the news that there is none. */
@Composable
internal fun nextOccurrenceLabel(
    rule: String,
    dueAt: Long?,
    from: Long,
    zone: ZoneId,
): String =
    when (val outcome = RecurrenceAdvancer(zone).after(rule, dueAt, from)) {
        RecurrenceOutcome.Ended -> stringResource(R.string.native_recurrence_noNext)
        RecurrenceOutcome.Unknown -> stringResource(R.string.native_recurrence_ruleUnreadable)
        is RecurrenceOutcome.Next ->
            stringResource(
                R.string.native_recurrence_next,
                Instant
                    .ofEpochMilli(outcome.dueAt)
                    .atZone(zone)
                    .toLocalDate()
                    .format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)),
            )
    }

/** How a repeat rule reads, in the user's own language. */
@Composable
fun recurrenceLabel(wording: RecurrenceWording): String =
    when (wording) {
        RecurrenceWording.Never -> stringResource(R.string.task_recurrence_noRepeat)
        RecurrenceWording.EveryDay -> stringResource(R.string.task_recurrence_daily)
        is RecurrenceWording.EveryNumberOfDays ->
            stringResource(R.string.task_recurrence_intervalDays, wording.days.toString())
        RecurrenceWording.EveryWeek -> stringResource(R.string.task_recurrence_weekly)
        RecurrenceWording.EveryWeekday -> stringResource(R.string.native_recurrence_weekdays)
        is RecurrenceWording.OnDaysOfTheWeek -> wording.days.map { weekdayLabel(it) }.joinToString(", ")
        RecurrenceWording.EveryMonth -> stringResource(R.string.native_recurrence_monthly)
        is RecurrenceWording.OnDayOfTheMonth ->
            stringResource(R.string.task_recurrence_monthlyDay, wording.day.toString())
        RecurrenceWording.EveryYear -> stringResource(R.string.native_recurrence_yearly)
        // Understood by the calculator but with no sentence of its own. Shown as
        // written, which is at least true.
        is RecurrenceWording.AsWritten -> wording.rule
        RecurrenceWording.Unreadable -> stringResource(R.string.native_recurrence_ruleUnreadable)
    }

@Composable
private fun presetLabel(preset: RecurrencePreset): String =
    when (preset) {
        RecurrencePreset.DAILY -> stringResource(R.string.task_recurrence_daily)
        RecurrencePreset.WEEKDAYS -> stringResource(R.string.native_recurrence_weekdays)
        RecurrencePreset.WEEKLY -> stringResource(R.string.task_recurrence_weekly)
        RecurrencePreset.MONTHLY -> stringResource(R.string.native_recurrence_monthly)
        RecurrencePreset.YEARLY -> stringResource(R.string.native_recurrence_yearly)
    }

@Composable
private fun weekdayLabel(day: RecurrenceWeekday): String =
    when (day) {
        RecurrenceWeekday.MONDAY -> stringResource(R.string.task_recurrence_weekday_mo)
        RecurrenceWeekday.TUESDAY -> stringResource(R.string.task_recurrence_weekday_tu)
        RecurrenceWeekday.WEDNESDAY -> stringResource(R.string.task_recurrence_weekday_we)
        RecurrenceWeekday.THURSDAY -> stringResource(R.string.task_recurrence_weekday_th)
        RecurrenceWeekday.FRIDAY -> stringResource(R.string.task_recurrence_weekday_fr)
        RecurrenceWeekday.SATURDAY -> stringResource(R.string.task_recurrence_weekday_sa)
        RecurrenceWeekday.SUNDAY -> stringResource(R.string.task_recurrence_weekday_su)
    }

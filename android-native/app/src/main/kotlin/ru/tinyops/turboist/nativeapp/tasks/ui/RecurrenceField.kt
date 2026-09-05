package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.FilterChip
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
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
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
 * Underneath sits the date the rule would move the task to next, worked out by
 * the same calculator the completion path uses. That is the answer people are
 * really after, and having it before the choice is made is what makes the
 * notation usable at all.
 */
@Composable
fun RecurrenceField(
    rule: String?,
    dueAt: Long?,
    from: Long,
    zone: ZoneId,
    onChange: (String?) -> Unit,
    modifier: Modifier = Modifier,
) {
    var draft by remember(rule) { mutableStateOf(rule.orEmpty()) }
    var writingByHand by remember(rule) {
        mutableStateOf(rule != null && rule.isNotBlank() && RecurrencePreset.of(rule) == null)
    }
    val wording = recurrenceWording(draft)
    val unreadable = wording == RecurrenceWording.Unreadable

    Column(modifier = modifier.fillMaxWidth()) {
        ChoiceRow(label = stringResource(R.string.page_task_repeat)) {
            RecurrenceChoice(
                text = stringResource(R.string.task_recurrence_noRepeat),
                selected = !writingByHand && draft.isBlank(),
                onSelect = {
                    draft = ""
                    writingByHand = false
                    onChange(null)
                },
            )
            for (preset in RecurrencePreset.entries) {
                RecurrenceChoice(
                    text = presetLabel(preset),
                    selected = !writingByHand && RecurrencePreset.of(draft) == preset,
                    onSelect = {
                        draft = preset.rule
                        writingByHand = false
                        onChange(preset.rule)
                    },
                )
            }
            RecurrenceChoice(
                text = stringResource(R.string.native_recurrence_custom),
                selected = writingByHand,
                onSelect = { writingByHand = true },
            )
        }
        if (writingByHand) {
            HandWrittenRule(
                draft = draft,
                unreadable = unreadable,
                onType = { draft = it },
                onApply = { onChange(draft.trim().takeIf { it.isNotEmpty() }) },
            )
        }
        if (!unreadable && draft.isNotBlank()) {
            // What the rule says, but only where the choices above do not
            // already say it — a chip and a sentence repeating each other is
            // noise, and a rule that matches no chip is the one nobody can read.
            if (RecurrencePreset.of(draft) == null) {
                Text(
                    text = recurrenceLabel(wording),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 2.dp),
                )
            }
            Text(
                text = nextOccurrenceLabel(draft, dueAt, from, zone),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 2.dp),
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
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            OutlinedTextField(
                value = draft,
                onValueChange = onType,
                label = { Text(stringResource(R.string.native_recurrence_ruleLabel)) },
                singleLine = true,
                isError = unreadable,
                modifier = Modifier.weight(1f),
            )
            TextButton(enabled = !unreadable, onClick = onApply) {
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

@Composable
private fun RecurrenceChoice(
    text: String,
    selected: Boolean,
    onSelect: () -> Unit,
) {
    FilterChip(
        selected = selected,
        onClick = onSelect,
        label = { Text(text) },
        modifier = Modifier.semantics { contentDescription = text },
    )
}

/** The date the rule would move the task to, or the news that there is none. */
@Composable
private fun nextOccurrenceLabel(
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

package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.FlowRowScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TimePicker
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberTimePickerState
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
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.nativeapp.R
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * One fact about a task, with its name on the left and its value on the right.
 *
 * The whole row is the control where there is something to open, because a value
 * that can be changed should be reachable by aiming at the value itself rather
 * than at an icon next to it.
 */
@Composable
fun DetailRow(
    label: String,
    value: String,
    modifier: Modifier = Modifier,
    onClick: (() -> Unit)? = null,
) {
    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
                .padding(horizontal = 16.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(text = value, style = MaterialTheme.typography.bodyMedium)
    }
}

/** A yes-or-no fact about a task, changed in place. */
@Composable
fun DetailSwitchRow(
    label: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier.fillMaxWidth().padding(start = 16.dp, end = 8.dp, top = 4.dp, bottom = 4.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Switch(
            checked = checked,
            onCheckedChange = onCheckedChange,
            modifier = Modifier.semantics { contentDescription = label },
        )
    }
}

/**
 * The four priority levels, as one row of choices.
 *
 * Shown open rather than behind a menu: a priority is the field most often
 * changed on this screen, and the current level reads at a glance only when the
 * others are beside it.
 *
 * [locked] is set when the task's project stands in the daily plan and decides
 * the priority itself. The levels stay visible so the current one still reads,
 * but none of them can be picked and the reason is spelled out underneath —
 * an unexplained dead control would look like a broken screen.
 */
@Composable
fun PrioritySelector(
    priority: Priority,
    onSelect: (Priority) -> Unit,
    modifier: Modifier = Modifier,
    locked: Boolean = false,
) {
    ChoiceRow(
        label = stringResource(R.string.page_task_priority),
        modifier = modifier,
        note = stringResource(R.string.page_task_priorityLockedByTroiki).takeIf { locked },
    ) {
        for (level in PRIORITY_CHOICES) {
            val text = priorityLabel(level)
            FilterChip(
                selected = level == priority,
                enabled = !locked,
                onClick = { onSelect(level) },
                label = { Text(text) },
                modifier = Modifier.semantics { contentDescription = text },
            )
        }
    }
}

/** The phase of the day a task is meant for. */
@Composable
fun DayPartSelector(
    dayPart: DayPart,
    onSelect: (DayPart) -> Unit,
    modifier: Modifier = Modifier,
) {
    ChoiceRow(
        label = stringResource(R.string.page_task_dayPart),
        modifier = modifier,
    ) {
        for (part in DAY_PART_CHOICES) {
            val text = stringResource(dayPartLabel(part))
            FilterChip(
                selected = part == dayPart,
                onClick = { onSelect(part) },
                label = { Text(text) },
                modifier = Modifier.semantics { contentDescription = text },
            )
        }
    }
}

/**
 * Where the task stands in the week's planning: committed to it, parked out of
 * it, or neither.
 */
@Composable
fun PlanSelector(
    planState: PlanState,
    onSelect: (PlanState) -> Unit,
    modifier: Modifier = Modifier,
) {
    ChoiceRow(
        label = stringResource(R.string.native_task_plan),
        modifier = modifier,
    ) {
        for (state in PLAN_CHOICES) {
            val text = planLabel(state)
            FilterChip(
                selected = state == planState,
                onClick = { onSelect(state) },
                label = { Text(text) },
                modifier = Modifier.semantics { contentDescription = text },
            )
        }
    }
}

/**
 * Every label in the workspace, with the ones on this task marked.
 *
 * The whole set is offered rather than a search box: labels are few and
 * deliberately so, and a set the user can see is a set they can keep tidy.
 */
@Composable
fun LabelSelector(
    known: List<Label>,
    selected: List<String>,
    onChange: (List<String>) -> Unit,
    modifier: Modifier = Modifier,
) {
    if (known.isEmpty()) return
    ChoiceRow(
        label = stringResource(R.string.page_task_labels),
        modifier = modifier,
    ) {
        for (label in known) {
            val on = label.name in selected
            FilterChip(
                selected = on,
                onClick = { onChange(if (on) selected - label.name else selected + label.name) },
                label = { Text(label.name) },
                modifier = Modifier.semantics { contentDescription = label.name },
            )
        }
    }
}

/**
 * A named row of choices, wrapping onto as many lines as it needs.
 *
 * [note] is a line under the choices, for when the row cannot be used and the
 * user is owed the reason.
 */
@Composable
internal fun ChoiceRow(
    label: String,
    modifier: Modifier = Modifier,
    note: String? = null,
    content: @Composable FlowRowScope.() -> Unit,
) {
    Column(modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp)) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(2.dp),
            content = content,
        )
        if (note != null) {
            Text(
                text = note,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/**
 * A date the user can set, move or empty.
 *
 * Emptying is offered as its own control rather than as an option inside the
 * picker, because "no date" is a decision about the task and not a date the user
 * failed to pick.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DateField(
    label: String,
    date: LocalDate?,
    clearLabel: String,
    onPick: (LocalDate?) -> Unit,
    modifier: Modifier = Modifier,
) {
    var picking by remember { mutableStateOf(false) }
    val shown =
        date?.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
            ?: stringResource(R.string.common_noDate)
    Row(modifier = modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        DetailRow(
            label = label,
            value = shown,
            onClick = { picking = true },
            modifier = Modifier.weight(1f),
        )
        if (date != null) {
            TextButton(onClick = { onPick(null) }) { Text(clearLabel) }
        }
    }
    if (!picking) return
    // The picker speaks in UTC midnights, so the day is read back at that offset
    // and only then handed to the caller, which knows the user's own zone.
    val state =
        rememberDatePickerState(
            initialSelectedDateMillis = date?.atStartOfDay(ZoneOffset.UTC)?.toInstant()?.toEpochMilli(),
        )
    DatePickerDialog(
        onDismissRequest = { picking = false },
        confirmButton = {
            TextButton(
                onClick = {
                    picking = false
                    val picked = state.selectedDateMillis ?: return@TextButton
                    onPick(Instant.ofEpochMilli(picked).atZone(ZoneOffset.UTC).toLocalDate())
                },
            ) { Text(stringResource(R.string.common_save)) }
        },
        dismissButton = {
            TextButton(onClick = { picking = false }) { Text(stringResource(R.string.common_cancel)) }
        },
    ) {
        DatePicker(state = state)
    }
}

/**
 * The time of day attached to a date.
 *
 * Only offered once there is a date to attach it to: a time on no day is not
 * something the product can store or the user can act on.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TimeField(
    label: String,
    time: LocalTime?,
    onPick: (LocalTime?) -> Unit,
    modifier: Modifier = Modifier,
) {
    var picking by remember { mutableStateOf(false) }
    Row(modifier = modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        DetailRow(
            label = label,
            value =
                time?.format(DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT))
                    ?: stringResource(R.string.native_task_unset),
            onClick = { picking = true },
            modifier = Modifier.weight(1f),
        )
        if (time != null) {
            TextButton(onClick = { onPick(null) }) { Text(stringResource(R.string.native_task_clearTime)) }
        }
    }
    if (!picking) return
    val state = rememberTimePickerState(initialHour = time?.hour ?: 9, initialMinute = time?.minute ?: 0)
    AlertDialog(
        onDismissRequest = { picking = false },
        confirmButton = {
            TextButton(
                onClick = {
                    picking = false
                    onPick(LocalTime.of(state.hour, state.minute))
                },
            ) { Text(stringResource(R.string.common_save)) }
        },
        dismissButton = {
            TextButton(onClick = { picking = false }) { Text(stringResource(R.string.common_cancel)) }
        },
        text = { TimePicker(state = state) },
    )
}

/** A moment, written the way the device writes dates and times. */
@Composable
fun momentLabel(
    at: Long?,
    zone: ZoneId,
): String {
    at ?: return stringResource(R.string.native_task_unset)
    return Instant
        .ofEpochMilli(at)
        .atZone(zone)
        .format(DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT))
}

/** The short code a priority is shown as, the same scale the web client uses. */
@Composable
fun priorityLabel(priority: Priority): String =
    when (priority) {
        Priority.HIGH -> stringResource(R.string.native_priority_p1)
        Priority.MEDIUM -> stringResource(R.string.native_priority_p2)
        Priority.LOW -> stringResource(R.string.native_priority_p3)
        Priority.NONE -> stringResource(R.string.native_priority_p4)
        // A level this build does not recognise is shown as unset rather than
        // guessed at: the task must still render, and naming it wrongly is worse
        // than admitting the app cannot read it.
        Priority.UNKNOWN -> stringResource(R.string.native_task_unset)
    }

@Composable
fun planLabel(planState: PlanState): String =
    when (planState) {
        PlanState.WEEK -> stringResource(R.string.task_weekPlannedLabel)
        PlanState.BACKLOG -> stringResource(R.string.task_backlogLabel)
        PlanState.NONE -> stringResource(R.string.native_task_planNone)
        PlanState.UNKNOWN -> stringResource(R.string.native_task_unset)
    }

/** The levels a user can choose, most urgent first. The unreadable one is not offered. */
private val PRIORITY_CHOICES = listOf(Priority.HIGH, Priority.MEDIUM, Priority.LOW, Priority.NONE)

private val DAY_PART_CHOICES = listOf(DayPart.MORNING, DayPart.AFTERNOON, DayPart.EVENING, DayPart.NONE)

private val PLAN_CHOICES = listOf(PlanState.WEEK, PlanState.BACKLOG, PlanState.NONE)

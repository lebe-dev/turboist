package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.outlined.CalendarMonth
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TimePicker
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.material3.rememberTimePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.nativeapp.R
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * A field of the task, edited in a sheet raised over the screen.
 *
 * Everything the screen used to lay out in the open — seven repeat chips, every
 * label in the workspace, a date field with its own clear button beside it —
 * lives behind one of these. The rule is the field's shape rather than how often
 * it is used: a choice of one out of several is a list of choices, and a list is
 * legible in a sheet and a wall in a column.
 *
 * The sheet always says which field it is about, because it covers the row that
 * was tapped and the user has nothing else to go on.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun FieldSheet(
    title: String,
    onDismiss: () -> Unit,
    content: @Composable ColumnScope.() -> Unit,
) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
    ) {
        Column(
            modifier =
                Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(bottom = 24.dp),
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.padding(start = 24.dp, end = 24.dp, top = 4.dp, bottom = 12.dp),
            )
            content()
        }
    }
}

/**
 * One choice out of several, in a sheet.
 *
 * The whole row is the control rather than the button on it: a target the width
 * of the sheet is one nobody has to aim at, and the wording beside a radio
 * button is what a person is actually reading when they pick.
 */
@Composable
internal fun SheetChoiceRow(
    text: String,
    selected: Boolean,
    onSelect: () -> Unit,
    modifier: Modifier = Modifier,
    supporting: String? = null,
) {
    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .selectable(selected = selected, onClick = onSelect)
                .defaultMinSize(minHeight = 56.dp)
                .padding(horizontal = 24.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(16.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = null)
        Column(modifier = Modifier.weight(1f)) {
            Text(text = text, style = MaterialTheme.typography.bodyLarge)
            if (supporting != null) {
                Text(
                    text = supporting,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/** A row in a sheet that opens something else, or does something. */
@Composable
internal fun SheetActionRow(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    leading: ImageVector? = null,
    supporting: String? = null,
    enabled: Boolean = true,
    destructive: Boolean = false,
) {
    val content =
        when {
            destructive -> MaterialTheme.colorScheme.error
            enabled -> MaterialTheme.colorScheme.onSurface
            else -> MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = DISABLED_ALPHA)
        }
    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .clickable(enabled = enabled, onClick = onClick)
                .defaultMinSize(minHeight = 56.dp)
                .padding(horizontal = 24.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(16.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (leading != null) {
            Icon(imageVector = leading, contentDescription = null, tint = content)
        }
        Column(modifier = Modifier.weight(1f)) {
            Text(text = text, style = MaterialTheme.typography.bodyLarge, color = content)
            if (supporting != null) {
                Text(
                    text = supporting,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/**
 * A date the task carries, with the time of day attached to it.
 *
 * The three dates people actually pick are offered as one tap each, because
 * "tomorrow" is a decision and opening a calendar to find out which day that is
 * is not. The calendar is still there for the fourth case.
 *
 * Emptying is offered as its own control rather than as an option inside the
 * picker, because "no date" is a decision about the task and not a date the user
 * failed to pick. The time is only offered once there is a date to attach it to:
 * a time on no day is not something the product can store or the user can act
 * on.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun TaskDateSheet(
    title: String,
    date: LocalDate?,
    time: LocalTime?,
    today: LocalDate,
    onPickDate: (LocalDate?) -> Unit,
    onPickTime: (LocalTime?) -> Unit,
    onDismiss: () -> Unit,
) {
    var pickingDate by remember { mutableStateOf(false) }
    var pickingTime by remember { mutableStateOf(false) }

    FieldSheet(title = title, onDismiss = onDismiss) {
        FlowRow(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            QuickDateChip(stringResource(R.string.common_today), today, date, onPickDate)
            QuickDateChip(stringResource(R.string.common_tomorrow), today.plusDays(1), date, onPickDate)
            QuickDateChip(
                text = stringResource(R.string.native_task_inAWeek),
                day = today.plusWeeks(1),
                current = date,
                onPick = onPickDate,
            )
        }
        SheetActionRow(
            text = stringResource(R.string.native_task_pickDate),
            leading = Icons.Outlined.CalendarMonth,
            supporting = date?.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)),
            onClick = { pickingDate = true },
        )
        if (date != null) {
            SheetActionRow(
                text = stringResource(R.string.native_task_time),
                leading = Icons.Outlined.Schedule,
                supporting =
                    time?.format(DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT))
                        ?: stringResource(R.string.native_task_unset),
                onClick = { pickingTime = true },
            )
        }
        HorizontalDivider(modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp))
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (date != null) {
                TextButton(
                    onClick = {
                        onPickDate(null)
                        onDismiss()
                    },
                ) { Text(stringResource(R.string.task_actions_clearDate)) }
            }
            Button(onClick = onDismiss) { Text(stringResource(R.string.native_task_done)) }
        }
    }

    if (pickingDate) {
        // The picker speaks in UTC midnights, so the day is read back at that
        // offset and only then handed to the caller, which knows the user's zone.
        val state =
            rememberDatePickerState(
                initialSelectedDateMillis = date?.atStartOfDay(ZoneOffset.UTC)?.toInstant()?.toEpochMilli(),
            )
        DatePickerDialog(
            onDismissRequest = { pickingDate = false },
            confirmButton = {
                TextButton(
                    onClick = {
                        pickingDate = false
                        val picked = state.selectedDateMillis ?: return@TextButton
                        onPickDate(Instant.ofEpochMilli(picked).atZone(ZoneOffset.UTC).toLocalDate())
                    },
                ) { Text(stringResource(R.string.common_save)) }
            },
            dismissButton = {
                TextButton(onClick = { pickingDate = false }) { Text(stringResource(R.string.common_cancel)) }
            },
        ) {
            DatePicker(state = state)
        }
    }

    if (!pickingTime) return
    val state = rememberTimePickerState(initialHour = time?.hour ?: DEFAULT_HOUR, initialMinute = time?.minute ?: 0)
    AlertDialog(
        onDismissRequest = { pickingTime = false },
        confirmButton = {
            TextButton(
                onClick = {
                    pickingTime = false
                    onPickTime(LocalTime.of(state.hour, state.minute))
                },
            ) { Text(stringResource(R.string.common_save)) }
        },
        dismissButton = {
            if (time != null) {
                TextButton(
                    onClick = {
                        pickingTime = false
                        onPickTime(null)
                    },
                ) { Text(stringResource(R.string.native_task_clearTime)) }
            }
        },
        text = { TimePicker(state = state) },
    )
}

@Composable
private fun QuickDateChip(
    text: String,
    day: LocalDate,
    current: LocalDate?,
    onPick: (LocalDate?) -> Unit,
) {
    FilterChip(
        selected = current == day,
        onClick = { onPick(day) },
        label = { Text(text) },
        modifier = Modifier.semantics { contentDescription = text },
    )
}

/** The phase of the day the task is meant for. */
@Composable
internal fun DayPartSheet(
    dayPart: DayPart,
    onSelect: (DayPart) -> Unit,
    onDismiss: () -> Unit,
) {
    FieldSheet(title = stringResource(R.string.page_task_dayPart), onDismiss = onDismiss) {
        for (part in DAY_PART_CHOICES) {
            SheetChoiceRow(
                text = stringResource(dayPartLabel(part)),
                selected = part == dayPart,
                onSelect = {
                    onSelect(part)
                    onDismiss()
                },
            )
        }
    }
}

/**
 * Where the task stands in the week's planning: committed to it, parked out of
 * it, or neither.
 */
@Composable
internal fun PlanSheet(
    planState: PlanState,
    onSelect: (PlanState) -> Unit,
    onDismiss: () -> Unit,
) {
    FieldSheet(title = stringResource(R.string.native_task_plan), onDismiss = onDismiss) {
        for (state in PLAN_CHOICES) {
            SheetChoiceRow(
                text = planLabel(state),
                selected = state == planState,
                onSelect = {
                    onSelect(state)
                    onDismiss()
                },
            )
        }
    }
}

/**
 * Every label in the workspace, with the ones on this task marked.
 *
 * The whole set lives here rather than on the screen because a workspace with
 * twenty labels turned the page into a wall of chips through which the task's
 * own two were indistinguishable. On the screen a label is a fact about the
 * task; in here it is a choice, and the search box is what makes a long list of
 * choices usable.
 */
@Composable
internal fun LabelsSheet(
    known: List<Label>,
    selected: List<String>,
    onChange: (List<String>) -> Unit,
    onDismiss: () -> Unit,
) {
    var query by remember { mutableStateOf("") }
    val matches = known.filter { it.name.contains(query.trim(), ignoreCase = true) }

    FieldSheet(title = stringResource(R.string.page_task_labels), onDismiss = onDismiss) {
        Text(
            text = stringResource(R.string.native_task_labelsSelected, selected.size),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(start = 24.dp, end = 24.dp, bottom = 12.dp),
        )
        OutlinedTextField(
            value = query,
            onValueChange = { query = it },
            singleLine = true,
            placeholder = { Text(stringResource(R.string.native_task_findLabel)) },
            leadingIcon = { Icon(imageVector = Icons.Filled.Search, contentDescription = null) },
            modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp),
        )
        FlowRow(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp, vertical = 12.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            for (label in matches) {
                val on = label.name in selected
                FilterChip(
                    selected = on,
                    onClick = { onChange(if (on) selected - label.name else selected + label.name) },
                    label = { Text(label.name) },
                    modifier = Modifier.semantics { contentDescription = label.name },
                )
            }
        }
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
        ) {
            Button(onClick = onDismiss) { Text(stringResource(R.string.native_task_done)) }
        }
    }
}

/** How a date reads on a row: the day itself, and what it is to the user today. */
@Composable
internal fun dateLabel(
    date: LocalDate?,
    today: LocalDate,
): String {
    date ?: return stringResource(R.string.common_noDate)
    val day = date.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
    return when (date) {
        today -> stringResource(R.string.native_task_dayNamed, stringResource(R.string.common_today), day)
        today.plusDays(1) ->
            stringResource(R.string.native_task_dayNamed, stringResource(R.string.common_tomorrow), day)
        else -> day
    }
}

/** The time of day attached to a date, or nothing at all when none is set. */
@Composable
internal fun timeLabel(time: LocalTime?): String? = time?.format(DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT))

/** The hour a time picker opens on when the task has no time of its own. */
private const val DEFAULT_HOUR = 9

/** How much of itself a row that cannot be used keeps. */
private const val DISABLED_ALPHA = 0.5f

package ru.tinyops.turboist.nativeapp.quickadd.ui

import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.ui.ChoiceColors
import ru.tinyops.turboist.nativeapp.tasks.ui.ConnectedChoiceGroup
import ru.tinyops.turboist.nativeapp.tasks.ui.futureSelectableDates
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter

/** The four answers to "when is it for", in the order the control offers them. */
private enum class DueChoice { NONE, TODAY, TOMORROW, DATE }

/**
 * No date, today, tomorrow, or a day off the calendar, as one connected control.
 *
 * Under the focus lock "today" carries a padlock, so the restriction reads as
 * belonging to that choice; when it is the choice that clashes, it is drawn in
 * the lock's colour rather than as an ordinary pick.
 */
@Composable
internal fun DueChoices(
    dueDate: LocalDate?,
    today: LocalDate,
    locked: Boolean,
    conflict: Boolean,
    onPick: (LocalDate?) -> Unit,
) {
    var picking by remember { mutableStateOf(false) }
    val current = dueChoiceOf(dueDate, today)
    val tertiary = MaterialTheme.colorScheme.tertiary
    val pickedLabel = dueDate?.takeIf { current == DueChoice.DATE }?.format(SHORT_DATE)
    ConnectedChoiceGroup(
        options = DueChoice.entries,
        selected = current,
        onSelect = { choice ->
            // Choosing what is already chosen keeps it: in a connected control a
            // second tap reads as "yes, that one", not as "clear it".
            if (choice == DueChoice.DATE) {
                picking = true
            } else if (choice != current) {
                onPick(choice.day(today))
            }
        },
        label = { choice -> dueChoiceLabel(choice, pickedLabel) },
        selectedColors = { choice ->
            if (conflict && choice == DueChoice.TODAY) {
                ChoiceColors(tertiary.copy(alpha = CONFLICT_FILL_ALPHA), tertiary)
            } else {
                ChoiceColors(MaterialTheme.colorScheme.primaryContainer, MaterialTheme.colorScheme.onPrimaryContainer)
            }
        },
        leading = if (locked) ::TodayPadlock else null,
    )
    if (picking) {
        DueDatePickerDialog(
            dueDate = dueDate,
            today = today,
            onDismiss = { picking = false },
            onPick = { day ->
                picking = false
                if (day != dueDate) onPick(day)
            },
        )
    }
}

private fun dueChoiceOf(
    dueDate: LocalDate?,
    today: LocalDate,
): DueChoice =
    when (dueDate) {
        null -> DueChoice.NONE
        today -> DueChoice.TODAY
        today.plusDays(1) -> DueChoice.TOMORROW
        else -> DueChoice.DATE
    }

/** The day a fixed choice stands for; a picked date is answered by the calendar instead. */
private fun DueChoice.day(today: LocalDate): LocalDate? =
    when (this) {
        DueChoice.TODAY -> today
        DueChoice.TOMORROW -> today.plusDays(1)
        DueChoice.NONE, DueChoice.DATE -> null
    }

@Composable
private fun dueChoiceLabel(
    choice: DueChoice,
    pickedLabel: String?,
): String =
    when (choice) {
        DueChoice.NONE -> stringResource(R.string.native_quickAdd_noDue)
        DueChoice.TODAY -> stringResource(R.string.common_today)
        DueChoice.TOMORROW -> stringResource(R.string.common_tomorrow)
        DueChoice.DATE -> pickedLabel ?: stringResource(R.string.native_quickAdd_pickDate)
    }

@Composable
private fun TodayPadlock(choice: DueChoice) {
    if (choice == DueChoice.TODAY) {
        Icon(Icons.Outlined.Lock, contentDescription = null, modifier = Modifier.size(14.dp))
    }
}

/**
 * The calendar behind the "date" choice. It speaks in UTC midnights, so the day
 * is read back at that offset and only then handed on, to a caller that knows
 * the user's own zone.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DueDatePickerDialog(
    dueDate: LocalDate?,
    today: LocalDate,
    onDismiss: () -> Unit,
    onPick: (LocalDate) -> Unit,
) {
    val pickerState =
        rememberDatePickerState(
            initialSelectedDateMillis = dueDate?.atStartOfDay(ZoneOffset.UTC)?.toInstant()?.toEpochMilli(),
            selectableDates = remember(today) { futureSelectableDates(today) },
        )
    val picked = pickerState.selectedDateMillis?.let { Instant.ofEpochMilli(it).atZone(ZoneOffset.UTC).toLocalDate() }
    DatePickerDialog(
        onDismissRequest = onDismiss,
        confirmButton = {
            TextButton(
                enabled = picked != null && !picked.isBefore(today),
                onClick = { picked?.let(onPick) },
            ) { Text(stringResource(R.string.common_save)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    ) {
        DatePicker(state = pickerState)
    }
}

/** A picked day as short as it can be said: the control gives it a quarter of the width. */
private val SHORT_DATE: DateTimeFormatter = DateTimeFormatter.ofPattern("d MMM")

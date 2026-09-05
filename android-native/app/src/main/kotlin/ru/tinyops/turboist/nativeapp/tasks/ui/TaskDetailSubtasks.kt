package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import java.time.LocalDate
import java.time.ZoneId

/**
 * The work under a task, and the way to add more of it.
 *
 * The rows are the same ones every list draws, so a subtask reads and behaves
 * here exactly as it does on the day it is due. Finished work is kept in a block
 * of its own: it is worth being able to see, and worth being out of the way.
 *
 * The bar across the top is the one thing this screen can say that a list
 * cannot: how much of the work under this task is done. It is drawn only when
 * there is work to measure — a bar at zero of zero is a decoration.
 *
 * A task in the inbox has no subtasks and cannot be given any — the inbox holds
 * loose captures — so the notice explains that instead of offering a field that
 * would be refused.
 */
@Composable
fun TaskDetailSubtasks(
    open: List<TaskListRow>,
    done: List<TaskListRow>,
    inInbox: Boolean,
    zone: ZoneId,
    today: LocalDate,
    onToggleComplete: (Task) -> Unit,
    onOpen: (Task) -> Unit,
    onAdd: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val total = open.size + done.size

    SectionHeading(text = stringResource(R.string.page_task_subtasks), modifier = modifier)
    DetailCard {
        if (total > 0) {
            SubtaskProgress(finished = done.size, total = total)
        }
        for (row in open) {
            SubtaskRow(row, zone, today, onToggleComplete, onOpen)
        }
        if (done.isNotEmpty()) {
            Text(
                text = stringResource(R.string.native_task_completedSubtasks),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 2.dp),
            )
            for (row in done) {
                SubtaskRow(row, zone, today, onToggleComplete, onOpen)
            }
        }
        if (inInbox) {
            Text(
                text = stringResource(R.string.page_task_inboxSubtasksNoticePre),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
            )
            return@DetailCard
        }
        AddSubtaskField(onAdd)
    }
}

/** How much of the work under the task is finished. */
@Composable
private fun SubtaskProgress(
    finished: Int,
    total: Int,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 10.dp, bottom = 6.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        LinearProgressIndicator(
            progress = { finished.toFloat() / total },
            trackColor = MaterialTheme.colorScheme.surfaceContainerHighest,
            modifier = Modifier.weight(1f).height(6.dp),
        )
        Text(
            text = stringResource(R.string.native_task_subtaskProgress, finished, total),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun SubtaskRow(
    row: TaskListRow,
    zone: ZoneId,
    today: LocalDate,
    onToggleComplete: (Task) -> Unit,
    onOpen: (Task) -> Unit,
) {
    TaskRow(
        row = row,
        zone = zone,
        today = today,
        selectionMode = false,
        selected = false,
        onToggleComplete = { onToggleComplete(row.task) },
        onOpen = { onOpen(row.task) },
        onSelectToggle = {},
        onStartSelection = {},
    )
}

/**
 * The field that adds a subtask.
 *
 * It is a row until it is asked for, because a card that ends in an empty text
 * box reads as unfinished work rather than as an invitation. Once open it empties
 * itself and stays where it is, because subtasks are written down in runs: the
 * user has a list in their head, and making them find the field again after every
 * line is what turns a run into a chore.
 */
@Composable
private fun AddSubtaskField(onAdd: (String) -> Unit) {
    var writing by remember { mutableStateOf(false) }
    var title by remember { mutableStateOf("") }
    val focusRequester = remember { FocusRequester() }

    if (!writing) {
        Row(
            modifier =
                Modifier
                    .fillMaxWidth()
                    .clickable { writing = true }
                    .defaultMinSize(minHeight = ROW_HEIGHT_DP.dp)
                    .padding(horizontal = 16.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(
                imageVector = Icons.Filled.Add,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = stringResource(R.string.page_task_addSubtaskPlaceholder),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        return
    }

    val submit = {
        val trimmed = title.trim()
        if (trimmed.isNotEmpty()) {
            onAdd(trimmed)
            title = ""
        }
    }
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        OutlinedTextField(
            value = title,
            onValueChange = { title = it },
            placeholder = { Text(stringResource(R.string.page_task_addSubtaskPlaceholder)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
            keyboardActions = KeyboardActions(onDone = { submit() }),
            modifier = Modifier.weight(1f).focusRequester(focusRequester),
        )
        IconButton(onClick = submit) {
            Icon(
                imageVector = Icons.Filled.Add,
                contentDescription = stringResource(R.string.page_task_addSubtaskPlaceholder),
            )
        }
    }
    LaunchedEffect(Unit) { focusRequester.requestFocus() }
}

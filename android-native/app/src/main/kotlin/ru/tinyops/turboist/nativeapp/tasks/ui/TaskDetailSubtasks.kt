package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
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
    Column(modifier = modifier.fillMaxWidth()) {
        SectionLabel(stringResource(R.string.page_task_subtasks))
        for (row in open) {
            SubtaskRow(row, zone, today, onToggleComplete, onOpen)
        }
        if (done.isNotEmpty()) {
            SectionLabel(stringResource(R.string.native_task_completedSubtasks))
            for (row in done) {
                SubtaskRow(row, zone, today, onToggleComplete, onOpen)
            }
        }
        if (inInbox) {
            Text(
                text = stringResource(R.string.page_task_inboxSubtasksNoticePre),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
            )
            return@Column
        }
        AddSubtaskField(onAdd)
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
 * It empties itself and stays where it is, because subtasks are written down in
 * runs: the user has a list in their head, and making them find the field again
 * after every line is what turns a run into a chore.
 */
@Composable
private fun AddSubtaskField(onAdd: (String) -> Unit) {
    var title by remember { mutableStateOf("") }
    val submit = {
        val trimmed = title.trim()
        if (trimmed.isNotEmpty()) {
            onAdd(trimmed)
            title = ""
        }
    }
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
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
            modifier = Modifier.weight(1f),
        )
        IconButton(onClick = submit) {
            Icon(
                imageVector = Icons.Filled.Add,
                contentDescription = stringResource(R.string.page_task_addSubtaskPlaceholder),
            )
        }
    }
}

/** A heading inside the detail screen. */
@Composable
fun SectionLabel(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 16.dp, bottom = 4.dp),
    )
}

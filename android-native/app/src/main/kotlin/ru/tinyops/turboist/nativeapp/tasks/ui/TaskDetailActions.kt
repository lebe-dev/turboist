package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Bookmark
import androidx.compose.material.icons.outlined.CallSplit
import androidx.compose.material.icons.outlined.Cancel
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.DriveFileMove
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.MoveProject

/**
 * Everything that can be done to the task as a whole.
 *
 * It is one sheet behind one control rather than seven buttons laid out down the
 * page. The buttons were findable — that was the argument for them — but they
 * also sat between the fields a user came to read, in a block that wrapped
 * differently for every task, and none of them is reached often enough to earn
 * that. The control that opens this is in the top bar, where a person already
 * looks for what a screen can do.
 *
 * Completing is not here: it is the ring beside the title, where the state it
 * changes is shown. Deleting is, under a rule of its own, because it is the only
 * one with nothing behind it.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TaskActionsSheet(
    task: Task,
    onCancelTask: () -> Unit,
    onDuplicate: () -> Unit,
    onOpenMove: () -> Unit,
    onDelete: () -> Unit,
    onDismiss: () -> Unit,
    canDecompose: Boolean = false,
    onDecompose: (List<String>) -> Unit = {},
    onCreateTemplate: () -> Unit = {},
) {
    var confirmingDelete by remember { mutableStateOf(false) }
    var splitting by remember { mutableStateOf(false) }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
    ) {
        Column(modifier = Modifier.fillMaxWidth().padding(top = 8.dp, bottom = 24.dp)) {
            SheetActionRow(
                text = stringResource(R.string.task_actions_moveToProject),
                leading = Icons.Outlined.DriveFileMove,
                onClick = {
                    onDismiss()
                    onOpenMove()
                },
            )
            SheetActionRow(
                text = stringResource(R.string.task_actions_duplicate),
                leading = Icons.Outlined.ContentCopy,
                onClick = {
                    onDismiss()
                    onDuplicate()
                },
            )
            SheetActionRow(
                text = stringResource(R.string.task_actions_createTemplate),
                leading = Icons.Outlined.Bookmark,
                onClick = {
                    onDismiss()
                    onCreateTemplate()
                },
            )
            // Splitting a task that already has work under it is refused, here as
            // on the server: the work underneath would have nowhere to go. The row
            // stays and says which of the two it is rather than disappearing, so
            // the action is still findable and its condition is stated.
            SheetActionRow(
                text = stringResource(R.string.task_actions_decompose),
                leading = Icons.Outlined.CallSplit,
                supporting =
                    stringResource(R.string.task_actions_decomposeDisabled).takeUnless { canDecompose },
                enabled = canDecompose,
                onClick = { splitting = true },
            )
            SheetActionRow(
                text = stringResource(R.string.native_task_cancelTask),
                leading = Icons.Outlined.Cancel,
                onClick = {
                    onDismiss()
                    onCancelTask()
                },
            )
            HorizontalDivider(modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp))
            SheetActionRow(
                text = stringResource(R.string.common_delete),
                leading = Icons.Outlined.DeleteOutline,
                destructive = true,
                onClick = { confirmingDelete = true },
            )
        }
    }

    if (splitting) {
        DecomposeDialog(
            onDismiss = { splitting = false },
            onSplit = { titles ->
                splitting = false
                onDismiss()
                onDecompose(titles)
            },
        )
    }

    if (!confirmingDelete) return
    // Deleting is the one action with nothing behind it: there are no tombstones
    // in this product, on the device or on the server, so the confirmation says
    // what goes with the task rather than offering an undo that cannot exist.
    AlertDialog(
        onDismissRequest = { confirmingDelete = false },
        title = { Text(stringResource(R.string.common_delete)) },
        text = { Text(stringResource(R.string.task_toast_confirmDelete, task.title)) },
        confirmButton = {
            TextButton(
                onClick = {
                    confirmingDelete = false
                    onDismiss()
                    onDelete()
                },
            ) { Text(stringResource(R.string.common_delete)) }
        },
        dismissButton = {
            TextButton(onClick = { confirmingDelete = false }) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

/**
 * Turning one task into several, written as an outline.
 *
 * A line per task, typed the way an outline is written on paper, because that is
 * the gesture: the user has already worked out that this was really three
 * things. The wording says what becomes of the original, since it is replaced
 * rather than added to and there is no undo anywhere in this product.
 */
@Composable
private fun DecomposeDialog(
    onDismiss: () -> Unit,
    onSplit: (List<String>) -> Unit,
) {
    var outline by remember { mutableStateOf("") }
    val titles = outline.lines().map { it.trim() }.filter { it.isNotEmpty() }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.dialog_decompose_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    text = stringResource(R.string.dialog_decompose_description),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = outline,
                    onValueChange = { outline = it },
                    label = { Text(stringResource(R.string.dialog_decompose_label)) },
                    placeholder = { Text(stringResource(R.string.dialog_decompose_placeholder)) },
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    text = stringResource(R.string.dialog_decompose_count, titles.size),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { onSplit(titles) }, enabled = titles.isNotEmpty()) {
                Text(stringResource(R.string.dialog_decompose_submit))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

/**
 * Where a task can be moved to: the inbox, a project, or one of that project's
 * columns.
 *
 * The columns of a project are listed under it rather than behind a second step,
 * because picking a project and then picking a column is one decision, and the
 * board a task lands on is part of where it now lives.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TaskMoveSheet(
    projects: List<MoveProject>,
    onPick: (TaskDestination) -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState) {
        Column(modifier = Modifier.fillMaxWidth().padding(bottom = 24.dp)) {
            Text(
                text = stringResource(R.string.dialog_moveTask_title),
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp),
            )
            ListItem(
                headlineContent = { Text(stringResource(R.string.nav_inbox)) },
                modifier = Modifier.clickable { onPick(TaskDestination.Inbox) },
            )
            if (projects.isEmpty()) {
                Text(
                    text = stringResource(R.string.dialog_moveTask_noProjects),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                )
            }
            for (project in projects) {
                ListItem(
                    headlineContent = { Text(project.title) },
                    modifier = Modifier.clickable { onPick(TaskDestination.InProject(project.projectLocalId)) },
                )
                for (section in project.sections) {
                    ListItem(
                        headlineContent = { Text(section.title) },
                        supportingContent = { Text(project.title) },
                        modifier =
                            Modifier
                                .padding(start = 16.dp)
                                .clickable { onPick(TaskDestination.InSection(section.sectionLocalId)) },
                    )
                }
            }
        }
    }
}

package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Bookmark
import androidx.compose.material.icons.outlined.CallSplit
import androidx.compose.material.icons.outlined.Cancel
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.DriveFileMove
import androidx.compose.material.icons.outlined.PushPin
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.MoveProject

/**
 * Everything that can be done to the task as a whole.
 *
 * Laid out as buttons that wrap rather than hidden behind an overflow menu: this
 * is the screen the user came to in order to act on one task, so the actions are
 * the point of it and not an afterthought. Completing is not here — it is the
 * control beside the title, where the state it changes is shown.
 */
@Composable
fun TaskDetailActionRow(
    task: Task,
    onTogglePin: () -> Unit,
    onCancelTask: () -> Unit,
    onDuplicate: () -> Unit,
    onOpenMove: () -> Unit,
    onDelete: () -> Unit,
    modifier: Modifier = Modifier,
    canDecompose: Boolean = false,
    onDecompose: (List<String>) -> Unit = {},
    onCreateTemplate: () -> Unit = {},
) {
    var confirmingDelete by remember { mutableStateOf(false) }
    var splitting by remember { mutableStateOf(false) }

    FlowRow(
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        ActionButton(
            text = stringResource(if (task.isPinned) R.string.task_actions_unpin else R.string.task_actions_pin),
            icon = Icons.Outlined.PushPin,
            onClick = onTogglePin,
        )
        ActionButton(
            text = stringResource(R.string.task_actions_moveToProject),
            icon = Icons.Outlined.DriveFileMove,
            onClick = onOpenMove,
        )
        ActionButton(
            text = stringResource(R.string.task_actions_duplicate),
            icon = Icons.Outlined.ContentCopy,
            onClick = onDuplicate,
        )
        ActionButton(
            text = stringResource(R.string.native_task_cancelTask),
            icon = Icons.Outlined.Cancel,
            onClick = onCancelTask,
        )
        ActionButton(
            text = stringResource(R.string.task_actions_createTemplate),
            icon = Icons.Outlined.Bookmark,
            onClick = onCreateTemplate,
        )
        // Splitting a task that already has work under it is refused, here as on
        // the server: the work underneath would have nowhere to go. The button
        // says which of the two it is rather than disappearing, so the action is
        // still findable and its condition is stated.
        ActionButton(
            text =
                stringResource(
                    if (canDecompose) {
                        R.string.task_actions_decompose
                    } else {
                        R.string.task_actions_decomposeDisabled
                    },
                ),
            icon = Icons.Outlined.CallSplit,
            enabled = canDecompose,
            onClick = { splitting = true },
        )
        ActionButton(
            text = stringResource(R.string.common_delete),
            icon = Icons.Outlined.DeleteOutline,
            onClick = { confirmingDelete = true },
        )
    }

    if (splitting) {
        DecomposeDialog(
            onDismiss = { splitting = false },
            onSplit = { titles ->
                splitting = false
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

@Composable
private fun ActionButton(
    text: String,
    icon: ImageVector,
    onClick: () -> Unit,
    enabled: Boolean = true,
) {
    OutlinedButton(onClick = onClick, enabled = enabled) {
        Icon(imageVector = icon, contentDescription = null, modifier = Modifier.padding(end = 6.dp))
        Text(text)
    }
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
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
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

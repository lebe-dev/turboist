package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.outlined.AccountTree
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.DriveFileMove
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.BulkDestinations
import ru.tinyops.turboist.nativeapp.tasks.MoveProject

/**
 * What can be done to a picked set of tasks.
 *
 * `null` on a list that offers selection but no actions on it — the count and the
 * way out are still worth having on their own, and a bar of buttons that do
 * nothing would be worse than no buttons.
 */
data class SelectionActions(
    val onComplete: () -> Unit,
    val onMove: () -> Unit,
    val onGroup: () -> Unit,
    val onPriority: (Priority) -> Unit,
    val onPlan: (PlanState) -> Unit,
    val onDelete: () -> Unit,
)

/**
 * The bar shown while tasks are being picked.
 *
 * The three actions a selection is usually made for — finish it, file it
 * somewhere, make one thing out of it — are on the bar itself; the rest are
 * behind the overflow, because a bar of seven buttons on a phone is a row of
 * targets too small to hit. The count and the way out flank them, and they are
 * what make the mode usable at all.
 *
 * Grouping needs at least two tasks: one task under a new parent is not a group,
 * it is a rename with extra steps, and the server refuses the shape anyway.
 */
@Composable
fun SelectionBar(
    count: Int,
    actions: SelectionActions?,
    onClear: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var overflowOpen by remember { mutableStateOf(false) }
    var confirmingDelete by remember { mutableStateOf(false) }
    val barLabel = stringResource(R.string.selection_bar_aria)
    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .padding(start = 16.dp, end = 4.dp)
                .semantics { contentDescription = barLabel },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = stringResource(R.string.selection_bar_count, count),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onPrimaryContainer,
            modifier = Modifier.weight(1f),
        )
        if (actions != null) {
            BarAction(
                labelRes = R.string.selection_bar_complete,
                onClick = actions.onComplete,
                icon = { tint -> Icon(Icons.Outlined.CheckCircle, contentDescription = null, tint = tint) },
            )
            BarAction(
                labelRes = R.string.selection_bar_move,
                onClick = actions.onMove,
                icon = { tint -> Icon(Icons.Outlined.DriveFileMove, contentDescription = null, tint = tint) },
            )
            if (count >= MIN_TASKS_IN_A_GROUP) {
                BarAction(
                    labelRes = R.string.selection_bar_group,
                    onClick = actions.onGroup,
                    icon = { tint -> Icon(Icons.Outlined.AccountTree, contentDescription = null, tint = tint) },
                )
            }
            BarAction(
                labelRes = R.string.selection_bar_more,
                onClick = { overflowOpen = true },
                icon = { tint -> Icon(Icons.Filled.MoreVert, contentDescription = null, tint = tint) },
            )
            SelectionOverflow(
                open = overflowOpen,
                actions = actions,
                onDismiss = { overflowOpen = false },
                onDeleteRequested = { confirmingDelete = true },
            )
        }
        IconButton(onClick = onClear) {
            Icon(
                imageVector = Icons.Filled.Close,
                contentDescription = stringResource(R.string.selection_bar_cancel),
                tint = MaterialTheme.colorScheme.onPrimaryContainer,
            )
        }
    }
    if (confirmingDelete && actions != null) {
        ConfirmBulkDelete(
            count = count,
            onConfirm = {
                confirmingDelete = false
                actions.onDelete()
            },
            onDismiss = { confirmingDelete = false },
        )
    }
}

/** One button of the bar: an icon, described by the action it takes. */
@Composable
private fun BarAction(
    @StringRes labelRes: Int,
    onClick: () -> Unit,
    icon: @Composable (Color) -> Unit,
) {
    val label = stringResource(labelRes)
    IconButton(onClick = onClick, modifier = Modifier.semantics { contentDescription = label }) {
        icon(MaterialTheme.colorScheme.onPrimaryContainer)
    }
}

/** The actions that do not fit on the bar: the priority levels, planning, and deleting. */
@Composable
private fun SelectionOverflow(
    open: Boolean,
    actions: SelectionActions,
    onDismiss: () -> Unit,
    onDeleteRequested: () -> Unit,
) {
    DropdownMenu(expanded = open, onDismissRequest = onDismiss) {
        for (level in PRIORITY_CHOICES) {
            DropdownMenuItem(
                text = { Text(priorityLabel(level)) },
                onClick = {
                    onDismiss()
                    actions.onPriority(level)
                },
            )
        }
        HorizontalDivider()
        DropdownMenuItem(
            text = { Text(stringResource(R.string.page_nextWeek_planForWeek)) },
            onClick = {
                onDismiss()
                actions.onPlan(PlanState.WEEK)
            },
        )
        DropdownMenuItem(
            text = { Text(stringResource(R.string.task_actions_toBacklog)) },
            onClick = {
                onDismiss()
                actions.onPlan(PlanState.BACKLOG)
            },
        )
        HorizontalDivider()
        DropdownMenuItem(
            text = { Text(stringResource(R.string.common_delete)) },
            onClick = {
                onDismiss()
                onDeleteRequested()
            },
        )
    }
}

/**
 * The one selection action that is asked about first.
 *
 * Deleting is the only one of them that cannot be undone by doing the opposite —
 * the rows go, and so does everything under them — so it is the only one worth
 * a question.
 */
@Composable
private fun ConfirmBulkDelete(
    count: Int,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.selection_confirmDelete_title)) },
        text = { Text(stringResource(R.string.selection_confirmDelete_description, count)) },
        confirmButton = {
            TextButton(onClick = onConfirm) { Text(stringResource(R.string.common_delete)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

/**
 * Where a selection is being sent.
 *
 * The projects this device files work into lead the list, and are lifted out of
 * the body below so nothing is offered twice. A project's columns are listed
 * under it rather than behind a second step: picking a project and then a column
 * is one decision, and the board a task lands on is part of where it now lives.
 *
 * The inbox is offered only where a selection may go there. It can hold loose
 * tasks moved back out of a project, but it cannot hold the parent of a group —
 * the inbox is raw capture and carries no structure.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BulkDestinationSheet(
    title: String,
    destinations: BulkDestinations,
    offerInbox: Boolean,
    onPick: (TaskDestination) -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState) {
        Text(
            text = title,
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        LazyColumn(
            modifier = Modifier.fillMaxWidth().heightIn(max = 420.dp),
            contentPadding = PaddingValues(bottom = 24.dp),
        ) {
            if (offerInbox) {
                item {
                    DestinationRow(stringResource(R.string.nav_inbox)) { onPick(TaskDestination.Inbox) }
                }
            }
            if (destinations.recent.isNotEmpty()) {
                item { DestinationHeading(stringResource(R.string.native_quickAdd_recentProjects)) }
                items(destinations.recent, key = { "recent-${it.projectLocalId}" }) { project ->
                    ProjectDestinations(project, onPick)
                }
                item { HorizontalDivider() }
            }
            items(destinations.others, key = { it.projectLocalId }) { project ->
                ProjectDestinations(project, onPick)
            }
            if (destinations.isEmpty) {
                item { DestinationHeading(stringResource(R.string.dialog_moveTask_noProjects)) }
            }
        }
    }
}

/** A project, with each of its columns offered under it. */
@Composable
private fun ProjectDestinations(
    project: MoveProject,
    onPick: (TaskDestination) -> Unit,
) {
    DestinationRow(project.title) { onPick(TaskDestination.InProject(project.projectLocalId)) }
    for (section in project.sections) {
        DestinationRow(
            title = section.title,
            supporting = project.title,
            indented = true,
        ) { onPick(TaskDestination.InSection(section.sectionLocalId)) }
    }
}

@Composable
private fun DestinationRow(
    title: String,
    supporting: String? = null,
    indented: Boolean = false,
    onClick: () -> Unit,
) {
    ListItem(
        headlineContent = { Text(title) },
        supportingContent = supporting?.let { { Text(it) } },
        modifier =
            Modifier
                .fillMaxWidth()
                .padding(start = if (indented) 16.dp else 0.dp)
                .clickable(onClick = onClick)
                .semantics { contentDescription = title },
    )
}

@Composable
private fun DestinationHeading(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

/**
 * Naming the task the selection is about to hang from.
 *
 * The note under the field is not a warning about an unusual case: grouping
 * always rewrites the children to match their new parent — where they live, what
 * they are tagged with, how urgent they are — and that is worth saying before it
 * happens rather than after.
 *
 * The destination is picked in the sheet that follows, and the inbox is not
 * among its answers: a group is structure and the inbox holds none.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GroupTasksSheet(
    count: Int,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var title by rememberSaveable { mutableStateOf("") }
    val titleLabel = stringResource(R.string.dialog_quickAdd_titlePlaceholder)
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 24.dp)) {
            Text(
                text = stringResource(R.string.dialog_quickAdd_wrap_title, count),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.padding(vertical = 8.dp),
            )
            OutlinedTextField(
                value = title,
                onValueChange = { title = it },
                singleLine = true,
                placeholder = { Text(titleLabel) },
                modifier = Modifier.fillMaxWidth().semantics { contentDescription = titleLabel },
            )
            Text(
                text = stringResource(R.string.selection_group_effect),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 8.dp),
            )
            Row(
                modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                horizontalArrangement = Arrangement.End,
            ) {
                TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
                TextButton(
                    onClick = { onConfirm(title.trim()) },
                    enabled = title.isNotBlank(),
                ) { Text(stringResource(R.string.dialog_quickAdd_wrap_submit)) }
            }
        }
    }
}

/** Below this, a group is a rename rather than a grouping, and the server refuses the shape. */
const val MIN_TASKS_IN_A_GROUP: Int = 2

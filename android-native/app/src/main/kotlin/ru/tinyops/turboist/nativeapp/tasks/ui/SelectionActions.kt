package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.MoreHoriz
import androidx.compose.material.icons.outlined.AccountTree
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.DriveFileMove
import androidx.compose.material.icons.outlined.EventAvailable
import androidx.compose.material.icons.outlined.Flag
import androidx.compose.material.icons.outlined.Inventory2
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuDefaults
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
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
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
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
 * The header shown above the list while tasks are being picked: the way out and
 * the count. It replaces nothing and pushes nothing off screen; it is the
 * reminder that taps now pick rather than open.
 */
@Composable
fun SelectionHeader(
    count: Int,
    onClear: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainer,
        shape = RoundedCornerShape(bottomStart = HEADER_CORNER, bottomEnd = HEADER_CORNER),
        modifier = modifier.fillMaxWidth(),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(start = 4.dp, end = 16.dp, top = 4.dp, bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IconButton(onClick = onClear) {
                Icon(
                    imageVector = Icons.Filled.Close,
                    contentDescription = stringResource(R.string.selection_bar_cancel),
                )
            }
            Text(
                text = stringResource(R.string.selection_bar_count, count),
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.weight(1f),
            )
        }
    }
}

/**
 * The toolbar that floats over the bottom of the list while tasks are picked.
 *
 * Every action carries its name under its icon: a row of bare icons is a row of
 * guesses, and these are the gestures that change many tasks at once. Finishing
 * the selection is what it is most often made for, so it is the one filled
 * button. Priority and plan open their own short menus; grouping and deleting,
 * the two that reshape or remove work, sit behind "more".
 *
 * Grouping needs at least two tasks: one task under a new parent is not a group,
 * it is a rename with extra steps, and the server refuses the shape anyway.
 */
@Composable
fun SelectionToolbar(
    count: Int,
    actions: SelectionActions,
    modifier: Modifier = Modifier,
) {
    var confirmingDelete by remember { mutableStateOf(false) }
    val barLabel = stringResource(R.string.selection_bar_aria)
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerHighest,
        shape = RoundedCornerShape(TOOLBAR_CORNER),
        shadowElevation = 6.dp,
        modifier = modifier.fillMaxWidth().semantics { contentDescription = barLabel },
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ToolbarAction(
                label = stringResource(R.string.selection_bar_complete),
                icon = Icons.Outlined.CheckCircle,
                onClick = actions.onComplete,
                emphasised = true,
                modifier = Modifier.weight(1f),
            )
            ToolbarAction(
                label = stringResource(R.string.selection_bar_move),
                icon = Icons.Outlined.DriveFileMove,
                onClick = actions.onMove,
                modifier = Modifier.weight(1f),
            )
            ToolbarMenu(
                label = stringResource(R.string.selection_bar_priority),
                icon = Icons.Outlined.Flag,
                modifier = Modifier.weight(1f),
            ) { close ->
                for (level in PRIORITY_CHOICES) {
                    DropdownMenuItem(
                        text = { Text(priorityLabel(level)) },
                        leadingIcon = { PriorityDot(priorityTint(level)) },
                        onClick = {
                            close()
                            actions.onPriority(level)
                        },
                    )
                }
            }
            ToolbarMenu(
                label = stringResource(R.string.native_selection_plan),
                icon = Icons.Outlined.EventAvailable,
                modifier = Modifier.weight(1f),
            ) { close ->
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.page_nextWeek_planForWeek)) },
                    leadingIcon = { Icon(Icons.Outlined.EventAvailable, contentDescription = null) },
                    onClick = {
                        close()
                        actions.onPlan(PlanState.WEEK)
                    },
                )
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.task_actions_toBacklog)) },
                    leadingIcon = { Icon(Icons.Outlined.Inventory2, contentDescription = null) },
                    onClick = {
                        close()
                        actions.onPlan(PlanState.BACKLOG)
                    },
                )
            }
            ToolbarMenu(
                label = stringResource(R.string.native_selection_more),
                description = stringResource(R.string.selection_bar_more),
                icon = Icons.Filled.MoreHoriz,
                modifier = Modifier.weight(1f),
            ) { close ->
                if (count >= MIN_TASKS_IN_A_GROUP) {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.selection_bar_group)) },
                        leadingIcon = { Icon(Icons.Outlined.AccountTree, contentDescription = null) },
                        onClick = {
                            close()
                            actions.onGroup()
                        },
                    )
                    HorizontalDivider()
                }
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.common_delete)) },
                    leadingIcon = { Icon(Icons.Outlined.Delete, contentDescription = null) },
                    colors =
                        MenuDefaults.itemColors(
                            textColor = MaterialTheme.colorScheme.error,
                            leadingIconColor = MaterialTheme.colorScheme.error,
                        ),
                    onClick = {
                        close()
                        confirmingDelete = true
                    },
                )
            }
        }
    }
    if (confirmingDelete) {
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

/** One button of the toolbar: an icon with its name under it. */
@Composable
private fun ToolbarAction(
    label: String,
    icon: ImageVector,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    description: String = label,
    emphasised: Boolean = false,
) {
    Surface(
        onClick = onClick,
        color = if (emphasised) MaterialTheme.colorScheme.primary else Color.Transparent,
        contentColor = if (emphasised) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
        shape = RoundedCornerShape(TOOLBAR_CORNER),
        modifier = modifier.height(TOOLBAR_ACTION_HEIGHT).semantics { contentDescription = description },
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(4.dp, Alignment.CenterVertically),
            modifier = Modifier.padding(horizontal = 2.dp),
        ) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(22.dp))
            Text(
                text = label,
                style = MaterialTheme.typography.labelSmall,
                fontWeight = if (emphasised) FontWeight.Bold else FontWeight.Medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

/** A toolbar button that opens a short menu of its own, anchored to it. */
@Composable
private fun ToolbarMenu(
    label: String,
    icon: ImageVector,
    modifier: Modifier = Modifier,
    description: String = label,
    items: @Composable ColumnScope.(close: () -> Unit) -> Unit,
) {
    var open by remember { mutableStateOf(false) }
    Box(modifier = modifier) {
        ToolbarAction(label = label, description = description, icon = icon, onClick = { open = true })
        DropdownMenu(
            expanded = open,
            onDismissRequest = { open = false },
            shape = MaterialTheme.shapes.large,
        ) {
            items { open = false }
        }
    }
}

/** The mark a level carries in every list, repeated in the menu that sets it. */
@Composable
private fun PriorityDot(tint: Color) {
    Box(modifier = Modifier.size(10.dp).background(tint, CircleShape))
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

/** The corner the toolbar and its buttons are cut with: a pill at their height. */
private val TOOLBAR_CORNER = 32.dp

/** How tall each toolbar button is, which is also its hit target. */
private val TOOLBAR_ACTION_HEIGHT = 64.dp

/** The corner the selection header is rounded with where it meets the list. */
private val HEADER_CORNER = 24.dp

/** Below this, a group is a rename rather than a grouping, and the server refuses the shape. */
const val MIN_TASKS_IN_A_GROUP: Int = 2

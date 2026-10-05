package ru.tinyops.turboist.nativeapp.projects.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.sync.write.ProjectStatusAction
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.projects.BoardColumn
import ru.tinyops.turboist.nativeapp.projects.ProjectBoardUiState
import ru.tinyops.turboist.nativeapp.projects.ProjectBoardViewModel
import ru.tinyops.turboist.nativeapp.projects.ProjectMessage
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.tasks.ui.TaskRow
import java.time.LocalDate
import java.time.ZoneId

/** What the project board can be asked to do. */
data class ProjectBoardCallbacks(
    val onRefresh: () -> Unit,
    val onOpenTask: (Task) -> Unit,
    val onToggleComplete: (Task) -> Unit,
    val onMoveTask: (taskLocalId: Long, sectionLocalId: Long?) -> Unit,
    val onAddTask: (sectionLocalId: Long?, title: String) -> Unit,
    val onCreateSection: (String) -> Unit,
    val onRenameSection: (sectionLocalId: Long, title: String) -> Unit,
    val onDeleteSection: (sectionLocalId: Long) -> Unit,
    val onMoveSectionEarlier: (sectionLocalId: Long) -> Unit,
    val onMoveSectionLater: (sectionLocalId: Long) -> Unit,
    val onRename: (String) -> Unit,
    val onSetStatus: (ProjectStatusAction) -> Unit,
    val onTogglePin: () -> Unit,
    val onSetTroiki: (TroikiCategory?) -> Unit,
    val onSetPrivate: (Boolean) -> Unit,
    val onDelete: () -> Unit,
)

/**
 * One project and its board.
 *
 * The board is drawn as a column of columns rather than side by side: a phone is
 * a tall screen, and reading a board sideways would put every column but one out
 * of sight. The project's own column leads — the work that is in the project but
 * in none of its columns — and the named columns follow in board order.
 *
 * Everything here is applied to the replica the moment it is asked for, so the
 * whole screen works with no connection. Moving a column renumbers the board the
 * way the server will renumber it, so what the user sees after a move is what
 * they will still see after the phone reconnects.
 *
 * Moving a task between columns is a long press rather than a drag: a drag on a
 * touch screen is ambiguous with a scroll, and a long press also reaches anyone
 * driving the screen with an assistive reader, which a drag never does.
 */
@Composable
fun ProjectScreen(
    state: ProjectBoardUiState,
    zone: ZoneId,
    today: LocalDate,
    messages: Flow<ProjectMessage>,
    callbacks: ProjectBoardCallbacks,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    MessageHost(messages, snackbars)

    var addingTo by remember { mutableStateOf<BoardColumn?>(null) }
    var renaming by remember { mutableStateOf<BoardColumn?>(null) }
    var deleting by remember { mutableStateOf<BoardColumn?>(null) }
    var moving by remember { mutableStateOf<Task?>(null) }
    var addingSection by remember { mutableStateOf(false) }
    var renamingProject by remember { mutableStateOf(false) }
    var deletingProject by remember { mutableStateOf(false) }

    Box(modifier = modifier.fillMaxSize()) {
        val project = state.project
        when {
            state.missing ->
                EmptyMessage(
                    title = stringResource(R.string.page_project_notFound),
                    description = stringResource(R.string.page_project_emptyDescription),
                )

            project != null ->
                Column(modifier = Modifier.fillMaxSize()) {
                    ProjectHeader(
                        state = state,
                        onRename = { renamingProject = true },
                        onAddSection = { addingSection = true },
                        onDelete = { deletingProject = true },
                        callbacks = callbacks,
                    )
                    BoardBody(
                        state = state,
                        zone = zone,
                        today = today,
                        callbacks = callbacks,
                        onAddTo = { addingTo = it },
                        onRename = { renaming = it },
                        onDelete = { deleting = it },
                        onMoveTask = { moving = it },
                    )
                }
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }

    BoardDialogs(
        state = state,
        addingTo = addingTo,
        renaming = renaming,
        deleting = deleting,
        moving = moving,
        addingSection = addingSection,
        renamingProject = renamingProject,
        deletingProject = deletingProject,
        dismissAll = {
            addingTo = null
            renaming = null
            deleting = null
            moving = null
            addingSection = false
            renamingProject = false
            deletingProject = false
        },
        callbacks = callbacks,
    )
}

/**
 * The same screen, driven by a view model.
 *
 * The stateless form above is the one that is exercised in tests; this is the
 * wiring the app uses, and it holds nothing of its own so the two cannot drift.
 */
@Composable
fun ProjectScreen(
    projectLocalId: Long,
    onOpenTask: (Task) -> Unit,
    onLeave: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ProjectBoardViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    val today by viewModel.today.collectAsStateWithLifecycle()
    LaunchedEffect(projectLocalId) { viewModel.open(projectLocalId) }

    ProjectScreen(
        state = state,
        zone = viewModel.zone,
        today = today,
        messages = presenter.messages,
        callbacks =
            remember(presenter, onOpenTask, onLeave) {
                ProjectBoardCallbacks(
                    onRefresh = presenter::refresh,
                    onOpenTask = onOpenTask,
                    onToggleComplete = presenter::toggleComplete,
                    onMoveTask = presenter::moveTask,
                    onAddTask = presenter::addTask,
                    onCreateSection = presenter::createSection,
                    onRenameSection = presenter::renameSection,
                    onDeleteSection = presenter::deleteSection,
                    onMoveSectionEarlier = presenter::moveSectionEarlier,
                    onMoveSectionLater = presenter::moveSectionLater,
                    onRename = presenter::rename,
                    onSetStatus = presenter::setStatus,
                    onTogglePin = presenter::togglePin,
                    onSetTroiki = presenter::setTroikiCategory,
                    onSetPrivate = presenter::setPrivate,
                    // Deleting the project takes the screen with it: what it was
                    // showing no longer exists anywhere.
                    onDelete = {
                        presenter.delete()
                        onLeave()
                    },
                )
            },
        modifier = modifier,
    )
}

/** The project itself: what it is called, where it stands, and everything that can be done to it. */
@Composable
private fun ProjectHeader(
    state: ProjectBoardUiState,
    onRename: () -> Unit,
    onAddSection: () -> Unit,
    onDelete: () -> Unit,
    callbacks: ProjectBoardCallbacks,
) {
    val project = state.project ?: return
    var open by remember { mutableStateOf(false) }
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, top = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Folder,
            contentDescription = null,
            tint = projectTint(project.color) ?: MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(18.dp),
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = project.title,
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.onSurface,
            )
            val status = statusLabel(project.status)
            val note =
                listOfNotNull(
                    state.contextName,
                    status?.takeIf { project.status != ProjectStatus.OPEN }?.let { stringResource(it) },
                ).joinToString(" · ")
            if (note.isNotEmpty()) {
                Text(
                    text = note,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Box {
            IconButton(onClick = { open = true }) {
                Icon(
                    imageVector = Icons.Filled.MoreVert,
                    contentDescription = stringResource(R.string.project_actionsAriaLabel),
                )
            }
            ProjectActionsMenu(
                expanded = open,
                state = state,
                onDismiss = { open = false },
                onRename = onRename,
                onAddSection = onAddSection,
                onDelete = onDelete,
                callbacks = callbacks,
            )
        }
    }
    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
}

/**
 * Everything that can be done to the project.
 *
 * Which entries appear depends on where the project stands: an open project can
 * be finished, a finished one reopened, and only an open one can take a place in
 * the daily plan — the server refuses the rest, and offering an action that will
 * be refused is worse than not offering it.
 */
@Composable
private fun ProjectActionsMenu(
    expanded: Boolean,
    state: ProjectBoardUiState,
    onDismiss: () -> Unit,
    onRename: () -> Unit,
    onAddSection: () -> Unit,
    onDelete: () -> Unit,
    callbacks: ProjectBoardCallbacks,
) {
    val project = state.project ?: return
    DropdownMenu(expanded = expanded, onDismissRequest = onDismiss) {
        MenuItem(stringResource(R.string.dialog_project_editTitle)) {
            onDismiss()
            onRename()
        }
        MenuItem(stringResource(R.string.project_addSection)) {
            onDismiss()
            onAddSection()
        }
        MenuItem(stringResource(if (project.isPinned) R.string.project_unpin else R.string.project_pin)) {
            onDismiss()
            callbacks.onTogglePin()
        }
        MenuItem(
            stringResource(
                if (project.isPrivate) R.string.common_unmarkPrivate else R.string.common_markPrivate,
            ),
        ) {
            onDismiss()
            callbacks.onSetPrivate(!project.isPrivate)
        }
        HorizontalDivider()
        if (project.status == ProjectStatus.OPEN) {
            MenuItem(stringResource(R.string.project_complete)) {
                onDismiss()
                callbacks.onSetStatus(ProjectStatusAction.COMPLETE)
            }
            MenuItem(stringResource(R.string.project_cancel)) {
                onDismiss()
                callbacks.onSetStatus(ProjectStatusAction.CANCEL)
            }
            MenuItem(stringResource(R.string.project_archive)) {
                onDismiss()
                callbacks.onSetStatus(ProjectStatusAction.ARCHIVE)
            }
        } else {
            MenuItem(stringResource(R.string.project_reopen)) {
                onDismiss()
                callbacks.onSetStatus(ProjectStatusAction.UNCOMPLETE)
            }
            if (project.status == ProjectStatus.ARCHIVED) {
                MenuItem(stringResource(R.string.project_unarchive)) {
                    onDismiss()
                    callbacks.onSetStatus(ProjectStatusAction.UNARCHIVE)
                }
            }
        }
        HorizontalDivider()
        if (state.canJoinDailyPlan) {
            for (category in listOf(TroikiCategory.IMPORTANT, TroikiCategory.MEDIUM, TroikiCategory.REST)) {
                val name = troikiLabel(category)?.let { stringResource(it) } ?: continue
                MenuItem(stringResource(R.string.project_assignToTroiki) + ": " + name) {
                    onDismiss()
                    callbacks.onSetTroiki(category)
                }
            }
        }
        if (state.dailyPlanEnabled && project.troikiCategory != null) {
            MenuItem(stringResource(R.string.project_removeFromTroiki)) {
                onDismiss()
                callbacks.onSetTroiki(null)
            }
        }
        HorizontalDivider()
        MenuItem(stringResource(R.string.common_delete)) {
            onDismiss()
            onDelete()
        }
    }
}

@Composable
private fun MenuItem(
    text: String,
    onClick: () -> Unit,
) {
    DropdownMenuItem(text = { Text(text) }, onClick = onClick)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun BoardBody(
    state: ProjectBoardUiState,
    zone: ZoneId,
    today: LocalDate,
    callbacks: ProjectBoardCallbacks,
    onAddTo: (BoardColumn) -> Unit,
    onRename: (BoardColumn) -> Unit,
    onDelete: (BoardColumn) -> Unit,
    onMoveTask: (Task) -> Unit,
) {
    PullToRefreshBox(
        isRefreshing = state.refreshing,
        onRefresh = callbacks.onRefresh,
        modifier = Modifier.fillMaxSize(),
    ) {
        LazyColumn(modifier = Modifier.fillMaxSize()) {
            for (column in state.columns) {
                item(key = "heading-" + column.key) {
                    ColumnHeading(
                        column = column,
                        onAdd = { onAddTo(column) },
                        onRename = { onRename(column) },
                        onDelete = { onDelete(column) },
                        onMoveEarlier = { column.sectionLocalId?.let(callbacks.onMoveSectionEarlier) },
                        onMoveLater = { column.sectionLocalId?.let(callbacks.onMoveSectionLater) },
                    )
                }
                if (column.isEmpty) {
                    item(key = "empty-" + column.key) { ColumnNote(stringResource(R.string.section_noTasks)) }
                }
                items(column.open, key = { "task-" + it.task.localId }) { row ->
                    BoardTaskRow(row, zone, today, callbacks, onMoveTask)
                }
                if (column.done.isNotEmpty()) {
                    item(key = "done-" + column.key) {
                        ColumnNote(stringResource(R.string.native_project_finished))
                    }
                    items(column.done, key = { "done-task-" + it.task.localId }) { row ->
                        BoardTaskRow(row, zone, today, callbacks, onMoveTask)
                    }
                }
            }
        }
    }
}

@Composable
private fun BoardTaskRow(
    row: TaskListRow,
    zone: ZoneId,
    today: LocalDate,
    callbacks: ProjectBoardCallbacks,
    onMoveTask: (Task) -> Unit,
) {
    TaskRow(
        row = row,
        zone = zone,
        today = today,
        selectionMode = false,
        selected = false,
        onToggleComplete = { callbacks.onToggleComplete(row.task) },
        onOpen = { callbacks.onOpenTask(row.task) },
        onSelectToggle = { onMoveTask(row.task) },
        // A long press asks where the task should go. It is the board's one
        // rearranging gesture, and it is a press rather than a drag so that it
        // cannot be confused with a scroll.
        onStartSelection = { onMoveTask(row.task) },
    )
}

/** A column heading: what it is called, how much is in it, and what can be done to it. */
@Composable
private fun ColumnHeading(
    column: BoardColumn,
    onAdd: () -> Unit,
    onRename: () -> Unit,
    onDelete: () -> Unit,
    onMoveEarlier: () -> Unit,
    onMoveLater: () -> Unit,
) {
    var open by remember { mutableStateOf(false) }
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, top = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(
            text = column.title ?: stringResource(R.string.dialog_moveSection_noSection),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurface,
        )
        Text(
            text = (column.open.size + column.done.size).toString(),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        IconButton(onClick = onAdd) {
            Icon(
                imageVector = Icons.Filled.Add,
                contentDescription = stringResource(R.string.section_addTaskAriaLabel),
            )
        }
        if (column.sectionLocalId != null) {
            Box {
                IconButton(onClick = { open = true }) {
                    Icon(
                        imageVector = Icons.Filled.MoreVert,
                        contentDescription = stringResource(R.string.project_actionsAriaLabel),
                    )
                }
                DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
                    MenuItem(stringResource(R.string.section_renameAriaLabel)) {
                        open = false
                        onRename()
                    }
                    if (column.canMoveEarlier) {
                        MenuItem(stringResource(R.string.native_section_moveEarlier)) {
                            open = false
                            onMoveEarlier()
                        }
                    }
                    if (column.canMoveLater) {
                        MenuItem(stringResource(R.string.native_section_moveLater)) {
                            open = false
                            onMoveLater()
                        }
                    }
                    MenuItem(stringResource(R.string.section_deleteAriaLabel)) {
                        open = false
                        onDelete()
                    }
                }
            }
        }
    }
    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
}

@Composable
private fun ColumnNote(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp),
    )
}

/** Everything the board asks before it does it. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun BoardDialogs(
    state: ProjectBoardUiState,
    addingTo: BoardColumn?,
    renaming: BoardColumn?,
    deleting: BoardColumn?,
    moving: Task?,
    addingSection: Boolean,
    renamingProject: Boolean,
    deletingProject: Boolean,
    dismissAll: () -> Unit,
    callbacks: ProjectBoardCallbacks,
) {
    if (addingTo != null) {
        NameDialog(
            title = stringResource(R.string.page_context_addTask),
            label = stringResource(R.string.common_title),
            initial = "",
            confirmLabel = stringResource(R.string.common_add),
            onConfirm = { title -> callbacks.onAddTask(addingTo.sectionLocalId, title) },
            onDismiss = dismissAll,
        )
    }
    if (addingSection) {
        NameDialog(
            title = stringResource(R.string.dialog_section_newTitle),
            label = stringResource(R.string.common_title),
            initial = "",
            confirmLabel = stringResource(R.string.common_create),
            onConfirm = callbacks.onCreateSection,
            onDismiss = dismissAll,
        )
    }
    if (renaming?.sectionLocalId != null) {
        NameDialog(
            title = stringResource(R.string.dialog_section_renameTitle),
            label = stringResource(R.string.common_title),
            initial = renaming.title.orEmpty(),
            confirmLabel = stringResource(R.string.common_save),
            onConfirm = { title -> callbacks.onRenameSection(renaming.sectionLocalId, title) },
            onDismiss = dismissAll,
        )
    }
    if (deleting?.sectionLocalId != null) {
        ConfirmDeleteDialog(
            title = stringResource(R.string.page_project_confirmDeleteSectionTitle),
            body = stringResource(R.string.page_project_confirmDeleteSectionDesc),
            onConfirm = { callbacks.onDeleteSection(deleting.sectionLocalId) },
            onDismiss = dismissAll,
        )
    }
    if (renamingProject && state.project != null) {
        NameDialog(
            title = stringResource(R.string.dialog_project_editTitle),
            label = stringResource(R.string.common_title),
            initial = state.project.title,
            confirmLabel = stringResource(R.string.common_save),
            onConfirm = callbacks.onRename,
            onDismiss = dismissAll,
        )
    }
    if (deletingProject) {
        ConfirmDeleteDialog(
            title = stringResource(R.string.page_project_confirmDeleteTitle),
            body = stringResource(R.string.page_project_confirmDeleteDesc),
            onConfirm = callbacks.onDelete,
            onDismiss = dismissAll,
        )
    }
    if (moving != null) {
        ModalBottomSheet(onDismissRequest = dismissAll, sheetState = rememberModalBottomSheetState()) {
            Text(
                text = stringResource(R.string.dialog_moveSection_title),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
            )
            for (column in state.columns) {
                val here = column.sectionLocalId == moving.sectionLocalId
                ListItem(
                    headlineContent = {
                        Text(column.title ?: stringResource(R.string.dialog_moveSection_noSection))
                    },
                    supportingContent =
                        if (here) {
                            { Text(stringResource(R.string.native_section_currentColumn)) }
                        } else {
                            null
                        },
                    modifier =
                        Modifier.clickable(enabled = !here) {
                            callbacks.onMoveTask(moving.localId, column.sectionLocalId)
                            dismissAll()
                        },
                )
            }
        }
    }
}

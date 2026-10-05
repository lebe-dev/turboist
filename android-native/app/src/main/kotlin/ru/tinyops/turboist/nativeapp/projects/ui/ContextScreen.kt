package ru.tinyops.turboist.nativeapp.projects.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
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
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.projects.ContextUiState
import ru.tinyops.turboist.nativeapp.projects.ContextViewModel
import ru.tinyops.turboist.nativeapp.projects.ProjectMessage
import ru.tinyops.turboist.nativeapp.tasks.ui.TaskRow
import java.time.LocalDate
import java.time.ZoneId

/** What the context screen can be asked to do. */
data class ContextCallbacks(
    val onRefresh: () -> Unit,
    val onOpenProject: (Project) -> Unit,
    val onOpenTask: (Task) -> Unit,
    val onToggleComplete: (Task) -> Unit,
    val onRename: (String) -> Unit,
    val onDelete: () -> Unit,
)

/**
 * One context: the projects filed under it, and the work in the whole branch.
 *
 * Both halves are shown because a context is a place rather than a list: the
 * projects say how the work is organised, and the tasks say what the work
 * actually is — including the tasks inside those projects, which is what makes
 * this a view of a branch instead of a view of a folder.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContextScreen(
    state: ContextUiState,
    zone: ZoneId,
    today: LocalDate,
    messages: Flow<ProjectMessage>,
    callbacks: ContextCallbacks,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    MessageHost(messages, snackbars)

    var renaming by remember { mutableStateOf(false) }
    var deleting by remember { mutableStateOf(false) }
    val context = state.context

    Box(modifier = modifier.fillMaxSize()) {
        when {
            state.missing ->
                EmptyMessage(
                    title = stringResource(R.string.page_context_notFound),
                    description = stringResource(R.string.page_context_emptyDescription),
                )

            context != null ->
                Column(modifier = Modifier.fillMaxSize()) {
                    ContextHeader(
                        name = context.name,
                        onRename = { renaming = true },
                        onDelete = { deleting = true },
                    )
                    PullToRefreshBox(
                        isRefreshing = state.refreshing,
                        onRefresh = callbacks.onRefresh,
                        modifier = Modifier.fillMaxSize(),
                    ) {
                        if (state.isEmpty) {
                            EmptyMessage(
                                title = stringResource(R.string.page_context_emptyTitle),
                                description = stringResource(R.string.page_context_emptyDescription),
                            )
                            return@PullToRefreshBox
                        }
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            if (state.projects.isNotEmpty()) {
                                item(key = "projects-heading") { Heading(stringResource(R.string.nav_projects)) }
                                items(state.projects, key = { "project-" + it.localId }) { project ->
                                    ProjectRow(
                                        project,
                                        onOpen = { callbacks.onOpenProject(project) },
                                        dailyPlanEnabled = state.dailyPlanEnabled,
                                    )
                                }
                            }
                            if (state.rows.isNotEmpty()) {
                                item(key = "tasks-heading") {
                                    Heading(stringResource(R.string.native_context_tasks))
                                }
                                items(state.rows, key = { "task-" + it.task.localId }) { row ->
                                    TaskRow(
                                        row = row,
                                        zone = zone,
                                        today = today,
                                        selectionMode = false,
                                        selected = false,
                                        onToggleComplete = { callbacks.onToggleComplete(row.task) },
                                        onOpen = { callbacks.onOpenTask(row.task) },
                                        onSelectToggle = { callbacks.onOpenTask(row.task) },
                                        onStartSelection = { callbacks.onOpenTask(row.task) },
                                    )
                                }
                            }
                        }
                    }
                }
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }

    if (renaming && context != null) {
        NameDialog(
            title = stringResource(R.string.dialog_context_editTitle),
            label = stringResource(R.string.common_name),
            initial = context.name,
            confirmLabel = stringResource(R.string.common_save),
            onConfirm = callbacks.onRename,
            onDismiss = { renaming = false },
        )
    }
    if (deleting && context != null) {
        ConfirmDeleteDialog(
            title = stringResource(R.string.page_context_confirmDeleteTitle),
            body = stringResource(R.string.page_context_confirmDeleteNamed, context.name),
            onConfirm = callbacks.onDelete,
            onDismiss = { deleting = false },
        )
    }
}

/**
 * The same screen, driven by a view model.
 *
 * The stateless form above is the one that is exercised in tests; this is the
 * wiring the app uses, and it holds nothing of its own so the two cannot drift.
 */
@Composable
fun ContextScreen(
    contextLocalId: Long,
    onOpenProject: (Project) -> Unit,
    onOpenTask: (Task) -> Unit,
    onLeave: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ContextViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    val today by viewModel.today.collectAsStateWithLifecycle()
    LaunchedEffect(contextLocalId) { viewModel.open(contextLocalId) }

    ContextScreen(
        state = state,
        zone = viewModel.zone,
        today = today,
        messages = presenter.messages,
        callbacks =
            remember(presenter, onOpenProject, onOpenTask, onLeave) {
                ContextCallbacks(
                    onRefresh = presenter::refresh,
                    onOpenProject = onOpenProject,
                    onOpenTask = onOpenTask,
                    onToggleComplete = presenter::toggleComplete,
                    onRename = presenter::rename,
                    // Deleting the context takes the screen with it: what it was
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

@Composable
private fun ContextHeader(
    name: String,
    onRename: () -> Unit,
    onDelete: () -> Unit,
) {
    var open by remember { mutableStateOf(false) }
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, top = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = name,
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.onSurface,
            modifier = Modifier.weight(1f),
        )
        Box {
            IconButton(onClick = { open = true }) {
                Icon(
                    imageVector = Icons.Filled.MoreVert,
                    contentDescription = stringResource(R.string.context_actionsAriaLabel),
                )
            }
            DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.dialog_context_editTitle)) },
                    onClick = {
                        open = false
                        onRename()
                    },
                )
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.common_delete)) },
                    onClick = {
                        open = false
                        onDelete()
                    },
                )
            }
        }
    }
    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
}

@Composable
private fun Heading(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, top = 12.dp, bottom = 4.dp),
    )
}

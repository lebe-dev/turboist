package ru.tinyops.turboist.nativeapp.labels.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Star
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
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
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.labels.LabelDetailUiState
import ru.tinyops.turboist.nativeapp.labels.LabelMessage
import ru.tinyops.turboist.nativeapp.labels.LabelTasksViewModel
import ru.tinyops.turboist.nativeapp.projects.ui.EmptyMessage
import ru.tinyops.turboist.nativeapp.tasks.ui.EmptyListText
import ru.tinyops.turboist.nativeapp.tasks.ui.TaskListScreen

/** What the header of a label's own screen can be asked to do. */
data class LabelDetailCallbacks(
    val onEdit: (name: String, color: String) -> Unit,
    val onToggleFavourite: () -> Unit,
    val onTogglePrivate: () -> Unit,
    val onDelete: () -> Unit,
)

/**
 * The header of one label's screen: the label itself and everything that can be
 * done to it.
 *
 * The work carrying the label is drawn below by the shared list, so nothing
 * about a task is decided here — only what the label is called, how it is drawn,
 * whether it is kept to hand, whether it is hidden from the shared read-only
 * view, and whether it exists at all.
 */
@Composable
fun LabelHeader(
    label: Label,
    taggedTasks: Int,
    callbacks: LabelDetailCallbacks,
    modifier: Modifier = Modifier,
) {
    var menuOpen by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf(false) }
    var deleting by remember { mutableStateOf(false) }

    Column(modifier = modifier.fillMaxWidth()) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
        ) {
            LabelName(label, modifier = Modifier.weight(1f, fill = false))
            if (label.isPrivate) {
                Icon(
                    imageVector = Icons.Outlined.Lock,
                    contentDescription = stringResource(R.string.common_privateMarker),
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(16.dp),
                )
            }
            Box(modifier = Modifier.weight(1f))
            IconButton(onClick = callbacks.onToggleFavourite) {
                Icon(
                    imageVector = Icons.Outlined.Star,
                    contentDescription =
                        stringResource(
                            if (label.isFavourite) R.string.common_unfavourite else R.string.common_favourite,
                        ),
                    tint =
                        if (label.isFavourite) {
                            MaterialTheme.colorScheme.primary
                        } else {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        },
                )
            }
            Box {
                IconButton(onClick = { menuOpen = true }) {
                    Icon(
                        imageVector = Icons.Filled.MoreVert,
                        contentDescription = stringResource(R.string.label_actionsAriaLabel),
                    )
                }
                DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.common_edit)) },
                        onClick = {
                            menuOpen = false
                            editing = true
                        },
                    )
                    DropdownMenuItem(
                        text = {
                            Text(
                                stringResource(
                                    if (label.isPrivate) {
                                        R.string.common_unmarkPrivate
                                    } else {
                                        R.string.common_markPrivate
                                    },
                                ),
                            )
                        },
                        onClick = {
                            menuOpen = false
                            callbacks.onTogglePrivate()
                        },
                    )
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.common_delete)) },
                        onClick = {
                            menuOpen = false
                            deleting = true
                        },
                    )
                }
            }
        }
        HorizontalDivider()
    }

    if (editing) {
        LabelEditorSheet(
            initial = LabelDraft(label.name, label.color),
            onConfirm = { draft -> callbacks.onEdit(draft.name, draft.color) },
            onDismiss = { editing = false },
        )
    }
    if (deleting) {
        ConfirmLabelDeleteDialog(
            taggedTasks = taggedTasks,
            onConfirm = callbacks.onDelete,
            onDismiss = { deleting = false },
        )
    }
}

/**
 * One label and the work carrying it.
 *
 * The list is the shared one, so a row here behaves exactly as it does on the
 * day view — ticked off, parked, planned, picked as part of a selection — and
 * nothing about a task had to be reimplemented for this screen. What is added is
 * the header, which is about the label rather than about the work.
 *
 * Finished and abandoned work stays on the list. A label is a cross-cutting
 * marking of the workspace rather than a place work lives, so the question it
 * answers is "what did I mark this way", not "what is left to do".
 */
@Composable
fun LabelTasksScreen(
    labelLocalId: Long,
    onOpenTask: (Task) -> Unit,
    onLeave: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: LabelTasksViewModel = hiltViewModel(),
) {
    LaunchedEffect(labelLocalId) { viewModel.open(labelLocalId) }
    val detail = viewModel.detail
    val state by detail.state.collectAsStateWithLifecycle()

    LabelTasksScreen(
        state = state,
        messages = detail.messages,
        callbacks =
            remember(detail, onLeave) {
                LabelDetailCallbacks(
                    onEdit = detail::save,
                    onToggleFavourite = detail::toggleFavourite,
                    onTogglePrivate = detail::togglePrivate,
                    // Deleting the label takes the screen with it: what it was
                    // showing no longer exists anywhere.
                    onDelete = {
                        detail.delete()
                        onLeave()
                    },
                )
            },
        modifier = modifier,
    ) {
        TaskListScreen(
            viewModel = viewModel,
            empty =
                EmptyListText(
                    title = stringResource(R.string.page_label_emptyTitle),
                    description = stringResource(R.string.page_label_emptyDescription),
                ),
            onOpenTask = onOpenTask,
        )
    }
}

/**
 * The same screen with the list handed in, which is the form the tests drive.
 *
 * The list is a slot rather than a parameter because it is a whole screen of its
 * own with its own view model; taking it as content keeps the header's behaviour
 * checkable without the replica behind that list.
 */
@Composable
fun LabelTasksScreen(
    state: LabelDetailUiState,
    messages: Flow<LabelMessage>,
    callbacks: LabelDetailCallbacks,
    modifier: Modifier = Modifier,
    tasks: @Composable () -> Unit,
) {
    val snackbars = remember { SnackbarHostState() }
    LabelMessageHost(messages, snackbars)

    Box(modifier = modifier.fillMaxSize()) {
        val label = state.label
        when {
            state.missing ->
                EmptyMessage(
                    title = stringResource(R.string.page_label_notFound),
                    description = stringResource(R.string.page_label_emptyDescription),
                )

            label != null ->
                Column(modifier = Modifier.fillMaxSize()) {
                    LabelHeader(label = label, taggedTasks = state.taggedTasks, callbacks = callbacks)
                    tasks()
                }
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }
}

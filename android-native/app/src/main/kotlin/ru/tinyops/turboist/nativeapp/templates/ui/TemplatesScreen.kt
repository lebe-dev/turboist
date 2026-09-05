package ru.tinyops.turboist.nativeapp.templates.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.PlayArrow
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.TaskTemplate
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.templates.TemplateEditor
import ru.tinyops.turboist.nativeapp.templates.TemplateMessage
import ru.tinyops.turboist.nativeapp.templates.TemplatesUiState
import ru.tinyops.turboist.nativeapp.templates.TemplatesViewModel

/** What the templates screen can be asked to do. */
data class TemplatesCallbacks(
    val onRefresh: () -> Unit,
    val onStartNew: () -> Unit,
    val onEdit: (TaskTemplate) -> Unit,
    val onDelete: (TaskTemplate) -> Unit,
    val onRename: (String) -> Unit,
    val onDescribe: (String) -> Unit,
    val onAddLine: () -> Unit,
    val onRetitleLine: (Int, String) -> Unit,
    val onRemoveLine: (Int) -> Unit,
    val onSave: () -> Unit,
    val onCancelEdit: () -> Unit,
    val onStartInstantiate: (TaskTemplate) -> Unit,
    val onInstantiate: (projectLocalId: Long) -> Unit,
    val onCancelInstantiate: () -> Unit,
)

/**
 * The reusable blueprints.
 *
 * A template is a task and the checklist that comes with it, so the screen shows
 * exactly that: a row per template saying how many lines it carries, and three
 * things to do with it — use it, change it, throw it away.
 *
 * The editor writes the name, the description and the checklist. What it does not
 * offer, a template's labels and its urgency, it still carries: saving a template
 * replaces it whole, so an editor that dropped what it cannot show would quietly
 * strip a template edited on the web.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TemplatesScreen(
    state: TemplatesUiState,
    messages: Flow<TemplateMessage>,
    callbacks: TemplatesCallbacks,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    TemplateMessageHost(messages, snackbars)
    var confirmingDelete by remember { mutableStateOf<TaskTemplate?>(null) }

    Box(modifier = modifier.fillMaxSize()) {
        PullToRefreshBox(
            isRefreshing = state.refreshing,
            onRefresh = callbacks.onRefresh,
            modifier = Modifier.fillMaxSize(),
        ) {
            if (state.isEmpty) {
                EmptyTemplates()
            } else {
                LazyColumn(modifier = Modifier.fillMaxSize()) {
                    items(state.templates, key = { it.localId }) { template ->
                        TemplateRow(
                            template = template,
                            onUse = { callbacks.onStartInstantiate(template) },
                            onEdit = { callbacks.onEdit(template) },
                            onDelete = { confirmingDelete = template },
                        )
                        HorizontalDivider()
                    }
                }
            }
        }
        ExtendedFloatingActionButton(
            onClick = callbacks.onStartNew,
            icon = { Icon(imageVector = Icons.Filled.Add, contentDescription = null) },
            text = { Text(stringResource(R.string.settings_templates_new)) },
            modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp),
        )
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }

    state.editor?.let { editor ->
        TemplateEditorDialog(editor = editor, callbacks = callbacks)
    }

    // The picker names no template: it is the one the user just tapped, and
    // repeating its name would say nothing new.
    if (state.choosingProjectFor != null) {
        ProjectPickerDialog(
            projects = state.projects,
            onPick = callbacks.onInstantiate,
            onDismiss = callbacks.onCancelInstantiate,
        )
    }

    confirmingDelete?.let { template ->
        AlertDialog(
            onDismissRequest = { confirmingDelete = null },
            title = { Text(stringResource(R.string.settings_templates_confirmDeleteTitle)) },
            text = { Text(stringResource(R.string.settings_templates_confirmDeleteNamed, template.name)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmingDelete = null
                        callbacks.onDelete(template)
                    },
                ) { Text(stringResource(R.string.common_delete)) }
            },
            dismissButton = {
                TextButton(onClick = { confirmingDelete = null }) {
                    Text(stringResource(R.string.common_cancel))
                }
            },
        )
    }
}

/** The screen as the graph shows it, wired to the replica. */
@Composable
fun TemplatesScreen(
    onOpenTask: (Long) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: TemplatesViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    // A template that has just been used made work somewhere the user is not
    // looking, so the screen goes there rather than leaving them to find it.
    LaunchedEffect(presenter) { presenter.instantiated.collect { taskLocalId -> onOpenTask(taskLocalId) } }
    TemplatesScreen(
        state = state,
        messages = presenter.messages,
        callbacks =
            remember(presenter) {
                TemplatesCallbacks(
                    onRefresh = presenter::refresh,
                    onStartNew = presenter::startNewTemplate,
                    onEdit = presenter::edit,
                    onDelete = presenter::delete,
                    onRename = presenter::rename,
                    onDescribe = presenter::describe,
                    onAddLine = presenter::addLine,
                    onRetitleLine = presenter::retitleLine,
                    onRemoveLine = presenter::removeLine,
                    onSave = presenter::save,
                    onCancelEdit = presenter::cancelEdit,
                    onStartInstantiate = presenter::startInstantiate,
                    onInstantiate = presenter::instantiate,
                    onCancelInstantiate = presenter::cancelInstantiate,
                )
            },
        modifier = modifier,
    )
}

@Composable
private fun TemplateRow(
    template: TaskTemplate,
    onUse: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    ListItem(
        headlineContent = { Text(template.name) },
        supportingContent = {
            Text(
                pluralStringResource(
                    R.plurals.settings_templates_subtaskCount,
                    template.subtasks.size,
                    template.subtasks.size,
                ),
            )
        },
        trailingContent = {
            Row {
                IconButton(onClick = onUse) {
                    Icon(
                        imageVector = Icons.Outlined.PlayArrow,
                        contentDescription = stringResource(R.string.topbar_fromTemplate),
                    )
                }
                IconButton(onClick = onEdit) {
                    Icon(
                        imageVector = Icons.Outlined.Edit,
                        contentDescription = stringResource(R.string.settings_templates_edit),
                    )
                }
                IconButton(onClick = onDelete) {
                    Icon(
                        imageVector = Icons.Outlined.Delete,
                        contentDescription = stringResource(R.string.settings_templates_delete),
                    )
                }
            }
        },
        modifier = Modifier.clickable(onClick = onEdit),
    )
}

/**
 * The editor, as a dialog over the list.
 *
 * The checklist is a column of fields rather than anything cleverer: a template's
 * lines are titles, they are short, and typing them one under another is the same
 * gesture as writing the outline down on paper.
 */
@Composable
private fun TemplateEditorDialog(
    editor: TemplateEditor,
    callbacks: TemplatesCallbacks,
) {
    AlertDialog(
        onDismissRequest = callbacks.onCancelEdit,
        title = {
            Text(
                stringResource(
                    if (editor.templateLocalId == null) {
                        R.string.settings_templates_newTitle
                    } else {
                        R.string.settings_templates_editTitle
                    },
                ),
            )
        },
        text = {
            Column(
                modifier = Modifier.heightIn(max = EDITOR_MAX_HEIGHT).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    text = stringResource(R.string.settings_templates_dialogDescription),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = editor.name,
                    onValueChange = callbacks.onRename,
                    singleLine = true,
                    label = { Text(stringResource(R.string.settings_templates_nameLabel)) },
                    placeholder = { Text(stringResource(R.string.settings_templates_namePlaceholder)) },
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = editor.description,
                    onValueChange = callbacks.onDescribe,
                    placeholder = { Text(stringResource(R.string.settings_templates_descriptionPlaceholder)) },
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    text = stringResource(R.string.settings_templates_subtasksHeading),
                    style = MaterialTheme.typography.titleSmall,
                )
                editor.lines.forEachIndexed { index, line ->
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp),
                    ) {
                        OutlinedTextField(
                            value = line.title,
                            onValueChange = { title -> callbacks.onRetitleLine(index, title) },
                            singleLine = true,
                            placeholder = {
                                Text(stringResource(R.string.settings_templates_subtaskTitlePlaceholder))
                            },
                            modifier = Modifier.weight(1f),
                        )
                        IconButton(onClick = { callbacks.onRemoveLine(index) }) {
                            Icon(
                                imageVector = Icons.Outlined.Delete,
                                contentDescription = stringResource(R.string.settings_templates_removeSubtask),
                            )
                        }
                    }
                }
                TextButton(onClick = callbacks.onAddLine) {
                    Text(stringResource(R.string.settings_templates_addSubtask))
                }
            }
        },
        confirmButton = {
            TextButton(onClick = callbacks.onSave, enabled = editor.canSave) {
                Text(stringResource(R.string.common_save))
            }
        },
        dismissButton = {
            TextButton(onClick = callbacks.onCancelEdit) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

/** Where a template's work is to go. */
@Composable
private fun ProjectPickerDialog(
    projects: List<Project>,
    onPick: (Long) -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.template_pickProject_title)) },
        text = {
            Column(modifier = Modifier.heightIn(max = EDITOR_MAX_HEIGHT).verticalScroll(rememberScrollState())) {
                Text(
                    text = stringResource(R.string.template_pickProject_description),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                for (project in projects) {
                    ListItem(
                        headlineContent = { Text(project.title) },
                        modifier = Modifier.clickable { onPick(project.localId) },
                    )
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

@Composable
private fun EmptyTemplates() {
    Column(
        modifier = Modifier.fillMaxSize().padding(32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Text(
            text = stringResource(R.string.settings_templates_empty),
            style = MaterialTheme.typography.titleMedium,
            textAlign = TextAlign.Center,
        )
        Text(
            text = stringResource(R.string.settings_templates_description),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
    }
}

/** Shows what the presenter has to say, one sentence at a time. */
@Composable
private fun TemplateMessageHost(
    messages: Flow<TemplateMessage>,
    snackbars: SnackbarHostState,
) {
    var pending by remember { mutableStateOf<TemplateMessage?>(null) }
    LaunchedEffect(messages) { messages.collect { pending = it } }
    val message = pending ?: return
    val text = templateMessageText(message)
    LaunchedEffect(message, text) {
        snackbars.showSnackbar(text)
        pending = null
    }
}

@Composable
private fun templateMessageText(message: TemplateMessage): String =
    when (message) {
        TemplateMessage.Saved -> stringResource(R.string.settings_templates_toastSaved)
        TemplateMessage.SaveFailed -> stringResource(R.string.settings_templates_toastSaveFailed)
        TemplateMessage.Deleted -> stringResource(R.string.settings_templates_toastDeleted)
        TemplateMessage.DeleteFailed -> stringResource(R.string.settings_templates_toastDeleteFailed)
        is TemplateMessage.Instantiated ->
            pluralStringResource(R.plurals.template_toast_created, message.taskCount, message.taskCount)

        TemplateMessage.InstantiateFailed -> stringResource(R.string.template_toast_failed)
    }

/** How tall a dialog's scrolling body may grow before it scrolls instead. */
private val EDITOR_MAX_HEIGHT = 420.dp

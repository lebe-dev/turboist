package ru.tinyops.turboist.nativeapp.projects.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.ui.LabelSelector

/**
 * Asks for one line of text and hands it back.
 *
 * Every naming gesture on these screens — a new project, a new column, renaming
 * either, writing down a task — is the same question asked about a different
 * thing, so it is one dialog told what to call itself rather than five that
 * drift apart.
 */
@Composable
fun NameDialog(
    title: String,
    label: String,
    initial: String,
    confirmLabel: String,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var typed by remember { mutableStateOf(initial) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            OutlinedTextField(
                value = typed,
                onValueChange = { typed = it },
                label = { Text(label) },
                singleLine = true,
            )
        },
        confirmButton = {
            TextButton(
                enabled = typed.isNotBlank(),
                onClick = {
                    onConfirm(typed)
                    onDismiss()
                },
            ) { Text(confirmLabel) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

/**
 * Starts a project: its name, what it is for, and the labels it carries.
 *
 * The same three things the web's project form asks for, so a project does not
 * have to be created bare and edited afterwards to say what it is.
 */
@Composable
fun NewProjectDialog(
    knownLabels: List<Label>,
    onConfirm: (title: String, description: String, labels: List<String>) -> Unit,
    onDismiss: () -> Unit,
) {
    var title by remember { mutableStateOf("") }
    var description by remember { mutableStateOf("") }
    var labels by remember { mutableStateOf(emptyList<String>()) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.dialog_project_newTitle)) },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                OutlinedTextField(
                    value = title,
                    onValueChange = { title = it },
                    label = { Text(stringResource(R.string.common_title)) },
                    singleLine = true,
                )
                OutlinedTextField(
                    value = description,
                    onValueChange = { description = it },
                    label = { Text(stringResource(R.string.common_description)) },
                    minLines = 3,
                )
                if (knownLabels.isNotEmpty()) {
                    Text(
                        text = stringResource(R.string.common_labels),
                        style = MaterialTheme.typography.labelMedium,
                    )
                    LabelSelector(known = knownLabels, selected = labels, onChange = { labels = it })
                }
            }
        },
        confirmButton = {
            TextButton(
                enabled = title.isNotBlank(),
                onClick = {
                    onConfirm(title, description, labels)
                    onDismiss()
                },
            ) { Text(stringResource(R.string.common_create)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

/**
 * Asks before something is destroyed.
 *
 * Deletes here are hard on both sides and cascade — a project takes its columns
 * and tasks, a context takes its projects — and nothing in the product can undo
 * one, so the sentence says what goes rather than only asking whether to go on.
 */
@Composable
fun ConfirmDeleteDialog(
    title: String,
    body: String,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = { Text(body) },
        confirmButton = {
            TextButton(
                onClick = {
                    onConfirm()
                    onDismiss()
                },
            ) { Text(stringResource(R.string.common_delete)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

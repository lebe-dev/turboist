package ru.tinyops.turboist.nativeapp.account.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.dto.ApiTokenDto
import ru.tinyops.turboist.core.network.dto.CreatedApiTokenDto
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.account.ApiTokensUiState
import ru.tinyops.turboist.nativeapp.account.ApiTokensViewModel
import ru.tinyops.turboist.nativeapp.account.ScopePreset
import ru.tinyops.turboist.nativeapp.account.ScopeResource
import ru.tinyops.turboist.nativeapp.account.ScopeSelection
import ru.tinyops.turboist.nativeapp.account.grantsFullAccess
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** Everything the API tokens screen can be asked to do. */
data class ApiTokenCallbacks(
    val onEditName: (String) -> Unit = {},
    val onApplyPreset: (ScopePreset) -> Unit = {},
    val onSetFullAccess: (Boolean) -> Unit = {},
    val onSetRead: (ScopeResource, Boolean) -> Unit = { _, _ -> },
    val onSetWrite: (ScopeResource, Boolean) -> Unit = { _, _ -> },
    val onCreate: () -> Unit = {},
    val onDismissCreated: () -> Unit = {},
    val onAskDelete: (ApiTokenDto) -> Unit = {},
    val onCancelDelete: () -> Unit = {},
    val onConfirmDelete: () -> Unit = {},
    val onRetry: () -> Unit = {},
)

/**
 * The long-lived tokens external tools authenticate with.
 *
 * Two things about this screen are not decoration. The permissions are chosen
 * before the token exists and can never be changed afterwards, so the form makes
 * the whole grant visible rather than defaulting to everything. And the token's
 * plaintext appears exactly once, in the dialog that follows creating it — the
 * server keeps only a hash, so a user who closes that dialog without copying the
 * value has to issue another token. The dialog says so before it shows it.
 *
 * Nothing here is cached. The list is read from the server every time, and the
 * plaintext is held in memory only for as long as its dialog is up.
 */
@Composable
fun ApiTokensScreen(
    modifier: Modifier = Modifier,
    viewModel: ApiTokensViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    ApiTokensContent(
        state = state,
        callbacks =
            ApiTokenCallbacks(
                onEditName = viewModel::editName,
                onApplyPreset = viewModel::applyPreset,
                onSetFullAccess = viewModel::setFullAccess,
                onSetRead = viewModel::setRead,
                onSetWrite = viewModel::setWrite,
                onCreate = viewModel::create,
                onDismissCreated = viewModel::dismissCreated,
                onAskDelete = viewModel::askDelete,
                onCancelDelete = viewModel::cancelDelete,
                onConfirmDelete = viewModel::confirmDelete,
                onRetry = viewModel::load,
            ),
        modifier = modifier,
    )
}

@Composable
internal fun ApiTokensContent(
    state: ApiTokensUiState,
    callbacks: ApiTokenCallbacks,
    modifier: Modifier = Modifier,
) {
    LazyColumn(
        modifier = modifier.fillMaxSize().padding(horizontal = 16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        item {
            Text(
                text = stringResource(R.string.settings_api_description),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 16.dp),
            )
        }
        if (state.unreachable) {
            // The form goes with the list. A token minted against a server that
            // cannot be reached is a request that fails, and offering it would
            // put the user through filling in a grant for nothing.
            item { RequiresConnection(onRetry = callbacks.onRetry) }
            return@LazyColumn
        }

        item { NewTokenForm(state = state, callbacks = callbacks) }
        item {
            if (state.scopesMissing) {
                Text(
                    text = stringResource(R.string.settings_api_scopes_emptyError),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            AccountProblemText(state.problem, R.string.settings_api_createFailed)
        }
        item { HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp)) }

        if (!state.loading && state.tokens.isEmpty()) {
            item {
                Text(
                    text = stringResource(R.string.settings_api_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        items(state.tokens, key = { it.id }) { token ->
            TokenRow(token = token, enabled = !state.busy, onDelete = { callbacks.onAskDelete(token) })
        }
        item { Spacer(modifier = Modifier.padding(bottom = 16.dp)) }
    }

    state.created?.let { minted ->
        CreatedTokenDialog(token = minted, onDismiss = callbacks.onDismissCreated)
    }

    state.deleting?.let {
        AlertDialog(
            onDismissRequest = callbacks.onCancelDelete,
            title = { Text(stringResource(R.string.settings_api_confirmDeleteTitle)) },
            text = { Text(stringResource(R.string.settings_api_confirmDeleteDescription)) },
            confirmButton = {
                TextButton(onClick = callbacks.onConfirmDelete) {
                    Text(stringResource(R.string.settings_api_confirmDeleteAction))
                }
            },
            dismissButton = {
                TextButton(onClick = callbacks.onCancelDelete) { Text(stringResource(R.string.common_cancel)) }
            },
        )
    }
}

/**
 * The form that mints a token: a name, and the whole of what it will ever be
 * allowed to do.
 *
 * The permissions are laid out in full rather than hidden behind a default,
 * because they cannot be changed once the token exists — the only way to widen
 * or narrow one is to delete it and issue another.
 */
@Composable
private fun NewTokenForm(
    state: ApiTokensUiState,
    callbacks: ApiTokenCallbacks,
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedTextField(
            value = state.name,
            onValueChange = callbacks.onEditName,
            label = { Text(stringResource(R.string.settings_api_namePlaceholder)) },
            enabled = !state.busy,
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Text(
            text = stringResource(R.string.settings_api_scopes_title),
            style = MaterialTheme.typography.titleSmall,
        )
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                text = stringResource(R.string.settings_api_scopes_presetsLabel),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.align(Alignment.CenterVertically),
            )
            TextButton(onClick = { callbacks.onApplyPreset(ScopePreset.FULL_ACCESS) }, enabled = !state.busy) {
                Text(stringResource(R.string.settings_api_scopes_presets_full))
            }
            TextButton(onClick = { callbacks.onApplyPreset(ScopePreset.READ_ONLY) }, enabled = !state.busy) {
                Text(stringResource(R.string.settings_api_scopes_presets_readonly))
            }
            TextButton(onClick = { callbacks.onApplyPreset(ScopePreset.TASKS_FULL) }, enabled = !state.busy) {
                Text(stringResource(R.string.settings_api_scopes_presets_tasksFull))
            }
        }
        // The wildcard is not "every box ticked": it also covers scopes a later
        // server version adds, which is why it stands on its own line above the
        // table rather than as a tenth row in it.
        Row(verticalAlignment = Alignment.CenterVertically) {
            Checkbox(
                checked = state.selection.fullAccess,
                onCheckedChange = callbacks.onSetFullAccess,
                enabled = !state.busy,
            )
            Text(stringResource(R.string.settings_api_scopes_presets_full))
        }
        ScopeTableHeader()
        ScopeResource.entries.forEach { resource ->
            ScopeRow(resource = resource, state = state, callbacks = callbacks)
        }
        Button(
            onClick = callbacks.onCreate,
            enabled = state.canCreate,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(
                stringResource(
                    if (state.busy) R.string.settings_api_generating else R.string.settings_api_generate,
                ),
            )
        }
    }
}

@Composable
private fun ScopeTableHeader() {
    Row(modifier = Modifier.fillMaxWidth()) {
        Text(
            text = stringResource(R.string.settings_api_scopes_headers_resource),
            style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.weight(1f),
        )
        Text(
            text = stringResource(R.string.settings_api_scopes_headers_read),
            style = MaterialTheme.typography.labelMedium,
            textAlign = TextAlign.Center,
            modifier = Modifier.weight(SWITCH_COLUMN_WEIGHT),
        )
        Text(
            text = stringResource(R.string.settings_api_scopes_headers_write),
            style = MaterialTheme.typography.labelMedium,
            textAlign = TextAlign.Center,
            modifier = Modifier.weight(SWITCH_COLUMN_WEIGHT),
        )
    }
}

@Composable
private fun ScopeRow(
    resource: ScopeResource,
    state: ApiTokensUiState,
    callbacks: ApiTokenCallbacks,
) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = stringResource(scopeResourceLabel(resource)),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f),
        )
        Row(modifier = Modifier.weight(SWITCH_COLUMN_WEIGHT), horizontalArrangement = Arrangement.Center) {
            Checkbox(
                checked = state.selection.fullAccess || resource in state.selection.read,
                onCheckedChange = { granted -> callbacks.onSetRead(resource, granted) },
                enabled = !state.busy,
            )
        }
        Row(modifier = Modifier.weight(SWITCH_COLUMN_WEIGHT), horizontalArrangement = Arrangement.Center) {
            // A read-only resource has no write scope to grant, so there is
            // nothing to offer rather than a box that cannot be ticked.
            if (resource.writable) {
                Checkbox(
                    checked = state.selection.fullAccess || resource in state.selection.write,
                    onCheckedChange = { granted -> callbacks.onSetWrite(resource, granted) },
                    enabled = !state.busy,
                )
            }
        }
    }
}

@Composable
private fun TokenRow(
    token: ApiTokenDto,
    enabled: Boolean,
    onDelete: () -> Unit,
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp)) {
            Text(text = token.name, style = MaterialTheme.typography.titleMedium)
            Text(
                text = "${stringResource(R.string.settings_api_created)}: ${dateLabel(token.createdAt)}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            FlowRow(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                if (token.scopes.grantsFullAccess()) {
                    AssistChip(
                        onClick = {},
                        enabled = false,
                        label = { Text(stringResource(R.string.settings_api_scopes_fullBadge)) },
                    )
                } else {
                    token.scopes.forEach { scope ->
                        AssistChip(onClick = {}, enabled = false, label = { Text(scope) })
                    }
                }
            }
            Row(horizontalArrangement = Arrangement.End, modifier = Modifier.fillMaxWidth()) {
                TextButton(onClick = onDelete, enabled = enabled) {
                    Text(stringResource(R.string.settings_api_delete))
                }
            }
        }
    }
}

/**
 * The one and only sight of a token's plaintext.
 *
 * The warning comes before the value rather than after it: by the time the user
 * has read past the token they have already decided whether to copy it, and the
 * server cannot show it again.
 */
@Composable
private fun CreatedTokenDialog(
    token: CreatedApiTokenDto,
    onDismiss: () -> Unit,
) {
    val copy = rememberSecretCopier()
    var copied by remember(token.id) { mutableStateOf(false) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(token.name) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    text = stringResource(R.string.settings_api_warningOnce),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
                Text(text = token.token, style = MaterialTheme.typography.bodyMedium)
                if (copied) {
                    Text(
                        text = stringResource(R.string.settings_api_copied),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                copy(token.token)
                copied = true
            }) { Text(stringResource(R.string.settings_api_copy)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.settings_api_close)) }
        },
    )
}

/** The name each resource goes by, in the wording the web client already uses. */
private fun scopeResourceLabel(resource: ScopeResource): Int =
    when (resource) {
        ScopeResource.TASKS -> R.string.settings_api_scopes_resources_tasks
        ScopeResource.PROJECTS -> R.string.settings_api_scopes_resources_projects
        ScopeResource.CONTEXTS -> R.string.settings_api_scopes_resources_contexts
        ScopeResource.LABELS -> R.string.settings_api_scopes_resources_labels
        ScopeResource.SECTIONS -> R.string.settings_api_scopes_resources_sections
        ScopeResource.TEMPLATES -> R.string.settings_tabs_templates
        ScopeResource.TROIKI -> R.string.settings_api_scopes_resources_troiki
        ScopeResource.SETTINGS -> R.string.settings_api_scopes_resources_settings
        ScopeResource.SEARCH -> R.string.settings_api_scopes_resources_search
        ScopeResource.CALENDARS -> R.string.settings_api_scopes_resources_calendars
    }

/** A timestamp as a date in the device's own zone; the wire form is UTC to the millisecond. */
private fun dateLabel(wireTime: String): String {
    val epochMillis = WireTime.parseOrNull(wireTime) ?: return ""
    return Instant.ofEpochMilli(epochMillis)
        .atZone(ZoneId.systemDefault())
        .toLocalDate()
        .format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
}

/** How much of the row the read and write columns take next to the resource name. */
private const val SWITCH_COLUMN_WEIGHT = 0.3f

@Preview(showBackground = true)
@Composable
private fun ApiTokensScreenPreview() {
    TurboistTheme(dynamicColor = false) {
        ApiTokensContent(
            state =
                ApiTokensUiState(
                    loading = false,
                    tokens =
                        listOf(
                            ApiTokenDto(
                                id = 1,
                                name = "n8n",
                                scopes = listOf("tasks:read", "tasks:write"),
                                createdAt = "2026-08-18T10:00:00.000Z",
                            ),
                            ApiTokenDto(
                                id = 2,
                                name = "admin",
                                scopes = listOf("*"),
                                createdAt = "2026-08-19T10:00:00.000Z",
                            ),
                        ),
                    selection = ScopeSelection.TASKS_FULL,
                ),
            callbacks = ApiTokenCallbacks(),
        )
    }
}

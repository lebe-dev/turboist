package ru.tinyops.turboist.nativeapp.auth.passkey.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
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
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.dto.PasskeyDto
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * The passkeys the account can sign in with, and the way to add one.
 *
 * The screen never presents itself as the way in: the password stays the
 * recovery path, so removing the last credential here is allowed and the
 * description says what a passkey is *instead of* — typing a password — rather
 * than what it replaces.
 *
 * Nothing on it is cached. A stale list of credentials would answer "which
 * devices can sign in to this account" wrongly, which is the one question where
 * a stale answer is worse than no answer, so an unreachable server is stated as
 * such.
 */
@Composable
fun PasskeysScreen(
    modifier: Modifier = Modifier,
    viewModel: PasskeysViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val authenticator = rememberPasskeyAuthenticator()

    PasskeysContent(
        state = state,
        onAdd = { name -> viewModel.add(authenticator, name) },
        onRename = viewModel::rename,
        onRemove = viewModel::remove,
        onRetry = viewModel::load,
        modifier = modifier,
    )
}

@Composable
private fun PasskeysContent(
    state: PasskeysUiState,
    onAdd: (String) -> Unit,
    onRename: (Long, String) -> Unit,
    onRemove: (Long) -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var renaming by remember { mutableStateOf<PasskeyDto?>(null) }
    var removing by remember { mutableStateOf<PasskeyDto?>(null) }

    LazyColumn(
        modifier = modifier.fillMaxSize().padding(horizontal = 16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        item {
            Text(
                text = stringResource(R.string.settings_passkeys_description),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 16.dp),
            )
        }
        item { AddPasskey(state = state, onAdd = onAdd) }
        item {
            if (state.added) {
                Text(
                    text = stringResource(R.string.settings_passkeys_added),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
            PasskeyProblemText(state.problem)
        }
        item { HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp)) }

        if (state.unreachable) {
            item {
                Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.fillMaxWidth()) {
                    Text(
                        text = stringResource(R.string.settings_passkeys_loadFailed),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    TextButton(onClick = onRetry) { Text(stringResource(R.string.native_passkeys_retry)) }
                }
            }
            return@LazyColumn
        }

        if (!state.loading && state.passkeys.isEmpty()) {
            item {
                Text(
                    text = stringResource(R.string.settings_passkeys_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        items(state.passkeys, key = { it.id }) { passkey ->
            PasskeyRow(
                passkey = passkey,
                enabled = !state.busy,
                onRename = { renaming = passkey },
                onRemove = { removing = passkey },
            )
        }
    }

    renaming?.let { passkey ->
        RenameDialog(
            passkey = passkey,
            onDismiss = { renaming = null },
            onConfirm = { name ->
                renaming = null
                onRename(passkey.id, name)
            },
        )
    }

    removing?.let { passkey ->
        AlertDialog(
            onDismissRequest = { removing = null },
            title = { Text(stringResource(R.string.settings_passkeys_confirmRemoveTitle)) },
            text = { Text(stringResource(R.string.settings_passkeys_confirmRemoveDescription)) },
            confirmButton = {
                TextButton(onClick = {
                    removing = null
                    onRemove(passkey.id)
                }) { Text(stringResource(R.string.settings_passkeys_remove)) }
            },
            dismissButton = {
                TextButton(onClick = { removing = null }) { Text(stringResource(R.string.common_cancel)) }
            },
        )
    }
}

/**
 * The enrolment form: a name for the device, and the button that opens the
 * platform sheet. The name is optional — an empty one lets the server label the
 * credential rather than sending a placeholder this screen invented.
 */
@Composable
private fun AddPasskey(
    state: PasskeysUiState,
    onAdd: (String) -> Unit,
) {
    var name by remember { mutableStateOf("") }

    OutlinedTextField(
        value = name,
        onValueChange = { name = it },
        label = { Text(stringResource(R.string.settings_passkeys_namePlaceholder)) },
        enabled = !state.busy,
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    Button(
        onClick = {
            onAdd(name)
            name = ""
        },
        enabled = !state.busy,
        modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
    ) {
        Text(
            stringResource(
                if (state.awaitingDevice) R.string.settings_passkeys_adding else R.string.settings_passkeys_add,
            ),
        )
    }
}

@Composable
private fun PasskeyRow(
    passkey: PasskeyDto,
    enabled: Boolean,
    onRename: () -> Unit,
    onRemove: () -> Unit,
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp)) {
            Text(text = passkey.name, style = MaterialTheme.typography.titleMedium)
            Text(
                text = "${stringResource(R.string.settings_passkeys_created)}: ${dateLabel(passkey.createdAt)}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = lastUsedLabel(passkey),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(horizontalArrangement = Arrangement.End, modifier = Modifier.fillMaxWidth()) {
                TextButton(onClick = onRename, enabled = enabled) {
                    Text(stringResource(R.string.settings_passkeys_rename))
                }
                TextButton(onClick = onRemove, enabled = enabled) {
                    Text(stringResource(R.string.settings_passkeys_remove))
                }
            }
        }
    }
}

@Composable
private fun RenameDialog(
    passkey: PasskeyDto,
    onDismiss: () -> Unit,
    onConfirm: (String) -> Unit,
) {
    var name by remember(passkey.id) { mutableStateOf(passkey.name) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.settings_passkeys_rename)) },
        text = {
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text(stringResource(R.string.common_name)) },
                singleLine = true,
            )
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(name) }, enabled = name.isNotBlank()) {
                Text(stringResource(R.string.common_save))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) } },
    )
}

@Composable
private fun lastUsedLabel(passkey: PasskeyDto): String {
    val lastUsed = passkey.lastUsedAt
    if (lastUsed == null) return stringResource(R.string.settings_passkeys_neverUsed)
    return "${stringResource(R.string.settings_passkeys_lastUsed)}: ${dateLabel(lastUsed)}"
}

/**
 * A timestamp as a date in the device's own zone. The wire form is UTC to the
 * millisecond, which is not what a person reads a credential's age from.
 */
private fun dateLabel(wireTime: String): String {
    val epochMillis = WireTime.parseOrNull(wireTime) ?: return ""
    return Instant.ofEpochMilli(epochMillis)
        .atZone(ZoneId.systemDefault())
        .toLocalDate()
        .format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
}

@Preview(showBackground = true)
@Composable
private fun PasskeysScreenPreview() {
    TurboistTheme {
        PasskeysContent(
            state =
                PasskeysUiState(
                    loading = false,
                    passkeys =
                        listOf(
                            PasskeyDto(
                                id = 1,
                                name = "Pixel",
                                createdAt = "2026-08-18T10:00:00.000Z",
                                lastUsedAt = "2026-08-20T09:12:00.000Z",
                            ),
                            PasskeyDto(id = 2, name = "Security key", createdAt = "2026-08-19T10:00:00.000Z"),
                        ),
                ),
            onAdd = {},
            onRename = { _, _ -> },
            onRemove = {},
            onRetry = {},
        )
    }
}

package ru.tinyops.turboist.nativeapp.settings.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.os.Build
import android.widget.Toast
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.BugReport
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.DeleteSweep
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.PhoneAndroid
import androidx.compose.material.icons.outlined.SignalCellularAlt
import androidx.compose.material.icons.outlined.SyncAlt
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.SettingsConfirmation
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState

/**
 * The choices that stay on this phone, and the server it talks to.
 *
 * None of it reaches the server, and that is the point of drawing it apart from
 * the account's settings: whether a radio may spend a data allowance is a fact
 * about a device, and a tablet on wi-fi wants a different answer from a phone on
 * a metered plan.
 *
 * Label suggestions are switched on here as well: the model they run on is a
 * download onto this phone, and what it learns never leaves it.
 *
 * The two destructive actions live here too, under a heading of their own,
 * because both are about this device rather than about the workspace: one throws
 * the copy away and asks for it again, the other walks away from the server
 * altogether.
 */
@Composable
internal fun DeviceSection(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
    destinations: SettingsDestinations,
    labelSuggestions: @Composable () -> Unit = {},
) {
    val context = LocalContext.current
    val scheme = MaterialTheme.colorScheme

    Surface(
        shape = RoundedCornerShape(20.dp),
        color = scheme.surfaceContainerHighest,
        modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 8.dp),
    ) {
        SettingsNote(
            text = stringResource(R.string.native_settings_deviceDescription),
            icon = Icons.Outlined.PhoneAndroid,
        )
    }

    SettingsGroupLabel(stringResource(R.string.native_settings_syncHeading))
    SettingsGroup {
        row {
            SettingsSwitchRow(
                title = stringResource(R.string.native_settings_syncOnMetered),
                checked = state.device.syncOnMetered,
                onCheckedChange = callbacks.onSetSyncOnMetered,
                description = stringResource(R.string.native_settings_syncOnMeteredDescription),
                icon = Icons.Outlined.SignalCellularAlt,
            )
        }
        row { UnsentChangesRow(state.refusedChangeCount, destinations.openUnsentChanges) }
    }

    // A device choice like the two around it: the model and what it learns stay on this phone.
    labelSuggestions()

    if (state.device.errorReportingAvailable) {
        SettingsGroupLabel(stringResource(R.string.native_settings_diagnosticsHeading))
        SettingsGroup {
            row {
                SettingsSwitchRow(
                    title = stringResource(R.string.native_settings_sendErrorReports),
                    checked = state.device.sendErrorReports,
                    onCheckedChange = callbacks.onSetSendErrorReports,
                    description = stringResource(R.string.native_settings_sendErrorReportsDescription),
                    icon = Icons.Outlined.BugReport,
                )
            }
        }
    }

    SettingsGroupLabel(stringResource(R.string.native_settings_serverHeading))
    SettingsGroup {
        row {
            ListItem(
                headlineContent = {
                    Text(
                        text = state.serverAddress.ifEmpty { stringResource(R.string.native_settings_serverUnset) },
                        fontFamily = FontFamily.Monospace,
                        style = MaterialTheme.typography.bodyMedium,
                    )
                },
                leadingContent = { Icon(imageVector = Icons.Outlined.Dns, contentDescription = null) },
                trailingContent = {
                    if (state.serverAddress.isNotEmpty()) {
                        IconButton(onClick = { copyServerAddress(context, state.serverAddress) }) {
                            Icon(
                                imageVector = Icons.Outlined.ContentCopy,
                                contentDescription = stringResource(R.string.native_settings_copyServer),
                            )
                        }
                    }
                },
                colors = groupRowColors(),
            )
        }
    }

    SettingsGroupLabel(text = stringResource(R.string.native_settings_resetHeading), color = scheme.error)
    SettingsGroup {
        row {
            DestructiveRow(
                titleRes = R.string.native_settings_changeServer,
                descriptionRes = R.string.native_settings_changeServerDescription,
                icon = Icons.Outlined.SyncAlt,
                onClick = { callbacks.onAsk(SettingsConfirmation.CHANGE_SERVER) },
            )
        }
        row {
            DestructiveRow(
                titleRes = R.string.native_settings_clearLocalData,
                descriptionRes = R.string.native_settings_clearLocalDataDescription,
                icon = Icons.Outlined.DeleteSweep,
                onClick = { callbacks.onAsk(SettingsConfirmation.CLEAR_LOCAL_DATA) },
            )
        }
    }
}

/** A row that empties something, drawn in the colour that says so before it is tapped. */
@Composable
private fun DestructiveRow(
    titleRes: Int,
    descriptionRes: Int,
    icon: ImageVector,
    onClick: () -> Unit,
) {
    ListItem(
        headlineContent = { Text(stringResource(titleRes), color = MaterialTheme.colorScheme.error) },
        supportingContent = { Text(stringResource(descriptionRes)) },
        leadingContent = {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.error,
            )
        },
        colors = groupRowColors(),
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick),
    )
}

/** Puts the server's address on the clipboard; the system says it was copied on Android 13 and later. */
private fun copyServerAddress(
    context: Context,
    address: String,
) {
    val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager ?: return
    clipboard.setPrimaryClip(ClipData.newPlainText(null, address))
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
        Toast.makeText(context, R.string.native_settings_serverCopied, Toast.LENGTH_SHORT).show()
    }
}

/**
 * The question asked before anything that empties the device.
 *
 * The count of changes the server has not taken is part of the question rather
 * than a warning underneath it: "some changes will be lost" is not something a
 * person can weigh, and a number is.
 */
@Composable
internal fun DestructiveConfirmationDialog(
    confirmation: SettingsConfirmation,
    unsentChangeCount: Int,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    val changeServer = confirmation == SettingsConfirmation.CHANGE_SERVER
    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Text(
                stringResource(
                    if (changeServer) {
                        R.string.native_settings_confirmChangeServerTitle
                    } else {
                        R.string.native_settings_confirmClearLocalDataTitle
                    },
                ),
            )
        },
        text = {
            val body =
                stringResource(
                    if (changeServer) {
                        R.string.native_settings_confirmChangeServerBody
                    } else {
                        R.string.native_settings_confirmClearLocalDataBody
                    },
                )
            val unsent =
                if (unsentChangeCount > 0) {
                    " " +
                        pluralStringResource(
                            R.plurals.native_settings_confirmUnsent,
                            unsentChangeCount,
                            unsentChangeCount,
                        )
                } else {
                    ""
                }
            Text(body + unsent)
        },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text(
                    stringResource(
                        if (changeServer) {
                            R.string.native_settings_confirmChangeServerAction
                        } else {
                            R.string.native_settings_confirmClearLocalDataAction
                        },
                    ),
                )
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

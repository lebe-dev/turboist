package ru.tinyops.turboist.nativeapp.settings.ui

import androidx.compose.foundation.clickable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.DeleteSweep
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.settings.SettingsConfirmation
import ru.tinyops.turboist.nativeapp.settings.SettingsUiState
import ru.tinyops.turboist.nativeapp.settings.ThemeChoice
import ru.tinyops.turboist.nativeapp.tasks.ui.ChoiceRow

/** The three answers to "which colours", in the order they read best. */
private val THEME_CHOICES: List<Pair<ThemeChoice, Int>> =
    listOf(
        ThemeChoice.SYSTEM to R.string.settings_theme_system,
        ThemeChoice.LIGHT to R.string.settings_theme_light,
        ThemeChoice.DARK to R.string.settings_theme_dark,
    )

/**
 * The choices that stay on this phone.
 *
 * None of it is sent anywhere, and that is the point of drawing it apart from
 * everything above: which colours a screen uses and whether a radio may spend a
 * data allowance are facts about a device, and a tablet on wi-fi wants different
 * answers from a phone on a metered plan.
 *
 * The two destructive actions live here too, because both are about this device
 * rather than about the workspace: one throws the copy away and asks for it
 * again, the other walks away from the server altogether.
 */
@Composable
internal fun DeviceSection(
    state: SettingsUiState,
    callbacks: SettingsCallbacks,
) {
    SettingsSectionHeading(R.string.native_settings_deviceHeading, R.string.native_settings_deviceDescription)

    ChoiceRow(label = stringResource(R.string.settings_theme_ariaLabel)) {
        for ((choice, wording) in THEME_CHOICES) {
            FilterChip(
                selected = state.device.theme == choice,
                onClick = { callbacks.onSetTheme(choice) },
                label = { Text(stringResource(wording)) },
            )
        }
    }

    SettingsSwitchRow(
        titleRes = R.string.native_settings_syncOnMetered,
        checked = state.device.syncOnMetered,
        onCheckedChange = callbacks.onSetSyncOnMetered,
        descriptionRes = R.string.native_settings_syncOnMeteredDescription,
    )

    ListItem(
        headlineContent = { Text(stringResource(R.string.native_settings_serverHeading)) },
        supportingContent = {
            Text(state.serverAddress.ifEmpty { stringResource(R.string.native_settings_serverUnset) })
        },
        leadingContent = { Icon(imageVector = Icons.Outlined.Dns, contentDescription = null) },
    )
    ListItem(
        headlineContent = { Text(stringResource(R.string.native_settings_changeServer)) },
        supportingContent = { Text(stringResource(R.string.native_settings_changeServerDescription)) },
        modifier = Modifier.clickable { callbacks.onAsk(SettingsConfirmation.CHANGE_SERVER) },
    )
    ListItem(
        headlineContent = { Text(stringResource(R.string.native_settings_clearLocalData)) },
        supportingContent = { Text(stringResource(R.string.native_settings_clearLocalDataDescription)) },
        leadingContent = { Icon(imageVector = Icons.Outlined.DeleteSweep, contentDescription = null) },
        modifier = Modifier.clickable { callbacks.onAsk(SettingsConfirmation.CLEAR_LOCAL_DATA) },
    )
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

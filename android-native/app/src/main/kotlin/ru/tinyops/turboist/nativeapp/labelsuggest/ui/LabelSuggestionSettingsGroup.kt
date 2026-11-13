package ru.tinyops.turboist.nativeapp.labelsuggest.ui

import android.text.format.Formatter
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material.icons.outlined.CloudDownload
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.ErrorOutline
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.labelsuggest.LabelSuggestionSettings
import ru.tinyops.turboist.nativeapp.labelsuggest.LabelSuggestionSettingsViewModel
import ru.tinyops.turboist.nativeapp.labelsuggest.ModelFailure
import ru.tinyops.turboist.nativeapp.labelsuggest.ModelState
import ru.tinyops.turboist.nativeapp.settings.ui.SettingsGroup
import ru.tinyops.turboist.nativeapp.settings.ui.SettingsGroupLabel
import ru.tinyops.turboist.nativeapp.settings.ui.SettingsSwitchRow
import ru.tinyops.turboist.nativeapp.settings.ui.groupRowColors

/**
 * The label-suggestion group of the device settings: the switch, then how far
 * the model and the index have got while suggestions are on, then the way to
 * give the space back while the model takes any.
 */
@Composable
internal fun LabelSuggestionSettingsGroup(viewModel: LabelSuggestionSettingsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    LabelSuggestionSettingsGroup(
        state = state,
        onEnabledChange = viewModel::setEnabled,
        onRetry = viewModel::retry,
        onDelete = viewModel::deleteModel,
    )
}

@Composable
internal fun LabelSuggestionSettingsGroup(
    state: LabelSuggestionSettings,
    onEnabledChange: (Boolean) -> Unit,
    onRetry: () -> Unit,
    onDelete: () -> Unit,
) {
    if (!state.supported) return
    val holdsFile =
        when (val model = state.model) {
            ModelState.Ready, is ModelState.Downloading -> true
            is ModelState.Absent -> model.downloadedBytes > 0
            is ModelState.Failed -> model.downloadedBytes > 0
        }
    SettingsGroupLabel(stringResource(R.string.native_labelSuggest_heading))
    SettingsGroup {
        row { SwitchRow(state, onEnabledChange) }
        if (state.enabled) row { StatusRow(state, onRetry) }
        if (holdsFile) row { DeleteRow(state, onDelete) }
    }
}

@Composable
private fun SwitchRow(
    state: LabelSuggestionSettings,
    onEnabledChange: (Boolean) -> Unit,
) {
    val size = Formatter.formatShortFileSize(LocalContext.current, state.modelBytes)
    SettingsSwitchRow(
        title = stringResource(R.string.native_labelSuggest_toggle),
        checked = state.enabled,
        onCheckedChange = onEnabledChange,
        description = stringResource(R.string.native_labelSuggest_toggleDescription, size),
        icon = Icons.Outlined.AutoAwesome,
    )
}

@Composable
private fun StatusRow(
    state: LabelSuggestionSettings,
    onRetry: () -> Unit,
) {
    val context = LocalContext.current
    val size = { bytes: Long -> Formatter.formatShortFileSize(context, bytes) }
    when (val model = state.model) {
        is ModelState.Downloading ->
            StatusItem(
                text =
                    stringResource(
                        R.string.native_labelSuggest_downloading,
                        size(model.downloadedBytes),
                        size(model.totalBytes),
                    ),
                icon = { Icon(Icons.Outlined.CloudDownload, contentDescription = null) },
                progress = model.downloadedBytes.toFloat() / model.totalBytes.coerceAtLeast(1),
            )

        is ModelState.Absent ->
            StatusItem(
                text = stringResource(R.string.native_labelSuggest_waiting),
                icon = { Icon(Icons.Outlined.CloudDownload, contentDescription = null) },
                action = {
                    TextButton(
                        onClick = onRetry,
                    ) { Text(stringResource(R.string.native_labelSuggest_retry)) }
                },
            )

        is ModelState.Failed ->
            StatusItem(
                text =
                    when (model.reason) {
                        ModelFailure.NETWORK -> stringResource(R.string.native_labelSuggest_failedNetwork)
                        ModelFailure.STORAGE ->
                            stringResource(R.string.native_labelSuggest_failedStorage, size(state.modelBytes))
                        ModelFailure.CORRUPT -> stringResource(R.string.native_labelSuggest_failedCorrupt)
                    },
                icon = {
                    Icon(
                        Icons.Outlined.ErrorOutline,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.error,
                    )
                },
                action = {
                    TextButton(
                        onClick = onRetry,
                    ) { Text(stringResource(R.string.native_labelSuggest_retry)) }
                },
            )

        ModelState.Ready -> {
            val progress = state.progress
            val learning = progress.total > 0 && progress.learned < progress.total
            StatusItem(
                text =
                    if (progress.total == 0) {
                        stringResource(R.string.native_labelSuggest_ready)
                    } else {
                        stringResource(R.string.native_labelSuggest_learned, progress.learned, progress.total)
                    },
                icon = { Icon(Icons.Outlined.AutoAwesome, contentDescription = null) },
                progress = if (learning) progress.learned.toFloat() / progress.total else null,
            )
        }
    }
}

@Composable
private fun StatusItem(
    text: String,
    icon: @Composable () -> Unit,
    progress: Float? = null,
    action: (@Composable () -> Unit)? = null,
) {
    ListItem(
        headlineContent = {
            Column {
                Text(text, style = MaterialTheme.typography.bodyMedium)
                if (progress != null) {
                    Spacer(Modifier.height(8.dp))
                    LinearProgressIndicator(progress = { progress }, modifier = Modifier.fillMaxWidth())
                }
            }
        },
        leadingContent = icon,
        trailingContent = action,
        colors = groupRowColors(),
    )
}

@Composable
private fun DeleteRow(
    state: LabelSuggestionSettings,
    onDelete: () -> Unit,
) {
    val size = Formatter.formatShortFileSize(LocalContext.current, state.modelBytes)
    ListItem(
        headlineContent = { Text(stringResource(R.string.native_labelSuggest_delete)) },
        supportingContent = { Text(stringResource(R.string.native_labelSuggest_deleteDescription, size)) },
        leadingContent = { Icon(Icons.Outlined.Delete, contentDescription = null) },
        colors = groupRowColors(),
        modifier = Modifier.fillMaxWidth().clickable(onClick = onDelete),
    )
}

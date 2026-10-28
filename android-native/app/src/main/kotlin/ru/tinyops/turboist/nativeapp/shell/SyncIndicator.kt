package ru.tinyops.turboist.nativeapp.shell

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.CloudUpload
import androidx.compose.material.icons.outlined.SyncProblem
import androidx.compose.material3.AssistChip
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.sync.SyncNotice
import ru.tinyops.turboist.nativeapp.sync.SyncOutcome
import ru.tinyops.turboist.nativeapp.sync.SyncStatus
import ru.tinyops.turboist.nativeapp.sync.notice
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * How long a cycle has to run before the spinner appears. Most cycles are a
 * single small request and end well inside it, and a spinner that flashes on
 * and off several times a minute is noise rather than information.
 */
internal const val SPINNER_DELAY_MS = 400L

/**
 * What the shell says about how current the screen underneath it is, drawn into
 * the top bar's actions.
 *
 * Nothing here ever pushes the content of the screen around. A cycle that is
 * running earns a small spinner and nothing more — it happens constantly and it
 * needs no answer. A server that cannot be reached, a turn that did not complete,
 * or work of the user's that has not gone out earns a chip instead, because each
 * of those changes what the user should believe about what they are looking at.
 * The chip is short; the full sentence, and the retry where there is one, are a
 * tap away in the card it opens.
 *
 * Nothing here blocks anything. Every screen below keeps working: they are
 * queries over the device's own copy, which is exactly why the app can afford to
 * say "this is the copy" instead of refusing to draw.
 */
@Composable
fun SyncIndicator(
    status: SyncStatus,
    sessionUnverified: Boolean,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var spinnerShown by remember { mutableStateOf(false) }
    LaunchedEffect(status.syncing) {
        if (!status.syncing) {
            spinnerShown = false
            return@LaunchedEffect
        }
        delay(SPINNER_DELAY_MS)
        spinnerShown = true
    }

    Row(modifier = modifier, verticalAlignment = Alignment.CenterVertically) {
        if (spinnerShown) Spinner()
        when (val notice = status.notice(sessionUnverified)) {
            SyncNotice.None -> Unit
            is SyncNotice.Waiting ->
                NoticeChip(
                    label = notice.count.toString(),
                    description = pendingText(notice.count),
                    icon = Icons.Outlined.CloudUpload,
                    alarming = false,
                    message = pendingText(notice.count),
                    onRetry = null,
                )

            is SyncNotice.Unreachable ->
                NoticeChip(
                    label = stringResource(R.string.native_sync_offlineChip),
                    icon = Icons.Outlined.CloudOff,
                    alarming = true,
                    message = unreachableText(notice.lastSyncAt),
                    detail = notice.waiting.takeIf { it > 0 }?.let { pendingText(it) },
                    onRetry = onRetry,
                )

            is SyncNotice.Failed ->
                NoticeChip(
                    label = stringResource(R.string.native_sync_failedChip),
                    icon = Icons.Outlined.SyncProblem,
                    alarming = true,
                    message = stringResource(R.string.native_sync_failed),
                    detail = notice.waiting.takeIf { it > 0 }?.let { pendingText(it) },
                    onRetry = onRetry,
                )
        }
    }
}

/** The cycle that is running: a small ring where an icon button would sit. */
@Composable
private fun Spinner() {
    val description = stringResource(R.string.native_sync_inProgress)
    Box(
        modifier = Modifier.size(48.dp).semantics { contentDescription = description },
        contentAlignment = Alignment.Center,
    ) {
        CircularProgressIndicator(
            modifier = Modifier.size(20.dp),
            strokeWidth = 2.5.dp,
            color = MaterialTheme.colorScheme.primary,
        )
    }
}

/**
 * The chip, and the card it opens.
 *
 * Retry is offered only where trying again is a different question from waiting.
 * A queue that is simply on its way out has nothing to retry — it goes when the
 * server can be reached, and a button that repeats that is a button that does
 * nothing.
 *
 * [alarming] tints the chip with the error container: the screen is the device's
 * own copy, or the last turn did not go through. A queue on its way out is not
 * news of that weight and stays neutral.
 */
@Composable
private fun NoticeChip(
    label: String,
    icon: ImageVector,
    alarming: Boolean,
    message: String,
    onRetry: (() -> Unit)?,
    description: String? = null,
    detail: String? = null,
) {
    var open by remember { mutableStateOf(false) }
    val scheme = MaterialTheme.colorScheme
    val container = if (alarming) scheme.errorContainer else scheme.surfaceContainerHigh
    val content = if (alarming) scheme.onErrorContainer else scheme.onSurface
    Box(modifier = Modifier.padding(end = 4.dp)) {
        AssistChip(
            onClick = { open = true },
            label = { Text(label) },
            leadingIcon = { Icon(icon, contentDescription = null, modifier = Modifier.size(16.dp)) },
            shape = MaterialTheme.shapes.extraLarge,
            border = null,
            colors =
                AssistChipDefaults.assistChipColors(
                    containerColor = container,
                    labelColor = content,
                    leadingIconContentColor = content,
                ),
            modifier = if (description != null) Modifier.semantics { contentDescription = description } else Modifier,
        )
        DropdownMenu(
            expanded = open,
            onDismissRequest = { open = false },
            shape = MaterialTheme.shapes.large,
        ) {
            NoticeDetails(
                message = message,
                detail = detail,
                onRetry =
                    onRetry?.let { retry ->
                        {
                            open = false
                            retry()
                        }
                    },
            )
        }
    }
}

/** The full sentence behind a chip, with the one action there is. */
@Composable
private fun NoticeDetails(
    message: String,
    detail: String?,
    onRetry: (() -> Unit)?,
) {
    Column(
        modifier = Modifier.widthIn(max = 280.dp).padding(start = 16.dp, end = 8.dp, top = 8.dp, bottom = 4.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Text(
            text = message,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurface,
            modifier = Modifier.padding(end = 8.dp),
        )
        detail?.let {
            Text(
                text = it,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(end = 8.dp),
            )
        }
        if (onRetry != null) {
            TextButton(onClick = onRetry, modifier = Modifier.align(Alignment.End)) {
                Text(stringResource(R.string.offline_retry))
            }
        }
    }
}

@Composable
private fun pendingText(count: Int): String = stringResource(R.string.offline_pendingCount, count)

/**
 * "No connection", and when the copy on screen was last level with the server.
 *
 * The moment is worth saying because it is the difference between data that is a
 * minute old and data that is a week old, and the user is the only one who knows
 * which of those matters for what they are about to do. A device that has never
 * synced has nothing to date, so it says the shorter thing.
 */
@Composable
private fun unreachableText(lastSyncAt: Long?): String =
    if (lastSyncAt == null) {
        stringResource(R.string.offline_banner)
    } else {
        stringResource(R.string.offline_bannerStale, formatSyncTime(lastSyncAt))
    }

/** The stored instant as a wall-clock reading in the device's own zone. */
private fun formatSyncTime(millis: Long): String =
    DateTimeFormatter
        .ofLocalizedDateTime(FormatStyle.SHORT)
        .withZone(ZoneId.systemDefault())
        .format(Instant.ofEpochMilli(millis))

@Preview(showBackground = true)
@Composable
private fun SyncIndicatorPreview() {
    TurboistTheme {
        SyncIndicator(
            status = SyncStatus(syncing = true, outcome = SyncOutcome.UNREACHABLE, lastSyncAt = 0, waiting = 3),
            sessionUnverified = false,
            onRetry = {},
        )
    }
}

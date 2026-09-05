package ru.tinyops.turboist.nativeapp.shell

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
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

/** How tall the strip that says a cycle is running may be. Deliberately barely there. */
private val ProgressStripHeight = 2.dp

/**
 * What the shell says about how current the screen underneath it is.
 *
 * Two separate things, and they are separate because they interrupt to different
 * degrees. A cycle that is running earns a hairline at the top of the screen and
 * nothing more — it happens constantly, it needs no answer, and a phone screen is
 * small enough that a banner for it would be a banner that is always there. A
 * server that cannot be reached, a turn that did not complete, or work of the
 * user's that has not gone out earns an actual strip, because each of those
 * changes what the user should believe about what they are looking at.
 *
 * Nothing here blocks anything. Every screen below keeps working: they are
 * queries over the device's own copy, which is exactly why the app can afford to
 * say "this is the copy" instead of refusing to draw.
 */
@Composable
fun SyncBanner(
    status: SyncStatus,
    sessionUnverified: Boolean,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    // Read out here rather than inside the semantics block: a resource needs a
    // composition to be looked up in, and the block runs outside one.
    val syncingDescription = stringResource(R.string.native_sync_inProgress)
    Column(modifier = modifier.fillMaxWidth()) {
        if (status.syncing) {
            LinearProgressIndicator(
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .height(ProgressStripHeight)
                        .semantics { contentDescription = syncingDescription },
            )
        }
        when (val notice = status.notice(sessionUnverified)) {
            SyncNotice.None -> Unit
            is SyncNotice.Waiting -> NoticeStrip(message = pendingText(notice.count), onRetry = null)
            is SyncNotice.Unreachable ->
                NoticeStrip(
                    message = unreachableText(notice.lastSyncAt),
                    detail = notice.waiting.takeIf { it > 0 }?.let { pendingText(it) },
                    onRetry = onRetry,
                )

            is SyncNotice.Failed ->
                NoticeStrip(
                    message = stringResource(R.string.native_sync_failed),
                    detail = notice.waiting.takeIf { it > 0 }?.let { pendingText(it) },
                    onRetry = onRetry,
                )
        }
    }
}

/**
 * The strip itself: a sentence, an optional detail, and the one action there is.
 *
 * Retry is offered only where trying again is a different question from waiting.
 * A queue that is simply on its way out has nothing to retry — it goes when the
 * server can be reached, and a button that repeats that is a button that does
 * nothing.
 */
@Composable
private fun NoticeStrip(
    message: String,
    detail: String? = null,
    onRetry: (() -> Unit)? = null,
) {
    // Amber, and the same amber a queued write is badged with on the row it
    // belongs to: both say "this is on its way out", and the web client marks
    // them with one colour for exactly that reason. A neutral strip here would
    // read as a piece of furniture rather than as something to notice.
    Surface(color = TurboistTheme.accents.pendingContainer, modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 4.dp, top = 4.dp, bottom = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = message,
                    style = MaterialTheme.typography.labelLarge,
                    color = TurboistTheme.accents.pending,
                )
                detail?.let {
                    Text(
                        text = it,
                        style = MaterialTheme.typography.labelSmall,
                        color = TurboistTheme.accents.pending,
                    )
                }
            }
            onRetry?.let {
                TextButton(onClick = it) { Text(stringResource(R.string.offline_retry)) }
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
private fun SyncBannerPreview() {
    SyncBanner(
        status = SyncStatus(outcome = SyncOutcome.UNREACHABLE, lastSyncAt = 0, waiting = 3),
        sessionUnverified = false,
        onRetry = {},
    )
}

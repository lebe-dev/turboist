package ru.tinyops.turboist.nativeapp.unsent.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.core.sync.write.OutboxOpKind
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import ru.tinyops.turboist.nativeapp.unsent.UnsentChange
import ru.tinyops.turboist.nativeapp.unsent.UnsentChangesUiState
import ru.tinyops.turboist.nativeapp.unsent.UnsentChangesViewModel
import ru.tinyops.turboist.nativeapp.unsent.UnsentReason

/** What the unsent-changes screen can be asked to do. */
data class UnsentChangesCallbacks(
    val onDiscard: (UnsentChange) -> Unit,
    val onAskToDiscardAll: () -> Unit,
    val onCancelDiscardAll: () -> Unit,
    val onDiscardAll: () -> Unit,
)

/** The screen as the app builds it. */
@Composable
fun UnsentChangesScreen(
    modifier: Modifier = Modifier,
    viewModel: UnsentChangesViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    UnsentChangesScreen(
        state = state,
        callbacks =
            UnsentChangesCallbacks(
                onDiscard = presenter::discard,
                onAskToDiscardAll = presenter::askToDiscardAll,
                onCancelDiscardAll = presenter::cancelDiscardAll,
                onDiscardAll = presenter::discardAll,
            ),
        modifier = modifier,
    )
}

/**
 * Everything this device has not managed to tell the server.
 *
 * Two lists, because there are two situations and only one of them is the user's
 * problem. The refused pile is first: those changes are never going out, and the
 * only thing to decide about them is whether to keep reading the note. The
 * waiting queue is underneath, read-only, so that "it hasn't synced yet" is
 * something a person can look at rather than something they have to trust.
 */
@Composable
fun UnsentChangesScreen(
    state: UnsentChangesUiState,
    callbacks: UnsentChangesCallbacks,
    modifier: Modifier = Modifier,
) {
    if (state.confirmingDiscardAll) {
        DiscardAllDialog(
            count = state.setAside.size,
            onConfirm = callbacks.onDiscardAll,
            onDismiss = callbacks.onCancelDiscardAll,
        )
    }

    if (state.isEmpty) {
        NothingUnsent(modifier)
        return
    }

    LazyColumn(modifier = modifier.fillMaxSize()) {
        if (state.setAside.isNotEmpty()) {
            item {
                SetAsideHeading(onDiscardAll = callbacks.onAskToDiscardAll)
            }
            items(state.setAside, key = { it.id }) { change ->
                RefusedRow(change = change, onDiscard = { callbacks.onDiscard(change) })
                HorizontalDivider()
            }
        }
        if (state.waiting.isNotEmpty()) {
            item { SectionHeading(R.string.native_unsent_waitingHeading, R.string.native_unsent_waitingDescription) }
            items(state.waiting, key = { it.id }) { change ->
                WaitingRow(change)
                HorizontalDivider()
            }
        }
    }
}

@Composable
private fun NothingUnsent(modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = stringResource(R.string.offline_unsentEmpty),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
    }
}

@Composable
private fun SectionHeading(
    headingRes: Int,
    descriptionRes: Int,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp)) {
        Text(text = stringResource(headingRes), style = MaterialTheme.typography.titleSmall)
        Text(
            text = stringResource(descriptionRes),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * The refused pile's heading, with the one action that applies to all of it.
 *
 * "Discard all" sits here rather than in the top bar so it cannot be reached
 * when the pile is empty, and so it reads as belonging to this list rather than
 * to the screen.
 */
@Composable
private fun SetAsideHeading(onDiscardAll: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 8.dp, top = 12.dp, bottom = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = stringResource(R.string.native_unsent_setAsideHeading),
                style = MaterialTheme.typography.titleSmall,
            )
            Text(
                text = stringResource(R.string.native_unsent_setAsideDescription),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        TextButton(onClick = onDiscardAll) {
            Text(stringResource(R.string.native_unsent_discardAll))
        }
    }
}

@Composable
private fun RefusedRow(
    change: UnsentChange,
    onDiscard: () -> Unit,
) {
    ListItem(
        headlineContent = { Text(stringResource(labelFor(change.kind))) },
        supportingContent = {
            Column {
                change.target?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
                change.reason?.let {
                    Text(
                        text = stringResource(reasonFor(it)),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
                if (change.blockers.isNotEmpty()) {
                    Text(
                        text =
                            stringResource(
                                R.string.native_unsent_blockedBy,
                                change.blockers.joinToString(separator = ", "),
                            ),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        },
        trailingContent = {
            TextButton(onClick = onDiscard) {
                Text(stringResource(R.string.native_unsent_discard))
            }
        },
    )
}

/**
 * A change that is still going out.
 *
 * It carries no action on purpose: throwing away a write that will land would be
 * exactly the silent loss this screen exists to prevent, and there is nothing
 * else to do with it but wait.
 */
@Composable
private fun WaitingRow(change: UnsentChange) {
    ListItem(
        headlineContent = { Text(stringResource(labelFor(change.kind))) },
        supportingContent = {
            Column {
                change.target?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
                Text(
                    text = stringResource(R.string.offline_awaitingSend),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
    )
}

@Composable
private fun DiscardAllDialog(
    count: Int,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.native_unsent_discardAllTitle)) },
        text = { Text(stringResource(R.string.native_unsent_discardAllBody, count)) },
        confirmButton = {
            TextButton(onClick = onConfirm) { Text(stringResource(R.string.native_unsent_discardAllConfirm)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) }
        },
    )
}

@Preview(showBackground = true)
@Composable
private fun UnsentChangesScreenPreview() {
    TurboistTheme(dynamicColor = false) {
        UnsentChangesScreen(
            state =
                UnsentChangesUiState(
                    loading = false,
                    setAside =
                        listOf(
                            UnsentChange(
                                id = "a",
                                kind = OutboxOpKind.TASK_COMPLETE,
                                target = "Renew the domain",
                                at = 0,
                                reason = UnsentReason.BLOCKED,
                                blockers = listOf("Pay the invoice"),
                            ),
                        ),
                    waiting =
                        listOf(
                            UnsentChange(id = "b", kind = OutboxOpKind.TASK_CREATE, target = "Buy milk", at = 0),
                        ),
                ),
            callbacks =
                UnsentChangesCallbacks(
                    onDiscard = {},
                    onAskToDiscardAll = {},
                    onCancelDiscardAll = {},
                    onDiscardAll = {},
                ),
        )
    }
}

package ru.tinyops.turboist.nativeapp.account.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Card
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import ru.tinyops.turboist.core.model.ClientKind
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.dto.ActiveSessionDto
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.account.SessionsConfirmation
import ru.tinyops.turboist.nativeapp.account.SessionsMessage
import ru.tinyops.turboist.nativeapp.account.SessionsUiState
import ru.tinyops.turboist.nativeapp.account.SessionsViewModel
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Which devices and browsers can currently reach the account, and how to close
 * one.
 *
 * Read live and kept nowhere. A session list is the answer to "who is signed in
 * right now", and a copy on the phone would go on showing a session the user
 * revoked from somewhere else — false reassurance being the one failure this
 * screen cannot afford. So an unfetchable list says exactly that instead of
 * rendering as an account with nothing signed in.
 */
@Composable
fun SessionsScreen(
    modifier: Modifier = Modifier,
    viewModel: SessionsViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    SessionsContent(
        state = state,
        onAsk = viewModel::ask,
        onCancel = viewModel::cancelConfirmation,
        onConfirm = viewModel::confirm,
        onRetry = viewModel::load,
        modifier = modifier,
    )
}

@Composable
internal fun SessionsContent(
    state: SessionsUiState,
    onAsk: (SessionsConfirmation) -> Unit,
    onCancel: () -> Unit,
    onConfirm: () -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    LazyColumn(
        modifier = modifier.fillMaxSize().padding(horizontal = 16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        item {
            Text(
                text = stringResource(R.string.settings_sessions_description),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 16.dp),
            )
        }
        if (state.unreachable) {
            // The panel below already says why there is nothing here; repeating
            // the reason above it would be the same sentence twice.
            item { RequiresConnection(onRetry = onRetry) }
            return@LazyColumn
        }

        item {
            AccountProblemText(state.problem, R.string.settings_sessions_revokeFailed)
            if (state.message == SessionsMessage.OTHERS_SIGNED_OUT) {
                Text(
                    text = stringResource(R.string.settings_sessions_logoutOthersDone),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
        }

        if (!state.loading && state.sessions.isEmpty()) {
            item {
                Text(
                    text = stringResource(R.string.settings_sessions_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        items(state.sessions, key = { it.id }) { session ->
            SessionRow(
                session = session,
                enabled = !state.busy,
                onRevoke = { onAsk(SessionsConfirmation.Revoke(session)) },
            )
        }

        item { HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp)) }
        item {
            OutlinedButton(
                onClick = { onAsk(SessionsConfirmation.SignOutOthers) },
                enabled = !state.busy,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(
                    stringResource(
                        if (state.busy) {
                            R.string.settings_sessions_loggingOutOthers
                        } else {
                            R.string.settings_sessions_logoutOthers
                        },
                    ),
                )
            }
        }
        item {
            TextButton(
                onClick = { onAsk(SessionsConfirmation.SignOutEverywhere) },
                enabled = !state.busy,
                modifier = Modifier.fillMaxWidth().padding(bottom = 24.dp),
            ) {
                Text(stringResource(R.string.settings_session_logoutAll))
            }
        }
    }

    state.confirming?.let { pending ->
        SessionConfirmationDialog(
            confirmation = pending,
            unsentChangeCount = state.unsentChangeCount,
            onConfirm = onConfirm,
            onDismiss = onCancel,
        )
    }
}

/**
 * One way into the account.
 *
 * The raw user agent is offered rather than shown: it is how a user identifies a
 * browser they cannot otherwise place, and it is unreadable enough that putting
 * it on every row would bury the ones that matter.
 */
@Composable
private fun SessionRow(
    session: ActiveSessionDto,
    enabled: Boolean,
    onRevoke: () -> Unit,
) {
    var showUserAgent by rememberSaveable(session.id) { mutableStateOf(false) }

    Card(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    text = session.displayName.ifBlank { stringResource(clientKindLabel(session.clientKind)) },
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                if (session.isCurrent) {
                    AssistChip(
                        onClick = {},
                        enabled = false,
                        label = { Text(stringResource(R.string.settings_sessions_current)) },
                    )
                }
            }
            Text(
                text = stringResource(clientKindLabel(session.clientKind)),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "${stringResource(R.string.settings_sessions_signedIn)}: ${momentLabel(session.createdAt)}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "${stringResource(R.string.settings_sessions_lastActive)}: ${momentLabel(session.lastUsedAt)}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (session.ipAddress.isNotBlank()) {
                Text(
                    text = "${stringResource(R.string.settings_sessions_ipAddress)}: ${session.ipAddress}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (showUserAgent && session.userAgent.isNotBlank()) {
                Text(
                    text = session.userAgent,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Row(horizontalArrangement = Arrangement.End, modifier = Modifier.fillMaxWidth()) {
                if (session.userAgent.isNotBlank()) {
                    TextButton(onClick = { showUserAgent = !showUserAgent }) {
                        Text(
                            stringResource(
                                if (showUserAgent) {
                                    R.string.settings_sessions_hideUserAgent
                                } else {
                                    R.string.settings_sessions_showUserAgent
                                },
                            ),
                        )
                    }
                }
                // The session making this request has no revoke button: ending it
                // from here would sign the device out sideways, without the
                // question the sign-out below asks about the work still queued.
                if (!session.isCurrent) {
                    TextButton(onClick = onRevoke, enabled = enabled) {
                        Text(stringResource(R.string.settings_sessions_revoke))
                    }
                }
            }
        }
    }
}

/**
 * The question in front of every closure on this screen.
 *
 * The one that ends this device's own session says how much unsent work goes
 * with it, because that is the part the user cannot see for themselves.
 */
@Composable
private fun SessionConfirmationDialog(
    confirmation: SessionsConfirmation,
    unsentChangeCount: Int,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    val titleRes =
        when (confirmation) {
            is SessionsConfirmation.Revoke -> R.string.settings_sessions_confirmRevokeTitle
            SessionsConfirmation.SignOutOthers -> R.string.settings_sessions_confirmLogoutOthersTitle
            SessionsConfirmation.SignOutEverywhere -> R.string.settings_sessions_confirmLogoutAllTitle
        }
    val bodyRes =
        when (confirmation) {
            is SessionsConfirmation.Revoke -> R.string.settings_sessions_confirmRevokeDescription
            SessionsConfirmation.SignOutOthers -> R.string.settings_sessions_confirmLogoutOthersDescription
            SessionsConfirmation.SignOutEverywhere -> R.string.settings_sessions_confirmLogoutAllDescription
        }
    val confirmRes =
        when (confirmation) {
            is SessionsConfirmation.Revoke -> R.string.settings_sessions_revoke
            SessionsConfirmation.SignOutOthers -> R.string.settings_sessions_logoutOthers
            SessionsConfirmation.SignOutEverywhere -> R.string.settings_session_logoutAll
        }
    val unsent =
        if (confirmation == SessionsConfirmation.SignOutEverywhere && unsentChangeCount > 0) {
            " " + pluralStringResource(R.plurals.native_settings_confirmUnsent, unsentChangeCount, unsentChangeCount)
        } else {
            ""
        }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(titleRes)) },
        text = { Text(stringResource(bodyRes) + unsent) },
        confirmButton = { TextButton(onClick = onConfirm) { Text(stringResource(confirmRes)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.common_cancel)) } },
    )
}

/** The name this client kind goes by, with an unrecognised one shown as it arrived. */
private fun clientKindLabel(wire: String): Int =
    when (ClientKind.fromWire(wire)) {
        ClientKind.WEB -> R.string.settings_sessions_clientWeb
        ClientKind.IOS -> R.string.settings_sessions_clientIos
        ClientKind.ANDROID -> R.string.settings_sessions_clientAndroid
        ClientKind.CLI -> R.string.settings_sessions_clientCli
        ClientKind.UNKNOWN -> R.string.native_account_clientUnknown
    }

/**
 * A timestamp as a date and time in the device's own zone. The wire form is UTC
 * to the millisecond, which is not how a person recognises when they last used a
 * browser.
 */
@Composable
private fun momentLabel(wireTime: String): String {
    val epochMillis = WireTime.parseOrNull(wireTime) ?: return stringResource(R.string.native_task_unset)
    val formatter = remember { DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT) }
    return Instant.ofEpochMilli(epochMillis).atZone(ZoneId.systemDefault()).format(formatter)
}

@Preview(showBackground = true)
@Composable
private fun SessionsScreenPreview() {
    TurboistTheme {
        SessionsContent(
            state =
                SessionsUiState(
                    loading = false,
                    sessions =
                        listOf(
                            ActiveSessionDto(
                                id = 1,
                                clientKind = "android",
                                displayName = "Pixel 8",
                                ipAddress = "203.0.113.10",
                                createdAt = "2026-08-18T10:00:00.000Z",
                                lastUsedAt = "2026-08-20T09:12:00.000Z",
                                isCurrent = true,
                            ),
                            ActiveSessionDto(
                                id = 2,
                                clientKind = "web",
                                displayName = "Chrome on macOS",
                                userAgent = "Mozilla/5.0",
                                createdAt = "2026-08-01T10:00:00.000Z",
                                lastUsedAt = "2026-08-19T09:12:00.000Z",
                            ),
                        ),
                ),
            onAsk = {},
            onCancel = {},
            onConfirm = {},
            onRetry = {},
        )
    }
}

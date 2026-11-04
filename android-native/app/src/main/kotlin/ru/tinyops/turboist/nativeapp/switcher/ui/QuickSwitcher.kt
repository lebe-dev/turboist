package ru.tinyops.turboist.nativeapp.switcher.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.outlined.EventAvailable
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.switcher.QuickSwitcherMessage
import ru.tinyops.turboist.nativeapp.switcher.QuickSwitcherUiState
import ru.tinyops.turboist.nativeapp.switcher.QuickSwitcherViewModel
import ru.tinyops.turboist.nativeapp.switcher.RecentRef
import ru.tinyops.turboist.nativeapp.switcher.SwitcherKind
import ru.tinyops.turboist.nativeapp.switcher.SwitcherRow
import ru.tinyops.turboist.nativeapp.switcher.SwitcherTarget

/** Tags the checks find the panel's parts by. */
const val QUICK_SWITCHER_FIELD_TAG: String = "quick-switcher-field"
const val QUICK_SWITCHER_ADD_INBOX_TAG: String = "quick-switcher-add-inbox"
const val QUICK_SWITCHER_ADD_TODAY_TAG: String = "quick-switcher-add-today"

/** What the panel can do besides changing what is typed. */
data class QuickSwitcherCallbacks(
    val onType: (String) -> Unit,
    val onCreate: (SwitcherTarget) -> Unit,
    val onOpenTask: (Long) -> Unit,
    val onOpenProject: (Long) -> Unit,
    val onDismiss: () -> Unit,
)

/**
 * The quick switcher, wired to the app's graph.
 *
 * Composed over every screen, like the capture button: [visiting] is what is on
 * screen when that is a task or a project, and each one is remembered as
 * recently opened. [open] says whether the panel is up; [onDismiss] is called
 * when it should come down — put away, or done with by opening or adding
 * something.
 */
@Composable
fun QuickSwitcherHost(
    open: Boolean,
    onDismiss: () -> Unit,
    visiting: RecentRef?,
    onOpenTask: (Long) -> Unit,
    onOpenProject: (Long) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: QuickSwitcherViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()

    LaunchedEffect(visiting) { visiting?.let(presenter::visit) }
    LaunchedEffect(open) { if (open) presenter.open() else presenter.dismiss() }

    QuickSwitcherHost(
        open = open,
        state = state,
        messages = presenter.messages,
        callbacks =
            QuickSwitcherCallbacks(
                onType = presenter::type,
                onCreate = presenter::create,
                onOpenTask = { localId ->
                    onDismiss()
                    onOpenTask(localId)
                },
                onOpenProject = { localId ->
                    onDismiss()
                    onOpenProject(localId)
                },
                onDismiss = onDismiss,
            ),
        modifier = modifier,
    )
}

/** The same surface without the dependency graph behind it — the form the checks drive. */
@Composable
fun QuickSwitcherHost(
    open: Boolean,
    state: QuickSwitcherUiState,
    messages: Flow<QuickSwitcherMessage>,
    callbacks: QuickSwitcherCallbacks,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    val addedToInbox = stringResource(R.string.task_toast_addedToInbox)
    val addedTodayInbox = stringResource(R.string.quickSwitcher_toast_addedTodayInbox)
    val addedTodayTemplate = stringResource(R.string.quickSwitcher_toast_addedToday)
    val failed = stringResource(R.string.task_toast_failedAdd)
    val dismiss by rememberUpdatedState(callbacks.onDismiss)

    LaunchedEffect(messages) {
        messages.collect { message ->
            if (message !is QuickSwitcherMessage.Failed) dismiss()
            snackbars.showSnackbar(
                when (message) {
                    QuickSwitcherMessage.AddedToInbox -> addedToInbox
                    is QuickSwitcherMessage.AddedForToday ->
                        message.projectTitle?.let { String.format(addedTodayTemplate, it) } ?: addedTodayInbox
                    QuickSwitcherMessage.Failed -> failed
                },
            )
        }
    }

    Box(modifier = modifier.fillMaxSize()) {
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }

    if (open) {
        Dialog(
            onDismissRequest = callbacks.onDismiss,
            properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false),
        ) {
            QuickSwitcherPanel(state = state, callbacks = callbacks)
        }
    }
}

@Composable
private fun QuickSwitcherPanel(
    state: QuickSwitcherUiState,
    callbacks: QuickSwitcherCallbacks,
) {
    val focus = remember { FocusRequester() }
    LaunchedEffect(Unit) { focus.requestFocus() }

    Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.surface) {
        Column(modifier = Modifier.fillMaxSize().statusBarsPadding().imePadding()) {
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(4.dp)) {
                IconButton(onClick = callbacks.onDismiss) {
                    Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.common_back))
                }
                TextField(
                    value = state.typed,
                    onValueChange = callbacks.onType,
                    placeholder = { Text(stringResource(R.string.quickSwitcher_placeholder), maxLines = 1) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Go),
                    // The keyboard's action key takes the first row: on a phone that is
                    // the quickest way to open the best match.
                    keyboardActions =
                        KeyboardActions(
                            onGo = {
                                val first = state.rows.firstOrNull()
                                when {
                                    first != null -> open(first, callbacks)
                                    state.title.isNotEmpty() -> callbacks.onCreate(SwitcherTarget.INBOX)
                                }
                            },
                        ),
                    trailingIcon = {
                        if (state.typed.isNotEmpty()) {
                            IconButton(onClick = { callbacks.onType("") }) {
                                Icon(
                                    Icons.Filled.Close,
                                    contentDescription = stringResource(R.string.quickSwitcher_clear),
                                )
                            }
                        }
                    },
                    colors =
                        TextFieldDefaults.colors(
                            focusedContainerColor = MaterialTheme.colorScheme.surface,
                            unfocusedContainerColor = MaterialTheme.colorScheme.surface,
                            focusedIndicatorColor = MaterialTheme.colorScheme.surface,
                            unfocusedIndicatorColor = MaterialTheme.colorScheme.surface,
                        ),
                    modifier = Modifier.weight(1f).focusRequester(focus).testTag(QUICK_SWITCHER_FIELD_TAG),
                )
            }
            HorizontalDivider()
            LazyColumn(modifier = Modifier.fillMaxWidth().weight(1f)) {
                if (state.typed.isBlank() && state.rows.isNotEmpty()) {
                    item(key = "recent-heading") {
                        Text(
                            text = stringResource(R.string.quickSwitcher_recent),
                            style = MaterialTheme.typography.labelMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(start = 16.dp, top = 12.dp, bottom = 4.dp),
                        )
                    }
                }
                items(state.rows, key = { "${it.ref.kind}:${it.ref.localId}" }) { row ->
                    ResultRow(row = row, onClick = { open(row, callbacks) })
                }
                if (state.title.isNotEmpty()) {
                    item(key = "create") {
                        if (state.rows.isNotEmpty()) HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))
                        CreateRow(
                            icon = Icons.Outlined.Inbox,
                            text = stringResource(R.string.quickSwitcher_create_inbox, state.title),
                            enabled = !state.creating,
                            onClick = { callbacks.onCreate(SwitcherTarget.INBOX) },
                            modifier = Modifier.testTag(QUICK_SWITCHER_ADD_INBOX_TAG),
                        )
                        val project = state.todayProject
                        CreateRow(
                            icon = Icons.Outlined.EventAvailable,
                            text =
                                if (project != null) {
                                    stringResource(R.string.quickSwitcher_create_today, project.title, state.title)
                                } else {
                                    stringResource(R.string.quickSwitcher_create_todayInbox, state.title)
                                },
                            enabled = !state.creating,
                            onClick = { callbacks.onCreate(SwitcherTarget.TODAY) },
                            modifier = Modifier.testTag(QUICK_SWITCHER_ADD_TODAY_TAG),
                        )
                    }
                }
                if (state.rows.isEmpty() && state.title.isEmpty()) {
                    item(key = "empty") {
                        Text(
                            text = stringResource(R.string.quickSwitcher_empty),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(24.dp),
                        )
                    }
                }
            }
        }
    }
}

private fun open(
    row: SwitcherRow,
    callbacks: QuickSwitcherCallbacks,
) {
    when (row.ref.kind) {
        SwitcherKind.TASK -> callbacks.onOpenTask(row.ref.localId)
        SwitcherKind.PROJECT -> callbacks.onOpenProject(row.ref.localId)
    }
}

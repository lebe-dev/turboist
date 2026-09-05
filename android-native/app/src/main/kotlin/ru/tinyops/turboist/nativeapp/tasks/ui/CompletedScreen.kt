package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.tasks.CompletedDaySection
import ru.tinyops.turboist.nativeapp.tasks.CompletedRow
import ru.tinyops.turboist.nativeapp.tasks.CompletedUiState
import ru.tinyops.turboist.nativeapp.tasks.CompletedViewModel
import ru.tinyops.turboist.nativeapp.tasks.OlderHistory
import ru.tinyops.turboist.nativeapp.tasks.TaskListMessage
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import java.time.LocalDate
import java.time.ZoneId

/** What a line of the history can be asked to do. */
data class CompletedCallbacks(
    val onUncomplete: (CompletedRow) -> Unit,
    val onOpen: (Task) -> Unit,
    val onShowMore: () -> Unit,
    val onRefresh: () -> Unit,
)

/**
 * The completion history, day by day, newest first.
 *
 * It reads from two places and shows one list. The device's own copy covers a
 * recent stretch and is there with no signal at all; anything before that is on
 * the server and is fetched as the user reaches it. The seam is drawn at the
 * bottom of the list and always says something true about itself — going on,
 * finished, or needing a connection — because the one thing a boundary must not
 * do is look like loading when nothing is loading.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CompletedScreen(
    state: CompletedUiState,
    zone: ZoneId,
    today: LocalDate,
    messages: Flow<TaskListMessage>,
    callbacks: CompletedCallbacks,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    val failed = stringResource(R.string.task_toast_failedUpdate)
    LaunchedEffect(messages, failed) {
        messages.collect { snackbars.showSnackbar(failed) }
    }

    Box(modifier = modifier.fillMaxSize()) {
        PullToRefreshBox(
            isRefreshing = state.refreshing,
            onRefresh = callbacks.onRefresh,
            modifier = Modifier.fillMaxSize(),
        ) {
            if (state.isEmpty) {
                EmptyList(
                    EmptyListText(
                        title = stringResource(R.string.page_completed_emptyTitle),
                        description = stringResource(R.string.page_completed_emptyDescription),
                    ),
                )
                return@PullToRefreshBox
            }
            LazyColumn(modifier = Modifier.fillMaxSize()) {
                for (section in state.sections) {
                    item(key = "heading-" + section.key) { CompletedDayHeader(section) }
                    items(section.rows, key = { it.key }) { row ->
                        TaskRow(
                            row = TaskListRow(row.task, depth = 0, projectTitle = row.projectTitle),
                            zone = zone,
                            today = today,
                            selectionMode = false,
                            selected = false,
                            onToggleComplete = { callbacks.onUncomplete(row) },
                            onOpen = { callbacks.onOpen(row.task) },
                            onSelectToggle = { callbacks.onOpen(row.task) },
                            onStartSelection = { },
                        )
                    }
                }
                item(key = "older") {
                    OlderHistoryFooter(
                        state = state.older,
                        drawn = state.sections.sumOf { it.rows.size },
                        onShowMore = callbacks.onShowMore,
                    )
                }
            }
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }
}

/**
 * The same screen, driven by a view model.
 *
 * The stateless form above is the one the checks exercise; this holds nothing of
 * its own, so the two cannot drift apart.
 */
@Composable
fun CompletedScreen(
    onOpenTask: (Task) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: CompletedViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    val today by viewModel.today.collectAsStateWithLifecycle()

    CompletedScreen(
        state = state,
        zone = viewModel.zone,
        today = today,
        messages = presenter.messages,
        callbacks =
            remember(presenter, onOpenTask) {
                CompletedCallbacks(
                    onUncomplete = presenter::uncomplete,
                    onOpen = onOpenTask,
                    onShowMore = presenter::showMore,
                    onRefresh = presenter::refresh,
                )
            },
        modifier = modifier,
    )
}

/** The day a block of the history belongs to, and how much was finished on it. */
@Composable
private fun CompletedDayHeader(section: CompletedDaySection) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 12.dp, bottom = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(
            text = dayHeadingLabel(section.day, section.relative),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            text = section.rows.size.toString(),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
}

/**
 * The bottom of the history: what lies below the last line, and how to get it.
 *
 * More of the device's own copy, and the first pages from the server, are taken
 * as the user reaches them — scrolling backwards through time should not require
 * a decision at every screenful. Each request is made once per set of lines
 * drawn, so a request that came back with nothing new leaves the offer standing
 * instead of asking again forever.
 *
 * The two states that stop are stated in words. Neither shows a spinner: nothing
 * is on its way, and pretending otherwise would leave the user waiting for
 * something that is never going to arrive.
 */
@Composable
private fun OlderHistoryFooter(
    state: OlderHistory,
    drawn: Int,
    onShowMore: () -> Unit,
) {
    var asked by remember { mutableIntStateOf(-1) }
    LaunchedEffect(state, drawn) {
        if (state != OlderHistory.IN_REPLICA && state != OlderHistory.ON_SERVER) return@LaunchedEffect
        if (asked == drawn) return@LaunchedEffect
        asked = drawn
        onShowMore()
    }

    when (state) {
        OlderHistory.IN_REPLICA, OlderHistory.LOADING -> FooterProgress()
        OlderHistory.ON_SERVER -> FooterAction(R.string.native_completed_showOlder, onShowMore)
        OlderHistory.OFFLINE -> FooterNote(R.string.native_completed_olderNeedsConnection)
        OlderHistory.FAILED -> {
            FooterNote(R.string.native_completed_olderUnavailable)
            FooterAction(R.string.app_retry, onShowMore)
        }

        OlderHistory.EXHAUSTED -> FooterNote(R.string.native_completed_allShown)
    }
}

@Composable
private fun FooterProgress() {
    val label = stringResource(R.string.native_completed_loadingOlder)
    Box(
        modifier = Modifier.fillMaxWidth().padding(16.dp),
        contentAlignment = Alignment.Center,
    ) {
        CircularProgressIndicator(
            modifier = Modifier.size(20.dp).semantics { contentDescription = label },
            strokeWidth = 2.dp,
        )
    }
}

@Composable
private fun FooterAction(
    @StringRes textRes: Int,
    onClick: () -> Unit,
) {
    Box(modifier = Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
        TextButton(onClick = onClick) { Text(stringResource(textRes)) }
    }
}

@Composable
private fun FooterNote(
    @StringRes textRes: Int,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp)) {
        Text(
            text = stringResource(textRes),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

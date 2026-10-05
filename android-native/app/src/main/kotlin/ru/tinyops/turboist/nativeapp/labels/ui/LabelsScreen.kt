package ru.tinyops.turboist.nativeapp.labels.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Label
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.view.LabelUsagePeriod
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.labels.LabelMessage
import ru.tinyops.turboist.nativeapp.labels.LabelStatsRow
import ru.tinyops.turboist.nativeapp.labels.LabelStatsUiState
import ru.tinyops.turboist.nativeapp.labels.LabelStatsViewModel
import ru.tinyops.turboist.nativeapp.projects.ui.EmptyMessage
import ru.tinyops.turboist.nativeapp.projects.ui.projectTint

/** What the label report can be asked to do. */
data class LabelsCallbacks(
    val onChoosePeriod: (LabelUsagePeriod) -> Unit,
    val onRefresh: () -> Unit,
    val onCreateLabel: (name: String, color: String) -> Unit,
    val onOpenLabel: (Label) -> Unit,
)

/**
 * How often each label is being reached for.
 *
 * Every number here is counted on the device from its own copy of the workspace,
 * so the screen is as complete in a tunnel as it is online — which is the point
 * of it: this is a review screen, and a review happens where it happens.
 *
 * The report is read as two lists rather than one. The labels in use are ranked
 * by how often they were applied, because that is the question "which of these
 * am I actually using" is asking. The ones with nothing to show are gathered
 * underneath as what they are: candidates for cleanup, each with how long ago it
 * was last reached for.
 *
 * Switching the window costs nothing — all three are counted in the same pass —
 * so the control is a row of chips rather than something that has to be waited
 * for.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LabelsScreen(
    state: LabelStatsUiState,
    messages: Flow<LabelMessage>,
    callbacks: LabelsCallbacks,
    modifier: Modifier = Modifier,
) {
    val snackbars = remember { SnackbarHostState() }
    LabelMessageHost(messages, snackbars)

    var creating by remember { mutableStateOf(false) }

    Box(modifier = modifier.fillMaxSize()) {
        Column(modifier = Modifier.fillMaxSize()) {
            PeriodRow(state.period, callbacks.onChoosePeriod)
            PullToRefreshBox(
                isRefreshing = state.refreshing,
                onRefresh = callbacks.onRefresh,
                modifier = Modifier.fillMaxSize(),
            ) {
                if (state.isEmpty) {
                    EmptyMessage(
                        title = stringResource(R.string.page_labels_emptyTitle),
                        description = stringResource(R.string.page_labels_emptyDescription),
                    )
                    return@PullToRefreshBox
                }
                LabelReport(state, callbacks.onOpenLabel)
            }
        }
        FloatingActionButton(
            onClick = { creating = true },
            modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp),
        ) {
            Icon(Icons.Filled.Add, contentDescription = stringResource(R.string.dialog_label_newTitle))
        }
        SnackbarHost(hostState = snackbars, modifier = Modifier.align(Alignment.BottomCenter))
    }

    if (creating) {
        LabelEditorSheet(
            initial = null,
            onConfirm = { draft -> callbacks.onCreateLabel(draft.name, draft.color) },
            onDismiss = { creating = false },
        )
    }
}

@Composable
private fun LabelReport(
    state: LabelStatsUiState,
    onOpenLabel: (Label) -> Unit,
) {
    LazyColumn(modifier = Modifier.fillMaxSize()) {
        item(key = "totals") { Totals(state) }
        item(key = "ranking-heading") {
            SectionHeading(
                title = stringResource(R.string.page_labels_rankingTitle),
                hint = stringResource(R.string.page_labels_appliedHint),
            )
        }
        if (state.active.isEmpty()) {
            item(key = "no-activity") {
                Text(
                    text = stringResource(R.string.page_labels_noActivity),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
                )
            }
        }
        items(state.active, key = { "active-" + it.usage.label.localId }) { row ->
            RankedLabelRow(row, state.period, state.mostApplied, onOpenLabel)
        }
        if (state.idle.isNotEmpty()) {
            item(key = "idle-heading") {
                SectionHeading(
                    // The shared wording carries no type for its values, so the
                    // count is handed over as text.
                    title = stringResource(R.string.page_labels_idleTitle, state.idle.size.toString()),
                    hint = null,
                )
                Text(
                    text = stringResource(R.string.page_labels_idleDescription),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp),
                )
            }
            items(state.idle, key = { "idle-" + it.usage.label.localId }) { row ->
                IdleLabelRow(row, onOpenLabel)
            }
        }
    }
}

/**
 * The four headline counters.
 *
 * Three of them belong to the chosen window; the overdue one does not, and it is
 * the last on purpose — it is the backlog carried under these labels right now,
 * and it stays put when the window changes.
 */
@Composable
private fun Totals(state: LabelStatsUiState) {
    Column(
        verticalArrangement = Arrangement.spacedBy(4.dp),
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
    ) {
        Counter(stringResource(R.string.page_labels_stats_applied), state.totals.applied.toString())
        Counter(
            stringResource(R.string.page_labels_stats_active),
            // The shared wording counts a ratio the same way the plan does.
            stringResource(R.string.native_count_ratio, state.active.size, state.labelCount),
        )
        Counter(stringResource(R.string.page_labels_stats_completed), state.totals.completed.toString())
        Counter(stringResource(R.string.page_labels_stats_overdue), state.totals.overdue.toString())
    }
}

@Composable
private fun Counter(
    name: String,
    value: String,
) {
    Row(
        horizontalArrangement = Arrangement.SpaceBetween,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Text(text = name, style = MaterialTheme.typography.bodyMedium)
        Text(
            text = value,
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.onSurface,
        )
    }
}

@Composable
private fun SectionHeading(
    title: String,
    hint: String?,
) {
    HorizontalDivider()
    Row(
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        Text(
            text = title,
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (hint != null) {
            Text(
                text = hint,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/**
 * One label in use: what it is called, how often it was reached for, how that
 * compares with the window before, and what state its work is in.
 *
 * The bar is drawn against the busiest label rather than against a fixed scale,
 * so the shape of the list is readable whether the user applied four labels this
 * week or four hundred.
 */
@Composable
private fun RankedLabelRow(
    row: LabelStatsRow,
    period: LabelUsagePeriod,
    mostApplied: Int,
    onOpenLabel: (Label) -> Unit,
) {
    val counters = row.usage.period(period)
    Column(
        verticalArrangement = Arrangement.spacedBy(4.dp),
        modifier =
            Modifier
                .fillMaxWidth()
                .clickable { onOpenLabel(row.usage.label) }
                .padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        Row(
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.fillMaxWidth(),
        ) {
            LabelName(row.usage.label, modifier = Modifier.weight(1f, fill = false))
            Text(text = counters.applied.toString(), style = MaterialTheme.typography.titleSmall)
        }
        AppliedBar(applied = counters.applied, mostApplied = mostApplied)
        Text(
            text = rowFacts(row, period),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (counters.delta != 0) {
            Text(
                // The trend is stated as what the window before held, which is a
                // fact the reader can check, rather than as an arrow they have to
                // interpret.
                text = stringResource(R.string.page_labels_trendTooltip, counters.previousApplied.toString()),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** The lines under a ranked row: what the work carrying this label is doing. */
@Composable
private fun rowFacts(
    row: LabelStatsRow,
    period: LabelUsagePeriod,
): String {
    val facts =
        buildList {
            add(stringResource(R.string.page_labels_row_completed, row.usage.period(period).completed.toString()))
            add(stringResource(R.string.page_labels_row_open, row.usage.openTasks.toString()))
            if (row.usage.overdue > 0) {
                add(stringResource(R.string.page_labels_row_overdue, row.usage.overdue.toString()))
            }
            add(stringResource(R.string.page_labels_row_total, row.usage.totalTasks.toString()))
            if (row.usage.projects > 0) {
                add(stringResource(R.string.page_labels_row_projects, row.usage.projects.toString()))
            }
        }
    return facts.joinToString(FACT_SEPARATOR)
}

private const val FACT_SEPARATOR = " · "

@Composable
private fun AppliedBar(
    applied: Int,
    mostApplied: Int,
) {
    val fraction = if (mostApplied <= 0) 0f else applied.toFloat() / mostApplied.toFloat()
    Box(
        modifier =
            Modifier
                .fillMaxWidth()
                .height(BAR_HEIGHT)
                .clip(MaterialTheme.shapes.extraSmall)
                .background(MaterialTheme.colorScheme.surfaceVariant),
    ) {
        Box(
            modifier =
                Modifier
                    .fillMaxWidth(fraction.coerceIn(MIN_BAR_FRACTION, 1f))
                    .height(BAR_HEIGHT)
                    .clip(MaterialTheme.shapes.extraSmall)
                    .background(MaterialTheme.colorScheme.primary),
        )
    }
}

private val BAR_HEIGHT = 4.dp

/** A bar for a label that was used at all is never invisible, however small its share. */
private const val MIN_BAR_FRACTION = 0.02f

/** One unused label: its name, and how long ago it was last reached for. */
@Composable
private fun IdleLabelRow(
    row: LabelStatsRow,
    onOpenLabel: (Label) -> Unit,
) {
    Row(
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
        modifier =
            Modifier
                .fillMaxWidth()
                .clickable { onOpenLabel(row.usage.label) }
                .padding(horizontal = 16.dp, vertical = 10.dp),
    ) {
        LabelName(row.usage.label, modifier = Modifier.weight(1f, fill = false))
        Text(
            text = lastUsedText(row.lastUsedDaysAgo),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** A label, drawn in its own colour when it has one the app can read. */
@Composable
fun LabelName(
    label: Label,
    modifier: Modifier = Modifier,
) {
    Row(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
        modifier = modifier,
    ) {
        Icon(
            imageVector = Icons.AutoMirrored.Outlined.Label,
            contentDescription = null,
            tint = projectTint(label.color) ?: MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(18.dp),
        )
        Text(
            text = label.name,
            style = MaterialTheme.typography.bodyLarge,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PeriodRow(
    chosen: LabelUsagePeriod,
    onChoose: (LabelUsagePeriod) -> Unit,
) {
    val group = stringResource(R.string.page_labels_periodAria)
    Column(modifier = Modifier.fillMaxWidth().semantics { contentDescription = group }) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
        ) {
            for (period in LabelUsagePeriod.entries) {
                FilterChip(
                    selected = period == chosen,
                    onClick = { onChoose(period) },
                    label = { Text(stringResource(periodLabel(period))) },
                )
            }
        }
        Text(
            text = stringResource(periodRangeLabel(chosen)),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 2.dp),
        )
    }
}

/** Shows whatever a label screen says back, once each. */
@Composable
fun LabelMessageHost(
    messages: Flow<LabelMessage>,
    snackbars: SnackbarHostState,
) {
    var pending by remember { mutableStateOf<LabelMessage?>(null) }
    LaunchedEffect(messages) { messages.collect { pending = it } }
    val message = pending ?: return
    val text = labelMessageText(message)
    LaunchedEffect(message, text) {
        snackbars.showSnackbar(text)
        pending = null
    }
}

/**
 * The same screen, driven by a view model.
 *
 * The stateless form above is the one that is exercised in tests; this is the
 * wiring the app uses, and it holds nothing of its own so the two cannot drift.
 */
@Composable
fun LabelsScreen(
    onOpenLabel: (Label) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: LabelStatsViewModel = hiltViewModel(),
) {
    val presenter = viewModel.presenter
    val state by presenter.state.collectAsStateWithLifecycle()
    LabelsScreen(
        state = state,
        messages = presenter.messages,
        callbacks =
            remember(presenter, onOpenLabel) {
                LabelsCallbacks(
                    onChoosePeriod = presenter::choosePeriod,
                    onRefresh = presenter::refresh,
                    onCreateLabel = presenter::createLabel,
                    onOpenLabel = onOpenLabel,
                )
            },
        modifier = modifier,
    )
}

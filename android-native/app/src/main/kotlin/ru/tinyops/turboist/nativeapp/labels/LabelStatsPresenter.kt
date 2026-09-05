package ru.tinyops.turboist.nativeapp.labels

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.view.LabelUsage
import ru.tinyops.turboist.core.model.view.LabelUsagePeriod
import ru.tinyops.turboist.core.model.view.LabelUsageTotals
import ru.tinyops.turboist.core.model.view.labelUsageTotals
import ru.tinyops.turboist.core.model.view.lastUsedDaysAgo
import ru.tinyops.turboist.core.model.view.splitLabelUsage
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import java.time.Instant
import java.time.ZoneId

/**
 * One label as the report draws it.
 *
 * The age of the last use is worked out here rather than on screen: it is a
 * question about calendar days in the user's zone, and a screen that divided
 * elapsed milliseconds would call a tagging made at one minute to midnight
 * "today" the following morning.
 */
data class LabelStatsRow(
    val usage: LabelUsage,
    val lastUsedDaysAgo: Long?,
)

/** Everything the label report renders. */
data class LabelStatsUiState(
    val loading: Boolean = true,
    val period: LabelUsagePeriod = LabelUsagePeriod.WEEK,
    /** Labels reached for, or whose work was finished, inside the window — ranked. */
    val active: List<LabelStatsRow> = emptyList(),
    /** Labels with no activity in the window at all, most recently used first. */
    val idle: List<LabelStatsRow> = emptyList(),
    val totals: LabelUsageTotals = LabelUsageTotals(),
    val labelCount: Int = 0,
    /** The busiest label's count, which the bars beside the rows are drawn against. */
    val mostApplied: Int = 0,
    val refreshing: Boolean = false,
) {
    /** True once the workspace is known to hold no labels, as opposed to not being known yet. */
    val isEmpty: Boolean get() = !loading && labelCount == 0
}

/**
 * The behaviour of the label report.
 *
 * A plain object driven by a scope rather than a view model, so all of it can be
 * exercised without Compose and without the platform.
 *
 * Switching the period re-reads numbers already in hand rather than asking the
 * replica again: all three windows are counted in the same pass, which is what
 * makes the control instant and what keeps the report honest — the three columns
 * are always cut against the same moment.
 */
class LabelStatsPresenter(
    private val scope: CoroutineScope,
    usage: Flow<List<LabelUsage>>,
    today: Flow<Instant>,
    private val zone: ZoneId,
    private val actions: LabelActions,
    private val sync: SyncScheduler,
) {
    private val period = MutableStateFlow(LabelUsagePeriod.WEEK)
    private val refreshing = MutableStateFlow(false)
    private val outgoing = MutableSharedFlow<LabelMessage>(extraBufferCapacity = 1)

    /** What the screen renders. */
    val state: StateFlow<LabelStatsUiState> =
        combine(usage, period, today, refreshing) { rows, chosen, at, isRefreshing ->
            val split = splitLabelUsage(rows, chosen)
            LabelStatsUiState(
                loading = false,
                period = chosen,
                active = split.active.map { it.asRow(at) },
                idle = split.idle.map { it.asRow(at) },
                totals = labelUsageTotals(rows, chosen),
                labelCount = rows.size,
                mostApplied = split.active.maxOfOrNull { it.period(chosen).applied } ?: 0,
                refreshing = isRefreshing,
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), LabelStatsUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<LabelMessage> = outgoing.asSharedFlow()

    /** The user chose how far back to look. */
    fun choosePeriod(chosen: LabelUsagePeriod) {
        period.value = chosen
    }

    /**
     * Asks the sync engine to catch up.
     *
     * The report is not refetched — it cannot be, it is counted from the replica
     * — so what the gesture asks for is a catch-up, and the numbers change
     * because the catch-up wrote to the replica underneath them.
     */
    fun refresh() {
        scope.launch {
            refreshing.value = true
            try {
                sync.requestSyncNow()
            } finally {
                refreshing.value = false
            }
        }
    }

    /** Starts a label. A blank name is not a label, so nothing is written. */
    fun createLabel(
        name: String,
        color: String,
    ) {
        val named = name.trim()
        if (named.isEmpty()) return
        scope.launch { outgoing.reporting { actions.createLabel(named, color, isFavourite = false) } }
    }

    private fun LabelUsage.asRow(at: Instant) = LabelStatsRow(this, lastUsedDaysAgo(lastUsedAt, at, zone))

    private companion object {
        /**
         * How long the queries behind the screen stay open after it stops being
         * watched. Long enough to survive a rotation, short enough that a screen
         * left behind stops observing the replica.
         */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}

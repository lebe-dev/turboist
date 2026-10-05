package ru.tinyops.turboist.nativeapp.tasks

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import java.time.LocalDate
import java.time.ZoneId
import javax.inject.Inject

/**
 * The completion history, day by day.
 *
 * It is not one of the dated task lists and is deliberately not built as one:
 * those are standing queries over the replica with nothing behind them, while
 * this one has an edge where the device's copy runs out and the server takes
 * over. The day it calls "today" still has to move at midnight, though — the most
 * recent heading of the history is the one that names it.
 */
@HiltViewModel
class CompletedViewModel
    @Inject
    constructor(
        history: CompletedHistorySource,
        older: OlderCompletions,
        actions: CompletedHistoryActions,
        sync: SyncScheduler,
        clock: DayClock,
    ) : ViewModel() {
        /** The zone every date on the screen is read in. */
        val zone: ZoneId = clock.zone

        /** The day the screen calls "today", and again when it turns over. */
        val today: StateFlow<LocalDate> =
            clock.days().stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), clock.today())

        val presenter =
            CompletedHistoryPresenter(
                scope = viewModelScope,
                history = history,
                older = older,
                actions = actions,
                sync = sync,
                zone = clock.zone,
                today = clock.days(),
            )

        private companion object {
            /** Matches the presenter's own grace period, so both stop watching together. */
            const val STOP_TIMEOUT_MILLIS = 5_000L
        }
    }

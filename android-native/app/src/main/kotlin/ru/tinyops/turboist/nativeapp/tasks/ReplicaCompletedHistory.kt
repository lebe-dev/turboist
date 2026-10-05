package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.view.TimeWindow
import ru.tinyops.turboist.core.model.view.ViewWindows
import ru.tinyops.turboist.core.sync.maintenance.REPLICATED_COMPLETED_HISTORY_DAYS
import java.time.Instant
import javax.inject.Inject
import javax.inject.Singleton

/**
 * [CompletedHistorySource] over the device's own copy.
 *
 * The window is recomputed as the day turns, so a screen left open overnight
 * stops offering yesterday as the last day of the history and starts offering
 * today. The queries themselves are handed instants, never a clock, which is what
 * lets the boundary between the two halves of the screen be stated exactly.
 */
@Singleton
class ReplicaCompletedHistory
    @Inject
    constructor(
        private val repository: TaskListRepository,
        private val clock: DayClock,
    ) : CompletedHistorySource {
        @OptIn(ExperimentalCoroutinesApi::class)
        override fun observe(limit: Int): Flow<List<Task>> =
            windows().flatMapLatest { window -> repository.observeCompleted(window, limit) }

        @OptIn(ExperimentalCoroutinesApi::class)
        override fun observeCount(): Flow<Int> =
            windows().flatMapLatest { window -> repository.observeCompletedCount(window) }

        override fun observeProjectTitles(): Flow<Map<Long, String>> = repository.observeProjectTitles()

        override fun windowStart(): Long = window(clock.now()).from

        private fun windows(): Flow<TimeWindow> = clock.ticks().map(::window).distinctUntilChanged()

        private fun window(now: Instant): TimeWindow =
            ViewWindows.completedHistory(now, clock.zone, REPLICATED_COMPLETED_HISTORY_DAYS)
    }

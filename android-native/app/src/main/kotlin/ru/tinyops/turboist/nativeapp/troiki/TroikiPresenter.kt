package ru.tinyops.turboist.nativeapp.troiki

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.nativeapp.projects.ProjectMessage
import ru.tinyops.turboist.nativeapp.projects.reporting
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.tasks.TaskListActions

/** Everything the daily plan renders. */
data class TroikiUiState(
    val loading: Boolean = true,
    val slots: List<TroikiSlot> = emptyList(),
    /** The open projects that are in no bucket yet, offered when a place is filled. */
    val assignable: List<Project> = emptyList(),
    val refreshing: Boolean = false,
) {
    /** True once the plan is known to hold nothing at all, as opposed to not being known yet. */
    val isEmpty: Boolean get() = !loading && slots.all { it.projects.isEmpty() }
}

/**
 * The behaviour of the daily plan.
 *
 * A plain object driven by a scope rather than a view model, so all of it can be
 * exercised without Compose and without the platform.
 *
 * Two things about the plan are the server's alone and are deliberately not
 * modelled here: how much room a bucket has earned, and whether a cycle is
 * currently running. Both are counters kept beside the user's account rather
 * than records that are copied to the device, so the screen offers both controls
 * — begin a cycle, end one — and each is harmless when it is the wrong one: the
 * server treats beginning a running cycle, and ending a stopped one, as changing
 * nothing.
 */
class TroikiPresenter(
    private val scope: CoroutineScope,
    slots: Flow<List<TroikiSlot>>,
    assignable: Flow<List<Project>> = flowOf(emptyList()),
    private val actions: TroikiActions,
    private val taskActions: TaskListActions,
    private val sync: SyncScheduler,
) {
    private val refreshing = MutableStateFlow(false)
    private val outgoing = MutableSharedFlow<ProjectMessage>(extraBufferCapacity = 1)

    /** What the screen renders. */
    val state: StateFlow<TroikiUiState> =
        combine(slots, assignable, refreshing) { buckets, candidates, isRefreshing ->
            TroikiUiState(
                loading = false,
                slots = buckets,
                assignable = candidates,
                refreshing = isRefreshing,
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), TroikiUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<ProjectMessage> = outgoing.asSharedFlow()

    /** Asks the sync engine to catch up; the plan changes when it writes to the replica. */
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

    // --- the work in the plan ------------------------------------------------

    /** Ticks a task off, or puts it back, depending on where it stands now. */
    fun toggleComplete(task: Task) {
        queue {
            if (task.status == TaskStatus.COMPLETED) {
                taskActions.uncomplete(task.localId)
            } else {
                taskActions.complete(task.localId)
            }
        }
    }

    /** Writes down a task in one of the plan's projects. */
    fun addTask(
        projectLocalId: Long,
        title: String,
    ) {
        val named = title.trim()
        if (named.isEmpty()) return
        queue { actions.addTask(projectLocalId, named) }
    }

    // --- the buckets ---------------------------------------------------------

    /**
     * Puts a project into a bucket.
     *
     * A bucket that is already full at its starting size is refused here rather
     * than a day later: the rule is the server's, and answering it while the
     * user is still looking at the picker is the whole reason the device repeats
     * it at all.
     */
    fun assign(
        projectLocalId: Long,
        category: TroikiCategory,
    ) = queue { actions.setCategory(projectLocalId, category) }

    /** Takes a project out of the plan. Its tasks keep the priority they were given. */
    fun remove(projectLocalId: Long) = queue { actions.setCategory(projectLocalId, null) }

    /** Begins a cycle, which is what fixes how much room the lower buckets have. */
    fun start() = queue { actions.start() }

    /**
     * Ends a cycle, which empties every bucket.
     *
     * The projects leave the plan on screen straight away, because that part is
     * this device's to apply; the counters the server keeps are zeroed when the
     * request lands.
     */
    fun reset() = queue { actions.reset() }

    private fun queue(write: suspend () -> Unit) {
        scope.launch { outgoing.reporting(write) }
    }

    private companion object {
        /** Matches the other screens' grace period, so all of them stop watching together. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}

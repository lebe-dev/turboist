package ru.tinyops.turboist.nativeapp.projects

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
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.tasks.TaskListActions
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.tasks.taskListRows

/** Everything the context screen renders. */
data class ContextUiState(
    val loading: Boolean = true,
    val context: Context? = null,
    /** The projects filed under the context, in the same reading order the projects screen uses. */
    val projects: List<Project> = emptyList(),
    /** Every task under the context, the work inside its projects included. */
    val rows: List<TaskListRow> = emptyList(),
    /**
     * Whether the daily plan is part of this installation's product. With it off
     * nothing here says a project stands in one.
     */
    val dailyPlanEnabled: Boolean = false,
    val refreshing: Boolean = false,
) {
    /** True once the context is known not to be here, as opposed to not being known yet. */
    val missing: Boolean get() = !loading && context == null

    /** True once the context is known to hold nothing at all. */
    val isEmpty: Boolean get() = !loading && context != null && projects.isEmpty() && rows.isEmpty()
}

/**
 * The behaviour of the screen showing one context.
 *
 * A context is the top of the workspace tree, so the screen answers one question
 * — what is under this heading — with both halves of the answer: the projects
 * filed here, and every task that belongs to the branch, the work inside those
 * projects included. A task filed in a project carries its context too, which is
 * what lets one query answer for the whole branch.
 */
class ContextPresenter(
    private val scope: CoroutineScope,
    content: Flow<ContextContent?>,
    dailyPlanEnabled: Flow<Boolean>,
    private val actions: ContextActions,
    private val taskActions: TaskListActions,
    private val sync: SyncScheduler,
) {
    private val refreshing = MutableStateFlow(false)
    private val outgoing = MutableSharedFlow<ProjectMessage>(extraBufferCapacity = 1)

    /** What the screen renders. */
    val state: StateFlow<ContextUiState> =
        combine(content, refreshing, dailyPlanEnabled) { branch, isRefreshing, planEnabled ->
            ContextUiState(
                loading = false,
                context = branch?.context,
                projects = projectsInReadingOrder(branch?.projects.orEmpty(), ProjectFilter.ALL),
                rows =
                    taskListRows(
                        branch?.tasks.orEmpty(),
                        branch?.projects.orEmpty().associate { it.localId to it.title },
                    ),
                dailyPlanEnabled = planEnabled,
                refreshing = isRefreshing,
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), ContextUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<ProjectMessage> = outgoing.asSharedFlow()

    /** Asks the sync engine to catch up; the screen changes when it writes to the replica. */
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

    fun rename(name: String) {
        val named = name.trim()
        val contextLocalId = state.value.context?.localId ?: return
        if (named.isEmpty()) return
        queue { actions.renameContext(contextLocalId, named) }
    }

    /**
     * Removes the context, and with it its projects and their tasks — the same
     * cascade the server performs. It is the largest thing a single action in
     * this app destroys, and there is no tombstone anywhere to undo it from.
     */
    fun delete() {
        val contextLocalId = state.value.context?.localId ?: return
        queue { actions.deleteContext(contextLocalId) }
    }

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

    /** Parks a task out of the current week, taking the open work under it along. */
    fun park(task: Task) = queue { taskActions.park(task.localId) }

    /** Commits a task to the current week, taking the open work under it along. */
    fun planForWeek(task: Task) = queue { taskActions.planForWeek(task.localId) }

    private fun queue(write: suspend () -> Unit) {
        scope.launch { outgoing.reporting(write) }
    }

    private companion object {
        /** Matches the other screens' grace period, so all of them stop watching together. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}

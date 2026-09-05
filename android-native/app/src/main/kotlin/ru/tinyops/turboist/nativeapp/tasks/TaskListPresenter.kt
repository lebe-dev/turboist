package ru.tinyops.turboist.nativeapp.tasks

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
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.quickadd.pickRecent
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler

/** Everything a task list screen renders. */
data class TaskListUiState(
    val loading: Boolean = true,
    val sections: List<TaskListSection> = emptyList(),
    val announcement: String? = null,
    val refreshing: Boolean = false,
    val selectionMode: Boolean = false,
    val selected: Set<Long> = emptySet(),
    /**
     * When the calendar entries on screen were read, and `null` while they are
     * current. Non-null is the whole of what a screen says about a calendar the
     * app could not reach: the tasks are unaffected, so there is nothing to warn
     * about — only the age of the appointments beside them to be honest about.
     */
    val calendarAsOf: Long? = null,
) {
    /**
     * True once the list is known to hold nothing, as opposed to not being known
     * yet. A day with no tasks but an appointment on it is not empty: there is
     * something on screen, and saying "nothing to do" above it would be wrong.
     */
    val isEmpty: Boolean
        get() = !loading && sections.all { it.rows.isEmpty() && it.events.isEmpty() }
}

/**
 * Something a list has to say back to the user after an action.
 *
 * Carried as a value rather than as words so the presenter stays free of
 * resources and of a language; the screen turns it into a sentence.
 */
enum class TaskListMessage {
    /** The task cannot be completed while something still stands in its way. */
    BLOCKED,

    /** The two tasks are already linked that way, so there is nothing to add. */
    RELATION_EXISTS,

    /** The link would leave the tasks waiting on each other, and none could ever be finished. */
    RELATION_CYCLE,

    /** The task was made into a reusable template, which happened out of sight. */
    TEMPLATE_CREATED,

    /** The template was not made, and the user is not expected to know why. */
    TEMPLATE_FAILED,

    /** The task was not split — most often because there is already work under it. */
    DECOMPOSE_FAILED,

    /** The change did not happen, and the user is not expected to know why. */
    FAILED,
}

/**
 * What a change to a whole selection did, in the words a screen needs to report it.
 *
 * The count travels with the action because a selection action is the one place
 * a list has something to say when nothing went wrong: the tasks it changed have
 * usually left the screen, so without the sentence there is no evidence the
 * gesture landed at all.
 */
data class BulkOutcome(
    val action: BulkAction,
    val changed: Int,
    val leftBlocked: Int = 0,
    val leftLocked: Int = 0,
)

/**
 * Where a selection can be sent.
 *
 * [recent] leads the picker with the projects this device files work into, and
 * those are lifted out of [others] so nothing is offered twice. Both are empty
 * on a list wired without destinations, which is what a screen that offers no
 * selection actions gets.
 */
data class BulkDestinations(
    val recent: List<MoveProject> = emptyList(),
    val others: List<MoveProject> = emptyList(),
) {
    /** True while there is nowhere at all to send a selection. */
    val isEmpty: Boolean get() = recent.isEmpty() && others.isEmpty()
}

/**
 * The writes a task list can make.
 *
 * A narrow port onto the write path, so a list screen depends on the four things
 * it actually does rather than on the whole catalogue of task mutations — and so
 * a test of a screen needs no database.
 */
interface TaskListActions {
    suspend fun complete(taskLocalId: Long)

    suspend fun uncomplete(taskLocalId: Long)

    /** Parks a task out of the current week. */
    suspend fun park(taskLocalId: Long)

    /** Commits a task to the current week. */
    suspend fun planForWeek(taskLocalId: Long)
}

/**
 * The behaviour every task list screen shares.
 *
 * Written as a plain object driven by a scope rather than as a view model, so
 * the four screens differ only in the query behind them, and so the whole of it
 * can be exercised without Compose and without the platform.
 *
 * Every action here is optimistic, and deliberately so: the write path applies
 * the change to the replica inside the same transaction that queues it for the
 * server, and the list is a standing query over that replica, so a ticked task
 * leaves every list it was on before the device has spoken to anything. The one
 * thing that is not optimistic is a refusal — a task something still blocks
 * cannot be completed, and saying so immediately is the whole reason the rule is
 * repeated on the device at all.
 */
class TaskListPresenter(
    private val scope: CoroutineScope,
    sections: Flow<List<TaskListSection>>,
    private val actions: TaskListActions,
    private val sync: SyncScheduler,
    announcement: Flow<String?> = flowOf(null),
    calendarAsOf: Flow<Long?> = flowOf(null),
    private val bulk: BulkTaskActions? = null,
    moveOptions: Flow<List<MoveProject>> = flowOf(emptyList()),
    recentProjects: Flow<List<Long>> = flowOf(emptyList()),
) {
    private val refreshing = MutableStateFlow(false)
    private val selection = MutableStateFlow(Selection())
    private val outgoing = MutableSharedFlow<TaskListMessage>(extraBufferCapacity = 4)
    private val bulkOutgoing = MutableSharedFlow<BulkOutcome>(extraBufferCapacity = 4)

    /** What the screen renders. */
    val state: StateFlow<TaskListUiState> =
        combine(
            sections,
            announcement,
            refreshing,
            selection,
            calendarAsOf,
        ) { blocks, banner, isRefreshing, picked, asOf ->
            TaskListUiState(
                loading = false,
                sections = blocks,
                announcement = banner,
                refreshing = isRefreshing,
                selectionMode = picked.active,
                selected = picked.ids,
                calendarAsOf = asOf,
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), TaskListUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<TaskListMessage> = outgoing.asSharedFlow()

    /** What each selection action did, once each. */
    val bulkOutcomes: SharedFlow<BulkOutcome> = bulkOutgoing.asSharedFlow()

    /** True while this list has the selection actions wired in at all. */
    val offersBulkActions: Boolean = bulk != null

    /**
     * Where a selection can be sent, with the projects this device uses first.
     *
     * Built here rather than in the screen because the recent row is a
     * subtraction from the full list — a project offered at the top is taken out
     * of the body — and the two halves have to be worked out together or a
     * project appears twice.
     */
    val destinations: StateFlow<BulkDestinations> =
        combine(moveOptions, recentProjects) { projects, order ->
            val leading = pickRecent(order, projects, MoveProject::projectLocalId)
            val lifted = leading.mapTo(HashSet()) { it.projectLocalId }
            BulkDestinations(leading, projects.filterNot { it.projectLocalId in lifted })
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), BulkDestinations())

    /**
     * Asks the sync engine to catch up.
     *
     * The list is not refetched — it cannot be, it is a query — so what the
     * gesture asks for is a catch-up, and the rows change because the catch-up
     * wrote to the replica underneath them. The spinner is up for as long as the
     * catch-up lasts: it is the only thing on screen that says the gesture was
     * received, and hiding it before the answer arrives would say the opposite.
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

    /** Ticks a task off, or puts it back, depending on where it stands now. */
    fun toggleComplete(task: Task) {
        scope.launch {
            report {
                if (task.status == TaskStatus.COMPLETED) {
                    actions.uncomplete(task.localId)
                } else {
                    actions.complete(task.localId)
                }
            }
        }
    }

    /** Parks a task out of the current week, taking the open work under it along. */
    fun park(task: Task) {
        scope.launch { report { actions.park(task.localId) } }
    }

    /** Commits a task to the current week, taking the open work under it along. */
    fun planForWeek(task: Task) {
        scope.launch { report { actions.planForWeek(task.localId) } }
    }

    /** Starts picking tasks, with the one that was long-pressed already picked. */
    fun startSelection(taskLocalId: Long) {
        selection.value = Selection(active = true, ids = setOf(taskLocalId))
    }

    /**
     * Adds or removes a task from the picked set. Un-picking the last one leaves
     * selection mode, so the bar cannot linger over an empty selection.
     */
    fun toggleSelection(taskLocalId: Long) {
        val current = selection.value
        val next = if (taskLocalId in current.ids) current.ids - taskLocalId else current.ids + taskLocalId
        selection.value = if (next.isEmpty()) Selection() else Selection(active = true, ids = next)
    }

    /** Leaves selection mode, picking nothing. */
    fun clearSelection() {
        selection.value = Selection()
    }

    /**
     * Picks, or un-picks, every task of one block at once.
     *
     * The block hands over the tasks it is drawing rather than being named, so
     * the gesture picks exactly what is on screen under that heading — including
     * nothing at all, on a block that has been emptied since it was drawn.
     *
     * Tapping it again with the whole block already picked un-picks it, which is
     * the only way to undo the gesture without losing the rest of the selection.
     */
    fun toggleSelectAll(taskLocalIds: Collection<Long>) {
        if (taskLocalIds.isEmpty()) return
        val current = selection.value
        val next =
            if (taskLocalIds.all { it in current.ids }) {
                current.ids - taskLocalIds.toSet()
            } else {
                current.ids + taskLocalIds
            }
        selection.value = if (next.isEmpty()) Selection() else Selection(active = true, ids = next)
    }

    /** Ticks off the selection, leaving out whatever is still waiting on something. */
    fun completeSelected() = runOnSelection(BulkAction.COMPLETED) { bulk, ids -> bulk.complete(ids) }

    /** Sends the selection somewhere else. */
    fun moveSelected(destination: TaskDestination) =
        runOnSelection(BulkAction.MOVED) { bulk, ids -> bulk.move(ids, destination) }

    /**
     * Gives the selection one priority, leaving out the tasks whose project
     * already decides theirs.
     */
    fun prioritiseSelected(priority: Priority) =
        runOnSelection(BulkAction.PRIORITISED) { bulk, ids -> bulk.prioritise(ids, priority) }

    /**
     * Commits the selection to the week, or parks it out of the week.
     *
     * Reported as two different outcomes, because "parked 6 tasks" and "planned
     * 6 tasks" are opposite decisions and one sentence for both would leave the
     * user unsure which they had just made.
     */
    fun planSelected(state: PlanState) {
        val action = if (state == PlanState.BACKLOG) BulkAction.PARKED else BulkAction.PLANNED
        runOnSelection(action) { bulk, ids -> bulk.plan(ids, state) }
    }

    /** Deletes the selection, and with it whatever hangs under it. */
    fun deleteSelected() = runOnSelection(BulkAction.DELETED) { bulk, ids -> bulk.delete(ids) }

    /** Gathers the selection under a task created for the purpose. */
    fun groupSelected(
        title: String,
        destination: TaskDestination,
    ) = runOnSelection(BulkAction.GROUPED) { bulk, ids -> bulk.group(title, ids, destination) }

    /**
     * Runs one selection action and clears the selection behind it.
     *
     * The ids are taken before the write and the mode is left as soon as it
     * returns: the tasks that were picked have usually left the list by then, and
     * a bar still counting them would be counting rows that are no longer there.
     *
     * The order the ids are taken in is the order they were picked, which is the
     * order they reach the server. Nothing depends on it, but a stable one makes
     * a queued request the same every time it is looked at.
     */
    private fun runOnSelection(
        action: BulkAction,
        write: suspend (BulkTaskActions, List<Long>) -> BulkChange,
    ) {
        val port = bulk ?: return
        val ids = selection.value.ids.toList()
        if (ids.isEmpty()) return
        scope.launch {
            report {
                val change = write(port, ids)
                clearSelection()
                bulkOutgoing.emit(
                    BulkOutcome(action, change.changed, change.leftBlocked, change.leftLocked),
                )
            }
        }
    }

    /**
     * Runs a write and turns whatever it refuses with into something the user can
     * read. A refusal is expected traffic — the device repeats the server's rules
     * precisely so the user meets them here — so it is reported, not thrown.
     *
     * Only the blocked-task refusal is told apart, because it is the only one a
     * list can provoke that the user can act on: something else has to be
     * finished first. Every other refusal, and every unexpected failure, says the
     * same thing to the user — the change did not happen — and inventing a
     * different sentence for each would be noise rather than help.
     */
    private suspend fun report(write: suspend () -> Unit) {
        try {
            write()
        } catch (refusal: WriteRefused) {
            outgoing.emit(
                if (refusal is WriteRefused.TaskBlocked) TaskListMessage.BLOCKED else TaskListMessage.FAILED,
            )
        } catch (failure: RuntimeException) {
            outgoing.emit(TaskListMessage.FAILED)
        }
    }

    private data class Selection(
        val active: Boolean = false,
        val ids: Set<Long> = emptySet(),
    )

    private companion object {
        /**
         * How long the queries behind a list stay open after the screen stops
         * being watched. Long enough to survive a rotation, short enough that a
         * screen left behind stops observing the replica.
         */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}

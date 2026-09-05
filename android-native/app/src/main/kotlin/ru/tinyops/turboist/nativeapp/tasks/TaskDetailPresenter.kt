package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
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
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.TaskRelationGroup
import ru.tinyops.turboist.core.model.view.splitByRootCompletion
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskEdit
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.templates.TemplateCapture
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId

/** Everything the task detail screen renders. */
data class TaskDetailUiState(
    val loading: Boolean = true,
    val task: Task? = null,
    val placement: TaskPlacement = TaskPlacement(),
    val blockers: List<BlockerRef> = emptyList(),
    val relations: List<TaskRelationRef> = emptyList(),
    val relationCandidates: List<TaskRelationCandidate> = emptyList(),
    val openSubtasks: List<TaskListRow> = emptyList(),
    val doneSubtasks: List<TaskListRow> = emptyList(),
    val knownLabels: List<Label> = emptyList(),
    val priorityLocked: Boolean = false,
    val refreshing: Boolean = false,
) {
    /** True once the task is known not to be here, as opposed to not being known yet. */
    val missing: Boolean get() = !loading && task == null

    /** True while something still open stands in the way of finishing this task. */
    val blocked: Boolean get() = blockers.isNotEmpty() && task?.status != TaskStatus.COMPLETED

    /** The links under each of the three headings, in the order they were made. */
    fun relationsIn(group: TaskRelationGroup): List<TaskRelationRef> = relations.filter { it.group == group }
}

/**
 * The writes the task detail screen can make.
 *
 * It is the list screens' port plus everything a screen showing one whole task
 * can do to it, rather than a second port beside it: the detail screen embeds a
 * list of subtasks and ticks them off exactly as a list does, and two ports that
 * both meant "complete this task" would be one rule with two implementations.
 *
 * The two planning writes a list makes are stated here in terms of the general
 * one, so a screen offering "park" and a screen offering a plan state cannot
 * disagree about what either does.
 */
interface TaskDetailActions : TaskListActions {
    /** Applies a field edit. The edit carries only the fields that changed. */
    suspend fun edit(
        taskLocalId: Long,
        edit: TaskEdit,
    )

    /** Closes a task without claiming it was finished. */
    suspend fun cancel(taskLocalId: Long)

    suspend fun pin(taskLocalId: Long)

    suspend fun unpin(taskLocalId: Long)

    suspend fun duplicate(taskLocalId: Long)

    /**
     * Splits a task into the several it turned out to be.
     *
     * The pieces replace it: they inherit where it sat and everything about it
     * except its wording, and the task they were cut from stops being one. A task
     * with work already under it cannot be split, and refuses.
     */
    suspend fun decompose(
        taskLocalId: Long,
        titles: List<String>,
    )

    suspend fun plan(
        taskLocalId: Long,
        state: PlanState,
    )

    suspend fun move(
        taskLocalId: Long,
        destination: TaskDestination,
    )

    suspend fun delete(taskLocalId: Long)

    suspend fun createSubtask(
        parentTaskLocalId: Long,
        title: String,
    )

    /**
     * Links this task to another one.
     *
     * [group] carries both halves of what the user said — which kind of link, and
     * which way round it runs — because the two are one choice on screen and
     * splitting them here would let a caller make a combination the picker never
     * offers.
     */
    suspend fun addRelation(
        taskLocalId: Long,
        peerTaskLocalId: Long,
        group: TaskRelationGroup,
    )

    /** Takes a link off, from the side the user is looking at. */
    suspend fun removeRelation(
        taskLocalId: Long,
        relationLocalId: Long,
    )

    override suspend fun park(taskLocalId: Long) {
        plan(taskLocalId, PlanState.BACKLOG)
    }

    override suspend fun planForWeek(taskLocalId: Long) {
        plan(taskLocalId, PlanState.WEEK)
    }
}

/**
 * The behaviour of the task detail screen.
 *
 * Written as a plain object driven by a scope, like the list presenter, so the
 * whole of it runs without Compose and without the platform.
 *
 * Every field editor here goes through [apply], which drops an edit that changes
 * nothing and otherwise sends exactly the fields that did change. That is the
 * single rule the screen depends on: a queued write must never carry a field the
 * user did not touch, because a write that carries the whole task would undo
 * another device's edit to a different field the moment it drained.
 */
class TaskDetailPresenter(
    private val scope: CoroutineScope,
    content: Flow<TaskDetailContent?>,
    private val actions: TaskDetailActions,
    private val sync: SyncScheduler,
    private val relationSearch: TaskRelationSearch = TaskRelationSearch.None,
    private val templates: TemplateCapture = TemplateCapture.Unavailable,
) {
    private val refreshing = MutableStateFlow(false)
    private val candidates = MutableStateFlow<List<TaskRelationCandidate>>(emptyList())
    private var searching: Job? = null
    private val outgoing = MutableSharedFlow<TaskListMessage>(extraBufferCapacity = 4)
    private val removals = MutableSharedFlow<Unit>(extraBufferCapacity = 1)

    /** What the screen renders. */
    val state: StateFlow<TaskDetailUiState> =
        combine(content, refreshing, candidates) { detail, isRefreshing, offered ->
            render(detail, isRefreshing, offered)
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), TaskDetailUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<TaskListMessage> = outgoing.asSharedFlow()

    /** Fires once the task this screen was showing has been deleted, so the screen can leave. */
    val deleted: SharedFlow<Unit> = removals.asSharedFlow()

    /** Asks the sync engine to catch up. The screen itself is a query and needs no reload. */
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

    // --- field editors -------------------------------------------------

    fun rename(title: String) {
        val trimmed = title.trim()
        // An empty title would leave the task with nothing to identify it by, and
        // the server refuses one. The screen keeps the text it has instead.
        if (trimmed.isEmpty()) return
        apply { task -> TaskEdit(title = trimmed.takeIf { it != task.title }) }
    }

    fun describe(description: String) {
        apply { task -> TaskEdit(description = description.takeIf { it != task.description }) }
    }

    /**
     * Sets the priority, unless the project the task sits in already decides it.
     *
     * A project standing in the daily plan fixes the priority of its work and the
     * server turns a direct edit down, so nothing is applied and nothing is
     * queued: an edit made here would be shown, sent, refused and taken away
     * again. The picker is shown locked for the same reason.
     */
    fun setPriority(priority: Priority) {
        if (state.value.priorityLocked) return
        apply { task -> TaskEdit(priority = priority.takeIf { it != task.priority }) }
    }

    fun setDayPart(dayPart: DayPart) {
        apply { task -> TaskEdit(dayPart = dayPart.takeIf { it != task.dayPart }) }
    }

    fun setPlanState(planState: PlanState) {
        // Planning is its own write rather than a field edit: it carries the open
        // work beneath the task with it, which a field edit alone would not.
        val task = state.value.task ?: return
        scope.launch { report { actions.plan(task.localId, planState) } }
    }

    fun setDueDate(
        date: LocalDate?,
        zone: ZoneId,
    ) {
        apply { task -> dueDateEdit(task, date, zone) }
    }

    fun setDueTime(
        time: LocalTime?,
        zone: ZoneId,
    ) {
        apply { task -> dueTimeEdit(task, time, zone) }
    }

    fun setDeadlineDate(
        date: LocalDate?,
        zone: ZoneId,
    ) {
        apply { task -> deadlineDateEdit(task, date, zone) }
    }

    fun setDeadlineTime(
        time: LocalTime?,
        zone: ZoneId,
    ) {
        apply { task -> deadlineTimeEdit(task, time, zone) }
    }

    /** Sets the rule the task repeats by, or stops it repeating. */
    fun setRecurrence(rule: String?) {
        apply { task -> recurrenceEdit(task, rule) }
    }

    /**
     * Replaces the task's labels with exactly [names].
     *
     * Sent as the whole set rather than as an add or a remove, because that is
     * what the field is: the endpoint reads a list of names and the replica
     * stores the resulting edges. It is still one field.
     */
    fun setLabels(names: List<String>) {
        apply { task ->
            val current = task.labels.map { it.name }
            TaskEdit(labels = names.takeIf { it != current })
        }
    }

    fun setComplex(isComplex: Boolean) {
        apply { task -> TaskEdit(isComplex = isComplex.takeIf { it != task.isComplex }) }
    }

    fun setPrivate(isPrivate: Boolean) {
        apply { task -> TaskEdit(isPrivate = isPrivate.takeIf { it != task.isPrivate }) }
    }

    // --- actions -------------------------------------------------------

    /** Ticks the task off, or puts it back. Refused while anything still blocks it. */
    fun toggleComplete() {
        val task = state.value.task ?: return
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

    fun cancel() {
        val task = state.value.task ?: return
        scope.launch { report { actions.cancel(task.localId) } }
    }

    fun togglePin() {
        val task = state.value.task ?: return
        scope.launch {
            report { if (task.isPinned) actions.unpin(task.localId) else actions.pin(task.localId) }
        }
    }

    fun duplicate() {
        val task = state.value.task ?: return
        scope.launch { report { actions.duplicate(task.localId) } }
    }

    /**
     * Splits the task into the several it turned out to be.
     *
     * The pieces are written and the task they came from stops being one, so the
     * screen is left showing the first piece rather than something that no longer
     * exists. A blank outline asks for nothing and is ignored, as an empty title
     * would be.
     */
    fun decompose(titles: List<String>) {
        val task = state.value.task ?: return
        val named = titles.map { it.trim() }.filter { it.isNotEmpty() }
        if (named.isEmpty()) return
        scope.launch { report(TaskListMessage.DECOMPOSE_FAILED) { actions.decompose(task.localId, named) } }
    }

    /**
     * Saves the task and its subtree as a reusable template.
     *
     * It is worth saying out loud when it works, unlike every other action here:
     * the template it makes is kept somewhere the user is not looking, so nothing
     * on this screen would otherwise change.
     */
    fun createTemplate() {
        val task = state.value.task ?: return
        scope.launch {
            if (report(TaskListMessage.TEMPLATE_FAILED) { templates.captureFromTask(task.localId) }) {
                outgoing.emit(TaskListMessage.TEMPLATE_CREATED)
            }
        }
    }

    fun move(destination: TaskDestination) {
        val task = state.value.task ?: return
        scope.launch { report { actions.move(task.localId, destination) } }
    }

    /** Removes the task and, on success, tells the screen it has nothing left to show. */
    fun delete() {
        val task = state.value.task ?: return
        scope.launch {
            val removed = report { actions.delete(task.localId) }
            if (removed) removals.emit(Unit)
        }
    }

    // --- subtasks ------------------------------------------------------

    fun addSubtask(title: String) {
        val parent = state.value.task ?: return
        val trimmed = title.trim()
        if (trimmed.isEmpty()) return
        scope.launch { report { actions.createSubtask(parent.localId, trimmed) } }
    }

    fun toggleSubtaskComplete(task: Task) {
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

    // --- relations -----------------------------------------------------

    /**
     * Offers the tasks that match what has been typed into the picker.
     *
     * The search runs against the device, so it is cheap enough to run on every
     * keystroke and needs no waiting period before it starts. What it does need
     * is for an older answer never to land after a newer one, so each run
     * replaces the one before it.
     */
    fun searchRelationCandidates(typed: String) {
        searching?.cancel()
        if (typed.isBlank()) {
            candidates.value = emptyList()
            return
        }
        searching = scope.launch { candidates.value = relationSearch.candidates(typed) }
    }

    /** Empties the picker, so reopening it does not start on the last search. */
    fun clearRelationCandidates() {
        searching?.cancel()
        candidates.value = emptyList()
    }

    /** Links this task to another one, and empties the picker once it is done. */
    fun addRelation(
        peerTaskLocalId: Long,
        group: TaskRelationGroup,
    ) {
        val task = state.value.task ?: return
        scope.launch {
            if (report { actions.addRelation(task.localId, peerTaskLocalId, group) }) {
                clearRelationCandidates()
            }
        }
    }

    fun removeRelation(relationLocalId: Long) {
        val task = state.value.task ?: return
        scope.launch { report { actions.removeRelation(task.localId, relationLocalId) } }
    }

    // --- internals -----------------------------------------------------

    private fun render(
        detail: TaskDetailContent?,
        isRefreshing: Boolean,
        offered: List<TaskRelationCandidate>,
    ): TaskDetailUiState {
        if (detail == null) return TaskDetailUiState(loading = false, refreshing = isRefreshing)
        // A completed subtask is pulled out of the open list only when its own
        // topmost ancestor inside this subtree is finished, so a finished child
        // of an open parent stays where the reader expects it.
        val split = splitByRootCompletion(detail.subtasks)
        return TaskDetailUiState(
            loading = false,
            task = detail.task,
            placement = detail.placement,
            blockers = detail.blockers,
            relations = detail.relations,
            // A task cannot be linked to itself, and a pair already linked would
            // only be turned down — so neither is offered rather than being
            // offered and then refused.
            relationCandidates =
                offered.filterNot { candidate ->
                    candidate.taskLocalId == detail.task.localId ||
                        detail.relations.any { it.peerLocalId == candidate.taskLocalId }
                },
            openSubtasks = taskListRows(split.open, detail.projectTitles),
            doneSubtasks = taskListRows(split.done, detail.projectTitles),
            knownLabels = detail.knownLabels,
            priorityLocked = detail.priorityLockedByTroiki,
            refreshing = isRefreshing,
        )
    }

    /**
     * Builds an edit from the task as it stands and sends it, unless it turns out
     * to change nothing.
     *
     * The "unless" is the whole reason this exists. Screens re-emit values freely
     * — a text field reports its content when it loses focus whether or not it
     * was touched, a picker reports the option that was already chosen — and each
     * of those would otherwise become a queued write, a request, and a change
     * notification to every other device.
     */
    private fun apply(build: (Task) -> TaskEdit) {
        val task = state.value.task ?: return
        val edit = build(task)
        if (edit.touchedFields().isEmpty()) return
        scope.launch { report { actions.edit(task.localId, edit) } }
    }

    /**
     * Runs a write and turns a refusal into something the user can read.
     *
     * The refusals told apart are the ones the user can act on: work that has to
     * happen first, a link that is already there, and a link that would leave two
     * tasks waiting on each other. Each of those is a sentence with something to
     * do in it. Everything else says only that the change did not happen, which
     * is all that is honestly known.
     */
    private suspend fun report(
        onFailure: TaskListMessage = TaskListMessage.FAILED,
        write: suspend () -> Unit,
    ): Boolean {
        try {
            write()
            return true
        } catch (refusal: WriteRefused) {
            outgoing.emit(
                when (refusal) {
                    is WriteRefused.TaskBlocked -> TaskListMessage.BLOCKED
                    is WriteRefused.RelationExists -> TaskListMessage.RELATION_EXISTS
                    is WriteRefused.RelationCycle -> TaskListMessage.RELATION_CYCLE
                    else -> onFailure
                },
            )
        } catch (failure: RuntimeException) {
            outgoing.emit(onFailure)
        }
        return false
    }

    private companion object {
        /** Matches the list presenter, so a screen and the queries behind it stop together. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}

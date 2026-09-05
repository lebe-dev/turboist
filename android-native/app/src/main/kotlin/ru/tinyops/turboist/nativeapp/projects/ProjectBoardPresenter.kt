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
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.sync.write.ProjectEdit
import ru.tinyops.turboist.core.sync.write.ProjectStatusAction
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.tasks.TaskListActions

/** Everything the project board renders. */
data class ProjectBoardUiState(
    val loading: Boolean = true,
    val project: Project? = null,
    val contextName: String? = null,
    /** The project's own column first, then the board's columns left to right. */
    val columns: List<BoardColumn> = emptyList(),
    /**
     * Whether the daily plan is part of this installation's product. With it off
     * the plan does not exist for this user, so the screen neither says the
     * project stands in one nor offers to put it there.
     */
    val dailyPlanEnabled: Boolean = false,
    val refreshing: Boolean = false,
) {
    /** True once the project is known not to be here, as opposed to not being known yet. */
    val missing: Boolean get() = !loading && project == null

    /** True once the whole board is known to hold no work at all. */
    val isEmpty: Boolean get() = !loading && project != null && columns.all { it.isEmpty }

    /**
     * True while the project can take a place in the daily plan. Only open work
     * can: a finished project holding a slot would occupy a place nobody could
     * use, and the server refuses it for the same reason. A user who does not
     * keep a daily plan is not offered one either.
     */
    val canJoinDailyPlan: Boolean get() = dailyPlanEnabled && project?.status == ProjectStatus.OPEN
}

/**
 * The behaviour of the screen showing one project.
 *
 * A plain object driven by a scope rather than a view model, so all of it can be
 * exercised without Compose and without the platform.
 *
 * Everything here is optimistic and deliberately so: the write path applies each
 * change to the replica inside the same transaction that queues it for the
 * server, and the board is a standing query over that replica, so a column
 * added, renamed or moved is on screen before the device has spoken to anything.
 * Moving a column renumbers the whole board exactly as the server will, so the
 * board settles once rather than twice.
 */
class ProjectBoardPresenter(
    private val scope: CoroutineScope,
    content: Flow<ProjectBoardContent?>,
    dailyPlanEnabled: Flow<Boolean>,
    private val actions: ProjectActions,
    private val taskActions: TaskListActions,
    private val sync: SyncScheduler,
) {
    private val refreshing = MutableStateFlow(false)
    private val outgoing = MutableSharedFlow<ProjectMessage>(extraBufferCapacity = 1)

    /** What the screen renders. */
    val state: StateFlow<ProjectBoardUiState> =
        combine(content, refreshing, dailyPlanEnabled) { board, isRefreshing, planEnabled ->
            ProjectBoardUiState(
                loading = false,
                project = board?.project,
                contextName = board?.contextName,
                columns = board?.let { boardColumns(it.sections, it.tasks) }.orEmpty(),
                dailyPlanEnabled = planEnabled,
                refreshing = isRefreshing,
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), ProjectBoardUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<ProjectMessage> = outgoing.asSharedFlow()

    /** Asks the sync engine to catch up; the board changes when it writes to the replica. */
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

    // --- the work on the board -----------------------------------------------

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

    /** Writes down a task in one column, or in the project itself when [sectionLocalId] is null. */
    fun addTask(
        sectionLocalId: Long?,
        title: String,
    ) {
        val named = title.trim()
        val projectLocalId = state.value.project?.localId ?: return
        if (named.isEmpty()) return
        queue { actions.addTask(destination(projectLocalId, sectionLocalId), named) }
    }

    /**
     * Moves a task to another column of this board, or out of every column.
     *
     * The open work under it follows, here as on the server: a subtask filed in
     * a different column from its parent would be work nobody could find.
     */
    fun moveTask(
        taskLocalId: Long,
        sectionLocalId: Long?,
    ) {
        val projectLocalId = state.value.project?.localId ?: return
        queue { actions.moveTask(taskLocalId, destination(projectLocalId, sectionLocalId)) }
    }

    // --- the columns ---------------------------------------------------------

    fun createSection(title: String) {
        val named = title.trim()
        val projectLocalId = state.value.project?.localId ?: return
        if (named.isEmpty()) return
        queue { actions.createSection(projectLocalId, named) }
    }

    fun renameSection(
        sectionLocalId: Long,
        title: String,
    ) {
        val named = title.trim()
        if (named.isEmpty()) return
        queue { actions.renameSection(sectionLocalId, named) }
    }

    /**
     * Removes a column. The tasks filed in it stay in the project and become
     * unfiled, which is what the server does too — deleting a heading must not
     * delete the work under it.
     */
    fun deleteSection(sectionLocalId: Long) = queue { actions.deleteSection(sectionLocalId) }

    /** Moves a column one place towards the front of the board. */
    fun moveSectionEarlier(sectionLocalId: Long) = moveSection(sectionLocalId, -1)

    /** Moves a column one place towards the back of the board. */
    fun moveSectionLater(sectionLocalId: Long) = moveSection(sectionLocalId, +1)

    private fun moveSection(
        sectionLocalId: Long,
        by: Int,
    ) {
        val column = state.value.columns.firstOrNull { it.sectionLocalId == sectionLocalId } ?: return
        val position = column.position ?: return
        queue { actions.reorderSection(sectionLocalId, position + by) }
    }

    // --- the project itself --------------------------------------------------

    fun rename(title: String) {
        val named = title.trim()
        if (named.isEmpty()) return
        edit(ProjectEdit(title = named))
    }

    fun describe(description: String) = edit(ProjectEdit(description = description))

    fun setColor(color: String) = edit(ProjectEdit(color = color))

    fun setType(type: ProjectType) = edit(ProjectEdit(type = type))

    fun setPrivate(isPrivate: Boolean) = edit(ProjectEdit(isPrivate = isPrivate))

    /** Finishes, reopens, abandons, files away or brings back the project. */
    fun setStatus(action: ProjectStatusAction) {
        val projectLocalId = state.value.project?.localId ?: return
        queue { actions.setProjectStatus(projectLocalId, action) }
    }

    fun togglePin() {
        val project = state.value.project ?: return
        queue { if (project.isPinned) actions.unpinProject(project.localId) else actions.pinProject(project.localId) }
    }

    /** Puts the project into one of the daily slots, or takes it out of all of them. */
    fun setTroikiCategory(category: TroikiCategory?) {
        val projectLocalId = state.value.project?.localId ?: return
        queue { actions.setTroikiCategory(projectLocalId, category) }
    }

    /**
     * Removes the project, and with it its columns and everything filed under
     * them — the same cascade the server performs, because a hard delete is the
     * only kind this product has on either side.
     */
    fun delete() {
        val projectLocalId = state.value.project?.localId ?: return
        queue { actions.deleteProject(projectLocalId) }
    }

    private fun edit(change: ProjectEdit) {
        val projectLocalId = state.value.project?.localId ?: return
        queue { actions.editProject(projectLocalId, change) }
    }

    private fun destination(
        projectLocalId: Long,
        sectionLocalId: Long?,
    ): TaskDestination =
        if (sectionLocalId == null) {
            TaskDestination.InProject(projectLocalId)
        } else {
            TaskDestination.InSection(sectionLocalId)
        }

    private fun queue(write: suspend () -> Unit) {
        scope.launch { outgoing.reporting(write) }
    }

    private companion object {
        /** Matches the other screens' grace period, so all of them stop watching together. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}

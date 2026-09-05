package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.TaskRelationGroup

/**
 * How a task is addressed by whoever is opening it.
 *
 * The device's own id is the address inside the app: it names every task the
 * replica holds, including the ones written down offline that the server has
 * never heard of. A link arriving from outside carries the server's id instead —
 * it is the id the web client puts in its URL — so it has to be translated
 * before anything can be shown. Keeping the two apart is what stops a screen
 * from silently treating one number as the other.
 */
sealed interface TaskAddress {
    /** The task with this device id. */
    data class Local(val taskLocalId: Long) : TaskAddress

    /** The task the server calls this. Unknown until the replica has seen it. */
    data class Server(val serverId: Long) : TaskAddress
}

/**
 * Where a task sits, with every reference already resolved to a name.
 *
 * Placement is exclusive on the server and it is exclusive here: a task is in
 * the inbox, or under a context, or in a project, or in one of that project's
 * columns. [parentTitle] is filled in as well when the task is a subtask, since
 * a subtask's real home is the task above it.
 */
data class TaskPlacement(
    val inInbox: Boolean = false,
    val contextLocalId: Long? = null,
    val contextName: String? = null,
    val projectLocalId: Long? = null,
    val projectTitle: String? = null,
    val sectionLocalId: Long? = null,
    val sectionTitle: String? = null,
    val parentLocalId: Long? = null,
    val parentTitle: String? = null,
)

/**
 * A still-open task standing in the way of the one on screen.
 *
 * Named rather than counted: the screen refuses the completion, so it owes the
 * user the work that has to happen first and a way to reach it.
 */
data class BlockerRef(
    val taskLocalId: Long,
    val serverId: Long?,
    val title: String,
)

/**
 * One link on the task being shown, from that task's side.
 *
 * The peer is named and its state travels with it, because a link to work that
 * is already finished reads differently from a link to work that is not: the
 * first is history, the second is something to do. [group] is which of the three
 * things the link says, so the screen never has to combine a kind and a
 * direction of its own.
 */
data class TaskRelationRef(
    val relationLocalId: Long,
    val group: TaskRelationGroup,
    val peerLocalId: Long,
    val peerServerId: Long?,
    val peerTitle: String,
    val peerStatus: TaskStatus,
) {
    /** True once the peer is finished or abandoned — either way it is no longer pending. */
    val peerSettled: Boolean get() = peerStatus == TaskStatus.COMPLETED || peerStatus == TaskStatus.CANCELLED
}

/**
 * A task the user could link this one to.
 *
 * Found on the device, in the replica, which is why the picker keeps working
 * with no connection. The project title rides along because two tasks called
 * "Write it up" are told apart by where they live and by nothing else.
 */
data class TaskRelationCandidate(
    val taskLocalId: Long,
    val serverId: Long?,
    val title: String,
    val status: TaskStatus,
    val projectTitle: String?,
)

/**
 * Everything the detail screen reads about one task.
 *
 * Assembled from the replica and from nothing else, which is why the screen has
 * no loading state past the first frame and no error state at all: what the
 * device knows is what is shown, connected or not.
 *
 * [priorityLockedByTroiki] is true while the task sits in a project that stands
 * in the daily plan. Such a project fixes the priority of its work, so the field
 * is not the user's to set here and the screen says so rather than accepting an
 * edit the server would turn down.
 */
data class TaskDetailContent(
    val task: Task,
    val placement: TaskPlacement,
    val blockers: List<BlockerRef>,
    val relations: List<TaskRelationRef> = emptyList(),
    val subtasks: List<Task>,
    val projectTitles: Map<Long, String>,
    val knownLabels: List<Label>,
    val priorityLockedByTroiki: Boolean = false,
)

/** A project a task can be moved into, with the columns it offers. */
data class MoveProject(
    val projectLocalId: Long,
    val title: String,
    val sections: List<MoveSection>,
)

/** One column of a project's board, as a move destination. */
data class MoveSection(
    val sectionLocalId: Long,
    val title: String,
)

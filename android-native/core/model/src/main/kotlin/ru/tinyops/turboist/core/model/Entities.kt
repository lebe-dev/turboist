package ru.tinyops.turboist.core.model

/**
 * A grouping of projects — the top level of the workspace tree.
 *
 * @property createdAt epoch milliseconds, as every timestamp in this package is.
 */
data class Context(
    override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val name: String,
    val color: String = "",
    val isFavourite: Boolean = false,
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaEntity

/**
 * A tag attachable to tasks and projects.
 *
 * @property isPrivate hides the label, and anything carrying it, from the public view.
 */
data class Label(
    override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val name: String,
    val color: String = "",
    val isFavourite: Boolean = false,
    val isPrivate: Boolean = false,
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaEntity

/**
 * A project: a container of tasks and sections inside exactly one context.
 *
 * @property troikiCategory set only while the project sits in a capacity bucket
 *   of the daily plan.
 */
data class Project(
    override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val contextLocalId: Long,
    val title: String,
    val description: String = "",
    val color: String = "",
    val status: ProjectStatus = ProjectStatus.OPEN,
    val type: ProjectType = ProjectType.GENERIC,
    val isPinned: Boolean = false,
    val pinnedAt: Long? = null,
    val isPrivate: Boolean = false,
    val troikiCategory: TroikiCategory? = null,
    val labels: List<Label> = emptyList(),
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaEntity

/** A column of a project board. [position] is the left-to-right order. */
data class ProjectSection(
    override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val projectLocalId: Long,
    val title: String,
    val position: Int = 0,
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaEntity

/**
 * A task.
 *
 * Placement is exclusive: a task sits in the inbox, in a context, in a project or
 * in a section of one, and [parentLocalId] makes it a subtask of another task.
 * All four placement references and the parent are local ids, resolved by the
 * replica; [inboxId] is the exception — the inbox is a single server-side row
 * whose id is the same everywhere ([INBOX_ID]).
 *
 * @property dueHasTime distinguishes "due on the 9th" from "due at 12:34 on the
 *   9th"; the timestamp alone cannot express the difference.
 * @property isComplex marks work the user considers demanding. Purely
 *   descriptive — nothing depends on it.
 * @property recurrenceRule an RRULE. Completing a recurring task advances it and
 *   leaves a completed snapshot behind.
 * @property sourceTaskLocalId points such a snapshot back at the recurring task
 *   it was cut from.
 * @property troikiCategory the capacity bucket of the daily plan this task sits
 *   in. The task payloads of the REST API do not carry it — it arrives with the
 *   plan view, which is what fills this field in the replica.
 * @property relationSummary the cheap rollup that every read path carries, so a
 *   list can render a blocked task as blocked without loading its relations.
 * @property relations the edges themselves, loaded only where they are shown.
 */
data class Task(
    override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val title: String,
    val description: String = "",
    val inboxId: Long? = null,
    val contextLocalId: Long? = null,
    val projectLocalId: Long? = null,
    val sectionLocalId: Long? = null,
    val parentLocalId: Long? = null,
    val priority: Priority = Priority.NONE,
    val status: TaskStatus = TaskStatus.OPEN,
    val dueAt: Long? = null,
    val dueHasTime: Boolean = false,
    val deadlineAt: Long? = null,
    val deadlineHasTime: Boolean = false,
    val dayPart: DayPart = DayPart.NONE,
    val planState: PlanState = PlanState.NONE,
    val isPinned: Boolean = false,
    val pinnedAt: Long? = null,
    val isPrivate: Boolean = false,
    val isComplex: Boolean = false,
    val completedAt: Long? = null,
    val recurrenceRule: String? = null,
    val sourceTaskLocalId: Long? = null,
    val postponeCount: Int = 0,
    val troikiCategory: TroikiCategory? = null,
    val labels: List<Label> = emptyList(),
    val relationSummary: TaskRelationSummary = TaskRelationSummary.NONE,
    val relations: List<TaskRelation> = emptyList(),
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaEntity {
    /**
     * True while at least one task that blocks this one is still open. Completing
     * such a task is refused by the server, so the client refuses it too rather
     * than queueing a write that is certain to come back rejected.
     */
    val isBlocked: Boolean
        get() = relationSummary.blockedByOpen > 0

    /**
     * The task's address in the web app, for sharing and deep links. A task
     * created offline has no server id yet and therefore no address to share —
     * hence the `null`.
     */
    fun url(baseUrl: String): String? {
        val id = serverId ?: return null
        return baseUrl.trimEnd('/') + "/task/" + id
    }
}

/**
 * One edge of the task graph.
 *
 * The stored row names both of its endpoints and nothing else; [direction] is
 * filled in only when the edge was loaded from the point of view of one task, and
 * describes the peer end from there. The same row reads as "blocked by X" on one
 * task and "blocks Y" on the other, which is what [directionFor] computes.
 *
 * @property other the peer task, hydrated only where the edge is rendered.
 */
data class TaskRelation(
    override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val sourceTaskLocalId: Long,
    val targetTaskLocalId: Long,
    val type: RelationType,
    val direction: RelationDirection? = null,
    val createdAt: Long,
    val other: Task? = null,
) : ReplicaEntity {
    /**
     * How this edge reads from one of its endpoints: outgoing when that task
     * blocks the peer, incoming when the peer blocks it. `null` when the task is
     * not an endpoint of this edge at all.
     */
    fun directionFor(taskLocalId: Long): RelationDirection? =
        when (taskLocalId) {
            sourceTaskLocalId -> RelationDirection.OUTGOING
            targetTaskLocalId -> RelationDirection.INCOMING
            else -> null
        }
}

/**
 * What a task's relations imply, without listing them.
 *
 * @property blockedByOpen how many still-open tasks block this one, counting the
 *   blockers inherited from its ancestors. Non-zero means completion is refused.
 * @property total every relation the task itself has, both directions, both
 *   kinds. Inherited blockers are not counted here — they belong to an ancestor.
 */
data class TaskRelationSummary(
    val blockedByOpen: Int = 0,
    val total: Int = 0,
) {
    companion object {
        /** A task with no relations at all — the common case, worth not allocating for. */
        val NONE: TaskRelationSummary = TaskRelationSummary()
    }
}

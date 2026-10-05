package ru.tinyops.turboist.core.sync.write

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.Clearable
import ru.tinyops.turboist.core.network.dto.CreateContextRequest
import ru.tinyops.turboist.core.network.dto.CreateLabelRequest
import ru.tinyops.turboist.core.network.dto.CreateProjectRequest
import ru.tinyops.turboist.core.network.dto.CreateSectionRequest
import ru.tinyops.turboist.core.network.dto.CreateTaskRequest
import ru.tinyops.turboist.core.network.dto.PatchContextRequest
import ru.tinyops.turboist.core.network.dto.PatchLabelRequest
import ru.tinyops.turboist.core.network.dto.PatchSectionRequest
import ru.tinyops.turboist.core.network.dto.PatchTaskRequest
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest

/**
 * Every write this app can make, as a value that survives a process death.
 *
 * A user action is one transaction: the replica is changed as the user expects
 * and the write that will tell the server about it is queued alongside, so the
 * screen is right immediately and the change cannot be lost between the two.
 * This file is the catalog of those queued writes — one type per mutation the
 * API offers, no more and no fewer, so that "which request does this become"
 * has exactly one answer.
 *
 * **Payloads are wire-shaped except for references, which are local.** Field
 * values are already in the form the server reads (timestamps as the wire's
 * UTC text, enums as their wire spellings), because the queue is a queue of
 * requests. References to other rows are the device's own ids: a task created
 * with no network has no server id for hours, and a reference recorded as
 * "nothing yet" could never be repaired. Whoever sends an op therefore
 * translates its references at that moment, by which time the rows they name
 * have been created and carry server ids — the queue drains strictly in order,
 * so a child is never sent before the parent it was created under.
 *
 * The two exceptions are the ids inside the preference and rule documents. Those
 * are server ids in both directions by contract: the server wrote them into the
 * document and reads them back out of it, and rewriting them to local ids would
 * corrupt a document this client only passes through.
 *
 * ## The catalog
 *
 * | Op | Method | Path |
 * |----|--------|------|
 * | `task.create` | POST | `api/v1/inbox/tasks`, or `api/v1/contexts/{id}/tasks`, `api/v1/projects/{id}/tasks`, `api/v1/sections/{id}/tasks`, `api/v1/tasks/{id}/subtasks` by destination |
 * | `task.patch` | PATCH | `api/v1/tasks/{id}` |
 * | `task.delete` | DELETE | `api/v1/tasks/{id}` |
 * | `task.complete` | POST | `api/v1/tasks/{id}/complete` |
 * | `task.uncomplete` | POST | `api/v1/tasks/{id}/uncomplete` |
 * | `task.cancel` | POST | `api/v1/tasks/{id}/cancel` |
 * | `task.move` | POST | `api/v1/tasks/{id}/move` |
 * | `task.plan` | POST | `api/v1/tasks/{id}/plan` |
 * | `task.pin` | POST | `api/v1/tasks/{id}/pin` |
 * | `task.unpin` | POST | `api/v1/tasks/{id}/unpin` |
 * | `task.duplicate` | POST | `api/v1/tasks/{id}/duplicate` |
 * | `task.decompose` | POST | `api/v1/tasks/{id}/decompose` |
 * | `task.relation.add` | POST | `api/v1/tasks/{id}/relations` |
 * | `task.relation.remove` | DELETE | `api/v1/tasks/{id}/relations/{relationId}` |
 * | `task.bulk.complete` | POST | `api/v1/tasks/bulk/complete` |
 * | `task.bulk.move` | POST | `api/v1/tasks/bulk/move` |
 * | `task.bulk.priority` | POST | `api/v1/tasks/bulk/priority` |
 * | `task.group` | POST | `api/v1/tasks/group` |
 * | `project.create` | POST | `api/v1/contexts/{id}/projects` |
 * | `project.patch` | PATCH | `api/v1/projects/{id}` |
 * | `project.delete` | DELETE | `api/v1/projects/{id}` |
 * | `project.status` | POST | `api/v1/projects/{id}/complete`, `/uncomplete`, `/cancel`, `/archive`, `/unarchive` by action |
 * | `project.pin` | POST | `api/v1/projects/{id}/pin` |
 * | `project.unpin` | POST | `api/v1/projects/{id}/unpin` |
 * | `project.troiki` | POST | `api/v1/projects/{id}/troiki` |
 * | `section.create` | POST | `api/v1/projects/{id}/sections` |
 * | `section.patch` | PATCH | `api/v1/sections/{id}` |
 * | `section.delete` | DELETE | `api/v1/sections/{id}` |
 * | `section.reorder` | POST | `api/v1/sections/{id}/reorder` |
 * | `context.create` | POST | `api/v1/contexts` |
 * | `context.patch` | PATCH | `api/v1/contexts/{id}` |
 * | `context.delete` | DELETE | `api/v1/contexts/{id}` |
 * | `label.create` | POST | `api/v1/labels` |
 * | `label.patch` | PATCH | `api/v1/labels/{id}` |
 * | `label.delete` | DELETE | `api/v1/labels/{id}` |
 * | `template.create` | POST | `api/v1/task-templates` |
 * | `template.replace` | PATCH | `api/v1/task-templates/{id}` |
 * | `template.delete` | DELETE | `api/v1/task-templates/{id}` |
 * | `template.instantiate` | POST | `api/v1/task-templates/{id}/instantiate` |
 * | `settings.patch` | PATCH | `api/v1/settings` |
 * | `appSettings.autoLabels` | PUT | `api/v1/app-settings/auto-labels` |
 * | `appSettings.projectSuggestions` | PUT | `api/v1/app-settings/project-suggestions` |
 * | `state.patch` | PATCH | `api/v1/state` |
 * | `troiki.start` | POST | `api/v1/troiki/start` |
 * | `troiki.reset` | POST | `api/v1/troiki/reset` |
 * | `harpoon.attach` | POST | `api/v1/harpoon/attach` |
 * | `harpoon.detach` | POST | `api/v1/harpoon/detach` |
 *
 * ## What is deliberately not here
 *
 * The table is the whole vocabulary, so a mutation the API offers and this
 * file does not name is one the app never queues. Signing in, second-factor
 * and passkey enrolment, session and API-token administration, backup export
 * and restore, and anything touching the calendar integration are all made
 * against the server directly, while the user waits. Each of them either needs
 * the server's answer to mean anything, or changes the ground the replica
 * itself stands on — a restore, in particular, makes every device start over.
 * Queuing such a thing for later would promise an outcome nobody could honour.
 *
 * The same mapping exists as code in [endpoint], which the compiler checks is
 * total; the table is here so the answer can be read without following a
 * `when`.
 */
@Serializable
sealed interface OutboxOp {
    /**
     * The op's stable name, stored beside its payload. It is part of the queue's
     * on-disk form: renaming one orphans every write a device has already queued
     * under the old name, so the vocabulary is append-only.
     */
    val kind: OutboxOpKind
}

/** The name of every op, and nothing else — the endpoint it maps to is [endpoint]. */
enum class OutboxOpKind(val stored: String) {
    TASK_CREATE(OpNames.TASK_CREATE),
    TASK_PATCH(OpNames.TASK_PATCH),
    TASK_DELETE(OpNames.TASK_DELETE),
    TASK_COMPLETE(OpNames.TASK_COMPLETE),
    TASK_UNCOMPLETE(OpNames.TASK_UNCOMPLETE),
    TASK_CANCEL(OpNames.TASK_CANCEL),
    TASK_MOVE(OpNames.TASK_MOVE),
    TASK_PLAN(OpNames.TASK_PLAN),
    TASK_PIN(OpNames.TASK_PIN),
    TASK_UNPIN(OpNames.TASK_UNPIN),
    TASK_DUPLICATE(OpNames.TASK_DUPLICATE),
    TASK_DECOMPOSE(OpNames.TASK_DECOMPOSE),
    TASK_RELATION_ADD(OpNames.TASK_RELATION_ADD),
    TASK_RELATION_REMOVE(OpNames.TASK_RELATION_REMOVE),
    TASK_BULK_COMPLETE(OpNames.TASK_BULK_COMPLETE),
    TASK_BULK_MOVE(OpNames.TASK_BULK_MOVE),
    TASK_BULK_PRIORITY(OpNames.TASK_BULK_PRIORITY),
    TASK_GROUP(OpNames.TASK_GROUP),
    PROJECT_CREATE(OpNames.PROJECT_CREATE),
    PROJECT_PATCH(OpNames.PROJECT_PATCH),
    PROJECT_DELETE(OpNames.PROJECT_DELETE),
    PROJECT_STATUS(OpNames.PROJECT_STATUS),
    PROJECT_PIN(OpNames.PROJECT_PIN),
    PROJECT_UNPIN(OpNames.PROJECT_UNPIN),
    PROJECT_TROIKI(OpNames.PROJECT_TROIKI),
    SECTION_CREATE(OpNames.SECTION_CREATE),
    SECTION_PATCH(OpNames.SECTION_PATCH),
    SECTION_DELETE(OpNames.SECTION_DELETE),
    SECTION_REORDER(OpNames.SECTION_REORDER),
    CONTEXT_CREATE(OpNames.CONTEXT_CREATE),
    CONTEXT_PATCH(OpNames.CONTEXT_PATCH),
    CONTEXT_DELETE(OpNames.CONTEXT_DELETE),
    LABEL_CREATE(OpNames.LABEL_CREATE),
    LABEL_PATCH(OpNames.LABEL_PATCH),
    LABEL_DELETE(OpNames.LABEL_DELETE),
    TEMPLATE_CREATE(OpNames.TEMPLATE_CREATE),
    TEMPLATE_REPLACE(OpNames.TEMPLATE_REPLACE),
    TEMPLATE_DELETE(OpNames.TEMPLATE_DELETE),
    TEMPLATE_INSTANTIATE(OpNames.TEMPLATE_INSTANTIATE),
    SETTINGS_PATCH(OpNames.SETTINGS_PATCH),
    APP_SETTINGS_AUTO_LABELS(OpNames.APP_SETTINGS_AUTO_LABELS),
    APP_SETTINGS_PROJECT_SUGGESTIONS(OpNames.APP_SETTINGS_PROJECT_SUGGESTIONS),
    STATE_PATCH(OpNames.STATE_PATCH),
    TROIKI_START(OpNames.TROIKI_START),
    TROIKI_RESET(OpNames.TROIKI_RESET),
    HARPOON_ATTACH(OpNames.HARPOON_ATTACH),
    HARPOON_DETACH(OpNames.HARPOON_DETACH),
    ;

    /**
     * True for the writes that bring a row into existence and name it by the id
     * this device gave it.
     *
     * The distinction matters when a write is thrown away rather than sent: every
     * other write was made *to* a row the server also holds, so discarding it
     * leaves nothing behind, while discarding one of these leaves a row that
     * exists on this device and nowhere else and never will.
     */
    val creates: Boolean get() = this in CREATES

    companion object {
        private val byStored: Map<String, OutboxOpKind> = entries.associateBy { it.stored }

        private val CREATES: Set<OutboxOpKind> =
            setOf(TASK_CREATE, PROJECT_CREATE, SECTION_CREATE, CONTEXT_CREATE, LABEL_CREATE, TEMPLATE_CREATE)

        /** The op a stored name denotes, or `null` when this build has never heard of it. */
        fun fromStored(value: String?): OutboxOpKind? = byStored[value]
    }
}

/**
 * The op names as compile-time constants.
 *
 * They exist as constants because the same string is needed in two places that
 * cannot see each other's values: the serialized form of the payload, which an
 * annotation fixes at compile time, and [OutboxOpKind], which is what the rest
 * of the code names an op by. Writing the string twice is how the two drift.
 */
object OpNames {
    const val TASK_CREATE = "task.create"
    const val TASK_PATCH = "task.patch"
    const val TASK_DELETE = "task.delete"
    const val TASK_COMPLETE = "task.complete"
    const val TASK_UNCOMPLETE = "task.uncomplete"
    const val TASK_CANCEL = "task.cancel"
    const val TASK_MOVE = "task.move"
    const val TASK_PLAN = "task.plan"
    const val TASK_PIN = "task.pin"
    const val TASK_UNPIN = "task.unpin"
    const val TASK_DUPLICATE = "task.duplicate"
    const val TASK_DECOMPOSE = "task.decompose"
    const val TASK_RELATION_ADD = "task.relation.add"
    const val TASK_RELATION_REMOVE = "task.relation.remove"
    const val TASK_BULK_COMPLETE = "task.bulk.complete"
    const val TASK_BULK_MOVE = "task.bulk.move"
    const val TASK_BULK_PRIORITY = "task.bulk.priority"
    const val TASK_GROUP = "task.group"
    const val PROJECT_CREATE = "project.create"
    const val PROJECT_PATCH = "project.patch"
    const val PROJECT_DELETE = "project.delete"
    const val PROJECT_STATUS = "project.status"
    const val PROJECT_PIN = "project.pin"
    const val PROJECT_UNPIN = "project.unpin"
    const val PROJECT_TROIKI = "project.troiki"
    const val SECTION_CREATE = "section.create"
    const val SECTION_PATCH = "section.patch"
    const val SECTION_DELETE = "section.delete"
    const val SECTION_REORDER = "section.reorder"
    const val CONTEXT_CREATE = "context.create"
    const val CONTEXT_PATCH = "context.patch"
    const val CONTEXT_DELETE = "context.delete"
    const val LABEL_CREATE = "label.create"
    const val LABEL_PATCH = "label.patch"
    const val LABEL_DELETE = "label.delete"
    const val TEMPLATE_CREATE = "template.create"
    const val TEMPLATE_REPLACE = "template.replace"
    const val TEMPLATE_DELETE = "template.delete"
    const val TEMPLATE_INSTANTIATE = "template.instantiate"
    const val SETTINGS_PATCH = "settings.patch"
    const val APP_SETTINGS_AUTO_LABELS = "appSettings.autoLabels"
    const val APP_SETTINGS_PROJECT_SUGGESTIONS = "appSettings.projectSuggestions"
    const val STATE_PATCH = "state.patch"
    const val TROIKI_START = "troiki.start"
    const val TROIKI_RESET = "troiki.reset"
    const val HARPOON_ATTACH = "harpoon.attach"
    const val HARPOON_DETACH = "harpoon.detach"
}

// --- references and destinations -------------------------------------------

/**
 * A row of the replica, named the way an op names one: by the table it is in and
 * the device's own id for it. Turning that into the server id a URL needs is the
 * job of whoever sends the op, at the moment it is sent.
 */
data class OpRef(
    val entity: ReplicaEntityKind,
    val localId: Long,
)

/** The HTTP verbs the API's mutations use. */
enum class OpMethod { POST, PATCH, PUT, DELETE }

/**
 * Where an op's request goes.
 *
 * [path] carries `{id}` — and, for one endpoint, `{relationId}` — rather than a
 * number, because the number is a server id that may not exist yet when the op
 * is queued. [pathRef] and [secondaryRef] name the rows those placeholders stand
 * for.
 */
data class OpEndpoint(
    val method: OpMethod,
    val path: String,
    val pathRef: OpRef? = null,
    val secondaryRef: OpRef? = null,
)

/**
 * Where a task sits.
 *
 * Placement is exclusive — a task is in the inbox, in a context, in a project, in
 * a section of one, or under another task — which is why this is a closed set of
 * alternatives rather than five nullable ids. Five nullable ids can express
 * "in a project and in a context at once", which is not a thing, and the check
 * for it then has to be written at every call site.
 */
@Serializable
sealed interface TaskDestination {
    /** The row the destination points at, or `null` for the inbox, which is not a replicated row. */
    val ref: OpRef?

    @Serializable
    @SerialName("inbox")
    data object Inbox : TaskDestination {
        override val ref: OpRef? get() = null
    }

    @Serializable
    @SerialName("context")
    data class InContext(val contextLocalId: Long) : TaskDestination {
        override val ref: OpRef get() = OpRef(ReplicaEntityKind.CONTEXT, contextLocalId)
    }

    @Serializable
    @SerialName("project")
    data class InProject(val projectLocalId: Long) : TaskDestination {
        override val ref: OpRef get() = OpRef(ReplicaEntityKind.PROJECT, projectLocalId)
    }

    @Serializable
    @SerialName("section")
    data class InSection(val sectionLocalId: Long) : TaskDestination {
        override val ref: OpRef get() = OpRef(ReplicaEntityKind.SECTION, sectionLocalId)
    }

    @Serializable
    @SerialName("subtaskOf")
    data class SubtaskOf(val parentTaskLocalId: Long) : TaskDestination {
        override val ref: OpRef get() = OpRef(ReplicaEntityKind.TASK, parentTaskLocalId)
    }
}

/** The five one-shot transitions a project's status endpoint offers. */
@Serializable
enum class ProjectStatusAction(val segment: String) {
    @SerialName("complete")
    COMPLETE("complete"),

    @SerialName("uncomplete")
    UNCOMPLETE("uncomplete"),

    @SerialName("cancel")
    CANCEL("cancel"),

    @SerialName("archive")
    ARCHIVE("archive"),

    @SerialName("unarchive")
    UNARCHIVE("unarchive"),
}

/** What a harpoon slot points at. Kept as its own type because it names a table, not a value. */
@Serializable
enum class HarpoonTarget(val wire: String) {
    @SerialName("task")
    TASK("task"),

    @SerialName("project")
    PROJECT("project"),
}

/**
 * A template's own fields, with its labels named by local id.
 *
 * The wire form of a template carries label ids, and those have to be
 * translatable: a label created offline and used in a template the same evening
 * has no server id until both have been sent. So the queued form holds local
 * ids and the sent form is built from them.
 */
@Serializable
data class TemplateSubtaskPayload(
    val title: String,
    val description: String? = null,
    val priority: String? = null,
    val dayPart: String? = null,
    val labelLocalIds: List<Long> = emptyList(),
)

@Serializable
data class TemplatePayload(
    val name: String,
    val description: String? = null,
    val priority: String? = null,
    val dayPart: String? = null,
    val labelLocalIds: List<Long> = emptyList(),
    val subtasks: List<TemplateSubtaskPayload> = emptyList(),
)

// --- tasks ------------------------------------------------------------------

/**
 * A task the user wrote down.
 *
 * [taskLocalId] is the row already inserted in the replica. It is in the payload
 * because the answer to this request carries the server id, and something has to
 * know which local row to write it onto.
 */
@Serializable
@SerialName(OpNames.TASK_CREATE)
data class CreateTaskOp(
    val taskLocalId: Long,
    val destination: TaskDestination,
    val body: CreateTaskRequest,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_CREATE
}

/**
 * An edit to a task, carrying only what changed.
 *
 * A patch that resent every field would overwrite, on the server, whatever
 * changed there while this write sat in the queue. Field-scoped patches merge
 * instead: two devices that edited different fields both keep their edit.
 */
@Serializable
@SerialName(OpNames.TASK_PATCH)
data class PatchTaskOp(
    val taskLocalId: Long,
    val patch: PatchTaskPayload,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_PATCH
}

/**
 * A task edit as the queue stores it.
 *
 * The API reads three states out of a patch body — a key that is absent leaves
 * the field alone, an explicit null empties it, a value sets it — and the third
 * of those is what this shape exists for. On the wire "empty it" is a JSON null,
 * but a JSON null read back into a nullable field is indistinguishable from a
 * field that was never there: the queue would store "clear the due date" and
 * hand back "change nothing", and the write would silently do nothing days
 * later. So an emptying is stored as a flag of its own, which cannot be confused
 * with anything, and [toRequest] turns it back into the null the server reads.
 */
@Serializable
data class PatchTaskPayload(
    val title: String? = null,
    val description: String? = null,
    val priority: String? = null,
    val dueAt: String? = null,
    val clearDueAt: Boolean = false,
    val dueHasTime: Boolean? = null,
    val deadlineAt: String? = null,
    val clearDeadlineAt: Boolean = false,
    val deadlineHasTime: Boolean? = null,
    val dayPart: String? = null,
    val planState: String? = null,
    val recurrenceRule: String? = null,
    val clearRecurrenceRule: Boolean = false,
    val labels: List<String>? = null,
    val removedAutoLabels: List<String>? = null,
    val isPrivate: Boolean? = null,
    val isComplex: Boolean? = null,
) {
    /** The request body, with each emptying spelled the way the API reads it. */
    fun toRequest(): PatchTaskRequest =
        PatchTaskRequest(
            title = title,
            description = description,
            priority = priority,
            dueAt = clearable(dueAt, clearDueAt),
            dueHasTime = dueHasTime,
            deadlineAt = clearable(deadlineAt, clearDeadlineAt),
            deadlineHasTime = deadlineHasTime,
            dayPart = dayPart,
            planState = planState,
            recurrenceRule = clearable(recurrenceRule, clearRecurrenceRule),
            labels = labels,
            removedAutoLabels = removedAutoLabels,
            isPrivate = isPrivate,
            isComplex = isComplex,
        )

    private fun clearable(
        value: String?,
        cleared: Boolean,
    ): Clearable<String>? =
        when {
            cleared -> Clearable.Clear
            value != null -> Clearable.Set(value)
            else -> null
        }
}

/**
 * Removing a task.
 *
 * [serverId] is the exception to the rule that a queued write records references
 * as local ids and translates them when it is sent. A delete is the one write
 * whose subject is certainly gone by the time it goes out — the row leaves the
 * replica in the same transaction that queues this — so there is nothing left to
 * look the id up on afterwards, and it is written down here while it is still
 * knowable. A row the server has never heard of has none, and the create ahead
 * of it in the queue supplies it. Every delete below carries it for the same
 * reason.
 */
@Serializable
@SerialName(OpNames.TASK_DELETE)
data class DeleteTaskOp(
    val taskLocalId: Long,
    val serverId: Long? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_DELETE
}

/**
 * Ticking a task off.
 *
 * [completedAt] is the moment the user actually did it, not the moment the queue
 * drained, which is what keeps a day's history honest after a day offline.
 */
@Serializable
@SerialName(OpNames.TASK_COMPLETE)
data class CompleteTaskOp(
    val taskLocalId: Long,
    val completedAt: String? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_COMPLETE
}

@Serializable
@SerialName(OpNames.TASK_UNCOMPLETE)
data class UncompleteTaskOp(val taskLocalId: Long) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_UNCOMPLETE
}

@Serializable
@SerialName(OpNames.TASK_CANCEL)
data class CancelTaskOp(val taskLocalId: Long) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_CANCEL
}

@Serializable
@SerialName(OpNames.TASK_MOVE)
data class MoveTaskOp(
    val taskLocalId: Long,
    val destination: TaskDestination,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_MOVE
}

/** Committing a task to the week or parking it in the backlog. [state] is the wire spelling. */
@Serializable
@SerialName(OpNames.TASK_PLAN)
data class PlanTaskOp(
    val taskLocalId: Long,
    val state: String,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_PLAN
}

@Serializable
@SerialName(OpNames.TASK_PIN)
data class PinTaskOp(val taskLocalId: Long) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_PIN
}

@Serializable
@SerialName(OpNames.TASK_UNPIN)
data class UnpinTaskOp(val taskLocalId: Long) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_UNPIN
}

@Serializable
@SerialName(OpNames.TASK_DUPLICATE)
data class DuplicateTaskOp(val taskLocalId: Long) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_DUPLICATE
}

/**
 * Splitting one task into several siblings that inherit its placement.
 *
 * The split is made on the device as well as asked for: the pieces are written
 * into the replica and the task they were cut from is removed, in the same
 * transaction that queues this. [createdTaskLocalIds] are those pieces, in the
 * order of [titles], so the answer's tasks can be matched to them one for one.
 *
 * The list is empty in a write queued by a build that left the whole split to
 * the server. Such a write still sends; its pieces simply arrive with the next
 * catch-up instead of being named here.
 */
@Serializable
@SerialName(OpNames.TASK_DECOMPOSE)
data class DecomposeTaskOp(
    val taskLocalId: Long,
    val titles: List<String>,
    val createdTaskLocalIds: List<Long> = emptyList(),
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_DECOMPOSE
}

/**
 * Linking two tasks.
 *
 * [relationLocalId] is the edge already written into the replica, so the answer's
 * server id has a row to land on — the same reason a create op carries the id of
 * the row it created.
 */
@Serializable
@SerialName(OpNames.TASK_RELATION_ADD)
data class AddTaskRelationOp(
    val taskLocalId: Long,
    val targetTaskLocalId: Long,
    val relationLocalId: Long,
    val type: String,
    val direction: String? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_RELATION_ADD
}

@Serializable
@SerialName(OpNames.TASK_RELATION_REMOVE)
data class RemoveTaskRelationOp(
    val taskLocalId: Long,
    val relationLocalId: Long,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_RELATION_REMOVE
}

/**
 * A bulk action, which the server answers per item rather than all-or-nothing:
 * one task refused must not undo the nine that went through.
 */
@Serializable
@SerialName(OpNames.TASK_BULK_COMPLETE)
data class BulkCompleteTasksOp(val taskLocalIds: List<Long>) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_BULK_COMPLETE
}

@Serializable
@SerialName(OpNames.TASK_BULK_MOVE)
data class BulkMoveTasksOp(
    val taskLocalIds: List<Long>,
    val destination: TaskDestination,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_BULK_MOVE
}

@Serializable
@SerialName(OpNames.TASK_BULK_PRIORITY)
data class BulkTaskPriorityOp(
    val taskLocalIds: List<Long>,
    val priority: String,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_BULK_PRIORITY
}

/**
 * Gathering loose tasks under a new parent.
 *
 * The parent is created by the same request, so [parentTaskLocalId] names the row
 * the replica already holds for it and the children are re-parented onto it.
 */
@Serializable
@SerialName(OpNames.TASK_GROUP)
data class GroupTasksOp(
    val parentTaskLocalId: Long,
    val childTaskLocalIds: List<Long>,
    val destination: TaskDestination? = null,
    val title: String,
    val description: String? = null,
    val priority: String? = null,
    val dayPart: String? = null,
    val planState: String? = null,
    val labels: List<String> = emptyList(),
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TASK_GROUP
}

// --- projects and sections --------------------------------------------------

@Serializable
@SerialName(OpNames.PROJECT_CREATE)
data class CreateProjectOp(
    val projectLocalId: Long,
    val contextLocalId: Long,
    val body: CreateProjectRequest,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.PROJECT_CREATE
}

/**
 * An edit to a project. The context is a reference and therefore local, which is
 * why this op does not simply carry the wire body.
 */
@Serializable
@SerialName(OpNames.PROJECT_PATCH)
data class PatchProjectOp(
    val projectLocalId: Long,
    val title: String? = null,
    val description: String? = null,
    val color: String? = null,
    val contextLocalId: Long? = null,
    val labels: List<String>? = null,
    val isPrivate: Boolean? = null,
    val projectType: String? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.PROJECT_PATCH
}

@Serializable
@SerialName(OpNames.PROJECT_DELETE)
data class DeleteProjectOp(
    val projectLocalId: Long,
    val serverId: Long? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.PROJECT_DELETE
}

@Serializable
@SerialName(OpNames.PROJECT_STATUS)
data class ProjectStatusOp(
    val projectLocalId: Long,
    val action: ProjectStatusAction,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.PROJECT_STATUS
}

@Serializable
@SerialName(OpNames.PROJECT_PIN)
data class PinProjectOp(val projectLocalId: Long) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.PROJECT_PIN
}

@Serializable
@SerialName(OpNames.PROJECT_UNPIN)
data class UnpinProjectOp(val projectLocalId: Long) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.PROJECT_UNPIN
}

/** Putting a project in one of the three daily slots, or taking it out of all of them. */
@Serializable
@SerialName(OpNames.PROJECT_TROIKI)
data class SetProjectTroikiOp(
    val projectLocalId: Long,
    val category: String? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.PROJECT_TROIKI
}

@Serializable
@SerialName(OpNames.SECTION_CREATE)
data class CreateSectionOp(
    val sectionLocalId: Long,
    val projectLocalId: Long,
    val body: CreateSectionRequest,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.SECTION_CREATE
}

@Serializable
@SerialName(OpNames.SECTION_PATCH)
data class PatchSectionOp(
    val sectionLocalId: Long,
    val body: PatchSectionRequest,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.SECTION_PATCH
}

@Serializable
@SerialName(OpNames.SECTION_DELETE)
data class DeleteSectionOp(
    val sectionLocalId: Long,
    val serverId: Long? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.SECTION_DELETE
}

/** Moving a board column. The server owns the final ordering; this is a request, not a fact. */
@Serializable
@SerialName(OpNames.SECTION_REORDER)
data class ReorderSectionOp(
    val sectionLocalId: Long,
    val position: Int,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.SECTION_REORDER
}

// --- contexts and labels ----------------------------------------------------

@Serializable
@SerialName(OpNames.CONTEXT_CREATE)
data class CreateContextOp(
    val contextLocalId: Long,
    val body: CreateContextRequest,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.CONTEXT_CREATE
}

@Serializable
@SerialName(OpNames.CONTEXT_PATCH)
data class PatchContextOp(
    val contextLocalId: Long,
    val body: PatchContextRequest,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.CONTEXT_PATCH
}

@Serializable
@SerialName(OpNames.CONTEXT_DELETE)
data class DeleteContextOp(
    val contextLocalId: Long,
    val serverId: Long? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.CONTEXT_DELETE
}

@Serializable
@SerialName(OpNames.LABEL_CREATE)
data class CreateLabelOp(
    val labelLocalId: Long,
    val body: CreateLabelRequest,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.LABEL_CREATE
}

@Serializable
@SerialName(OpNames.LABEL_PATCH)
data class PatchLabelOp(
    val labelLocalId: Long,
    val body: PatchLabelRequest,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.LABEL_PATCH
}

@Serializable
@SerialName(OpNames.LABEL_DELETE)
data class DeleteLabelOp(
    val labelLocalId: Long,
    val serverId: Long? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.LABEL_DELETE
}

// --- templates --------------------------------------------------------------

@Serializable
@SerialName(OpNames.TEMPLATE_CREATE)
data class CreateTemplateOp(
    val templateLocalId: Long,
    val template: TemplatePayload,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TEMPLATE_CREATE
}

/**
 * Editing a template, which is a full replace rather than a patch: the editor
 * always submits the whole structure, and a partial update of a nested subtask
 * list has no meaning anyone could agree on.
 */
@Serializable
@SerialName(OpNames.TEMPLATE_REPLACE)
data class ReplaceTemplateOp(
    val templateLocalId: Long,
    val template: TemplatePayload,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TEMPLATE_REPLACE
}

@Serializable
@SerialName(OpNames.TEMPLATE_DELETE)
data class DeleteTemplateOp(
    val templateLocalId: Long,
    val serverId: Long? = null,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TEMPLATE_DELETE
}

/**
 * Turning a template into real tasks.
 *
 * The tree is written into the replica as the template describes it, so it is on
 * screen the moment the user asks for it and survives a relaunch with no network
 * at all. [rootTaskLocalId] and [subtaskLocalIds] are those rows, the subtasks in
 * the template's own order, which is how the answer's tasks are matched to them:
 * the server creates the root first and then each subtask in the same order, so
 * position for position is the only correspondence there can be.
 *
 * The rows are absent from a write queued by a build that left the whole
 * expansion to the server. Such a write still sends; its tasks simply arrive
 * with the next catch-up instead of being named here.
 */
@Serializable
@SerialName(OpNames.TEMPLATE_INSTANTIATE)
data class InstantiateTemplateOp(
    val templateLocalId: Long,
    val projectLocalId: Long,
    val rootTaskLocalId: Long? = null,
    val subtaskLocalIds: List<Long> = emptyList(),
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TEMPLATE_INSTANTIATE
}

// --- preferences, interface state and the daily plan ------------------------

/**
 * An edit to the user's own preferences. The label ids inside are server ids: the
 * document is the server's and travels back unchanged.
 */
@Serializable
@SerialName(OpNames.SETTINGS_PATCH)
data class PatchUserSettingsOp(val body: PatchUserSettingsRequest) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.SETTINGS_PATCH
}

/**
 * The installation's automatic labelling rules, replaced wholesale — the endpoint
 * takes the whole list, so a queued write is the whole list.
 */
@Serializable
@SerialName(OpNames.APP_SETTINGS_AUTO_LABELS)
data class PutAutoLabelsOp(val rules: List<AutoLabelRulePayload>) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.APP_SETTINGS_AUTO_LABELS
}

@Serializable
@SerialName(OpNames.APP_SETTINGS_PROJECT_SUGGESTIONS)
data class PutProjectSuggestionsOp(val rules: List<ProjectSuggestionRulePayload>) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.APP_SETTINGS_PROJECT_SUGGESTIONS
}

/** A rule's ids are server ids, for the same reason the preference document's are. */
@Serializable
data class AutoLabelRulePayload(
    val mask: String,
    val labelIds: List<Long> = emptyList(),
    val ignoreCase: Boolean = false,
)

@Serializable
data class ProjectSuggestionRulePayload(
    val mask: String,
    val projectIds: List<Long> = emptyList(),
    val ignoreCase: Boolean = false,
)

/**
 * A change to the interface state the server keeps for the user.
 *
 * The server merges this key by key, so the op carries only the keys that
 * changed and a key this build knows nothing about survives the write. [remove]
 * names the keys to drop, which the wire spells as an explicit null.
 */
@Serializable
@SerialName(OpNames.STATE_PATCH)
data class PatchUserStateOp(
    val activeContextId: Long? = null,
    val remove: List<String> = emptyList(),
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.STATE_PATCH
}

@Serializable
@SerialName(OpNames.TROIKI_START)
data object StartTroikiOp : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TROIKI_START
}

@Serializable
@SerialName(OpNames.TROIKI_RESET)
data object ResetTroikiOp : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.TROIKI_RESET
}

@Serializable
@SerialName(OpNames.HARPOON_ATTACH)
data class AttachHarpoonOp(
    val target: HarpoonTarget,
    val localId: Long,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.HARPOON_ATTACH
}

@Serializable
@SerialName(OpNames.HARPOON_DETACH)
data class DetachHarpoonOp(
    val target: HarpoonTarget,
    val localId: Long,
) : OutboxOp {
    override val kind: OutboxOpKind get() = OutboxOpKind.HARPOON_DETACH
}

// --- the endpoint mapping ---------------------------------------------------

private const val TASKS = "api/v1/tasks"
private const val PROJECTS = "api/v1/projects"
private const val SECTIONS = "api/v1/sections"
private const val CONTEXTS = "api/v1/contexts"
private const val LABELS = "api/v1/labels"
private const val TEMPLATES = "api/v1/task-templates"

private fun taskRef(localId: Long) = OpRef(ReplicaEntityKind.TASK, localId)

private fun projectRef(localId: Long) = OpRef(ReplicaEntityKind.PROJECT, localId)

private fun sectionRef(localId: Long) = OpRef(ReplicaEntityKind.SECTION, localId)

private fun contextRef(localId: Long) = OpRef(ReplicaEntityKind.CONTEXT, localId)

private fun labelRef(localId: Long) = OpRef(ReplicaEntityKind.LABEL, localId)

private fun templateRef(localId: Long) = OpRef(ReplicaEntityKind.TASK_TEMPLATE, localId)

private fun relationRef(localId: Long) = OpRef(ReplicaEntityKind.TASK_RELATION, localId)

/**
 * Which request an op becomes.
 *
 * The mapping lives here rather than inside each op so that the compiler proves
 * it is total: an op added without an endpoint fails to build instead of failing
 * to send, months later, on somebody's phone.
 */
fun OutboxOp.endpoint(): OpEndpoint =
    when (this) {
        is CreateTaskOp -> createEndpoint(destination)
        is PatchTaskOp -> OpEndpoint(OpMethod.PATCH, "$TASKS/{id}", taskRef(taskLocalId))
        is DeleteTaskOp -> OpEndpoint(OpMethod.DELETE, "$TASKS/{id}", taskRef(taskLocalId))
        is CompleteTaskOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/complete", taskRef(taskLocalId))
        is UncompleteTaskOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/uncomplete", taskRef(taskLocalId))
        is CancelTaskOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/cancel", taskRef(taskLocalId))
        is MoveTaskOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/move", taskRef(taskLocalId))
        is PlanTaskOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/plan", taskRef(taskLocalId))
        is PinTaskOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/pin", taskRef(taskLocalId))
        is UnpinTaskOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/unpin", taskRef(taskLocalId))
        is DuplicateTaskOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/duplicate", taskRef(taskLocalId))
        is DecomposeTaskOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/decompose", taskRef(taskLocalId))
        is AddTaskRelationOp -> OpEndpoint(OpMethod.POST, "$TASKS/{id}/relations", taskRef(taskLocalId))
        is RemoveTaskRelationOp ->
            OpEndpoint(
                OpMethod.DELETE,
                "$TASKS/{id}/relations/{relationId}",
                taskRef(taskLocalId),
                relationRef(relationLocalId),
            )
        is BulkCompleteTasksOp -> OpEndpoint(OpMethod.POST, "$TASKS/bulk/complete")
        is BulkMoveTasksOp -> OpEndpoint(OpMethod.POST, "$TASKS/bulk/move")
        is BulkTaskPriorityOp -> OpEndpoint(OpMethod.POST, "$TASKS/bulk/priority")
        is GroupTasksOp -> OpEndpoint(OpMethod.POST, "$TASKS/group")
        is CreateProjectOp -> OpEndpoint(OpMethod.POST, "$CONTEXTS/{id}/projects", contextRef(contextLocalId))
        is PatchProjectOp -> OpEndpoint(OpMethod.PATCH, "$PROJECTS/{id}", projectRef(projectLocalId))
        is DeleteProjectOp -> OpEndpoint(OpMethod.DELETE, "$PROJECTS/{id}", projectRef(projectLocalId))
        is ProjectStatusOp ->
            OpEndpoint(OpMethod.POST, "$PROJECTS/{id}/${action.segment}", projectRef(projectLocalId))
        is PinProjectOp -> OpEndpoint(OpMethod.POST, "$PROJECTS/{id}/pin", projectRef(projectLocalId))
        is UnpinProjectOp -> OpEndpoint(OpMethod.POST, "$PROJECTS/{id}/unpin", projectRef(projectLocalId))
        is SetProjectTroikiOp -> OpEndpoint(OpMethod.POST, "$PROJECTS/{id}/troiki", projectRef(projectLocalId))
        is CreateSectionOp -> OpEndpoint(OpMethod.POST, "$PROJECTS/{id}/sections", projectRef(projectLocalId))
        is PatchSectionOp -> OpEndpoint(OpMethod.PATCH, "$SECTIONS/{id}", sectionRef(sectionLocalId))
        is DeleteSectionOp -> OpEndpoint(OpMethod.DELETE, "$SECTIONS/{id}", sectionRef(sectionLocalId))
        is ReorderSectionOp -> OpEndpoint(OpMethod.POST, "$SECTIONS/{id}/reorder", sectionRef(sectionLocalId))
        is CreateContextOp -> OpEndpoint(OpMethod.POST, CONTEXTS)
        is PatchContextOp -> OpEndpoint(OpMethod.PATCH, "$CONTEXTS/{id}", contextRef(contextLocalId))
        is DeleteContextOp -> OpEndpoint(OpMethod.DELETE, "$CONTEXTS/{id}", contextRef(contextLocalId))
        is CreateLabelOp -> OpEndpoint(OpMethod.POST, LABELS)
        is PatchLabelOp -> OpEndpoint(OpMethod.PATCH, "$LABELS/{id}", labelRef(labelLocalId))
        is DeleteLabelOp -> OpEndpoint(OpMethod.DELETE, "$LABELS/{id}", labelRef(labelLocalId))
        is CreateTemplateOp -> OpEndpoint(OpMethod.POST, TEMPLATES)
        is ReplaceTemplateOp -> OpEndpoint(OpMethod.PATCH, "$TEMPLATES/{id}", templateRef(templateLocalId))
        is DeleteTemplateOp -> OpEndpoint(OpMethod.DELETE, "$TEMPLATES/{id}", templateRef(templateLocalId))
        is InstantiateTemplateOp ->
            OpEndpoint(OpMethod.POST, "$TEMPLATES/{id}/instantiate", templateRef(templateLocalId))
        is PatchUserSettingsOp -> OpEndpoint(OpMethod.PATCH, "api/v1/settings")
        is PutAutoLabelsOp -> OpEndpoint(OpMethod.PUT, "api/v1/app-settings/auto-labels")
        is PutProjectSuggestionsOp -> OpEndpoint(OpMethod.PUT, "api/v1/app-settings/project-suggestions")
        is PatchUserStateOp -> OpEndpoint(OpMethod.PATCH, "api/v1/state")
        StartTroikiOp -> OpEndpoint(OpMethod.POST, "api/v1/troiki/start")
        ResetTroikiOp -> OpEndpoint(OpMethod.POST, "api/v1/troiki/reset")
        is AttachHarpoonOp -> OpEndpoint(OpMethod.POST, "api/v1/harpoon/attach")
        is DetachHarpoonOp -> OpEndpoint(OpMethod.POST, "api/v1/harpoon/detach")
    }

/**
 * A task is created under whatever it belongs to, so the destination picks the
 * endpoint rather than being a field in the body.
 */
private fun createEndpoint(destination: TaskDestination): OpEndpoint =
    when (destination) {
        TaskDestination.Inbox -> OpEndpoint(OpMethod.POST, "api/v1/inbox/tasks")
        is TaskDestination.InContext ->
            OpEndpoint(OpMethod.POST, "$CONTEXTS/{id}/tasks", contextRef(destination.contextLocalId))
        is TaskDestination.InProject ->
            OpEndpoint(OpMethod.POST, "$PROJECTS/{id}/tasks", projectRef(destination.projectLocalId))
        is TaskDestination.InSection ->
            OpEndpoint(OpMethod.POST, "$SECTIONS/{id}/tasks", sectionRef(destination.sectionLocalId))
        is TaskDestination.SubtaskOf ->
            OpEndpoint(OpMethod.POST, "$TASKS/{id}/subtasks", taskRef(destination.parentTaskLocalId))
    }

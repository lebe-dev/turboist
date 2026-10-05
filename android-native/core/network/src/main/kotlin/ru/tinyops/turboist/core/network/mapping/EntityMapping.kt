package ru.tinyops.turboist.core.network.mapping

import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectSection
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.RelationDirection
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskRelation
import ru.tinyops.turboist.core.model.TaskRelationSummary
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TaskTemplate
import ru.tinyops.turboist.core.model.TaskTemplateSubtask
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.dto.ContextDto
import ru.tinyops.turboist.core.network.dto.LabelDto
import ru.tinyops.turboist.core.network.dto.ProjectDto
import ru.tinyops.turboist.core.network.dto.SectionDto
import ru.tinyops.turboist.core.network.dto.TaskDto
import ru.tinyops.turboist.core.network.dto.TaskRelationDto
import ru.tinyops.turboist.core.network.dto.TaskRelationEdgeDto
import ru.tinyops.turboist.core.network.dto.TaskTemplateDto
import ru.tinyops.turboist.core.network.dto.TaskTemplateSubtaskDto

/**
 * Turns wire payloads into domain objects.
 *
 * Three conversions happen here and nowhere else, which is the reason this file
 * exists at all:
 *
 * - **Timestamps** become epoch milliseconds. A missing or unreadable one becomes
 *   `0`, never an exception: losing the moment a row was created is a smaller
 *   failure than dropping the row.
 * - **Enums** decode tolerantly. A value this build has never seen becomes the
 *   unknown sentinel, so a task carrying a priority added after this build shipped
 *   is still readable, completable and syncable.
 * - **Ids** are resolved through [ReplicaIds], because every reference between
 *   replicated rows is local.
 */
private fun time(raw: String?): Long = WireTime.parseOrNull(raw) ?: 0L

fun LabelDto.toLabel(ids: ReplicaIds = ReplicaIds.Identity): Label =
    Label(
        localId = ids.label(id) ?: NO_LOCAL_ID,
        serverId = id,
        name = name,
        color = color,
        isFavourite = isFavourite,
        isPrivate = isPrivate,
        createdAt = time(createdAt),
        updatedAt = time(updatedAt),
    )

fun ContextDto.toContext(ids: ReplicaIds = ReplicaIds.Identity): Context =
    Context(
        localId = ids.context(id) ?: NO_LOCAL_ID,
        serverId = id,
        name = name,
        color = color,
        isFavourite = isFavourite,
        createdAt = time(createdAt),
        updatedAt = time(updatedAt),
    )

fun ProjectDto.toProject(ids: ReplicaIds = ReplicaIds.Identity): Project =
    Project(
        localId = ids.project(id) ?: NO_LOCAL_ID,
        serverId = id,
        contextLocalId = ids.context(contextId) ?: NO_LOCAL_ID,
        title = title,
        description = description,
        color = color,
        status = ProjectStatus.fromWire(status),
        type = ProjectType.fromWire(projectType),
        isPinned = isPinned,
        pinnedAt = WireTime.parseOrNull(pinnedAt),
        isPrivate = isPrivate,
        troikiCategory = troikiCategory?.let(TroikiCategory::fromWireOrNull),
        labels = labels.map { it.toLabel(ids) },
        createdAt = time(createdAt),
        updatedAt = time(updatedAt),
    )

fun SectionDto.toSection(ids: ReplicaIds = ReplicaIds.Identity): ProjectSection =
    ProjectSection(
        localId = ids.section(id) ?: NO_LOCAL_ID,
        serverId = id,
        projectLocalId = ids.project(projectId) ?: NO_LOCAL_ID,
        title = title,
        position = position,
        createdAt = time(createdAt),
        updatedAt = time(updatedAt),
    )

/**
 * A task.
 *
 * The troiki category is deliberately left unset: the task payloads of the API do
 * not carry it — it arrives with the plan view — so filling it in from here would
 * mean overwriting a known value with a guess on every pull.
 *
 * @param hydrateRelations whether to build the relation edges out of the payload.
 *   List payloads never carry them, and an empty list there means "not loaded",
 *   not "has none" — so a caller that is refreshing a row it already holds must
 *   not let a list payload wipe the relations it knows about.
 */
fun TaskDto.toTask(
    ids: ReplicaIds = ReplicaIds.Identity,
    hydrateRelations: Boolean = relations.isNotEmpty(),
): Task {
    val localId = ids.task(id) ?: NO_LOCAL_ID
    return Task(
        localId = localId,
        serverId = id,
        title = title,
        description = description,
        inboxId = inboxId,
        contextLocalId = contextId?.let(ids::context),
        projectLocalId = projectId?.let(ids::project),
        sectionLocalId = sectionId?.let(ids::section),
        parentLocalId = parentId?.let(ids::task),
        priority = Priority.fromWire(priority),
        status = TaskStatus.fromWire(status),
        dueAt = WireTime.parseOrNull(dueAt),
        dueHasTime = dueHasTime,
        deadlineAt = WireTime.parseOrNull(deadlineAt),
        deadlineHasTime = deadlineHasTime,
        dayPart = DayPart.fromWire(dayPart),
        planState = PlanState.fromWire(planState),
        isPinned = isPinned,
        pinnedAt = WireTime.parseOrNull(pinnedAt),
        isPrivate = isPrivate,
        isComplex = isComplex,
        completedAt = WireTime.parseOrNull(completedAt),
        recurrenceRule = recurrenceRule,
        sourceTaskLocalId = sourceTaskId?.let(ids::task),
        postponeCount = postponeCount,
        labels = labels.map { it.toLabel(ids) },
        relationSummary = TaskRelationSummary(blockedByOpen = blockedByCount, total = relationCount),
        relations = if (hydrateRelations) relations.map { it.toRelation(localId, ids) } else emptyList(),
        createdAt = time(createdAt),
        updatedAt = time(updatedAt),
    )
}

/**
 * One relation as the task it was read for sees it.
 *
 * The payload names the peer and which way the edge runs from here, so the stored
 * row's two endpoints are recovered by putting the owner on the blocking side for
 * an outgoing edge and on the blocked side for an incoming one. A symmetric
 * relation has no direction to recover, and none is invented.
 */
fun TaskRelationDto.toRelation(
    ownerLocalId: Long,
    ids: ReplicaIds = ReplicaIds.Identity,
): TaskRelation {
    val peer = task.toTask(ids, hydrateRelations = false)
    val heading = RelationDirection.fromWireOrNull(direction)
    val ownerBlocks = heading == RelationDirection.OUTGOING
    return TaskRelation(
        localId = NO_LOCAL_ID,
        serverId = id,
        sourceTaskLocalId = if (ownerBlocks) ownerLocalId else peer.localId,
        targetTaskLocalId = if (ownerBlocks) peer.localId else ownerLocalId,
        type = RelationType.fromWire(type),
        direction = heading,
        createdAt = time(createdAt),
        other = peer,
    )
}

/** The stored edge itself, which is the form the whole-graph payloads carry. */
fun TaskRelationEdgeDto.toRelation(ids: ReplicaIds = ReplicaIds.Identity): TaskRelation =
    TaskRelation(
        localId = NO_LOCAL_ID,
        serverId = id,
        sourceTaskLocalId = ids.task(sourceTaskId) ?: NO_LOCAL_ID,
        targetTaskLocalId = ids.task(targetTaskId) ?: NO_LOCAL_ID,
        type = RelationType.fromWire(type),
        direction = null,
        createdAt = time(createdAt),
    )

fun TaskTemplateDto.toTemplate(ids: ReplicaIds = ReplicaIds.Identity): TaskTemplate {
    val localId = ids.template(id) ?: NO_LOCAL_ID
    val created = time(createdAt)
    val updated = time(updatedAt)
    return TaskTemplate(
        localId = localId,
        serverId = id,
        name = name,
        description = description,
        priority = Priority.fromWire(priority),
        dayPart = DayPart.fromWire(dayPart),
        position = position,
        labels = labels.map { it.toLabel(ids) },
        subtasks = subtasks.mapIndexed { index, subtask -> subtask.toSubtask(localId, index, created, updated, ids) },
        createdAt = created,
        updatedAt = updated,
    )
}

/**
 * A subtask of a template.
 *
 * Order is positional on the wire — the payload is a list, not a set of numbered
 * rows — so the caller passes the index it was found at. Timestamps are absent
 * for the same reason of economy, so the subtask takes its template's: an honest
 * stand-in, and better than a zero that would read as "created at the epoch".
 */
fun TaskTemplateSubtaskDto.toSubtask(
    templateLocalId: Long,
    position: Int,
    createdAt: Long,
    updatedAt: Long,
    ids: ReplicaIds = ReplicaIds.Identity,
): TaskTemplateSubtask =
    TaskTemplateSubtask(
        localId = NO_LOCAL_ID,
        serverId = id,
        templateLocalId = templateLocalId,
        position = position,
        title = title,
        description = description,
        priority = Priority.fromWire(priority),
        dayPart = DayPart.fromWire(dayPart),
        labels = labels.map { it.toLabel(ids) },
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

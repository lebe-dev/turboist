package ru.tinyops.turboist.core.database.entity

import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectSection
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskRelation
import ru.tinyops.turboist.core.model.TaskTemplate
import ru.tinyops.turboist.core.model.TaskTemplateSubtask

/**
 * The translation between a stored row and the type the rest of the app works
 * in.
 *
 * It lives here, once, so that a screen and the code applying a change cannot
 * end up with two different ideas of what a row means.
 *
 * A domain object carries things a single row does not: the labels on a task,
 * the edges of the task graph, the rollup that says a task is blocked. Those are
 * gathered by whoever is answering the question — a list needs the rollup but
 * not the edges, a task page needs both — so `toDomain` leaves them empty and
 * the caller fills in what it loaded with `copy`. Going the other way they are
 * simply dropped: they live in their own tables and are written through them.
 */
fun ContextRow.toDomain(): Context =
    Context(
        localId = localId,
        serverId = serverId,
        name = name,
        color = color,
        isFavourite = isFavourite,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun Context.toRow(): ContextRow =
    ContextRow(
        localId = localId,
        serverId = serverId,
        name = name,
        color = color,
        isFavourite = isFavourite,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun LabelRow.toDomain(): Label =
    Label(
        localId = localId,
        serverId = serverId,
        name = name,
        color = color,
        isFavourite = isFavourite,
        isPrivate = isPrivate,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun Label.toRow(): LabelRow =
    LabelRow(
        localId = localId,
        serverId = serverId,
        name = name,
        color = color,
        isFavourite = isFavourite,
        isPrivate = isPrivate,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun ProjectRow.toDomain(): Project =
    Project(
        localId = localId,
        serverId = serverId,
        contextLocalId = contextLocalId,
        title = title,
        description = description,
        color = color,
        status = status,
        type = type,
        isPinned = isPinned,
        pinnedAt = pinnedAt,
        isPrivate = isPrivate,
        troikiCategory = troikiCategory,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun Project.toRow(): ProjectRow =
    ProjectRow(
        localId = localId,
        serverId = serverId,
        contextLocalId = contextLocalId,
        title = title,
        description = description,
        color = color,
        status = status,
        type = type,
        isPinned = isPinned,
        pinnedAt = pinnedAt,
        isPrivate = isPrivate,
        troikiCategory = troikiCategory,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun ProjectSectionRow.toDomain(): ProjectSection =
    ProjectSection(
        localId = localId,
        serverId = serverId,
        projectLocalId = projectLocalId,
        title = title,
        position = position,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun ProjectSection.toRow(): ProjectSectionRow =
    ProjectSectionRow(
        localId = localId,
        serverId = serverId,
        projectLocalId = projectLocalId,
        title = title,
        position = position,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun TaskRow.toDomain(): Task =
    Task(
        localId = localId,
        serverId = serverId,
        title = title,
        description = description,
        inboxId = inboxId,
        contextLocalId = contextLocalId,
        projectLocalId = projectLocalId,
        sectionLocalId = sectionLocalId,
        parentLocalId = parentLocalId,
        priority = priority,
        status = status,
        dueAt = dueAt,
        dueHasTime = dueHasTime,
        deadlineAt = deadlineAt,
        deadlineHasTime = deadlineHasTime,
        dayPart = dayPart,
        planState = planState,
        isPinned = isPinned,
        pinnedAt = pinnedAt,
        isPrivate = isPrivate,
        isComplex = isComplex,
        completedAt = completedAt,
        recurrenceRule = recurrenceRule,
        sourceTaskLocalId = sourceTaskLocalId,
        postponeCount = postponeCount,
        troikiCategory = troikiCategory,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun Task.toRow(): TaskRow =
    TaskRow(
        localId = localId,
        serverId = serverId,
        title = title,
        description = description,
        inboxId = inboxId,
        contextLocalId = contextLocalId,
        projectLocalId = projectLocalId,
        sectionLocalId = sectionLocalId,
        parentLocalId = parentLocalId,
        priority = priority,
        status = status,
        dueAt = dueAt,
        dueHasTime = dueHasTime,
        deadlineAt = deadlineAt,
        deadlineHasTime = deadlineHasTime,
        dayPart = dayPart,
        planState = planState,
        isPinned = isPinned,
        pinnedAt = pinnedAt,
        isPrivate = isPrivate,
        isComplex = isComplex,
        completedAt = completedAt,
        recurrenceRule = recurrenceRule,
        sourceTaskLocalId = sourceTaskLocalId,
        postponeCount = postponeCount,
        troikiCategory = troikiCategory,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun TaskRelationRow.toDomain(): TaskRelation =
    TaskRelation(
        localId = localId,
        serverId = serverId,
        sourceTaskLocalId = sourceTaskLocalId,
        targetTaskLocalId = targetTaskLocalId,
        type = type,
        createdAt = createdAt,
    )

fun TaskRelation.toRow(): TaskRelationRow =
    TaskRelationRow(
        localId = localId,
        serverId = serverId,
        sourceTaskLocalId = sourceTaskLocalId,
        targetTaskLocalId = targetTaskLocalId,
        type = type,
        createdAt = createdAt,
    )

fun TaskTemplateRow.toDomain(): TaskTemplate =
    TaskTemplate(
        localId = localId,
        serverId = serverId,
        name = name,
        description = description,
        priority = priority,
        dayPart = dayPart,
        position = position,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun TaskTemplate.toRow(): TaskTemplateRow =
    TaskTemplateRow(
        localId = localId,
        serverId = serverId,
        name = name,
        description = description,
        priority = priority,
        dayPart = dayPart,
        position = position,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun TaskTemplateSubtaskRow.toDomain(): TaskTemplateSubtask =
    TaskTemplateSubtask(
        localId = localId,
        serverId = serverId,
        templateLocalId = templateLocalId,
        position = position,
        title = title,
        description = description,
        priority = priority,
        dayPart = dayPart,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

fun TaskTemplateSubtask.toRow(): TaskTemplateSubtaskRow =
    TaskTemplateSubtaskRow(
        localId = localId,
        serverId = serverId,
        templateLocalId = templateLocalId,
        position = position,
        title = title,
        description = description,
        priority = priority,
        dayPart = dayPart,
        createdAt = createdAt,
        updatedAt = updatedAt,
    )

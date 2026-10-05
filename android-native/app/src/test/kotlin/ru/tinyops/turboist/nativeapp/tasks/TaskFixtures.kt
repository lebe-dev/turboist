package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskRelationSummary
import ru.tinyops.turboist.core.model.TaskStatus

/**
 * A task with nothing set but the two things every task has.
 *
 * The tests below are about one property at a time, so everything else is left
 * at its default: a fixture that spells out fifteen columns hides which one the
 * assertion is actually about.
 */
fun task(
    localId: Long,
    title: String = "task $localId",
    serverId: Long? = localId,
    description: String = "",
    parentLocalId: Long? = null,
    projectLocalId: Long? = null,
    sectionLocalId: Long? = null,
    inboxId: Long? = null,
    dayPart: DayPart = DayPart.NONE,
    planState: PlanState = PlanState.NONE,
    priority: Priority = Priority.NONE,
    status: TaskStatus = TaskStatus.OPEN,
    dueAt: Long? = null,
    dueHasTime: Boolean = false,
    deadlineAt: Long? = null,
    deadlineHasTime: Boolean = false,
    completedAt: Long? = null,
    labels: List<Label> = emptyList(),
    isPinned: Boolean = false,
    isPrivate: Boolean = false,
    isComplex: Boolean = false,
    postponeCount: Int = 0,
    recurrenceRule: String? = null,
    relationSummary: TaskRelationSummary = TaskRelationSummary.NONE,
): Task =
    Task(
        localId = localId,
        serverId = serverId,
        title = title,
        description = description,
        parentLocalId = parentLocalId,
        projectLocalId = projectLocalId,
        sectionLocalId = sectionLocalId,
        inboxId = inboxId,
        dayPart = dayPart,
        planState = planState,
        priority = priority,
        status = status,
        dueAt = dueAt,
        dueHasTime = dueHasTime,
        deadlineAt = deadlineAt,
        deadlineHasTime = deadlineHasTime,
        completedAt = completedAt,
        labels = labels,
        isPinned = isPinned,
        isPrivate = isPrivate,
        isComplex = isComplex,
        postponeCount = postponeCount,
        recurrenceRule = recurrenceRule,
        relationSummary = relationSummary,
        createdAt = 0,
        updatedAt = 0,
    )

package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateSubtaskLabelRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateSubtaskRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Priority

/** One line of a template's checklist. */
data class TemplateSubtaskDraft(
    val title: String,
    val description: String = "",
    val priority: Priority = Priority.NONE,
    val dayPart: DayPart = DayPart.NONE,
    val labelLocalIds: List<Long> = emptyList(),
)

/**
 * A template as the editor has it.
 *
 * Labels are named by the device's own ids rather than by the server's, so a
 * label created this evening can be used in a template written the same evening.
 * The ids are translated when the template is sent, by which time both exist on
 * the server.
 */
data class TemplateDraft(
    val name: String,
    val description: String = "",
    val priority: Priority = Priority.NONE,
    val dayPart: DayPart = DayPart.NONE,
    val labelLocalIds: List<Long> = emptyList(),
    val subtasks: List<TemplateSubtaskDraft> = emptyList(),
)

/**
 * Writes to the reusable task blueprints.
 *
 * Editing a template is a replace, not a patch: the editor always submits the
 * whole structure, and there is no agreed meaning for a partial update of a
 * nested list of subtasks.
 */
class TemplateWriteRepo(
    private val db: TurboistDatabase,
    private val writer: OutboxWriter,
    private val rules: ReplicaRules = ReplicaRules(db),
) {
    suspend fun create(draft: TemplateDraft): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val localId =
                db.taskTemplates().insert(
                    TaskTemplateRow(
                        name = draft.name,
                        description = draft.description,
                        priority = draft.priority,
                        dayPart = draft.dayPart,
                        position = db.taskTemplates().count(),
                        createdAt = at,
                        updatedAt = at,
                    ),
                )
            writeBody(localId, draft, at)
            val opId =
                writer.enqueue(
                    CreateTemplateOp(localId, draft.toPayload()),
                    ReplicaEntityKind.TASK_TEMPLATE,
                    localId,
                )
            QueuedWrite(opId, localId)
        }

    suspend fun replace(
        templateLocalId: Long,
        draft: TemplateDraft,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row =
                db.taskTemplates().byLocalId(templateLocalId)
                    ?: throw WriteRefused.RowMissing("template", templateLocalId)
            db.taskTemplates().update(
                row.copy(
                    name = draft.name,
                    description = draft.description,
                    priority = draft.priority,
                    dayPart = draft.dayPart,
                    updatedAt = at,
                ),
            )
            writeBody(templateLocalId, draft, at)
            val opId =
                writer.enqueue(
                    ReplaceTemplateOp(templateLocalId, draft.toPayload()),
                    ReplicaEntityKind.TASK_TEMPLATE,
                    templateLocalId,
                )
            QueuedWrite(opId, templateLocalId)
        }

    suspend fun delete(templateLocalId: Long): QueuedWrite =
        writer.transaction {
            val row =
                db.taskTemplates().byLocalId(templateLocalId)
                    ?: throw WriteRefused.RowMissing("template", templateLocalId)
            val opId =
                writer.enqueue(
                    DeleteTemplateOp(templateLocalId, row.serverId),
                    ReplicaEntityKind.TASK_TEMPLATE,
                    templateLocalId,
                )
            db.taskTemplates().delete(row)
            QueuedWrite(opId, templateLocalId)
        }

    /**
     * Turns a template into real tasks in a project.
     *
     * The tree is written here, not waited for: a template used on a train has to
     * produce work the user can see, reorder and tick off before any of it has
     * reached the server. The rows carry no server id until the queue drains, and
     * the queued write names them, so the answer's tasks land on the rows already
     * on screen instead of arriving as a second copy.
     *
     * What a task made from a template inherits is the template's own wording and
     * fields, the project it was dropped into (and the context above it), and the
     * labels the template names — plus whatever the installation's rules attach
     * to the title, exactly as writing the same task by hand would. Subtasks are
     * created under the root in the template's order, which is also the order the
     * server creates them in and therefore the only way the two sets can be
     * matched up afterwards.
     *
     * @return the queued write, naming the root task the user is now looking at.
     */
    suspend fun instantiate(
        templateLocalId: Long,
        projectLocalId: Long,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val template =
                db.taskTemplates().byLocalId(templateLocalId)
                    ?: throw WriteRefused.RowMissing("template", templateLocalId)
            val project =
                db.projects().byLocalId(projectLocalId)
                    ?: throw WriteRefused.RowMissing("project", projectLocalId)
            val rootLocalId =
                db.tasks().insert(
                    TaskRow(
                        title = template.name,
                        description = template.description,
                        contextLocalId = project.contextLocalId,
                        projectLocalId = project.localId,
                        priority = template.priority,
                        dayPart = template.dayPart,
                        createdAt = at,
                        updatedAt = at,
                    ),
                )
            applyLabels(rootLocalId, template.name, db.taskTemplates().labelLocalIds(templateLocalId), at)
            val subtaskLocalIds =
                db.taskTemplates().subtasksOf(templateLocalId).map { line ->
                    val localId =
                        db.tasks().insert(
                            TaskRow(
                                title = line.title,
                                description = line.description,
                                contextLocalId = project.contextLocalId,
                                projectLocalId = project.localId,
                                parentLocalId = rootLocalId,
                                priority = line.priority,
                                dayPart = line.dayPart,
                                createdAt = at,
                                updatedAt = at,
                            ),
                        )
                    applyLabels(localId, line.title, db.taskTemplates().subtaskLabelLocalIds(line.localId), at)
                    localId
                }
            val opId =
                writer.enqueue(
                    InstantiateTemplateOp(templateLocalId, projectLocalId, rootLocalId, subtaskLocalIds),
                    ReplicaEntityKind.TASK,
                    rootLocalId,
                )
            QueuedWrite(opId, rootLocalId)
        }

    /**
     * Gives a task made from a template the labels the template names, and the
     * ones the installation's rules attach to its title.
     *
     * The template's labels are turned back into names on the way, because that
     * is what the create endpoint reads and therefore what the rules are applied
     * against: a label the template names but this device no longer has drops out
     * on both sides rather than only on one. The list is empty rather than absent
     * when a template names none — an absent list means "inherit", which a task
     * made from a template never does.
     */
    private suspend fun applyLabels(
        taskLocalId: Long,
        title: String,
        labelLocalIds: List<Long>,
        at: Long,
    ) {
        val names = labelLocalIds.mapNotNull { db.labels().byLocalId(it)?.name }
        db.tasks().setLabels(taskLocalId, rules.labelsForTitle(title, names, emptyList()), at)
    }

    /** Rewrites a template's labels and checklist, which a replace always does whole. */
    private suspend fun writeBody(
        templateLocalId: Long,
        draft: TemplateDraft,
        at: Long,
    ) {
        db.taskTemplates().setLabels(templateLocalId, draft.labelLocalIds)
        db.taskTemplates().clearSubtasks(templateLocalId)
        draft.subtasks.forEachIndexed { index, subtask ->
            val subtaskLocalId =
                db.taskTemplates().insertSubtask(
                    TaskTemplateSubtaskRow(
                        templateLocalId = templateLocalId,
                        position = index,
                        title = subtask.title,
                        description = subtask.description,
                        priority = subtask.priority,
                        dayPart = subtask.dayPart,
                        createdAt = at,
                        updatedAt = at,
                    ),
                )
            db.taskTemplates().insertSubtaskLabels(
                subtask.labelLocalIds.distinct().map { TaskTemplateSubtaskLabelRow(subtaskLocalId, it) },
            )
        }
    }
}

private fun TemplateDraft.toPayload(): TemplatePayload =
    TemplatePayload(
        name = name,
        description = description.ifEmpty { null },
        priority = priority.wire.takeIf { priority != Priority.NONE },
        dayPart = dayPart.wire.takeIf { dayPart != DayPart.NONE },
        labelLocalIds = labelLocalIds,
        subtasks =
            subtasks.map {
                TemplateSubtaskPayload(
                    title = it.title,
                    description = it.description.ifEmpty { null },
                    priority = it.priority.wire.takeIf { _ -> it.priority != Priority.NONE },
                    dayPart = it.dayPart.wire.takeIf { _ -> it.dayPart != DayPart.NONE },
                    labelLocalIds = it.labelLocalIds,
                )
            },
    )

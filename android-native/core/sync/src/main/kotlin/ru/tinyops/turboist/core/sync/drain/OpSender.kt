package ru.tinyops.turboist.core.sync.drain

import android.util.Log
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import retrofit2.Response
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.TurboistNetwork
import ru.tinyops.turboist.core.network.api.ContextApi
import ru.tinyops.turboist.core.network.api.LabelApi
import ru.tinyops.turboist.core.network.api.ProjectApi
import ru.tinyops.turboist.core.network.api.SettingsApi
import ru.tinyops.turboist.core.network.api.TaskApi
import ru.tinyops.turboist.core.network.api.TemplateApi
import ru.tinyops.turboist.core.network.api.TroikiApi
import ru.tinyops.turboist.core.network.api.requireBody
import ru.tinyops.turboist.core.network.dto.AutoLabelRuleDto
import ru.tinyops.turboist.core.network.dto.AutoLabelsRequest
import ru.tinyops.turboist.core.network.dto.BulkIdsRequest
import ru.tinyops.turboist.core.network.dto.BulkMoveRequest
import ru.tinyops.turboist.core.network.dto.BulkPriorityRequest
import ru.tinyops.turboist.core.network.dto.BulkResultDto
import ru.tinyops.turboist.core.network.dto.CompleteTaskRequest
import ru.tinyops.turboist.core.network.dto.CreateTaskRelationRequest
import ru.tinyops.turboist.core.network.dto.DecomposeTaskRequest
import ru.tinyops.turboist.core.network.dto.GroupTasksRequest
import ru.tinyops.turboist.core.network.dto.HarpoonRefRequest
import ru.tinyops.turboist.core.network.dto.InstantiateTemplateRequest
import ru.tinyops.turboist.core.network.dto.MoveTaskRequest
import ru.tinyops.turboist.core.network.dto.PatchProjectRequest
import ru.tinyops.turboist.core.network.dto.PlanTaskRequest
import ru.tinyops.turboist.core.network.dto.ProjectSuggestionRuleDto
import ru.tinyops.turboist.core.network.dto.ProjectSuggestionsRequest
import ru.tinyops.turboist.core.network.dto.ReorderSectionRequest
import ru.tinyops.turboist.core.network.dto.SetTroikiCategoryRequest
import ru.tinyops.turboist.core.network.dto.TaskTemplateRequest
import ru.tinyops.turboist.core.network.dto.TemplateSubtaskRequest
import ru.tinyops.turboist.core.network.http.isIdempotentReplay
import ru.tinyops.turboist.core.sync.write.AddTaskRelationOp
import ru.tinyops.turboist.core.sync.write.AttachHarpoonOp
import ru.tinyops.turboist.core.sync.write.BulkCompleteTasksOp
import ru.tinyops.turboist.core.sync.write.BulkMoveTasksOp
import ru.tinyops.turboist.core.sync.write.BulkTaskPriorityOp
import ru.tinyops.turboist.core.sync.write.CancelTaskOp
import ru.tinyops.turboist.core.sync.write.CompleteTaskOp
import ru.tinyops.turboist.core.sync.write.CreateContextOp
import ru.tinyops.turboist.core.sync.write.CreateLabelOp
import ru.tinyops.turboist.core.sync.write.CreateProjectOp
import ru.tinyops.turboist.core.sync.write.CreateSectionOp
import ru.tinyops.turboist.core.sync.write.CreateTaskOp
import ru.tinyops.turboist.core.sync.write.CreateTemplateOp
import ru.tinyops.turboist.core.sync.write.DecomposeTaskOp
import ru.tinyops.turboist.core.sync.write.DeleteContextOp
import ru.tinyops.turboist.core.sync.write.DeleteLabelOp
import ru.tinyops.turboist.core.sync.write.DeleteProjectOp
import ru.tinyops.turboist.core.sync.write.DeleteSectionOp
import ru.tinyops.turboist.core.sync.write.DeleteTaskOp
import ru.tinyops.turboist.core.sync.write.DeleteTemplateOp
import ru.tinyops.turboist.core.sync.write.DetachHarpoonOp
import ru.tinyops.turboist.core.sync.write.DuplicateTaskOp
import ru.tinyops.turboist.core.sync.write.GroupTasksOp
import ru.tinyops.turboist.core.sync.write.HarpoonTarget
import ru.tinyops.turboist.core.sync.write.InstantiateTemplateOp
import ru.tinyops.turboist.core.sync.write.MoveTaskOp
import ru.tinyops.turboist.core.sync.write.OpRef
import ru.tinyops.turboist.core.sync.write.OutboxOp
import ru.tinyops.turboist.core.sync.write.PatchContextOp
import ru.tinyops.turboist.core.sync.write.PatchLabelOp
import ru.tinyops.turboist.core.sync.write.PatchProjectOp
import ru.tinyops.turboist.core.sync.write.PatchSectionOp
import ru.tinyops.turboist.core.sync.write.PatchTaskOp
import ru.tinyops.turboist.core.sync.write.PatchUserSettingsOp
import ru.tinyops.turboist.core.sync.write.PatchUserStateOp
import ru.tinyops.turboist.core.sync.write.PinProjectOp
import ru.tinyops.turboist.core.sync.write.PinTaskOp
import ru.tinyops.turboist.core.sync.write.PlanTaskOp
import ru.tinyops.turboist.core.sync.write.ProjectStatusAction
import ru.tinyops.turboist.core.sync.write.ProjectStatusOp
import ru.tinyops.turboist.core.sync.write.PutAutoLabelsOp
import ru.tinyops.turboist.core.sync.write.PutProjectSuggestionsOp
import ru.tinyops.turboist.core.sync.write.RemoveTaskRelationOp
import ru.tinyops.turboist.core.sync.write.ReorderSectionOp
import ru.tinyops.turboist.core.sync.write.ReplaceTemplateOp
import ru.tinyops.turboist.core.sync.write.ReplicaServerIds
import ru.tinyops.turboist.core.sync.write.ResetTroikiOp
import ru.tinyops.turboist.core.sync.write.SetProjectTroikiOp
import ru.tinyops.turboist.core.sync.write.StartTroikiOp
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TemplatePayload
import ru.tinyops.turboist.core.sync.write.UncompleteTaskOp
import ru.tinyops.turboist.core.sync.write.UnpinProjectOp
import ru.tinyops.turboist.core.sync.write.UnpinTaskOp

/**
 * The log tag every drain line carries.
 *
 * It is the same tag the pull and scheduling sides use, so one filter shows the
 * whole sync story in the order it happened instead of three interleaved streams.
 */
internal const val DRAIN_LOG_TAG = "TurboistSync"

/** The id the server gave a row that until now existed only on this device. */
data class ServerIdAssignment(
    val ref: OpRef,
    val serverId: Long,
)

/**
 * What one sent write left behind.
 *
 * @property replayed true when the server answered from its record of an earlier
 *   attempt instead of executing this one. It is a success either way — that is
 *   the whole point of sending a write under a key — and it is reported because
 *   it says the previous attempt did land, which is worth a log line when a
 *   connection has been unreliable.
 * @property assignments the rows this write brought into existence on the server.
 */
data class SendOutcome(
    val replayed: Boolean,
    val assignments: List<ServerIdAssignment> = emptyList(),
)

/**
 * Turns a queued write into the request it was always going to be.
 *
 * Two things happen here and nowhere else. The op picks its endpoint — the
 * mapping is the one written down in the op catalog, and this is where it is
 * actually walked — and every reference the op carries is translated from a
 * local id into the server's, at the last possible moment, which is also the
 * first moment at which the answer is certain to exist.
 *
 * Every request goes out under the op's own id as its `Idempotency-Key`. That
 * key was minted when the write was queued, so an attempt whose answer was lost
 * is safe to make again: the server recognises the repeat and replays what it
 * already decided rather than doing the work twice.
 *
 * The answers are read only for what the replica cannot work out on its own —
 * the server ids of rows this write created. Everything else the server decided
 * (a cascade, an advanced recurrence, a label a rule attached) is picked up by
 * the catch-up that follows the drain, so nothing here tries to mirror it.
 */
class OpSender(
    private val tasks: TaskApi,
    private val projects: ProjectApi,
    private val contexts: ContextApi,
    private val labels: LabelApi,
    private val templates: TemplateApi,
    private val settings: SettingsApi,
    private val troiki: TroikiApi,
    private val ids: ReplicaServerIds,
) {
    /** The stack as the app builds it, which is the shape every caller outside a test has. */
    constructor(network: TurboistNetwork, ids: ReplicaServerIds) : this(
        tasks = network.tasks,
        projects = network.projects,
        contexts = network.contexts,
        labels = network.labels,
        templates = network.templates,
        settings = network.settings,
        troiki = network.troiki,
        ids = ids,
    )

    /**
     * The ids of rows created earlier in this process, kept for the case the
     * replica cannot answer.
     *
     * A row created and then deleted while offline leaves the queue holding two
     * writes and the replica holding no row: the create is sent, and the delete
     * behind it would have nothing to name. Remembering what the create was
     * answered with lets the delete go out and the server end up as the user
     * left it, instead of keeping a record they threw away.
     */
    private val created = mutableMapOf<OpRef, Long>()

    /**
     * Sends one write and reports what came back.
     *
     * @param idempotencyKey the op's own id. It must be the same on every attempt
     *   at the same write; a fresh key would make the second attempt a second
     *   write.
     * @throws ru.tinyops.turboist.core.network.ApiException for every failure —
     *   the subclass says whether to wait, to give up, or to send the user to the
     *   sign-in screen.
     * @throws ru.tinyops.turboist.core.sync.write.UnsentReference when the write
     *   names a row the server has never been told about.
     */
    suspend fun send(
        op: OutboxOp,
        idempotencyKey: String,
    ): SendOutcome {
        val key = idempotencyKey
        return when (op) {
            is CreateTaskOp -> createTask(op, key)
            is PatchTaskOp -> tasks.patch(taskId(op.taskLocalId), op.patch.toRequest(), key).outcome()
            is DeleteTaskOp -> tasks.delete(op.serverId ?: taskId(op.taskLocalId), key).outcome()
            is CompleteTaskOp ->
                tasks.complete(taskId(op.taskLocalId), CompleteTaskRequest(op.completedAt), key).outcome()

            is UncompleteTaskOp -> tasks.uncomplete(taskId(op.taskLocalId), key).outcome()
            is CancelTaskOp -> tasks.cancel(taskId(op.taskLocalId), key).outcome()
            is MoveTaskOp -> tasks.move(taskId(op.taskLocalId), placement(op.destination), key).outcome()
            is PlanTaskOp -> tasks.plan(taskId(op.taskLocalId), PlanTaskRequest(op.state), key).outcome()
            is PinTaskOp -> tasks.pin(taskId(op.taskLocalId), key).outcome()
            is UnpinTaskOp -> tasks.unpin(taskId(op.taskLocalId), key).outcome()
            is DuplicateTaskOp -> tasks.duplicate(taskId(op.taskLocalId), key).outcome()
            is DecomposeTaskOp -> decompose(op, key)

            is AddTaskRelationOp -> addRelation(op, key)
            is RemoveTaskRelationOp ->
                tasks.removeRelation(
                    taskId(op.taskLocalId),
                    serverId(OpRef(ReplicaEntityKind.TASK_RELATION, op.relationLocalId)),
                    key,
                ).outcome()

            is BulkCompleteTasksOp -> tasks.bulkComplete(BulkIdsRequest(taskIds(op.taskLocalIds)), key).bulk(op)
            is BulkMoveTasksOp -> bulkMove(op, key)
            is BulkTaskPriorityOp ->
                tasks.bulkPriority(BulkPriorityRequest(taskIds(op.taskLocalIds), op.priority), key).bulk(op)

            is GroupTasksOp -> group(op, key)
            is CreateProjectOp -> createProject(op, key)
            is PatchProjectOp -> patchProject(op, key)
            is DeleteProjectOp -> projects.delete(op.serverId ?: projectId(op.projectLocalId), key).outcome()
            is ProjectStatusOp -> projectStatus(op, key)
            is PinProjectOp -> projects.pin(projectId(op.projectLocalId), key).outcome()
            is UnpinProjectOp -> projects.unpin(projectId(op.projectLocalId), key).outcome()
            is SetProjectTroikiOp ->
                projects.setTroikiCategory(
                    projectId(op.projectLocalId),
                    SetTroikiCategoryRequest(op.category),
                    key,
                ).outcome()

            is CreateSectionOp -> createSection(op, key)
            is PatchSectionOp -> projects.patchSection(sectionId(op.sectionLocalId), op.body, key).outcome()
            is DeleteSectionOp -> projects.deleteSection(op.serverId ?: sectionId(op.sectionLocalId), key).outcome()
            is ReorderSectionOp ->
                projects.reorderSection(
                    sectionId(op.sectionLocalId),
                    ReorderSectionRequest(op.position),
                    key,
                ).outcome()

            is CreateContextOp -> createContext(op, key)
            is PatchContextOp -> contexts.patch(contextId(op.contextLocalId), op.body, key).outcome()
            is DeleteContextOp -> contexts.delete(op.serverId ?: contextId(op.contextLocalId), key).outcome()
            is CreateLabelOp -> createLabel(op, key)
            is PatchLabelOp -> labels.patch(labelId(op.labelLocalId), op.body, key).outcome()
            is DeleteLabelOp -> labels.delete(op.serverId ?: labelId(op.labelLocalId), key).outcome()
            is CreateTemplateOp -> createTemplate(op, key)
            is ReplaceTemplateOp ->
                templates.replace(templateId(op.templateLocalId), templateBody(op.template), key).outcome()

            is DeleteTemplateOp -> templates.delete(op.serverId ?: templateId(op.templateLocalId), key).outcome()
            is InstantiateTemplateOp -> instantiate(op, key)

            is PatchUserSettingsOp -> settings.patchUserSettings(op.body, key).outcome()
            is PutAutoLabelsOp -> settings.putAutoLabels(AutoLabelsRequest(autoLabelRules(op)), key).outcome()
            is PutProjectSuggestionsOp ->
                settings.putProjectSuggestions(ProjectSuggestionsRequest(suggestionRules(op)), key).outcome()

            is PatchUserStateOp -> settings.patchState(userState(op), key).outcome()
            StartTroikiOp -> troiki.start(key).outcome()
            ResetTroikiOp -> troiki.reset(key).outcome()
            is AttachHarpoonOp -> settings.attachHarpoon(harpoonRef(op.target, op.localId), key).outcome()
            is DetachHarpoonOp -> settings.detachHarpoon(harpoonRef(op.target, op.localId), key).outcome()
        }
    }

    // --- the writes that bring a row into existence ---------------------------

    private suspend fun createTask(
        op: CreateTaskOp,
        key: String,
    ): SendOutcome {
        val answer =
            when (val destination = op.destination) {
                TaskDestination.Inbox -> tasks.createInInbox(op.body, key)
                is TaskDestination.InContext ->
                    tasks.createInContext(contextId(destination.contextLocalId), op.body, key)

                is TaskDestination.InProject ->
                    tasks.createInProject(projectId(destination.projectLocalId), op.body, key)

                is TaskDestination.InSection ->
                    tasks.createInSection(sectionId(destination.sectionLocalId), op.body, key)

                is TaskDestination.SubtaskOf ->
                    tasks.createSubtask(taskId(destination.parentTaskLocalId), op.body, key)
            }
        return answer.creating(ReplicaEntityKind.TASK, op.taskLocalId, answer.requireBody().id)
    }

    private suspend fun createProject(
        op: CreateProjectOp,
        key: String,
    ): SendOutcome {
        val answer = projects.create(contextId(op.contextLocalId), op.body, key)
        return answer.creating(ReplicaEntityKind.PROJECT, op.projectLocalId, answer.requireBody().id)
    }

    private suspend fun createSection(
        op: CreateSectionOp,
        key: String,
    ): SendOutcome {
        val answer = projects.createSection(projectId(op.projectLocalId), op.body, key)
        return answer.creating(ReplicaEntityKind.SECTION, op.sectionLocalId, answer.requireBody().id)
    }

    private suspend fun createContext(
        op: CreateContextOp,
        key: String,
    ): SendOutcome {
        val answer = contexts.create(op.body, key)
        return answer.creating(ReplicaEntityKind.CONTEXT, op.contextLocalId, answer.requireBody().id)
    }

    private suspend fun createLabel(
        op: CreateLabelOp,
        key: String,
    ): SendOutcome {
        val answer = labels.create(op.body, key)
        return answer.creating(ReplicaEntityKind.LABEL, op.labelLocalId, answer.requireBody().id)
    }

    private suspend fun createTemplate(
        op: CreateTemplateOp,
        key: String,
    ): SendOutcome {
        val answer = templates.create(templateBody(op.template), key)
        return answer.creating(ReplicaEntityKind.TASK_TEMPLATE, op.templateLocalId, answer.requireBody().id)
    }

    /**
     * Splits a task, and names the pieces this device already drew.
     *
     * The answer's tasks are in the order of the titles that were sent, which is
     * also the order the pieces were written in, so position for position is the
     * correspondence. A server that made a different number of them leaves the
     * odd ones nameless rather than guessing; the catch-up that follows the drain
     * carries the server's own copy either way.
     */
    private suspend fun decompose(
        op: DecomposeTaskOp,
        key: String,
    ): SendOutcome {
        val answer = tasks.decompose(taskId(op.taskLocalId), DecomposeTaskRequest(op.titles), key)
        return answer.creatingEach(op, op.createdTaskLocalIds, answer.requireBody().created.map { it.id })
    }

    /**
     * Turns a template into tasks, and names the tree this device already drew.
     *
     * The root comes first and the subtasks follow in the template's order, on
     * both sides, so the two sets line up position for position — there is no
     * other way to recognise a task the server named itself.
     */
    private suspend fun instantiate(
        op: InstantiateTemplateOp,
        key: String,
    ): SendOutcome {
        val answer =
            templates.instantiate(
                templateId(op.templateLocalId),
                InstantiateTemplateRequest(projectId(op.projectLocalId)),
                key,
            )
        val root = op.rootTaskLocalId ?: return answer.outcome()
        val body = answer.requireBody()
        return answer.creatingEach(
            op,
            listOf(root) + op.subtaskLocalIds,
            listOf(body.root.id) + body.subtasks.map { it.id },
        )
    }

    /**
     * Links two tasks, and finds the edge the server made in its answer.
     *
     * The answer is the task the link was added to, with its edges — so the new
     * one is the edge to the other task of this kind. If it cannot be picked out,
     * the local edge simply stays nameless: the next catch-up carries the
     * server's own copy, which is the authority anyway.
     */
    private suspend fun addRelation(
        op: AddTaskRelationOp,
        key: String,
    ): SendOutcome {
        val target = taskId(op.targetTaskLocalId)
        val answer =
            tasks.addRelation(
                taskId(op.taskLocalId),
                CreateTaskRelationRequest(target, op.type, op.direction),
                key,
            )
        val edge = answer.requireBody().relations.firstOrNull { it.task.id == target && it.type == op.type }
        if (edge == null) {
            Log.i(DRAIN_LOG_TAG, "The server did not name the new link; it will arrive with the next catch-up")
            return answer.outcome()
        }
        return answer.creating(ReplicaEntityKind.TASK_RELATION, op.relationLocalId, edge.id)
    }

    /**
     * Creates a parent and hangs the chosen tasks under it. The parent already
     * exists here as a row with no server id, so the answer's parent is what
     * names it.
     */
    private suspend fun group(
        op: GroupTasksOp,
        key: String,
    ): SendOutcome {
        val where = op.destination?.let { placement(it) }
        val answer =
            tasks.group(
                GroupTasksRequest(
                    title = op.title,
                    description = op.description,
                    priority = op.priority,
                    dayPart = op.dayPart,
                    planState = op.planState,
                    labels = op.labels.takeIf { it.isNotEmpty() },
                    projectId = where?.projectId,
                    sectionId = where?.sectionId,
                    contextId = where?.contextId,
                    childIds = taskIds(op.childTaskLocalIds),
                ),
                key,
            )
        val body = answer.requireBody()
        reportRefused(op, body.failed.map { it.id to it.error.code })
        return answer.creating(ReplicaEntityKind.TASK, op.parentTaskLocalId, body.parent.id)
    }

    // --- the rest ------------------------------------------------------------

    private suspend fun bulkMove(
        op: BulkMoveTasksOp,
        key: String,
    ): SendOutcome {
        val where = placement(op.destination)
        return tasks.bulkMove(
            BulkMoveRequest(
                ids = taskIds(op.taskLocalIds),
                inboxId = where.inboxId,
                contextId = where.contextId,
                projectId = where.projectId,
                sectionId = where.sectionId,
                parentId = where.parentId,
            ),
            key,
        ).bulk(op)
    }

    private suspend fun patchProject(
        op: PatchProjectOp,
        key: String,
    ): SendOutcome =
        projects.patch(
            projectId(op.projectLocalId),
            PatchProjectRequest(
                title = op.title,
                description = op.description,
                color = op.color,
                contextId = op.contextLocalId?.let { contextId(it) },
                labels = op.labels,
                isPrivate = op.isPrivate,
                projectType = op.projectType,
            ),
            key,
        ).outcome()

    private suspend fun projectStatus(
        op: ProjectStatusOp,
        key: String,
    ): SendOutcome {
        val id = projectId(op.projectLocalId)
        return when (op.action) {
            ProjectStatusAction.COMPLETE -> projects.complete(id, key)
            ProjectStatusAction.UNCOMPLETE -> projects.uncomplete(id, key)
            ProjectStatusAction.CANCEL -> projects.cancel(id, key)
            ProjectStatusAction.ARCHIVE -> projects.archive(id, key)
            ProjectStatusAction.UNARCHIVE -> projects.unarchive(id, key)
        }.outcome()
    }

    /**
     * Where a task is to sit, in the terms the server takes it in.
     *
     * The whole placement goes on the wire, not just the part the user picked:
     * the server keeps the pointers as a set that has to agree, and it refuses a
     * body that names a project without the context above it. Which rows those
     * are is the replica's answer; this only turns them into the server's ids.
     *
     * The one shape both the single and the bulk move endpoint read, and the
     * body a grouping carries too.
     */
    private suspend fun placement(destination: TaskDestination): MoveTaskRequest {
        val target = ids.placementOf(destination)
        return MoveTaskRequest(
            inboxId = target.inboxId,
            contextId = target.contextLocalId?.let { contextId(it) },
            projectId = target.projectLocalId?.let { projectId(it) },
            sectionId = target.sectionLocalId?.let { sectionId(it) },
            parentId = target.parentTaskLocalId?.let { taskId(it) },
        )
    }

    private suspend fun templateBody(template: TemplatePayload): TaskTemplateRequest =
        TaskTemplateRequest(
            name = template.name,
            description = template.description,
            priority = template.priority,
            dayPart = template.dayPart,
            labelIds = labelIds(template.labelLocalIds),
            subtasks =
                template.subtasks.map { subtask ->
                    TemplateSubtaskRequest(
                        title = subtask.title,
                        description = subtask.description,
                        priority = subtask.priority,
                        dayPart = subtask.dayPart,
                        labelIds = labelIds(subtask.labelLocalIds),
                    )
                },
        )

    private fun autoLabelRules(op: PutAutoLabelsOp): List<AutoLabelRuleDto> =
        op.rules.map { AutoLabelRuleDto(it.mask, it.labelIds, it.ignoreCase) }

    private fun suggestionRules(op: PutProjectSuggestionsOp): List<ProjectSuggestionRuleDto> =
        op.rules.map { ProjectSuggestionRuleDto(it.mask, it.projectIds, it.ignoreCase) }

    /**
     * The interface-state keys this write changes.
     *
     * Its ids are the server's, like the preference document's: the state blob is
     * the server's own and travels back to it unchanged, so a key this build has
     * never heard of survives a write from this build. A key being dropped is
     * said out loud as a null, because leaving it out is what means "not touching
     * it".
     */
    private fun userState(op: PatchUserStateOp): JsonObject =
        JsonObject(
            buildMap {
                op.activeContextId?.let { put("activeContextId", JsonPrimitive(it)) }
                for (key in op.remove) put(key, JsonNull)
            },
        )

    private suspend fun harpoonRef(
        target: HarpoonTarget,
        localId: Long,
    ): HarpoonRefRequest =
        when (target) {
            HarpoonTarget.TASK -> HarpoonRefRequest(target.wire, taskId(localId))
            HarpoonTarget.PROJECT -> HarpoonRefRequest(target.wire, projectId(localId))
        }

    // --- translating references ----------------------------------------------

    private suspend fun taskId(localId: Long): Long = serverId(OpRef(ReplicaEntityKind.TASK, localId))

    private suspend fun projectId(localId: Long): Long = serverId(OpRef(ReplicaEntityKind.PROJECT, localId))

    private suspend fun sectionId(localId: Long): Long = serverId(OpRef(ReplicaEntityKind.SECTION, localId))

    private suspend fun contextId(localId: Long): Long = serverId(OpRef(ReplicaEntityKind.CONTEXT, localId))

    private suspend fun labelId(localId: Long): Long = serverId(OpRef(ReplicaEntityKind.LABEL, localId))

    private suspend fun templateId(localId: Long): Long = serverId(OpRef(ReplicaEntityKind.TASK_TEMPLATE, localId))

    private suspend fun taskIds(localIds: List<Long>): List<Long> = localIds.map { taskId(it) }

    private suspend fun labelIds(localIds: List<Long>): List<Long> = localIds.map { labelId(it) }

    private suspend fun serverId(ref: OpRef): Long = created[ref] ?: ids.require(ref)

    // --- reading the answer ---------------------------------------------------

    private fun Response<*>.outcome(): SendOutcome = SendOutcome(isIdempotentReplay)

    private fun Response<*>.creating(
        entity: ReplicaEntityKind,
        localId: Long,
        serverId: Long,
    ): SendOutcome {
        val ref = OpRef(entity, localId)
        remember(ref, serverId)
        return SendOutcome(isIdempotentReplay, listOf(ServerIdAssignment(ref, serverId)))
    }

    /**
     * The same, for a write that brought several rows into existence at once.
     *
     * The two lists are matched by position and the shorter one wins. A server
     * that made fewer rows than were drawn here says so in its answer, and the
     * rows it did not make are left without a name rather than being given one
     * that belongs to something else — a mismatch is worth a line in the log,
     * because a row nothing on the server answers to will not come back.
     */
    private fun Response<*>.creatingEach(
        op: OutboxOp,
        localIds: List<Long>,
        serverIds: List<Long>,
    ): SendOutcome {
        if (localIds.size != serverIds.size) {
            Log.w(
                DRAIN_LOG_TAG,
                "The server made ${serverIds.size} row(s) for ${op.kind.stored}, " +
                    "where this device drew ${localIds.size}",
            )
        }
        val assignments =
            localIds.zip(serverIds) { localId, serverId ->
                val ref = OpRef(ReplicaEntityKind.TASK, localId)
                remember(ref, serverId)
                ServerIdAssignment(ref, serverId)
            }
        return SendOutcome(isIdempotentReplay, assignments)
    }

    /**
     * A bulk write's answer, which is per item rather than all-or-nothing.
     *
     * An item the server refused is reported and then let go: the call itself
     * succeeded, so there is nothing to retry and nothing to hold the queue up
     * for, and the row the change was not made to comes back as it really is
     * with the next catch-up.
     */
    private fun Response<BulkResultDto>.bulk(op: OutboxOp): SendOutcome {
        reportRefused(op, requireBody().failed.map { it.id to it.error.code })
        return outcome()
    }

    private fun reportRefused(
        op: OutboxOp,
        refused: List<Pair<Long, String>>,
    ) {
        if (refused.isEmpty()) return
        val listed = refused.joinToString { (id, code) -> "$id ($code)" }
        Log.w(DRAIN_LOG_TAG, "The server refused part of ${op.kind.stored}: $listed")
    }

    private fun remember(
        ref: OpRef,
        serverId: Long,
    ) {
        if (created.size >= MEMO_LIMIT) created.keys.firstOrNull()?.let(created::remove)
        created[ref] = serverId
    }

    private companion object {
        /**
         * How many freshly created rows to remember. Only a row deleted between
         * being created and being sent is ever looked up here, so a small number
         * covers it; the cap exists so a long-running process cannot grow this
         * without limit.
         */
        const val MEMO_LIMIT: Int = 256
    }
}

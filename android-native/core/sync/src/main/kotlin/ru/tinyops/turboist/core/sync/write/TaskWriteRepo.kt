package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.INBOX_ID
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.RelationDirection
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.model.view.relationEnds
import ru.tinyops.turboist.core.model.view.wouldCloseBlockingCycle
import ru.tinyops.turboist.core.network.Clearable
import ru.tinyops.turboist.core.network.dto.CreateTaskRequest
import java.time.Instant
import java.time.ZoneId
import java.time.ZonedDateTime

/**
 * A task the user is writing down.
 *
 * [labels] is nullable rather than empty-by-default because absent and empty mean
 * different things: a subtask created with no label field inherits its parent's
 * labels, while an explicitly empty list is a request for none.
 */
data class NewTask(
    val title: String,
    val description: String = "",
    val priority: Priority = Priority.NONE,
    val dueAt: Long? = null,
    val dueHasTime: Boolean = false,
    val deadlineAt: Long? = null,
    val deadlineHasTime: Boolean = false,
    val dayPart: DayPart = DayPart.NONE,
    val planState: PlanState = PlanState.NONE,
    val recurrenceRule: String? = null,
    val labels: List<String>? = null,
    val removedAutoLabels: List<String> = emptyList(),
)

/**
 * An edit to a task, field by field.
 *
 * Every property is "not being touched" when absent, which is what keeps the
 * request that carries it a patch: two devices that edited different fields of
 * the same task both keep their edit, with no merge rule needed anywhere.
 *
 * The three date-ish fields go further and use [Clearable], because emptying a
 * due date is a third state that a plain nullable cannot tell apart from leaving
 * it alone.
 */
data class TaskEdit(
    val title: String? = null,
    val description: String? = null,
    val priority: Priority? = null,
    val dueAt: Clearable<Long>? = null,
    val dueHasTime: Boolean? = null,
    val deadlineAt: Clearable<Long>? = null,
    val deadlineHasTime: Boolean? = null,
    val dayPart: DayPart? = null,
    val planState: PlanState? = null,
    val recurrenceRule: Clearable<String>? = null,
    val labels: List<String>? = null,
    val removedAutoLabels: List<String> = emptyList(),
    val isPrivate: Boolean? = null,
    val isComplex: Boolean? = null,
)

/**
 * Every write a screen can make to a task.
 *
 * Each method does the same two things in one transaction: change the replica the
 * way the user expects, and queue the request that will tell the server. Nothing
 * here waits for a network — the screen is correct the moment the method returns,
 * and stays correct whether the phone reconnects in a second or tomorrow.
 *
 * Where the server's answer would differ from what was applied here, the next
 * page of changes replaces it. That is the deal, and it is why so little is
 * re-implemented on this side: only the rules a user would otherwise be misled
 * by are repeated ([ReplicaRules]).
 *
 * @param zone the clock a repeating task's runs are counted by a day of. It has
 *   to be the one [recurrence] expands rules against, or a task could be moved on
 *   twice on what the two of them disagree is one day.
 */
class TaskWriteRepo(
    private val db: TurboistDatabase,
    private val writer: OutboxWriter,
    private val rules: ReplicaRules = ReplicaRules(db),
    private val recurrence: RecurrenceAdvance = RecurrenceAdvance.ServerDecides,
    private val zone: ZoneId = ZoneId.systemDefault(),
) {
    /**
     * Writes a task down wherever the user pointed.
     *
     * The row is created with no server id — it exists only here until the queue
     * drains — and that absence is the signal a screen uses to hide the actions
     * that need a server-side address, such as sharing a link to it.
     */
    suspend fun create(
        destination: TaskDestination,
        task: NewTask,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val placement = placementFor(destination)
            // A project standing in the daily plan fixes the priority of the work
            // inside it, and the server pins a task written into one the same way.
            // Matching that here means the chip is right from the first frame.
            val priority = rules.pinnedPriorityOf(placement.projectLocalId) ?: task.priority
            val localId =
                db.tasks().insert(
                    TaskRow(
                        title = task.title,
                        description = task.description,
                        inboxId = placement.inboxId,
                        contextLocalId = placement.contextLocalId,
                        projectLocalId = placement.projectLocalId,
                        sectionLocalId = placement.sectionLocalId,
                        parentLocalId = placement.parentLocalId,
                        priority = priority,
                        dueAt = task.dueAt,
                        dueHasTime = task.dueHasTime,
                        deadlineAt = task.deadlineAt,
                        deadlineHasTime = task.deadlineHasTime,
                        dayPart = task.dayPart,
                        planState = task.planState,
                        recurrenceRule = task.recurrenceRule,
                        createdAt = at,
                        updatedAt = at,
                    ),
                )
            val labelNames = task.labels ?: placement.inheritedLabelNames
            db.tasks().setLabels(
                localId,
                rules.labelsForTitle(task.title, labelNames, task.removedAutoLabels),
                at,
            )
            val opId =
                writer.enqueue(
                    CreateTaskOp(
                        taskLocalId = localId,
                        destination = destination,
                        body =
                            CreateTaskRequest(
                                title = task.title,
                                description = task.description.ifEmpty { null },
                                priority = task.priority.wireOrNull(),
                                dueAt = WireTime.formatOrNull(task.dueAt),
                                dueHasTime = task.dueHasTime.takeIf { it },
                                deadlineAt = WireTime.formatOrNull(task.deadlineAt),
                                deadlineHasTime = task.deadlineHasTime.takeIf { it },
                                dayPart = task.dayPart.wireOrNull(),
                                planState = task.planState.wireOrNull(),
                                recurrenceRule = task.recurrenceRule,
                                labels = task.labels,
                                removedAutoLabels = task.removedAutoLabels.ifEmpty { null },
                            ),
                    ),
                    ReplicaEntityKind.TASK,
                    localId,
                )
            QueuedWrite(opId, localId)
        }

    /** Applies an edit and queues a request carrying only the fields it changed. */
    suspend fun patch(
        taskLocalId: Long,
        edit: TaskEdit,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireTask(taskLocalId)
            val edited =
                row.copy(
                    title = edit.title ?: row.title,
                    description = edit.description ?: row.description,
                    priority = edit.priority ?: row.priority,
                    dueAt = edit.dueAt.applyTo(row.dueAt),
                    dueHasTime = edit.dueHasTime ?: row.dueHasTime,
                    deadlineAt = edit.deadlineAt.applyTo(row.deadlineAt),
                    deadlineHasTime = edit.deadlineHasTime ?: row.deadlineHasTime,
                    dayPart = edit.dayPart ?: row.dayPart,
                    planState = edit.planState ?: row.planState,
                    recurrenceRule = edit.recurrenceRule.applyTo(row.recurrenceRule),
                    isPrivate = edit.isPrivate ?: row.isPrivate,
                    isComplex = edit.isComplex ?: row.isComplex,
                    updatedAt = at,
                )
            db.tasks().update(edited)
            // Unticking an automatic label has to be stated, not merely left out
            // of the set: the rules run again on both sides and would put it
            // straight back. A caller that already knows what the user refused
            // says so; the rest is read off the set it handed over.
            val removedAuto =
                when (edit.labels) {
                    null -> edit.removedAutoLabels
                    else ->
                        (
                            edit.removedAutoLabels +
                                rules.rejectedAutoLabels(edited.title, taskLocalId, edit.labels)
                        ).distinct()
                }
            if (edit.labels != null || removedAuto.isNotEmpty()) {
                db.tasks().setLabels(
                    taskLocalId,
                    rules.labelsForTitle(
                        edited.title,
                        edit.labels,
                        removedAuto,
                        db.tasks().labelsOf(taskLocalId).map { it.labelLocalId },
                    ),
                    at,
                )
            }
            // A plan state set through an edit cascades exactly as one set
            // through the planning action does; the two are the same decision
            // made from two different screens.
            when (edit.planState) {
                PlanState.BACKLOG -> rules.cascadeBacklog(taskLocalId, at)
                PlanState.WEEK -> rules.cascadeWeek(taskLocalId, at)
                else -> Unit
            }
            val opId =
                writer.enqueue(
                    PatchTaskOp(
                        taskLocalId = taskLocalId,
                        patch =
                            PatchTaskPayload(
                                title = edit.title,
                                description = edit.description,
                                priority = edit.priority?.wire,
                                dueAt = (edit.dueAt as? Clearable.Set)?.let { WireTime.format(it.value) },
                                clearDueAt = edit.dueAt == Clearable.Clear,
                                dueHasTime = edit.dueHasTime,
                                deadlineAt = (edit.deadlineAt as? Clearable.Set)?.let { WireTime.format(it.value) },
                                clearDeadlineAt = edit.deadlineAt == Clearable.Clear,
                                deadlineHasTime = edit.deadlineHasTime,
                                dayPart = edit.dayPart?.wire,
                                planState = edit.planState?.wire,
                                recurrenceRule = (edit.recurrenceRule as? Clearable.Set)?.value,
                                clearRecurrenceRule = edit.recurrenceRule == Clearable.Clear,
                                labels = edit.labels,
                                removedAutoLabels = removedAuto.ifEmpty { null },
                                isPrivate = edit.isPrivate,
                                isComplex = edit.isComplex,
                            ),
                    ),
                    ReplicaEntityKind.TASK,
                    taskLocalId,
                )
            QueuedWrite(opId, taskLocalId)
        }

    /**
     * Removes a task.
     *
     * The row goes for good, here as on the server: there are no tombstones
     * anywhere in this product, and the subtasks the database takes with it are
     * the same ones the server's own cascade would.
     */
    suspend fun delete(taskLocalId: Long): QueuedWrite =
        writer.transaction {
            val row = requireTask(taskLocalId)
            val opId =
                writer.enqueue(DeleteTaskOp(taskLocalId, row.serverId), ReplicaEntityKind.TASK, taskLocalId)
            db.tasks().delete(row)
            QueuedWrite(opId, taskLocalId)
        }

    /**
     * Ticks a task off, refusing while anything still blocks it.
     *
     * The refusal is the point of doing this locally at all: without it the tick
     * would land on screen, sit in the queue, and be undone hours later by an
     * answer the user has long stopped waiting for.
     *
     * Completing a task completes the open work under it, one level at a time, so
     * that a subtask with a blocker of its own is left open rather than forced
     * through — finishing the parent must not quietly bypass something the user
     * said was in the way.
     */
    suspend fun complete(
        taskLocalId: Long,
        completedAt: Long? = null,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val moment = completedAt ?: at
            requireTask(taskLocalId)
            val blockers = rules.openBlockerLocalIds(taskLocalId)
            if (blockers.isNotEmpty()) throw WriteRefused.TaskBlocked(blockers)
            applyCompletion(taskLocalId, moment, at)
            val opId =
                writer.enqueue(
                    CompleteTaskOp(taskLocalId, WireTime.format(moment)),
                    ReplicaEntityKind.TASK,
                    taskLocalId,
                )
            QueuedWrite(opId, taskLocalId)
        }

    /** Puts a completed task back into the open work. */
    suspend fun uncomplete(taskLocalId: Long): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireTask(taskLocalId)
            db.tasks().update(row.copy(status = TaskStatus.OPEN, completedAt = null, updatedAt = at))
            val opId = writer.enqueue(UncompleteTaskOp(taskLocalId), ReplicaEntityKind.TASK, taskLocalId)
            QueuedWrite(opId, taskLocalId)
        }

    /**
     * Closes a task without claiming it was done. Like a completion it stops the
     * task holding anything up, which is why a cancelled blocker releases what it
     * was blocking instead of deadlocking it forever.
     */
    suspend fun cancel(taskLocalId: Long): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireTask(taskLocalId)
            db.tasks().update(row.copy(status = TaskStatus.CANCELLED, updatedAt = at))
            val opId = writer.enqueue(CancelTaskOp(taskLocalId), ReplicaEntityKind.TASK, taskLocalId)
            QueuedWrite(opId, taskLocalId)
        }

    /**
     * Moves a task somewhere else. Placement is exclusive, so this replaces it
     * rather than adding to it.
     *
     * Arriving in a project that stands in the daily plan fixes the task's
     * priority, exactly as the server fixes it: the plan's buckets decide the
     * priority of the work in them, and a moved task is now part of that work.
     */
    suspend fun move(
        taskLocalId: Long,
        destination: TaskDestination,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireTask(taskLocalId)
            val placement = placementFor(destination)
            db.tasks().update(
                row.copy(
                    inboxId = placement.inboxId,
                    contextLocalId = placement.contextLocalId,
                    projectLocalId = placement.projectLocalId,
                    sectionLocalId = placement.sectionLocalId,
                    parentLocalId = placement.parentLocalId,
                    updatedAt = at,
                ),
            )
            placement.projectLocalId?.let { projectLocalId ->
                rules.pinnedPriorityOf(projectLocalId)?.let { rules.pinProjectPriority(projectLocalId, it, at) }
            }
            val opId =
                writer.enqueue(MoveTaskOp(taskLocalId, destination), ReplicaEntityKind.TASK, taskLocalId)
            QueuedWrite(opId, taskLocalId)
        }

    /**
     * Commits a task to the week, or parks it in the backlog.
     *
     * Either decision carries the work under it: subtasks follow their parent, at
     * any depth. Doing that here rather than waiting for the server is what keeps
     * the list the user is looking at from contradicting the choice they just
     * made.
     */
    suspend fun plan(
        taskLocalId: Long,
        state: PlanState,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireTask(taskLocalId)
            val parked = state == PlanState.BACKLOG
            db.tasks().update(
                row.copy(
                    planState = state,
                    dueAt = if (parked) null else row.dueAt,
                    dueHasTime = if (parked) false else row.dueHasTime,
                    updatedAt = at,
                ),
            )
            when (state) {
                PlanState.BACKLOG -> rules.cascadeBacklog(taskLocalId, at)
                PlanState.WEEK -> rules.cascadeWeek(taskLocalId, at)
                else -> Unit
            }
            val opId =
                writer.enqueue(PlanTaskOp(taskLocalId, state.wire), ReplicaEntityKind.TASK, taskLocalId)
            QueuedWrite(opId, taskLocalId)
        }

    /** Pins a task to the shelf, refusing once the user's own cap is reached. */
    suspend fun pin(taskLocalId: Long): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireTask(taskLocalId)
            if (!row.isPinned) rules.assertRoomToPinTask()
            db.tasks().update(row.copy(isPinned = true, pinnedAt = at, updatedAt = at))
            val opId = writer.enqueue(PinTaskOp(taskLocalId), ReplicaEntityKind.TASK, taskLocalId)
            QueuedWrite(opId, taskLocalId)
        }

    suspend fun unpin(taskLocalId: Long): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireTask(taskLocalId)
            db.tasks().update(row.copy(isPinned = false, pinnedAt = null, updatedAt = at))
            val opId = writer.enqueue(UnpinTaskOp(taskLocalId), ReplicaEntityKind.TASK, taskLocalId)
            QueuedWrite(opId, taskLocalId)
        }

    /**
     * Asks for a copy of a task.
     *
     * Nothing is added to the replica: the copy is the server's to make, and
     * inventing one here would leave a duplicate on screen when the real one
     * arrived, since the two could never be recognised as the same row.
     */
    suspend fun duplicate(taskLocalId: Long): QueuedWrite =
        writer.transaction {
            requireTask(taskLocalId)
            val opId = writer.enqueue(DuplicateTaskOp(taskLocalId), ReplicaEntityKind.TASK, taskLocalId)
            QueuedWrite(opId, taskLocalId)
        }

    /**
     * Replaces a task with several cut from it.
     *
     * The split is made here as well as asked for. Each piece is a task in its
     * own right that inherits everything about the one it came from except its
     * wording — where it sits, how urgent it is, when it is due, what it is
     * labelled, whether it is private — and the task they were cut from is gone,
     * because it has been replaced rather than added to. Doing that on the device
     * is what makes the outline the user just typed the list they are looking at,
     * instead of one task sitting unchanged until the queue drains.
     *
     * Blank lines are dropped and the rest trimmed, as the server does, so an
     * outline typed with a trailing newline does not ask for an untitled task.
     *
     * A task with subtasks is refused, as the server refuses it: the work
     * underneath would have nowhere to go, and there is no answer to which of the
     * pieces should adopt it.
     *
     * @return the queued write, naming the first piece — which is the row the
     *   task was already in, so a screen showing it has something to show.
     */
    suspend fun decompose(
        taskLocalId: Long,
        titles: List<String>,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val source = requireTask(taskLocalId)
            val named = titles.map { it.trim() }.filter { it.isNotEmpty() }
            if (named.isEmpty()) throw WriteRefused.Invalid("nothing to decompose the task into")
            if (db.tasks().subtasksOf(taskLocalId).isNotEmpty()) {
                throw WriteRefused.Invalid("a task with subtasks cannot be split")
            }
            val inheritedLabels = labelNamesOf(taskLocalId) ?: emptyList()
            // The first piece takes over the row the work was already in, rather
            // than the row being deleted and a fresh one written beside it. That
            // is what keeps the task addressable: the queued write names the task
            // to split by the row that holds it, and a row deleted before the
            // write went out would leave nothing to name it by — the split would
            // be on screen and never reach the server. It also keeps whatever was
            // looking at the task looking at something.
            db.tasks().update(
                pieceOf(source, named.first(), at)
                    .copy(localId = source.localId, serverId = source.serverId, createdAt = source.createdAt),
            )
            // The links belonged to the task that is being replaced, and the
            // server drops them with it.
            for (edge in db.taskRelations().forTask(taskLocalId)) db.taskRelations().delete(edge)
            val created =
                named.mapIndexed { index, title ->
                    val localId =
                        if (index == 0) source.localId else db.tasks().insert(pieceOf(source, title, at))
                    db.tasks().setLabels(localId, rules.labelsForTitle(title, inheritedLabels, emptyList()), at)
                    localId
                }
            val opId =
                writer.enqueue(
                    DecomposeTaskOp(taskLocalId, named, created),
                    ReplicaEntityKind.TASK,
                    taskLocalId,
                )
            QueuedWrite(opId, source.localId)
        }

    /**
     * Links two tasks.
     *
     * The edge is written into the replica immediately, which is what lets a
     * blocker created offline actually block — and, when it is finished offline,
     * actually release what it was holding up.
     *
     * A symmetric link is stored the way the server stores it, with the lower id
     * as its source, so the same pair added from either end is one edge and not
     * two. Offline-created tasks have no server id to compare, so their local
     * ids stand in: the ordering is a tie-break, not a fact, and the next pull
     * replaces it with the server's own.
     *
     * The three links the server refuses are refused here first — a task to
     * itself, a pair already linked that way, and a wait that closes a loop. This
     * is not belt-and-braces: an accepted link would be drawn on screen, queued,
     * and then rejected whenever the queue next drained, leaving the user with a
     * failure they can no longer connect to anything they did. Refusing now means
     * a link the device turns down never reaches the queue at all.
     */
    suspend fun addRelation(
        taskLocalId: Long,
        otherTaskLocalId: Long,
        type: RelationType,
        direction: RelationDirection = RelationDirection.OUTGOING,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            requireTask(taskLocalId)
            requireTask(otherTaskLocalId)
            if (taskLocalId == otherTaskLocalId) {
                throw WriteRefused.Invalid("a task cannot be related to itself")
            }
            val (source, target) = relationEnds(taskLocalId, otherTaskLocalId, type, direction)
            db.taskRelations().localIdForEdge(source, target, type)?.let {
                throw WriteRefused.RelationExists(it)
            }
            if (type == RelationType.BLOCKS &&
                wouldCloseBlockingCycle(source, target, db.taskRelations().blockEdges())
            ) {
                throw WriteRefused.RelationCycle(blockerLocalId = source, blockedLocalId = target)
            }
            val relationLocalId =
                db.taskRelations().insert(
                    TaskRelationRow(
                        sourceTaskLocalId = source,
                        targetTaskLocalId = target,
                        type = type,
                        createdAt = at,
                    ),
                )
            val opId =
                writer.enqueue(
                    AddTaskRelationOp(
                        taskLocalId = taskLocalId,
                        targetTaskLocalId = otherTaskLocalId,
                        relationLocalId = relationLocalId,
                        type = type.wire,
                        direction = direction.wire.takeIf { type == RelationType.BLOCKS },
                    ),
                    ReplicaEntityKind.TASK_RELATION,
                    relationLocalId,
                )
            QueuedWrite(opId, relationLocalId)
        }

    suspend fun removeRelation(
        taskLocalId: Long,
        relationLocalId: Long,
    ): QueuedWrite =
        writer.transaction {
            val row =
                db.taskRelations().byLocalId(relationLocalId)
                    ?: throw WriteRefused.RowMissing("task relation", relationLocalId)
            val opId =
                writer.enqueue(
                    RemoveTaskRelationOp(taskLocalId, relationLocalId),
                    ReplicaEntityKind.TASK_RELATION,
                    relationLocalId,
                )
            db.taskRelations().delete(row)
            QueuedWrite(opId, relationLocalId)
        }

    /**
     * Ticks off several tasks at once.
     *
     * A blocked one is left out rather than failing the whole action, which is
     * how the server answers too: one task that cannot be finished must not undo
     * the nine that can.
     *
     * A batch names the tasks and nothing else, so the server times the
     * completion itself. Anything repeating in the batch is therefore left
     * exactly as it is, to be moved on by the answer: a next occurrence worked
     * out here would be worked out against the wrong instant, and the run
     * written down beside it could never be matched to the one the server
     * records, leaving a second copy in the history for good.
     */
    suspend fun bulkComplete(taskLocalIds: List<Long>): BulkQueuedWrite =
        writer.transaction {
            val at = writer.now()
            val accepted = mutableListOf<Long>()
            val refused = mutableMapOf<Long, List<Long>>()
            for (localId in taskLocalIds.distinct()) {
                if (db.tasks().byLocalId(localId) == null) continue
                val blockers = rules.openBlockerLocalIds(localId)
                if (blockers.isNotEmpty()) {
                    refused[localId] = blockers
                    continue
                }
                applyCompletion(localId, at, at, moveRepeatingOn = false)
                accepted += localId
            }
            if (accepted.isEmpty()) return@transaction BulkQueuedWrite(null, accepted, refused)
            val opId = writer.enqueue(BulkCompleteTasksOp(accepted), ReplicaEntityKind.TASK)
            BulkQueuedWrite(opId, accepted, refused)
        }

    /** Moves several tasks to the same place. */
    suspend fun bulkMove(
        taskLocalIds: List<Long>,
        destination: TaskDestination,
    ): BulkQueuedWrite =
        writer.transaction {
            val at = writer.now()
            val placement = placementFor(destination)
            val moved = mutableListOf<Long>()
            for (localId in taskLocalIds.distinct()) {
                val row = db.tasks().byLocalId(localId) ?: continue
                db.tasks().update(
                    row.copy(
                        inboxId = placement.inboxId,
                        contextLocalId = placement.contextLocalId,
                        projectLocalId = placement.projectLocalId,
                        sectionLocalId = placement.sectionLocalId,
                        parentLocalId = placement.parentLocalId,
                        updatedAt = at,
                    ),
                )
                moved += localId
            }
            if (moved.isEmpty()) return@transaction BulkQueuedWrite(null, moved, emptyMap())
            val opId = writer.enqueue(BulkMoveTasksOp(moved, destination), ReplicaEntityKind.TASK)
            BulkQueuedWrite(opId, moved, emptyMap())
        }

    /**
     * Gives several tasks the same priority.
     *
     * A task whose project stands in the daily plan is left out: that project
     * fixes the priority of its work, and the server turns such an edit down
     * outright. Sending it anyway would fail the whole request for everyone else
     * in the selection, so it is dropped here and counted, the way a blocked task
     * is dropped from a batch of completions.
     */
    suspend fun bulkPriority(
        taskLocalIds: List<Long>,
        priority: Priority,
    ): BulkQueuedWrite =
        writer.transaction {
            val at = writer.now()
            val changed = mutableListOf<Long>()
            val locked = mutableListOf<Long>()
            for (localId in taskLocalIds.distinct()) {
                val row = db.tasks().byLocalId(localId) ?: continue
                if (rules.pinnedPriorityOf(row.projectLocalId) != null) {
                    locked += localId
                    continue
                }
                db.tasks().update(row.copy(priority = priority, updatedAt = at))
                changed += localId
            }
            if (changed.isEmpty()) return@transaction BulkQueuedWrite(null, changed, emptyMap(), locked)
            val opId = writer.enqueue(BulkTaskPriorityOp(changed, priority.wire), ReplicaEntityKind.TASK)
            BulkQueuedWrite(opId, changed, emptyMap(), locked)
        }

    /**
     * Gathers loose tasks under a new parent.
     *
     * The parent is created here so the children have something to hang from
     * immediately; the same request creates it on the server and re-parents the
     * children, so the two agree as soon as it lands.
     *
     * A group has to live in a context, a project or one of its columns. The
     * inbox is raw capture and holds no structure, and hanging a group under
     * another task would be a subtask rather than a group, so both are refused
     * here rather than queued for the server to refuse hours later.
     *
     * Adopted children are rewritten to match the parent in three ways: they
     * take its placement, its labels and its priority. That is what grouping
     * means — the children stop being loose work and become parts of one thing —
     * and it is applied here so the screen shows the outcome at once instead of
     * showing the old labels until the next catch-up.
     */
    suspend fun group(
        title: String,
        childTaskLocalIds: List<Long>,
        destination: TaskDestination,
        description: String = "",
        priority: Priority = Priority.NONE,
        dayPart: DayPart = DayPart.NONE,
        planState: PlanState = PlanState.NONE,
        labels: List<String> = emptyList(),
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            if (destination is TaskDestination.Inbox || destination is TaskDestination.SubtaskOf) {
                throw WriteRefused.Placement("a group belongs to a context, a project or a section")
            }
            val placement = placementFor(destination)
            val parentLocalId =
                db.tasks().insert(
                    TaskRow(
                        title = title,
                        description = description,
                        contextLocalId = placement.contextLocalId,
                        projectLocalId = placement.projectLocalId,
                        sectionLocalId = placement.sectionLocalId,
                        priority = priority,
                        dayPart = dayPart,
                        planState = planState,
                        isComplex = true,
                        createdAt = at,
                        updatedAt = at,
                    ),
                )
            val parentLabels = rules.labelsForTitle(title, labels, emptyList())
            db.tasks().setLabels(parentLocalId, parentLabels, at)
            val children = childTaskLocalIds.distinct().filter { it != parentLocalId }
            for (childLocalId in children) {
                val child = db.tasks().byLocalId(childLocalId) ?: continue
                db.tasks().update(
                    child.copy(
                        parentLocalId = parentLocalId,
                        inboxId = null,
                        contextLocalId = placement.contextLocalId,
                        projectLocalId = placement.projectLocalId,
                        sectionLocalId = placement.sectionLocalId,
                        priority = priority,
                        updatedAt = at,
                    ),
                )
                db.tasks().setLabels(childLocalId, parentLabels, at)
            }
            val opId =
                writer.enqueue(
                    GroupTasksOp(
                        parentTaskLocalId = parentLocalId,
                        childTaskLocalIds = children,
                        destination = destination,
                        title = title,
                        description = description.ifEmpty { null },
                        priority = priority.wireOrNull(),
                        dayPart = dayPart.wireOrNull(),
                        planState = planState.wireOrNull(),
                        labels = labels,
                    ),
                    ReplicaEntityKind.TASK,
                    parentLocalId,
                )
            QueuedWrite(opId, parentLocalId)
        }

    // --- internals -----------------------------------------------------------

    /**
     * Marks a task done and carries the completion down its open subtasks, one
     * level at a time, skipping any that something outside still blocks.
     *
     * A task that repeats is not finished by being ticked off: it moves to its
     * next occurrence, and the run just done is recorded as a row of its own so
     * the history has something to show. Both steps happen here only when this
     * build can say when the next occurrence falls ([RecurrenceAdvance]); a build
     * that cannot leaves the task exactly as it was for the server to answer.
     *
     * @param moveRepeatingOn whether a repeating task may be moved on here at
     *   all. Only a completion that tells the server *when* it happened may do
     *   so: the run written down alongside the advance is recognised as the
     *   server's own run by the instant it carries, and a run the server timed
     *   differently is never recognised and stays in the history for good. A
     *   caller that cannot name the instant passes false, which leaves a
     *   repeating task untouched — and, since it has not been closed, leaves the
     *   work under it untouched too.
     * @return whether the task is now closed, which is what decides if the
     *   completion carries on down its subtasks. A task that merely moved on to
     *   its next run has not finished the work under it.
     */
    private suspend fun applyCompletion(
        taskLocalId: Long,
        moment: Long,
        at: Long,
        moveRepeatingOn: Boolean = true,
    ) {
        val row = db.tasks().byLocalId(taskLocalId) ?: return
        val rule = row.recurrenceRule
        val closed =
            when {
                rule.isNullOrBlank() -> {
                    db.tasks().update(
                        row.copy(status = TaskStatus.COMPLETED, completedAt = moment, updatedAt = at),
                    )
                    true
                }
                !moveRepeatingOn -> false
                else -> advanceRepeating(row, rule, moment, at)
            }
        if (!closed) return
        for (child in db.tasks().subtasksOf(taskLocalId)) {
            if (child.status != TaskStatus.OPEN) continue
            if (rules.openBlockerLocalIds(child.localId).isNotEmpty()) continue
            applyCompletion(child.localId, moment, at, moveRepeatingOn)
        }
    }

    /**
     * Moves a repeating task on, and records the run that was just finished.
     *
     * Ticking the same repeating task off twice in one day is one run, not two.
     * The second tick changes nothing at all — it neither moves the task on again
     * nor records a second run — which is the same answer the server gives, so
     * the screen does not have to be corrected afterwards. The day is measured on
     * [zone], the same clock the occurrence itself is computed against.
     *
     * @return whether the series ended here, in which case the task really is
     *   finished and the completion carries on down its subtasks.
     */
    private suspend fun advanceRepeating(
        row: TaskRow,
        rule: String,
        moment: Long,
        at: Long,
    ): Boolean {
        val day = ZonedDateTime.ofInstant(Instant.ofEpochMilli(moment), zone).toLocalDate()
        val dayStart = day.atStartOfDay(zone).toInstant().toEpochMilli()
        val dayEnd = day.plusDays(1).atStartOfDay(zone).toInstant().toEpochMilli()
        if (db.tasks().hasRecurrenceCompletionBetween(row.localId, dayStart, dayEnd)) return false
        return when (val advance = recurrence.after(rule, row.dueAt, moment)) {
            RecurrenceOutcome.Unknown -> false
            RecurrenceOutcome.Ended -> {
                // The last run there was ever going to be. Nothing is recorded
                // separately: the task itself is now the completed row.
                db.tasks().update(
                    row.copy(
                        status = TaskStatus.COMPLETED,
                        completedAt = moment,
                        planState = PlanState.NONE,
                        updatedAt = at,
                    ),
                )
                true
            }
            is RecurrenceOutcome.Next -> {
                db.tasks().update(
                    row.copy(
                        dueAt = advance.dueAt,
                        planState = PlanState.NONE,
                        postponeCount = 0,
                        updatedAt = at,
                    ),
                )
                recordCompletedRun(row, moment, at)
                false
            }
        }
    }

    /**
     * Writes down the run of a repeating task that has just been finished.
     *
     * The task itself stays open on its next date, so without this the work would
     * never appear in the completion history at all. The row is a copy of what
     * was done and where — its title, its notes, its place, its labels — and
     * deliberately not a copy of what is still pending about the task: it carries
     * no dates, no repeat rule of its own to repeat again, no place on the pinned
     * shelf and no place in the week's plan. It points back at the task it was
     * cut from, which is what marks it as a run rather than as a task in its own
     * right.
     *
     * It carries no server id, because the server has not been told yet. When the
     * completion is sent, the server records the same run and the catch-up
     * afterwards recognises this row as that one rather than adding a second.
     */
    private suspend fun recordCompletedRun(
        row: TaskRow,
        moment: Long,
        at: Long,
    ) {
        val recordedLocalId =
            db.tasks().insert(
                TaskRow(
                    title = row.title,
                    description = row.description,
                    inboxId = row.inboxId,
                    contextLocalId = row.contextLocalId,
                    projectLocalId = row.projectLocalId,
                    sectionLocalId = row.sectionLocalId,
                    priority = row.priority,
                    status = TaskStatus.COMPLETED,
                    dayPart = row.dayPart,
                    planState = PlanState.NONE,
                    isPrivate = row.isPrivate,
                    completedAt = moment,
                    sourceTaskLocalId = row.localId,
                    createdAt = at,
                    updatedAt = at,
                ),
            )
        val labels = db.tasks().labelsOf(row.localId).map { it.labelLocalId }
        if (labels.isNotEmpty()) db.tasks().setLabels(recordedLocalId, labels, at)
    }

    private suspend fun requireTask(taskLocalId: Long): TaskRow =
        db.tasks().byLocalId(taskLocalId) ?: throw WriteRefused.RowMissing("task", taskLocalId)

    /**
     * One piece of a task that is being split up.
     *
     * The fields listed are the ones the server carries over, and the ones left
     * out are left out on purpose: a piece is new work, so it starts open, unset
     * on the shelf, never postponed, and with no history of its own behind it.
     * Written out field by field rather than copied from the row for that reason
     * — a copy would quietly carry every column added to a task later.
     */
    private fun pieceOf(
        source: TaskRow,
        title: String,
        at: Long,
    ): TaskRow =
        TaskRow(
            title = title,
            description = source.description,
            inboxId = source.inboxId,
            contextLocalId = source.contextLocalId,
            projectLocalId = source.projectLocalId,
            sectionLocalId = source.sectionLocalId,
            parentLocalId = source.parentLocalId,
            priority = source.priority,
            dueAt = source.dueAt,
            dueHasTime = source.dueHasTime,
            deadlineAt = source.deadlineAt,
            deadlineHasTime = source.deadlineHasTime,
            dayPart = source.dayPart,
            planState = source.planState,
            isPrivate = source.isPrivate,
            recurrenceRule = source.recurrenceRule,
            createdAt = at,
            updatedAt = at,
        )

    /**
     * Where a destination puts a task, and what it inherits by being there.
     *
     * A subtask takes its parent's placement — a piece of work belongs to the
     * same project as the work it is part of — and, when the caller named no
     * labels, its parent's labels too.
     */
    private suspend fun placementFor(destination: TaskDestination): Placement =
        when (destination) {
            TaskDestination.Inbox -> Placement(inboxId = INBOX_ID)
            is TaskDestination.InContext -> Placement(contextLocalId = destination.contextLocalId)
            is TaskDestination.InProject -> Placement(projectLocalId = destination.projectLocalId)
            is TaskDestination.InSection -> {
                val section =
                    db.sections().byLocalId(destination.sectionLocalId)
                        ?: throw WriteRefused.RowMissing("section", destination.sectionLocalId)
                Placement(
                    projectLocalId = section.projectLocalId,
                    sectionLocalId = section.localId,
                )
            }
            is TaskDestination.SubtaskOf -> {
                val parent = requireTask(destination.parentTaskLocalId)
                if (parent.inboxId != null) {
                    throw WriteRefused.Placement("a task in the inbox cannot have subtasks")
                }
                Placement(
                    contextLocalId = parent.contextLocalId,
                    projectLocalId = parent.projectLocalId,
                    sectionLocalId = parent.sectionLocalId,
                    parentLocalId = parent.localId,
                    inheritedLabelNames = labelNamesOf(parent.localId),
                )
            }
        }

    private suspend fun labelNamesOf(taskLocalId: Long): List<String>? {
        val names =
            db.tasks().labelsOf(taskLocalId)
                .mapNotNull { db.labels().byLocalId(it.labelLocalId)?.name }
        return names.ifEmpty { null }
    }
}

/**
 * What a bulk write did.
 *
 * [refused] names the tasks the device left out and what stood in their way, so
 * the screen can say which ones did not go through instead of pretending they
 * all did. [locked] names the ones left out because the change was not theirs to
 * make — nothing stood in their way, a rule simply does not allow it. [opId] is
 * absent when nothing was accepted — there is no request to make, and queuing an
 * empty one would be a round-trip for nothing.
 */
data class BulkQueuedWrite(
    val opId: String?,
    val accepted: List<Long>,
    val refused: Map<Long, List<Long>>,
    val locked: List<Long> = emptyList(),
)

/** Where a task sits, as columns rather than as a choice. */
private data class Placement(
    val inboxId: Long? = null,
    val contextLocalId: Long? = null,
    val projectLocalId: Long? = null,
    val sectionLocalId: Long? = null,
    val parentLocalId: Long? = null,
    val inheritedLabelNames: List<String>? = null,
)

private fun Priority.wireOrNull(): String? = wire.takeIf { this != Priority.NONE }

private fun DayPart.wireOrNull(): String? = wire.takeIf { this != DayPart.NONE }

private fun PlanState.wireOrNull(): String? = wire.takeIf { this != PlanState.NONE }

private fun Clearable<Long>?.applyTo(current: Long?): Long? =
    when (this) {
        null -> current
        Clearable.Clear -> null
        is Clearable.Set -> value
    }

private fun Clearable<String>?.applyTo(current: String?): String? =
    when (this) {
        null -> current
        Clearable.Clear -> null
        is Clearable.Set -> value
    }

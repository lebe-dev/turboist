package ru.tinyops.turboist.core.sync.pull

import android.util.Log
import androidx.room.withTransaction
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.AppSettingsRow
import ru.tinyops.turboist.core.database.entity.SyncStateRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateSubtaskLabelRow
import ru.tinyops.turboist.core.database.entity.UserSettingsRow
import ru.tinyops.turboist.core.database.entity.UserStateRow
import ru.tinyops.turboist.core.database.entity.toRow
import ru.tinyops.turboist.core.database.search.rebuildSearchIndex
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.dto.ContextDto
import ru.tinyops.turboist.core.network.dto.LabelDto
import ru.tinyops.turboist.core.network.dto.ProjectDto
import ru.tinyops.turboist.core.network.dto.SectionDto
import ru.tinyops.turboist.core.network.dto.SyncChangesDto
import ru.tinyops.turboist.core.network.dto.SyncSnapshotDto
import ru.tinyops.turboist.core.network.dto.TaskDto
import ru.tinyops.turboist.core.network.dto.TaskRelationEdgeDto
import ru.tinyops.turboist.core.network.dto.TaskTemplateDto
import ru.tinyops.turboist.core.network.mapping.toContext
import ru.tinyops.turboist.core.network.mapping.toLabel
import ru.tinyops.turboist.core.network.mapping.toProject
import ru.tinyops.turboist.core.network.mapping.toRelation
import ru.tinyops.turboist.core.network.mapping.toSection
import ru.tinyops.turboist.core.network.mapping.toTask
import ru.tinyops.turboist.core.network.mapping.toTemplate

/** What became of one page of the delta feed. */
sealed interface PageOutcome {
    /**
     * The page is in the replica and the cursor moved with it, in one
     * transaction.
     */
    data class Applied(
        val cursor: Long,
        val records: Int,
    ) : PageOutcome

    /**
     * The page names records the replica does not hold and the page does not
     * carry — a reference to a row whose own change landed in a part of the
     * history this device has not read. Nothing was written and the cursor did
     * not move; the remedy is a complete copy, which is consistent by
     * construction.
     */
    data object Unresolvable : PageOutcome
}

/**
 * Writes server state into the replica.
 *
 * Everything here obeys four rules, and every one of them is a rule about
 * *when* rather than about what a field means:
 *
 * 1. **A batch and its position commit together.** The cursor is written in the
 *    same transaction as the records it accounts for, so a process killed
 *    mid-catch-up resumes from the last batch that actually landed. Replaying an
 *    overlap is free — a record is its own current state, so writing it twice
 *    leaves the same row.
 * 2. **Records are written in the order the schema requires**, not the order they
 *    arrived in. A reference is checked as its statement runs, so a project is
 *    written after the context it names. Where that is not enough — a subtask
 *    whose parent appears later in the same batch — the batch is passed over
 *    again until nothing more can be placed.
 * 3. **A row with an unsent write against it is left alone.** The user is looking
 *    at a change the server has not been told about; overwriting it would make
 *    their edit vanish and reappear. The write's own send, and the read after it,
 *    converge the row.
 * 4. **A delete beats that.** If the record is gone from the server, no queued
 *    write against it can ever land, so the row goes and its writes are kept
 *    where the user can see what was lost rather than retried forever.
 */
class ReplicaApplier(
    private val db: TurboistDatabase,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    /**
     * Seeds the replica from a complete copy, and takes out what the copy does
     * not mention.
     *
     * A complete copy names every record the server holds, so anything here that
     * it does not name is either gone from the server or older than the window
     * the copy was cut at — and in both cases it stops being part of the replica.
     * Two things survive that sweep: a row created on this device, which the
     * server has never heard of, and a row with an unsent write against it, whose
     * fate the send itself will settle.
     *
     * @return how many records the copy carried.
     */
    suspend fun applySnapshot(snapshot: SyncSnapshotDto): Int =
        db.withTransaction {
            val batch = snapshot.toBatch()
            val index = ReplicaIdIndex(db)
            val stranded = write(index, batch)
            if (stranded > 0) {
                Log.w(PULL_LOG_TAG, "A complete copy left $stranded records unplaced and they were dropped")
            }
            sweep(index, batch)
            // A complete copy rewrites whole tables rather than editing rows one
            // at a time, and the full-text index is maintained per row. Rebuilding
            // it here, inside the same transaction, is what keeps the index from
            // outliving the workspace it described.
            db.rebuildSearchIndex()
            db.syncState().save(
                SyncStateRow(epoch = snapshot.epoch, cursor = snapshot.cursor, lastSyncAt = clock()),
            )
            batch.size
        }

    /**
     * Applies one page of the delta feed together with the position it moves the
     * replica to.
     *
     * A page that cannot be placed leaves nothing behind: the transaction is
     * abandoned, so the cursor still points at the last page that did land and
     * the caller can start again from a complete copy without a half-applied page
     * underneath it.
     */
    suspend fun applyPage(page: SyncChangesDto): PageOutcome =
        try {
            db.withTransaction {
                val batch = page.toBatch()
                val index = ReplicaIdIndex(db)
                val stranded = write(index, batch)
                if (stranded > 0) throw UnplaceableBatch(stranded)
                db.syncState().save(
                    SyncStateRow(epoch = page.epoch, cursor = page.cursor, lastSyncAt = clock()),
                )
                PageOutcome.Applied(page.cursor, batch.size)
            }
        } catch (unplaceable: UnplaceableBatch) {
            Log.w(
                PULL_LOG_TAG,
                "A page of changes named ${unplaceable.records} rows this device does not hold; " +
                    "asking for a complete copy instead",
            )
            PageOutcome.Unresolvable
        }

    /**
     * Writes a batch, kind by kind, in the order the schema's references demand.
     *
     * @return how many records could not be placed at all.
     */
    private suspend fun write(
        index: ReplicaIdIndex,
        batch: ChangeBatch,
    ): Int {
        var stranded = 0
        stranded += eachPlaceable(batch.contexts) { applyContext(index, it) }
        stranded += eachPlaceable(batch.labels) { applyLabel(index, it) }
        stranded += eachPlaceable(batch.projects) { applyProject(index, it) }
        stranded += eachPlaceable(batch.sections) { applySection(index, it) }
        stranded += eachPlaceable(batch.tasks) { applyTask(index, it) }
        stranded += eachPlaceable(batch.relations) { applyRelation(index, it) }
        stranded += eachPlaceable(batch.templates) { applyTemplate(index, it) }

        val at = clock()
        batch.userSettings?.let { db.settings().saveUserSettings(UserSettingsRow(payload = it, updatedAt = at)) }
        batch.appSettings?.let { db.settings().saveAppSettings(AppSettingsRow(payload = it, updatedAt = at)) }
        batch.userState?.let { db.settings().saveUserState(UserStateRow(payload = it, updatedAt = at)) }

        // Removals come last so that a record deleted in the same batch it was
        // last changed in ends up deleted, and so that a reference written
        // earlier in the batch still had its target to point at.
        batch.tombstones.forEach { remove(index, it) }
        return stranded
    }

    /**
     * Writes every record it can, passing over the leftovers again while each
     * pass places at least one more.
     *
     * This is what lets a batch carry a subtask ahead of its parent: the child is
     * held back, the parent lands, and the next pass places the child. A pass that
     * places nothing means the rest name rows that are not coming.
     */
    private suspend fun <T> eachPlaceable(
        records: List<T>,
        write: suspend (T) -> Boolean,
    ): Int {
        var pending: List<T> = records
        while (pending.isNotEmpty()) {
            val held = mutableListOf<T>()
            for (record in pending) {
                if (!write(record)) held += record
            }
            if (held.size == pending.size) return held.size
            pending = held
        }
        return 0
    }

    private suspend fun applyContext(
        index: ReplicaIdIndex,
        dto: ContextDto,
    ): Boolean {
        val kind = ReplicaEntityKind.CONTEXT
        if (holdsUnsentWrite(index, kind, dto.id)) return true
        val localId = db.contexts().upsertByServerId(dto.toContext(index).toRow())
        index.remember(kind, dto.id, localId)
        return true
    }

    private suspend fun applyLabel(
        index: ReplicaIdIndex,
        dto: LabelDto,
    ): Boolean {
        val kind = ReplicaEntityKind.LABEL
        if (holdsUnsentWrite(index, kind, dto.id)) return true
        val localId = db.labels().upsertByServerId(dto.toLabel(index).toRow())
        index.remember(kind, dto.id, localId)
        return true
    }

    private suspend fun applyProject(
        index: ReplicaIdIndex,
        dto: ProjectDto,
    ): Boolean {
        if (!index.isPresent(ReplicaEntityKind.CONTEXT, dto.contextId)) return false
        val labelIds = writeLabels(index, dto.labels)
        val kind = ReplicaEntityKind.PROJECT
        if (holdsUnsentWrite(index, kind, dto.id)) return true
        val localId = db.projects().upsertByServerId(dto.toProject(index).toRow())
        index.remember(kind, dto.id, localId)
        db.projects().setLabels(localId, labelIds)
        return true
    }

    private suspend fun applySection(
        index: ReplicaIdIndex,
        dto: SectionDto,
    ): Boolean {
        if (!index.isPresent(ReplicaEntityKind.PROJECT, dto.projectId)) return false
        val kind = ReplicaEntityKind.SECTION
        if (holdsUnsentWrite(index, kind, dto.id)) return true
        val localId = db.sections().upsertByServerId(dto.toSection(index).toRow())
        index.remember(kind, dto.id, localId)
        return true
    }

    /**
     * A task.
     *
     * Every placement it names has to be here already — moving a task to "no
     * project" because the project's own change has not been read yet would be a
     * visible, wrong change on the user's screen. The one reference allowed to go
     * missing is the recurring task a completed snapshot was cut from: it is a
     * pointer back into history, the schema clears it when its target goes, and
     * losing it costs nothing that is shown.
     *
     * The capacity bucket of the daily plan is carried over from the row already
     * here, because task payloads do not include it — it arrives with the plan
     * view. Writing what the payload implies would erase it on every catch-up.
     */
    private suspend fun applyTask(
        index: ReplicaIdIndex,
        dto: TaskDto,
    ): Boolean {
        if (!index.isPresent(ReplicaEntityKind.TASK, dto.parentId)) return false
        if (!index.isPresent(ReplicaEntityKind.CONTEXT, dto.contextId)) return false
        if (!index.isPresent(ReplicaEntityKind.PROJECT, dto.projectId)) return false
        if (!index.isPresent(ReplicaEntityKind.SECTION, dto.sectionId)) return false
        index.resolve(ReplicaEntityKind.TASK, dto.sourceTaskId)

        val labelIds = writeLabels(index, dto.labels)
        val kind = ReplicaEntityKind.TASK
        val existing = db.tasks().byServerId(dto.id) ?: adoptRecordedRun(index, dto)
        if (existing != null) {
            index.remember(kind, dto.id, existing.localId)
            if (db.outbox().isDirty(kind, existing.localId)) return true
        }
        val row = dto.toTask(index, hydrateRelations = false).toRow().copy(troikiCategory = existing?.troikiCategory)
        val localId = db.tasks().upsertByServerId(row)
        index.remember(kind, dto.id, localId)
        db.tasks().setLabels(localId, labelIds, clock())
        return true
    }

    /**
     * Recognises a run of a repeating task the device recorded for itself as the
     * one the server has now recorded too, and gives the local row the server's
     * name.
     *
     * Ticking off a repeating task with no connection leaves the task open on its
     * next date and writes down the run that was finished, so the history has
     * something to show straight away. The server writes down the same run when
     * the completion reaches it — the same task, the same moment, because the
     * moment travelled with the request — and without this the two would sit side
     * by side in the history for ever: the device's copy carries no server name,
     * so nothing else would ever match them up or sweep it away.
     *
     * Matching on the exact moment rather than on the day is deliberate. It is
     * the one fact both sides are certain to agree on, and adopting the wrong row
     * would silently rewrite a different piece of history.
     */
    private suspend fun adoptRecordedRun(
        index: ReplicaIdIndex,
        dto: TaskDto,
    ): TaskRow? {
        val sourceLocalId = index.resolve(ReplicaEntityKind.TASK, dto.sourceTaskId) ?: return null
        val completedAt = WireTime.parseOrNull(dto.completedAt) ?: return null
        val recorded = db.tasks().unnamedRecurrenceCompletion(sourceLocalId, completedAt) ?: return null
        db.tasks().assignServerId(recorded.localId, dto.id, clock())
        Log.i(PULL_LOG_TAG, "Matched a run recorded on this device with the server's record of it")
        return recorded.copy(serverId = dto.id)
    }

    private suspend fun applyRelation(
        index: ReplicaIdIndex,
        dto: TaskRelationEdgeDto,
    ): Boolean {
        if (!index.isPresent(ReplicaEntityKind.TASK, dto.sourceTaskId)) return false
        if (!index.isPresent(ReplicaEntityKind.TASK, dto.targetTaskId)) return false
        val kind = ReplicaEntityKind.TASK_RELATION
        if (holdsUnsentWrite(index, kind, dto.id)) return true
        val localId = db.taskRelations().upsertByServerId(dto.toRelation(index).toRow())
        index.remember(kind, dto.id, localId)
        return true
    }

    /**
     * A template, with its subtasks replaced wholesale.
     *
     * Subtask rows are rewritten rather than matched up one by one: nothing in
     * the replica references a template's subtask, so their device ids are not
     * worth preserving, and a positional list is far easier to get right by
     * replacement than by diff.
     */
    private suspend fun applyTemplate(
        index: ReplicaIdIndex,
        dto: TaskTemplateDto,
    ): Boolean {
        val labelIds = writeLabels(index, dto.labels)
        val subtaskLabelIds = dto.subtasks.map { writeLabels(index, it.labels) }
        val kind = ReplicaEntityKind.TASK_TEMPLATE
        if (holdsUnsentWrite(index, kind, dto.id)) return true

        val template = dto.toTemplate(index)
        val localId = db.taskTemplates().upsertByServerId(template.toRow())
        index.remember(kind, dto.id, localId)
        db.taskTemplates().setLabels(localId, labelIds)
        db.taskTemplates().clearSubtasks(localId)
        template.subtasks.forEachIndexed { position, subtask ->
            val subtaskLocalId =
                db.taskTemplates().insertSubtask(
                    subtask.toRow().copy(localId = NO_LOCAL_ID, templateLocalId = localId),
                )
            db.taskTemplates().insertSubtaskLabels(
                subtaskLabelIds[position].map { TaskTemplateSubtaskLabelRow(subtaskLocalId, it) },
            )
        }
        return true
    }

    /**
     * Writes the labels a record carries with it and answers with their device
     * ids.
     *
     * The payload of a task or a project carries each of its labels in full, so
     * the label can be written from there rather than waited for: a label edge
     * whose own change happens to arrive in a later page would otherwise hold up
     * the record that wears it. A label deleted in the same batch is removed
     * again by the batch's removals, which run last.
     */
    private suspend fun writeLabels(
        index: ReplicaIdIndex,
        labels: List<LabelDto>,
    ): List<Long> =
        labels.mapNotNull { label ->
            applyLabel(index, label)
            index.resolve(ReplicaEntityKind.LABEL, label.id)
        }

    /**
     * Whether the row this record names is one the device has changed and not yet
     * sent — in which case the record is not written.
     *
     * It also files the row's device id either way, so the records that reference
     * it can still be placed.
     */
    private suspend fun holdsUnsentWrite(
        index: ReplicaIdIndex,
        kind: ReplicaEntityKind,
        serverId: Long,
    ): Boolean {
        val localId = index.resolve(kind, serverId) ?: return false
        return db.outbox().isDirty(kind, localId)
    }

    /**
     * Takes a record out of the replica because the server no longer has it.
     *
     * Anything the schema hangs off it goes with it — a project's tasks, a task's
     * subtasks — exactly as the server's own delete did. Writes still queued
     * against the record itself are moved to where the user can see them: the
     * server would answer them with "no such record", and there is no reason to
     * make the user wait for that answer.
     */
    private suspend fun remove(
        index: ReplicaIdIndex,
        tombstone: Tombstone,
    ) {
        val localId = index.resolve(tombstone.kind, tombstone.serverId) ?: return
        discardQueuedWrites(tombstone.kind, localId)
        deleteByServerId(tombstone.kind, tombstone.serverId)
        index.forget(tombstone.kind, tombstone.serverId)
    }

    private suspend fun discardQueuedWrites(
        kind: ReplicaEntityKind,
        localId: Long,
    ) {
        val queued = db.outbox().opsFor(kind, localId)
        if (queued.isEmpty()) return
        val at = clock()
        queued.forEach { op ->
            db.outbox().quarantine(
                op = op,
                errorCode = ApiErrorCodes.TARGET_GONE,
                errorMessage = "the record this change was going to update no longer exists on the server",
                httpStatus = null,
                quarantinedAt = at,
            )
        }
        Log.i(
            PULL_LOG_TAG,
            "Set aside ${queued.size} unsent changes because the record they targeted was deleted on the server",
        )
    }

    /**
     * Removes what a complete copy did not mention.
     *
     * Kinds are swept from the leaves inward so that a removal cascading through
     * the schema meets tables that have already been dealt with, which keeps the
     * work proportional to what actually changed.
     */
    private suspend fun sweep(
        index: ReplicaIdIndex,
        batch: ChangeBatch,
    ) {
        sweepKind(index, ReplicaEntityKind.TASK_RELATION, batch.relations.mapTo(mutableSetOf()) { it.id })
        sweepKind(index, ReplicaEntityKind.TASK, batch.tasks.mapTo(mutableSetOf()) { it.id })
        sweepKind(index, ReplicaEntityKind.SECTION, batch.sections.mapTo(mutableSetOf()) { it.id })
        sweepKind(index, ReplicaEntityKind.PROJECT, batch.projects.mapTo(mutableSetOf()) { it.id })
        sweepKind(index, ReplicaEntityKind.TASK_TEMPLATE, batch.templates.mapTo(mutableSetOf()) { it.id })
        sweepKind(index, ReplicaEntityKind.LABEL, batch.labels.mapTo(mutableSetOf()) { it.id })
        sweepKind(index, ReplicaEntityKind.CONTEXT, batch.contexts.mapTo(mutableSetOf()) { it.id })
    }

    private suspend fun sweepKind(
        index: ReplicaIdIndex,
        kind: ReplicaEntityKind,
        keep: Set<Long>,
    ) {
        for (serverId in knownServerIds(kind)) {
            if (serverId in keep) continue
            val localId = index.resolve(kind, serverId) ?: continue
            // A row the device has changed and not yet sent is left standing: a
            // copy that does not mention it is weaker evidence than a deletion,
            // and the send itself will find out which it was.
            if (db.outbox().isDirty(kind, localId)) continue
            deleteByServerId(kind, serverId)
            index.forget(kind, serverId)
        }
    }

    private suspend fun knownServerIds(kind: ReplicaEntityKind): List<Long> =
        when (kind) {
            ReplicaEntityKind.TASK -> db.tasks().knownServerIds()
            ReplicaEntityKind.PROJECT -> db.projects().knownServerIds()
            ReplicaEntityKind.SECTION -> db.sections().knownServerIds()
            ReplicaEntityKind.CONTEXT -> db.contexts().knownServerIds()
            ReplicaEntityKind.LABEL -> db.labels().knownServerIds()
            ReplicaEntityKind.TASK_RELATION -> db.taskRelations().knownServerIds()
            ReplicaEntityKind.TASK_TEMPLATE -> db.taskTemplates().knownServerIds()
            ReplicaEntityKind.USER_SETTINGS,
            ReplicaEntityKind.USER_STATE,
            ReplicaEntityKind.APP_SETTINGS,
            -> emptyList()
        }

    private suspend fun deleteByServerId(
        kind: ReplicaEntityKind,
        serverId: Long,
    ) {
        when (kind) {
            ReplicaEntityKind.TASK -> db.tasks().deleteByServerId(serverId)
            ReplicaEntityKind.PROJECT -> db.projects().deleteByServerId(serverId)
            ReplicaEntityKind.SECTION -> db.sections().deleteByServerId(serverId)
            ReplicaEntityKind.CONTEXT -> db.contexts().deleteByServerId(serverId)
            ReplicaEntityKind.LABEL -> db.labels().deleteByServerId(serverId)
            ReplicaEntityKind.TASK_RELATION -> db.taskRelations().deleteByServerId(serverId)
            ReplicaEntityKind.TASK_TEMPLATE -> db.taskTemplates().deleteByServerId(serverId)
            // The three documents are replaced, never removed.
            ReplicaEntityKind.USER_SETTINGS,
            ReplicaEntityKind.USER_STATE,
            ReplicaEntityKind.APP_SETTINGS,
            -> Log.w(PULL_LOG_TAG, "Ignoring a deletion of ${kind.stored}, which is a document and is never deleted")
        }
    }
}

/**
 * Abandons the transaction a page is being written in.
 *
 * The page cannot be placed, and a partially written one with its cursor moved
 * on would be worse than none: the missing records would never be asked for
 * again. Rolling back is what lets the caller fall back to a complete copy.
 */
private class UnplaceableBatch(
    val records: Int,
) : RuntimeException("a batch named $records rows the replica does not hold")

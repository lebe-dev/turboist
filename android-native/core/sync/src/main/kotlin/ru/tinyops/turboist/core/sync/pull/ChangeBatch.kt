package ru.tinyops.turboist.core.sync.pull

import android.util.Log
import kotlinx.serialization.KSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.AppSettingsDto
import ru.tinyops.turboist.core.network.dto.ContextDto
import ru.tinyops.turboist.core.network.dto.LabelDto
import ru.tinyops.turboist.core.network.dto.ProjectDto
import ru.tinyops.turboist.core.network.dto.SectionDto
import ru.tinyops.turboist.core.network.dto.SyncChangeDto
import ru.tinyops.turboist.core.network.dto.SyncChangesDto
import ru.tinyops.turboist.core.network.dto.SyncEntity
import ru.tinyops.turboist.core.network.dto.SyncOp
import ru.tinyops.turboist.core.network.dto.SyncSnapshotDto
import ru.tinyops.turboist.core.network.dto.TaskDto
import ru.tinyops.turboist.core.network.dto.TaskRelationEdgeDto
import ru.tinyops.turboist.core.network.dto.TaskTemplateDto
import ru.tinyops.turboist.core.network.dto.UserSettingsDto

internal const val PULL_LOG_TAG = "TurboistSync"

/**
 * How a single-row document is written down for storage.
 *
 * Fields at their default value are written out rather than omitted, which is
 * the opposite of what a request body wants and exactly what a stored copy
 * wants: the replica is keeping a document, not describing a change to one, and
 * a reader of that document should not have to know which keys were left out
 * because they happened to equal a default.
 */
internal val ReplicaDocumentJson: Json =
    Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        encodeDefaults = true
    }

/** A record the server says is gone, named the only way a gone record can be. */
internal data class Tombstone(
    val kind: ReplicaEntityKind,
    val serverId: Long,
)

/**
 * One unit of work for the applier: the records to write and the records to
 * remove.
 *
 * A page of changes and a complete seed both decode into this, which is the
 * point — the two are the same operation over different amounts of data, and
 * writing the applier twice would be writing two chances to disagree about what
 * a task is.
 *
 * The records are held per kind rather than in arrival order because the order
 * they must be *written* in is not the order they arrived in: a project cannot
 * be written before the context it names exists. Sorting that out is the
 * applier's job, and grouping is what lets it.
 */
internal class ChangeBatch {
    val contexts: MutableList<ContextDto> = mutableListOf()
    val labels: MutableList<LabelDto> = mutableListOf()
    val projects: MutableList<ProjectDto> = mutableListOf()
    val sections: MutableList<SectionDto> = mutableListOf()
    val tasks: MutableList<TaskDto> = mutableListOf()
    val relations: MutableList<TaskRelationEdgeDto> = mutableListOf()
    val templates: MutableList<TaskTemplateDto> = mutableListOf()

    /** The three documents, as the text they arrived as. Absent means "not in this batch". */
    var userSettings: String? = null
    var appSettings: String? = null
    var userState: String? = null

    val tombstones: MutableList<Tombstone> = mutableListOf()

    /** How many records this batch carries, for the log line and the caller's tally. */
    val size: Int
        get() =
            contexts.size + labels.size + projects.size + sections.size +
                tasks.size + relations.size + templates.size + tombstones.size +
                listOfNotNull(userSettings, appSettings, userState).size
}

/**
 * Reads a page of the delta feed.
 *
 * Two kinds of change are dropped rather than allowed to fail the page: one
 * naming an entity this build has no table for, and one whose payload will not
 * decode. Both mean the same thing — a server newer than this app — and refusing
 * the whole page would leave the replica stuck at a cursor it can never pass,
 * which is a far worse outcome than missing one record until the app is updated.
 */
internal fun SyncChangesDto.toBatch(): ChangeBatch {
    val batch = ChangeBatch()
    for (change in changes) {
        val kind = change.entityKind
        if (kind == SyncEntity.UNKNOWN) {
            Log.w(PULL_LOG_TAG, "Skipping a change for an entity this version does not know: ${change.entity}")
            continue
        }
        when (change.operation) {
            SyncOp.DELETE -> change.replicaKind()?.let { batch.tombstones += Tombstone(it, change.id) }
            SyncOp.UPSERT -> batch.addUpsert(change)
            SyncOp.UNKNOWN ->
                Log.w(PULL_LOG_TAG, "Skipping a change with an operation this version does not know: ${change.op}")
        }
    }
    return batch
}

/** The replica table a change names, or `null` for the documents, which are never deleted. */
private fun SyncChangeDto.replicaKind(): ReplicaEntityKind? =
    when (entityKind) {
        SyncEntity.TASK -> ReplicaEntityKind.TASK
        SyncEntity.PROJECT -> ReplicaEntityKind.PROJECT
        SyncEntity.SECTION -> ReplicaEntityKind.SECTION
        SyncEntity.CONTEXT -> ReplicaEntityKind.CONTEXT
        SyncEntity.LABEL -> ReplicaEntityKind.LABEL
        SyncEntity.TASK_RELATION -> ReplicaEntityKind.TASK_RELATION
        SyncEntity.TASK_TEMPLATE -> ReplicaEntityKind.TASK_TEMPLATE
        SyncEntity.USER_SETTINGS, SyncEntity.APP_SETTINGS, SyncEntity.USER_STATE, SyncEntity.UNKNOWN -> null
    }

private fun ChangeBatch.addUpsert(change: SyncChangeDto) {
    val data = change.data
    if (data == null) {
        Log.w(PULL_LOG_TAG, "Skipping an upsert that carried no payload for ${change.entity} ${change.id}")
        return
    }
    runCatching {
        when (change.entityKind) {
            SyncEntity.TASK -> tasks += decode(data, TaskDto.serializer())
            SyncEntity.PROJECT -> projects += decode(data, ProjectDto.serializer())
            SyncEntity.SECTION -> sections += decode(data, SectionDto.serializer())
            SyncEntity.CONTEXT -> contexts += decode(data, ContextDto.serializer())
            SyncEntity.LABEL -> labels += decode(data, LabelDto.serializer())
            SyncEntity.TASK_RELATION -> relations += decode(data, TaskRelationEdgeDto.serializer())
            SyncEntity.TASK_TEMPLATE -> templates += decode(data, TaskTemplateDto.serializer())
            SyncEntity.USER_SETTINGS -> userSettings = data.toString()
            SyncEntity.APP_SETTINGS -> appSettings = data.toString()
            SyncEntity.USER_STATE -> userState = data.toString()
            SyncEntity.UNKNOWN -> Unit
        }
    }.onFailure {
        Log.w(PULL_LOG_TAG, "Skipping a change whose payload could not be read: ${change.entity} ${change.id}", it)
    }
}

private fun <T> decode(
    data: JsonElement,
    serializer: KSerializer<T>,
): T = TurboistJson.decodeFromJsonElement(serializer, data)

/**
 * Reads a complete seed as the same batch a page decodes to.
 *
 * The documents are written back out rather than kept as they arrived, because
 * the seed hands them over already decoded. That costs the keys this build has
 * never heard of, and it is the one place where that is acceptable: the very
 * next change to either document arrives on the delta feed, which does keep the
 * text verbatim, and nothing is ever echoed back to the server from the stored
 * copy.
 */
internal fun SyncSnapshotDto.toBatch(): ChangeBatch {
    val batch = ChangeBatch()
    batch.contexts += contexts
    batch.labels += labels
    batch.projects += projects
    batch.sections += sections
    batch.tasks += tasks
    batch.relations += taskRelations
    batch.templates += taskTemplates
    batch.userSettings = ReplicaDocumentJson.encodeToString(UserSettingsDto.serializer(), userSettings)
    batch.appSettings = ReplicaDocumentJson.encodeToString(AppSettingsDto.serializer(), appSettings)
    batch.userState = userState.toString()
    return batch
}

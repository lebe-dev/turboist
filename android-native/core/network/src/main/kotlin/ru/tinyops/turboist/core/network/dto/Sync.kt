package ru.tinyops.turboist.core.network.dto

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import ru.tinyops.turboist.core.model.WireEnum
import ru.tinyops.turboist.core.model.WireEnumLookup

/**
 * The kinds of row the replica mirrors.
 *
 * The same vocabulary names a delta change and a snapshot collection, so an
 * applier written once handles both. An entity this build has never heard of
 * decodes to [UNKNOWN] and is skipped rather than aborting the page: a newer
 * server must be able to add a syncable table without bricking installed clients.
 */
enum class SyncEntity(override val wire: String) : WireEnum {
    TASK("task"),
    PROJECT("project"),
    SECTION("section"),
    CONTEXT("context"),
    LABEL("label"),
    TASK_RELATION("task_relation"),
    TASK_TEMPLATE("task_template"),
    USER_SETTINGS("user_settings"),
    APP_SETTINGS("app_settings"),
    USER_STATE("user_state"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<SyncEntity>(entries, UNKNOWN)
}

/**
 * What happened to a row. There is no soft delete anywhere in the product, so a
 * [DELETE] change *is* the tombstone — it is the only record that the row ever
 * existed after it is gone.
 */
enum class SyncOp(override val wire: String) : WireEnum {
    UPSERT("upsert"),
    DELETE("delete"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<SyncOp>(entries, UNKNOWN)
}

/**
 * One entity's net change.
 *
 * [data] is the row's current state and is absent on a delete — where [id] is the
 * whole message. It stays an undecoded JSON element on purpose: a page mixes
 * entities, and the applier decodes each payload with the serializer for the
 * entity it turned out to be.
 */
@Serializable
data class SyncChangeDto(
    val entity: String = "",
    val op: String = "",
    val seq: Long = 0,
    val id: Long = 0,
    val data: JsonElement? = null,
) {
    val entityKind: SyncEntity
        get() = SyncEntity.fromWire(entity)

    val operation: SyncOp
        get() = SyncOp.fromWire(op)
}

/**
 * One page of the delta feed.
 *
 * [cursor] is the position to continue from, and it is stored in the same
 * transaction as the rows it belongs to — a crash between applying a page and
 * recording its cursor would otherwise re-apply or, worse, skip changes.
 * [epoch] stamps which history that cursor belongs to; a cursor from a replaced
 * history is refused rather than silently misread.
 */
@Serializable
data class SyncChangesDto(
    val epoch: Long = 0,
    val cursor: Long = 0,
    val hasMore: Boolean = false,
    val changes: List<SyncChangeDto> = emptyList(),
)

/**
 * A complete replica seed: every syncable row plus the position to continue the
 * delta feed from. Taken on first launch, and again whenever the delta feed
 * refuses to resume.
 *
 * @property completedSince the start of the completed-task window this payload
 *   was cut at. Tasks completed before it are absent deliberately and no delta
 *   will ever deliver them; that history is read online when someone asks for it.
 */
@Serializable
data class SyncSnapshotDto(
    val epoch: Long = 0,
    val cursor: Long = 0,
    val completedSince: String? = null,
    val tasks: List<TaskDto> = emptyList(),
    val projects: List<ProjectDto> = emptyList(),
    val sections: List<SectionDto> = emptyList(),
    val contexts: List<ContextDto> = emptyList(),
    val labels: List<LabelDto> = emptyList(),
    val taskRelations: List<TaskRelationEdgeDto> = emptyList(),
    val taskTemplates: List<TaskTemplateDto> = emptyList(),
    val userSettings: UserSettingsDto = UserSettingsDto(),
    val appSettings: AppSettingsDto = AppSettingsDto(),
    val userState: JsonObject = JsonObject(emptyMap()),
)

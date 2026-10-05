package ru.tinyops.turboist.core.sync.write

import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.AppSettingsRow
import ru.tinyops.turboist.core.database.entity.UserSettingsRow
import ru.tinyops.turboist.core.database.entity.UserStateRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.AutoLabelRuleDto
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest
import ru.tinyops.turboist.core.network.dto.ProjectSuggestionRuleDto

/**
 * Writes to the three preference documents, and to the jump pair.
 *
 * The documents are stored and edited **whole**, as the text the server sent.
 * Spreading them into typed fields and writing them back from those fields would
 * silently drop every key this build has not been taught about — and this is a
 * self-hosted product, where the phone and the server are upgraded on different
 * days. Merging the changed keys into the stored text keeps an unknown key a
 * thing this build ignores rather than a thing it destroys.
 */
class SettingsWriteRepo(
    private val db: TurboistDatabase,
    private val writer: OutboxWriter,
) {
    /**
     * Changes some of the user's own preferences.
     *
     * The request carries only the keys that changed, and so does the merge into
     * the stored document: a preference this screen never showed cannot be lost
     * by a screen that did not know it existed.
     */
    suspend fun patchUserSettings(edit: PatchUserSettingsRequest): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val changed = TurboistJson.encodeToJsonElement(PatchUserSettingsRequest.serializer(), edit).jsonObject
            val merged = merge(db.settings().userSettings()?.payload, changed)
            db.settings().saveUserSettings(UserSettingsRow(payload = merged, updatedAt = at))
            val opId = writer.enqueue(PatchUserSettingsOp(edit), ReplicaEntityKind.USER_SETTINGS)
            QueuedWrite(opId, NO_TARGET_ROW)
        }

    /**
     * Replaces the installation's automatic labelling rules.
     *
     * The endpoint takes the whole list, so the queued write is the whole list —
     * there is no such thing as adding one rule, and pretending otherwise would
     * mean reconstructing the list at send time from a state that has since moved.
     *
     * The label ids inside are the server's, because the rules document is the
     * server's and travels back to it unchanged.
     */
    suspend fun putAutoLabels(rules: List<AutoLabelRule>): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val encoded =
                TurboistJson.encodeToJsonElement(
                    kotlinx.serialization.builtins.ListSerializer(AutoLabelRuleDto.serializer()),
                    rules.map { AutoLabelRuleDto(it.mask, it.labelIds, it.ignoreCase) },
                )
            val merged = merge(db.settings().appSettings()?.payload, JsonObject(mapOf("autoLabels" to encoded)))
            db.settings().saveAppSettings(AppSettingsRow(payload = merged, updatedAt = at))
            val opId =
                writer.enqueue(
                    PutAutoLabelsOp(rules.map { AutoLabelRulePayload(it.mask, it.labelIds, it.ignoreCase) }),
                    ReplicaEntityKind.APP_SETTINGS,
                )
            QueuedWrite(opId, NO_TARGET_ROW)
        }

    /**
     * Replaces the installation's project suggestions. Unlike the labelling rules
     * these are never applied to anything — they only offer projects while the
     * user types — so there is nothing to mirror beyond storing them.
     */
    suspend fun putProjectSuggestions(rules: List<ProjectSuggestionRule>): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val encoded =
                TurboistJson.encodeToJsonElement(
                    kotlinx.serialization.builtins.ListSerializer(ProjectSuggestionRuleDto.serializer()),
                    rules.map { ProjectSuggestionRuleDto(it.mask, it.projectIds, it.ignoreCase) },
                )
            val merged =
                merge(db.settings().appSettings()?.payload, JsonObject(mapOf("projectSuggestions" to encoded)))
            db.settings().saveAppSettings(AppSettingsRow(payload = merged, updatedAt = at))
            val opId =
                writer.enqueue(
                    PutProjectSuggestionsOp(
                        rules.map { ProjectSuggestionRulePayload(it.mask, it.projectIds, it.ignoreCase) },
                    ),
                    ReplicaEntityKind.APP_SETTINGS,
                )
            QueuedWrite(opId, NO_TARGET_ROW)
        }

    /**
     * Changes the interface state that follows the user between devices.
     *
     * The server merges it key by key and reads an explicit null as "drop this
     * key", which is why removing something is said out loud rather than by
     * omission — omission is what leaves a key alone.
     */
    suspend fun patchUserState(
        activeContextId: Long? = null,
        remove: List<String> = emptyList(),
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val changed =
                buildMap<String, JsonElement> {
                    activeContextId?.let { put("activeContextId", JsonPrimitive(it)) }
                    for (key in remove) put(key, JsonNull)
                }
            val merged = merge(db.settings().userState()?.payload, JsonObject(changed), dropNulls = true)
            db.settings().saveUserState(UserStateRow(payload = merged, updatedAt = at))
            val opId =
                writer.enqueue(PatchUserStateOp(activeContextId, remove), ReplicaEntityKind.USER_STATE)
            QueuedWrite(opId, NO_TARGET_ROW)
        }

    /**
     * Attaches an entity to the two-slot jump pair.
     *
     * Nothing is applied to the replica: the pair lives inside the preferences
     * document by server ids, a third attachment evicts the oldest, and both of
     * those are the server's to decide. The pair comes back on the next pull —
     * a moment later than the tap, and always right.
     */
    suspend fun attachHarpoon(
        target: HarpoonTarget,
        localId: Long,
    ): QueuedWrite =
        writer.transaction {
            requireHarpoonTarget(target, localId)
            val opId =
                writer.enqueue(AttachHarpoonOp(target, localId), ReplicaEntityKind.USER_SETTINGS, localId)
            QueuedWrite(opId, localId)
        }

    suspend fun detachHarpoon(
        target: HarpoonTarget,
        localId: Long,
    ): QueuedWrite =
        writer.transaction {
            requireHarpoonTarget(target, localId)
            val opId =
                writer.enqueue(DetachHarpoonOp(target, localId), ReplicaEntityKind.USER_SETTINGS, localId)
            QueuedWrite(opId, localId)
        }

    private suspend fun requireHarpoonTarget(
        target: HarpoonTarget,
        localId: Long,
    ) {
        when (target) {
            HarpoonTarget.TASK ->
                db.tasks().byLocalId(localId) ?: throw WriteRefused.RowMissing("task", localId)
            HarpoonTarget.PROJECT ->
                db.projects().byLocalId(localId) ?: throw WriteRefused.RowMissing("project", localId)
        }
    }

    /**
     * The stored document with [changed] written over it.
     *
     * A document that cannot be read is replaced rather than repaired: it is a
     * copy of something the server holds, and the next pull brings a good one.
     *
     * @param dropNulls whether a null in [changed] removes the key — which is what
     *   it means for the interface state, and only for it.
     */
    private fun merge(
        stored: String?,
        changed: JsonObject,
        dropNulls: Boolean = false,
    ): String {
        val base =
            stored
                ?.let { runCatching { TurboistJson.parseToJsonElement(it).jsonObject }.getOrNull() }
                ?: JsonObject(emptyMap())
        val out = base.toMutableMap()
        for ((key, value) in changed) {
            if (dropNulls && value is JsonNull) {
                out.remove(key)
            } else {
                out[key] = value
            }
        }
        return JsonObject(out).toString()
    }

    private companion object {
        /**
         * The local id an op carries when it changes a document rather than a row.
         * No row can stand for a preference document, and claiming one would make
         * exactly one unrelated row look protected from an incoming change.
         */
        const val NO_TARGET_ROW: Long = 0L
    }
}

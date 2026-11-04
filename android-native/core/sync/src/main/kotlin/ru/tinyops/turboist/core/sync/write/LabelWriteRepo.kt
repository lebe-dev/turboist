package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.dto.CreateLabelRequest
import ru.tinyops.turboist.core.network.dto.PatchLabelRequest

/** An edit to a label. Absent means untouched, exactly as it does for a task. */
data class LabelPatch(
    val name: String? = null,
    val color: String? = null,
    val description: String? = null,
    val isFavourite: Boolean? = null,
    val isPrivate: Boolean? = null,
)

/**
 * Writes to the labels.
 *
 * A label created here can be attached to tasks straight away, because the
 * replica references it by the device's own id. What it cannot do yet is travel
 * *by name* into a request the server has not heard the name from: the task
 * endpoints resolve labels by name and refuse an unknown one, which is why the
 * queue's order matters — the label's own creation is sent before anything that
 * mentions it.
 */
class LabelWriteRepo(
    private val db: TurboistDatabase,
    private val writer: OutboxWriter,
) {
    suspend fun create(
        name: String,
        color: String = "",
        isFavourite: Boolean = false,
        description: String = "",
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val localId =
                db.labels().insert(
                    LabelRow(
                        name = name,
                        color = color,
                        description = description,
                        isFavourite = isFavourite,
                        createdAt = at,
                        updatedAt = at,
                    ),
                )
            val opId =
                writer.enqueue(
                    CreateLabelOp(
                        labelLocalId = localId,
                        body =
                            CreateLabelRequest(
                                name = name,
                                color = color.ifEmpty { null },
                                description = description.ifEmpty { null },
                                isFavourite = isFavourite.takeIf { it },
                            ),
                    ),
                    ReplicaEntityKind.LABEL,
                    localId,
                )
            QueuedWrite(opId, localId)
        }

    suspend fun patch(
        labelLocalId: Long,
        edit: LabelPatch,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireLabel(labelLocalId)
            db.labels().update(
                row.copy(
                    name = edit.name ?: row.name,
                    color = edit.color ?: row.color,
                    description = edit.description ?: row.description,
                    isFavourite = edit.isFavourite ?: row.isFavourite,
                    isPrivate = edit.isPrivate ?: row.isPrivate,
                    updatedAt = at,
                ),
            )
            val opId =
                writer.enqueue(
                    PatchLabelOp(
                        labelLocalId = labelLocalId,
                        body =
                            PatchLabelRequest(
                                name = edit.name,
                                color = edit.color,
                                description = edit.description,
                                isFavourite = edit.isFavourite,
                                isPrivate = edit.isPrivate,
                            ),
                    ),
                    ReplicaEntityKind.LABEL,
                    labelLocalId,
                )
            QueuedWrite(opId, labelLocalId)
        }

    /** Removes a label, and with it every tagging of it. The taggings are edges, not history. */
    suspend fun delete(labelLocalId: Long): QueuedWrite =
        writer.transaction {
            val row = requireLabel(labelLocalId)
            val opId =
                writer.enqueue(DeleteLabelOp(labelLocalId, row.serverId), ReplicaEntityKind.LABEL, labelLocalId)
            db.labels().delete(row)
            QueuedWrite(opId, labelLocalId)
        }

    private suspend fun requireLabel(labelLocalId: Long): LabelRow =
        db.labels().byLocalId(labelLocalId) ?: throw WriteRefused.RowMissing("label", labelLocalId)
}

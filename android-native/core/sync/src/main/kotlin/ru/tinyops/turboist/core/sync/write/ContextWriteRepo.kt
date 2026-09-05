package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.dto.CreateContextRequest
import ru.tinyops.turboist.core.network.dto.PatchContextRequest

/**
 * Writes to the contexts — the grouping that sits above projects.
 *
 * Deleting one takes its projects and their tasks with it, here as on the server.
 * That is worth stating because it is the largest thing a single write in this
 * app can destroy, and because there is no tombstone anywhere to undo it from.
 */
class ContextWriteRepo(
    private val db: TurboistDatabase,
    private val writer: OutboxWriter,
) {
    suspend fun create(
        name: String,
        color: String = "",
        isFavourite: Boolean = false,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val localId =
                db.contexts().insert(
                    ContextRow(
                        name = name,
                        color = color,
                        isFavourite = isFavourite,
                        createdAt = at,
                        updatedAt = at,
                    ),
                )
            val opId =
                writer.enqueue(
                    CreateContextOp(
                        contextLocalId = localId,
                        body =
                            CreateContextRequest(
                                name = name,
                                color = color.ifEmpty { null },
                                isFavourite = isFavourite.takeIf { it },
                            ),
                    ),
                    ReplicaEntityKind.CONTEXT,
                    localId,
                )
            QueuedWrite(opId, localId)
        }

    suspend fun patch(
        contextLocalId: Long,
        name: String? = null,
        color: String? = null,
        isFavourite: Boolean? = null,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireContext(contextLocalId)
            db.contexts().update(
                row.copy(
                    name = name ?: row.name,
                    color = color ?: row.color,
                    isFavourite = isFavourite ?: row.isFavourite,
                    updatedAt = at,
                ),
            )
            val opId =
                writer.enqueue(
                    PatchContextOp(
                        contextLocalId = contextLocalId,
                        body = PatchContextRequest(name = name, color = color, isFavourite = isFavourite),
                    ),
                    ReplicaEntityKind.CONTEXT,
                    contextLocalId,
                )
            QueuedWrite(opId, contextLocalId)
        }

    suspend fun delete(contextLocalId: Long): QueuedWrite =
        writer.transaction {
            val row = requireContext(contextLocalId)
            val opId =
                writer.enqueue(
                    DeleteContextOp(contextLocalId, row.serverId),
                    ReplicaEntityKind.CONTEXT,
                    contextLocalId,
                )
            db.contexts().delete(row)
            QueuedWrite(opId, contextLocalId)
        }

    private suspend fun requireContext(contextLocalId: Long): ContextRow =
        db.contexts().byLocalId(contextLocalId)
            ?: throw WriteRefused.RowMissing("context", contextLocalId)
}

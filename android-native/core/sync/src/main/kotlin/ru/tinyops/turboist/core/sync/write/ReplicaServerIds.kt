package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.INBOX_ID

/**
 * Raised when an op names a row the server has never been told about.
 *
 * With the queue drained in order this cannot happen: whatever created the row
 * was queued before whatever refers to it, and the create's answer writes the
 * server id down. So this is the queue's ordering invariant failing, and it says
 * so rather than sending a request with a hole where an id should be.
 */
class UnsentReference(val ref: OpRef) :
    Exception("the ${ref.entity.stored} with local id ${ref.localId} has not reached the server yet")

/**
 * Turns the device's own ids into the server's.
 *
 * An op records what it changes by local id, because a row created without a
 * network has no other identity for hours. A request, by contrast, can only name
 * things the way the server does. This is the translation between the two, and it
 * happens at the moment of sending — the latest possible moment, which is also
 * the first one at which the answer is guaranteed to exist.
 */
class ReplicaServerIds(private val db: TurboistDatabase) {
    /** The server's id for a row, or `null` while the row exists only on this device. */
    suspend fun of(ref: OpRef): Long? =
        when (ref.entity) {
            ReplicaEntityKind.TASK -> db.tasks().byLocalId(ref.localId)?.serverId
            ReplicaEntityKind.PROJECT -> db.projects().byLocalId(ref.localId)?.serverId
            ReplicaEntityKind.SECTION -> db.sections().byLocalId(ref.localId)?.serverId
            ReplicaEntityKind.CONTEXT -> db.contexts().byLocalId(ref.localId)?.serverId
            ReplicaEntityKind.LABEL -> db.labels().byLocalId(ref.localId)?.serverId
            ReplicaEntityKind.TASK_RELATION -> db.taskRelations().byLocalId(ref.localId)?.serverId
            ReplicaEntityKind.TASK_TEMPLATE -> db.taskTemplates().byLocalId(ref.localId)?.serverId
            ReplicaEntityKind.USER_SETTINGS,
            ReplicaEntityKind.USER_STATE,
            ReplicaEntityKind.APP_SETTINGS,
            -> null
        }

    /** The same, for a reference a request cannot be built without. */
    suspend fun require(ref: OpRef): Long = of(ref) ?: throw UnsentReference(ref)

    /**
     * The full placement a destination denotes, as local ids.
     *
     * A placement on the server is a set of pointers that have to agree: a task
     * in a project is also in that project's context, and in the section's
     * project when it sits in a section. A request that names only the narrow end
     * is refused as an incomplete placement.
     *
     * Only the narrow end is a decision the user made, so only that is recorded
     * in the queued write — a write that also recorded the context would be a
     * second copy of a fact the replica already holds, and the two could
     * disagree after a catch-up moved the project. The rest is read here, at the
     * moment of sending, like every other id in the request.
     */
    suspend fun placementOf(destination: TaskDestination): DestinationPlacement =
        when (destination) {
            TaskDestination.Inbox -> DestinationPlacement(inboxId = INBOX_ID)
            is TaskDestination.InContext -> DestinationPlacement(contextLocalId = destination.contextLocalId)
            is TaskDestination.InProject -> inProject(destination.projectLocalId)
            is TaskDestination.InSection -> {
                val section =
                    db.sections().byLocalId(destination.sectionLocalId)
                        ?: throw UnsentReference(OpRef(ReplicaEntityKind.SECTION, destination.sectionLocalId))
                inProject(section.projectLocalId).copy(sectionLocalId = section.localId)
            }

            is TaskDestination.SubtaskOf -> {
                val parent =
                    db.tasks().byLocalId(destination.parentTaskLocalId)
                        ?: throw UnsentReference(OpRef(ReplicaEntityKind.TASK, destination.parentTaskLocalId))
                // A subtask joins its parent exactly where the parent is, which
                // is why the parent's own placement is what goes on the wire.
                val above =
                    parent.projectLocalId?.let { inProject(it) }
                        ?: DestinationPlacement(contextLocalId = parent.contextLocalId)
                above.copy(
                    sectionLocalId = parent.sectionLocalId,
                    parentTaskLocalId = parent.localId,
                )
            }
        }

    /**
     * A project, together with the context above it.
     *
     * A project always belongs to one, so a project row without a context is a
     * broken replica rather than a placement to send half of.
     */
    private suspend fun inProject(projectLocalId: Long): DestinationPlacement {
        val project =
            db.projects().byLocalId(projectLocalId)
                ?: throw UnsentReference(OpRef(ReplicaEntityKind.PROJECT, projectLocalId))
        return DestinationPlacement(
            contextLocalId = project.contextLocalId,
            projectLocalId = project.localId,
        )
    }

    /**
     * Records the id the server gave a row this device created.
     *
     * It is the other half of the same translation: until this runs the row has
     * no name the server would recognise, and every queued write that refers to
     * it is unsendable. The local id is left alone, so the screen that created
     * the row is still looking at it.
     *
     * The three single-row documents have no ids of their own and are silently
     * ignored, which is what lets a caller hand over whatever a write answered
     * with instead of asking first.
     */
    suspend fun assign(
        ref: OpRef,
        serverId: Long,
        at: Long,
    ) {
        when (ref.entity) {
            ReplicaEntityKind.TASK -> db.tasks().assignServerId(ref.localId, serverId, at)
            ReplicaEntityKind.PROJECT -> db.projects().assignServerId(ref.localId, serverId, at)
            ReplicaEntityKind.SECTION -> db.sections().assignServerId(ref.localId, serverId, at)
            ReplicaEntityKind.CONTEXT -> db.contexts().assignServerId(ref.localId, serverId, at)
            ReplicaEntityKind.LABEL -> db.labels().assignServerId(ref.localId, serverId, at)
            ReplicaEntityKind.TASK_RELATION -> db.taskRelations().assignServerId(ref.localId, serverId)
            ReplicaEntityKind.TASK_TEMPLATE -> db.taskTemplates().assignServerId(ref.localId, serverId, at)
            ReplicaEntityKind.USER_SETTINGS,
            ReplicaEntityKind.USER_STATE,
            ReplicaEntityKind.APP_SETTINGS,
            -> Unit
        }
    }
}

/**
 * Where a task is to sit, as the replica's own ids.
 *
 * [inboxId] is the odd one out and is already the server's: the inbox is a
 * single fixed row rather than a replicated entity, so its id is the same number
 * on every device and there is nothing to translate.
 */
data class DestinationPlacement(
    val inboxId: Long? = null,
    val contextLocalId: Long? = null,
    val projectLocalId: Long? = null,
    val sectionLocalId: Long? = null,
    val parentTaskLocalId: Long? = null,
)

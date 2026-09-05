package ru.tinyops.turboist.core.sync.pull

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.network.mapping.ReplicaIds

/**
 * Which device row each incoming server id belongs to, while one batch is being
 * written.
 *
 * Incoming records name each other by server id and the replica references rows
 * by its own, so something has to stand between the two. That something has to
 * be *stateful within a batch*: a project written a moment ago has a device id
 * that only exists because it was just inserted, and the task that names it is
 * three records later in the same transaction.
 *
 * So this remembers two things — what a lookup found, and what an insert
 * created — and both answers are equally authoritative. It also remembers a
 * lookup that found nothing, so a page naming a record this device does not have
 * asks the database once rather than once per mention.
 *
 * The mapping layer's [ReplicaIds] methods cannot suspend, which is why they read
 * only what is already remembered: the applier resolves what a record needs
 * *before* mapping it, and by then every answer is here.
 */
internal class ReplicaIdIndex(
    private val db: TurboistDatabase,
) : ReplicaIds {
    private val known: MutableMap<ReplicaEntityKind, MutableMap<Long, Long?>> = mutableMapOf()

    override fun task(serverId: Long?): Long? = remembered(ReplicaEntityKind.TASK, serverId)

    override fun project(serverId: Long?): Long? = remembered(ReplicaEntityKind.PROJECT, serverId)

    override fun section(serverId: Long?): Long? = remembered(ReplicaEntityKind.SECTION, serverId)

    override fun context(serverId: Long?): Long? = remembered(ReplicaEntityKind.CONTEXT, serverId)

    override fun label(serverId: Long?): Long? = remembered(ReplicaEntityKind.LABEL, serverId)

    override fun template(serverId: Long?): Long? = remembered(ReplicaEntityKind.TASK_TEMPLATE, serverId)

    /**
     * The device row for a server id, asking the replica the first time and
     * remembering the answer — including the answer "there is none", which is a
     * normal one during a catch-up.
     */
    suspend fun resolve(
        kind: ReplicaEntityKind,
        serverId: Long?,
    ): Long? {
        if (serverId == null) return null
        val slot = known.getOrPut(kind) { mutableMapOf() }
        if (slot.containsKey(serverId)) return slot[serverId]
        val local = lookUp(kind, serverId)
        slot[serverId] = local
        return local
    }

    /** True when the record is either already here or not referenced at all. */
    suspend fun isPresent(
        kind: ReplicaEntityKind,
        serverId: Long?,
    ): Boolean = serverId == null || resolve(kind, serverId) != null

    /** Records the row a write just created or updated, so later records in the batch see it. */
    fun remember(
        kind: ReplicaEntityKind,
        serverId: Long,
        localId: Long,
    ) {
        known.getOrPut(kind) { mutableMapOf() }[serverId] = localId
    }

    /** Forgets one record, which is what a delete inside the same batch means. */
    fun forget(
        kind: ReplicaEntityKind,
        serverId: Long,
    ) {
        known.getOrPut(kind) { mutableMapOf() }[serverId] = null
    }

    private fun remembered(
        kind: ReplicaEntityKind,
        serverId: Long?,
    ): Long? = serverId?.let { known[kind]?.get(it) }

    private suspend fun lookUp(
        kind: ReplicaEntityKind,
        serverId: Long,
    ): Long? =
        when (kind) {
            ReplicaEntityKind.TASK -> db.tasks().localIdForServerId(serverId)
            ReplicaEntityKind.PROJECT -> db.projects().localIdForServerId(serverId)
            ReplicaEntityKind.SECTION -> db.sections().localIdForServerId(serverId)
            ReplicaEntityKind.CONTEXT -> db.contexts().localIdForServerId(serverId)
            ReplicaEntityKind.LABEL -> db.labels().localIdForServerId(serverId)
            ReplicaEntityKind.TASK_RELATION -> db.taskRelations().localIdForServerId(serverId)
            ReplicaEntityKind.TASK_TEMPLATE -> db.taskTemplates().localIdForServerId(serverId)
            // The three documents are single rows addressed by nothing.
            ReplicaEntityKind.USER_SETTINGS,
            ReplicaEntityKind.USER_STATE,
            ReplicaEntityKind.APP_SETTINGS,
            -> null
        }
}

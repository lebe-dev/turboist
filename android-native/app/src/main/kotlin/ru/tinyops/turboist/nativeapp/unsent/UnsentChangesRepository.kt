package ru.tinyops.turboist.nativeapp.unsent

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.QuarantinedOpRow
import ru.tinyops.turboist.core.database.entity.blockerServerIds
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.sync.drain.UnsentChanges
import ru.tinyops.turboist.core.sync.write.OutboxOpKind
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The writes a person can see, named the way they would name them.
 *
 * The queue stores a request: an op name and a payload in the shape the server
 * reads. That is the right thing to store and the wrong thing to show, so this
 * is where the two are bridged — the row a change was aimed at is looked up in
 * the replica and its title is carried alongside, because "Complete task" is
 * only half an answer and "Complete task — Renew the domain" is the whole one.
 *
 * A title that cannot be resolved stays absent rather than being invented. The
 * commonest refusal there is comes from a row that was deleted somewhere else,
 * so a missing title is an ordinary outcome and not a fault.
 */
@Singleton
class UnsentChangesRepository
    @Inject
    constructor(
        private val database: TurboistDatabase,
        private val unsent: UnsentChanges,
    ) : UnsentChangesActions {
        /** Changes still queued, oldest first — the order they will be sent in. */
        fun observeWaiting(): Flow<List<UnsentChange>> = unsent.waiting().map { rows -> rows.map { describe(it) } }

        /** Changes the server refused, oldest first. */
        fun observeSetAside(): Flow<List<UnsentChange>> = unsent.setAside().map { rows -> rows.map { describe(it) } }

        override suspend fun discard(id: String): Boolean = unsent.discard(id)

        override suspend fun discardAll(): Int = unsent.discardAll()

        private suspend fun describe(row: OutboxOpRow): UnsentChange =
            UnsentChange(
                id = row.id,
                kind = OutboxOpKind.fromStored(row.op),
                target = titleOf(row.entity, row.entityLocalId),
                at = row.createdAt,
            )

        private suspend fun describe(row: QuarantinedOpRow): UnsentChange =
            UnsentChange(
                id = row.id,
                kind = OutboxOpKind.fromStored(row.op),
                target = titleOf(row.entity, row.entityLocalId),
                at = row.quarantinedAt,
                reason = UnsentReason.of(row.errorCode),
                blockers = blockerTitles(row),
            )

        /**
         * What the row a change was aimed at is called.
         *
         * The three single-row documents have no title because they are not
         * things a user names — the settings, the installation's rules, the
         * interface state — and neither does a relation, which is an edge rather
         * than a record. Those rows read by their kind alone, which is all there
         * is to say about them.
         */
        private suspend fun titleOf(
            entity: ReplicaEntityKind,
            localId: Long,
        ): String? =
            when (entity) {
                ReplicaEntityKind.TASK -> database.tasks().byLocalId(localId)?.title
                ReplicaEntityKind.PROJECT -> database.projects().byLocalId(localId)?.title
                ReplicaEntityKind.SECTION -> database.sections().byLocalId(localId)?.title
                ReplicaEntityKind.CONTEXT -> database.contexts().byLocalId(localId)?.name
                ReplicaEntityKind.LABEL -> database.labels().byLocalId(localId)?.name
                ReplicaEntityKind.TASK_TEMPLATE -> database.taskTemplates().byLocalId(localId)?.name
                ReplicaEntityKind.TASK_RELATION,
                ReplicaEntityKind.USER_SETTINGS,
                ReplicaEntityKind.USER_STATE,
                ReplicaEntityKind.APP_SETTINGS,
                -> null
            }?.takeIf { it.isNotBlank() }

        /**
         * The work that was in the way, by name.
         *
         * The ids the server named are its own, so they are resolved against the
         * replica's server ids. One that resolves to nothing is dropped rather
         * than shown as a number: a number is not something the user can go and
         * look at, and the sentence reads correctly with fewer names in it.
         */
        private suspend fun blockerTitles(row: QuarantinedOpRow): List<String> =
            row.blockerServerIds.mapNotNull { serverId ->
                database.tasks().byServerId(serverId)?.title?.takeIf { it.isNotBlank() }
            }
    }

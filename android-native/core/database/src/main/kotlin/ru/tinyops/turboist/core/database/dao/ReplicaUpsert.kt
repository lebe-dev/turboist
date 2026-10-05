package ru.tinyops.turboist.core.database.dao

import ru.tinyops.turboist.core.database.entity.ReplicaRow
import ru.tinyops.turboist.core.model.NO_LOCAL_ID

/**
 * The one rule every table's upsert follows, written once.
 *
 * A change arriving from the server names a record by its server id, and the
 * device may or may not already hold a row for it. Matching on the server id and
 * *updating in place* is what keeps the local id — and therefore every reference
 * to it, and every screen currently showing it — stable. Deleting and
 * re-inserting would produce the same visible fields and break all three.
 *
 * A row with no server id has nothing to match on and is inserted. Its primary
 * key is cleared first, so a caller that assembled the row from a payload cannot
 * accidentally claim an occupied key.
 *
 * Callers wrap this in a transaction; the lookup and the write have to be one
 * step, or two changes for the same record could each decide to insert.
 */
internal suspend fun <T : ReplicaRow<T>> upsertResolvingServerId(
    row: T,
    localIdForServerId: suspend (Long) -> Long?,
    insert: suspend (T) -> Long,
    update: suspend (T) -> Unit,
): Long {
    val serverId = row.serverId ?: return insert(row.withLocalId(NO_LOCAL_ID))
    val existing = localIdForServerId(serverId) ?: return insert(row.withLocalId(NO_LOCAL_ID))
    update(row.withLocalId(existing))
    return existing
}

package ru.tinyops.turboist.core.database.entity

/**
 * A row of the replica that stands for one server-side record.
 *
 * The replica carries two identities per row, and which one a column holds is
 * the single most load-bearing rule in this package:
 *
 * - `localId` is the primary key. The device assigns it the moment the row is
 *   written — including for a row created with no network in sight — and it
 *   never changes afterwards, so an open screen keeps pointing at the same row
 *   once the server answers.
 * - `serverId` is filled in once the record exists on the server. It is unique
 *   where present and is what an incoming change is matched against.
 *
 * **Every reference from one replica row to another is a local id.** Server ids
 * arrive late and would leave a freshly created child unable to name its
 * freshly created parent; local ids exist from the first insert, so the graph is
 * always complete on device. The two exceptions are named where they appear: the
 * singleton inbox, and the ids embedded in the settings payloads, which travel
 * back to the server unchanged.
 *
 * [withLocalId] is what lets the resolve-by-server-id upsert rewrite a row's
 * primary key to the one already on the device instead of inserting a duplicate.
 */
interface ReplicaRow<T : ReplicaRow<T>> {
    val localId: Long
    val serverId: Long?

    /** The same row addressed at [localId] — the update-in-place half of an upsert. */
    fun withLocalId(localId: Long): T
}

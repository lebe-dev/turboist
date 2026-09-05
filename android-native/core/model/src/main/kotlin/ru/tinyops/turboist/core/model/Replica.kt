package ru.tinyops.turboist.core.model

/**
 * Identity of a row that lives in the on-device replica.
 *
 * The server assigns ids, but a task created without a network connection cannot
 * wait for one. Every replicated row therefore has two identities:
 *
 * - [localId] is assigned by the device the moment the row is written and never
 *   changes afterwards, so an open screen does not flicker when the row is later
 *   reconciled with the server.
 * - [serverId] is filled in once the row exists on the server. It is `null` for a
 *   row created while offline and is what the pull applier matches incoming rows
 *   against.
 *
 * Every reference from one replicated row to another is a **local** id, which is
 * why those fields are named with a `LocalId` suffix. Ids that are not replica
 * rows — the singleton inbox, the label ids embedded in the settings blobs — stay
 * server ids and are documented where they appear.
 */
interface ReplicaEntity {
    val localId: Long
    val serverId: Long?
}

/**
 * The local id of a row that has not been inserted yet. Zero is what an
 * auto-generating primary key treats as "assign me one", so a freshly built
 * entity can be handed straight to the replica.
 */
const val NO_LOCAL_ID: Long = 0L

/**
 * The inbox is a single server-side row with a fixed id, not a replicated entity,
 * so a task's `inboxId` is the same number on every device and needs no local
 * resolution.
 */
const val INBOX_ID: Long = 1L

/** True once the row exists on the server and can be linked to or shared. */
val ReplicaEntity.isSynced: Boolean
    get() = serverId != null

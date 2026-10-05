package ru.tinyops.turboist.core.sync

import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * The lock the two halves of a sync cycle take turns holding.
 *
 * A cycle is always **send first, read second**. Sending before reading keeps the
 * server from answering with state that predates this device's own queued
 * writes; reading after picks up the server's authoritative result, including
 * everything a write cascaded into — parked subtasks, an advanced recurrence, an
 * automatically applied label. Run the two at the same time and the read can land
 * between a write leaving and its answer arriving, which writes stale rows over
 * exactly the ones the user just changed.
 *
 * So the rule is not "be careful about ordering" but "only one at a time", and
 * this is the one object that enforces it. Both halves take it; there is no
 * second lock anywhere, because two locks over the same invariant is the same as
 * none.
 *
 * It is fair: waiters are served in the order they arrived, so a stream of read
 * requests cannot starve a queued write out of its turn.
 */
class SyncMutex {
    private val mutex = Mutex()

    /** True while either half of a cycle is running. */
    val isHeld: Boolean
        get() = mutex.isLocked

    /**
     * Runs [block] with no other half of a cycle in progress, waiting for the
     * current one to finish if there is one.
     */
    suspend fun <T> withExclusiveAccess(block: suspend () -> T): T = mutex.withLock { block() }
}

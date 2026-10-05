package ru.tinyops.turboist.nativeapp.sync

import ru.tinyops.turboist.core.sync.trigger.SyncReason
import javax.inject.Inject
import javax.inject.Singleton
import ru.tinyops.turboist.core.sync.trigger.SyncScheduler as SyncEngine

/**
 * The one way anything on screen asks for a sync cycle to run now.
 *
 * A screen never fetches: it reads the replica and the replica is filled by the
 * sync engine. Pulling a list down is therefore not a request for that list — it
 * is the user saying "catch up", and it is answered by the same cycle every
 * other trigger runs.
 *
 * The call returns when that cycle is over, because the gesture that makes it
 * comes with a spinner and the spinner has to mean something. Everything the
 * engine expects to go wrong — an unreachable server, a refusal — is an outcome
 * rather than an exception, so a screen has nothing to catch and nothing to
 * decide: whatever the cycle wrote is already on screen, and whatever it could
 * not write the next cycle will.
 *
 * An interface so a screen can be exercised without an engine underneath it.
 */
interface SyncScheduler {
    /** Runs a sync cycle and returns once it has finished. Never throws. */
    suspend fun requestSyncNow()
}

/**
 * The screen-facing request, handed to the engine.
 *
 * Nothing here decides anything: what a cycle is, how long a burst of automatic
 * reasons is folded for, and what a failed one means all live in the engine, and
 * a manual refresh is deliberately the same cycle rather than a shortcut of its
 * own. All this adds is the reason, which the engine carries into its log so a
 * cycle can be told from the ones nobody asked for.
 */
@Singleton
class EngineSyncScheduler
    @Inject
    constructor(
        private val engine: SyncEngine,
    ) : SyncScheduler {
        override suspend fun requestSyncNow() {
            engine.requestSyncNow(SyncReason.MANUAL_REFRESH)
        }
    }

/**
 * A scheduler that accepts a request and drops it.
 *
 * For the places that render a screen with no engine behind them — a preview, a
 * test about layout rather than about syncing. A pull gesture there is a no-op
 * instead of an error, which is what a screen with no server should do.
 */
@Singleton
class InertSyncScheduler
    @Inject
    constructor() : SyncScheduler {
        override suspend fun requestSyncNow() = Unit
    }

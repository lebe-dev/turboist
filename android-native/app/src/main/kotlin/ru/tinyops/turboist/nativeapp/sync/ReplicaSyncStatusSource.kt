package ru.tinyops.turboist.nativeapp.sync

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.sync.drain.UnsentChanges
import ru.tinyops.turboist.nativeapp.di.ApplicationScope
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The sync status, assembled from what the device already records.
 *
 * Nothing here is a second source of truth. How far the replica has caught up is
 * a row of the replica, what has not been sent is the queue itself, and what the
 * server refused is the set-aside pile — all three are already written down
 * because the engine needs them, and reading them is what keeps the strip at the
 * top of the screen from being able to disagree with the queue underneath it.
 * Only whether a cycle is running has to be observed as it happens, and that is
 * what the engine is wrapped for.
 */
@Singleton
class ReplicaSyncStatusSource
    @Inject
    constructor(
        database: TurboistDatabase,
        unsent: UnsentChanges,
        activity: SyncActivity,
        @param:ApplicationScope scope: CoroutineScope,
    ) : SyncStatusSource {
        override val status: StateFlow<SyncStatus> =
            combine(
                activity.syncing,
                activity.outcome,
                unsent.waitingCount().distinctUntilChanged(),
                unsent.setAside().map { it.size }.distinctUntilChanged(),
                database.syncState().observe().map { it?.lastSyncAt }.distinctUntilChanged(),
            ) { syncing, outcome, waiting, setAside, lastSyncAt ->
                SyncStatus(
                    syncing = syncing,
                    outcome = outcome,
                    lastSyncAt = lastSyncAt,
                    waiting = waiting,
                    setAside = setAside,
                )
            }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), SyncStatus())

        private companion object {
            /**
             * How long the queries stay open after the last screen stops looking.
             * Long enough to survive a screen rotation, which would otherwise
             * tear down five database observers and open them again a moment
             * later.
             */
            const val STOP_TIMEOUT_MILLIS = 5_000L
        }
    }

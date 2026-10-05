package ru.tinyops.turboist.nativeapp.devtools

import android.content.Context
import dagger.hilt.EntryPoint
import dagger.hilt.InstallIn
import dagger.hilt.android.EntryPointAccessors
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.drain.OutboxDrainer
import ru.tinyops.turboist.core.sync.drain.UnsentChanges
import ru.tinyops.turboist.core.sync.pull.SyncPuller
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import ru.tinyops.turboist.nativeapp.auth.RefreshTokenStore
import ru.tinyops.turboist.nativeapp.auth.ServerAddressStore
import ru.tinyops.turboist.nativeapp.auth.SessionManager
import ru.tinyops.turboist.nativeapp.auth.SessionTokens

/**
 * Hands out the running process's own sync engine, session and replica.
 *
 * A test that talks to a real server has to drive the very objects the app
 * drives, not a second set built to look like them: the whole point of running
 * on a device is that the wiring is the thing under test. Rebuilding the engine
 * beside the app would prove only that the copy works, and the two would drift
 * the first time a binding changed.
 *
 * Debug builds only, and deliberately so. It is compiled from the debug source
 * set, so nothing that ships contains a way to reach into the graph from
 * outside — the doorway does not exist in a release build rather than being
 * guarded inside one.
 */
@EntryPoint
@InstallIn(SingletonComponent::class)
interface EngineAccess {
    fun database(): TurboistDatabase

    fun sessions(): SessionManager

    fun serverAddresses(): ServerAddressStore

    fun refreshTokens(): RefreshTokenStore

    fun accessTokens(): SessionTokens

    fun serverUrl(): ServerUrl

    fun taskWrites(): TaskWriteRepo

    fun drainer(): OutboxDrainer

    fun puller(): SyncPuller

    fun syncCycle(): SyncCycle

    fun unsentChanges(): UnsentChanges

    companion object {
        /** The graph belonging to this process's application object. */
        fun of(context: Context): EngineAccess =
            EntryPointAccessors.fromApplication(context.applicationContext, EngineAccess::class.java)
    }
}

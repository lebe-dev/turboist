package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.auth.SessionManager
import ru.tinyops.turboist.nativeapp.session.OfflineSessionSource
import ru.tinyops.turboist.nativeapp.session.SessionStateSource
import ru.tinyops.turboist.nativeapp.shell.DrawerCountsSource
import ru.tinyops.turboist.nativeapp.shell.EmptyDrawerCountsSource
import ru.tinyops.turboist.nativeapp.sync.ReplicaSyncStatusSource
import ru.tinyops.turboist.nativeapp.sync.SyncStatusSource
import javax.inject.Singleton

/**
 * Binds the seams the app shell depends on.
 *
 * The shell asks for interfaces, never for the classes behind them, so what
 * decides whether the user sees their tasks can be replaced without a screen
 * changing. Both session seams resolve to the same object: which screens are
 * allowed and whether the session has been proved are two readings of one
 * state, and splitting them across two implementations would let them disagree.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class ShellModule {
    @Binds
    @Singleton
    abstract fun bindSessionStateSource(source: SessionManager): SessionStateSource

    @Binds
    @Singleton
    abstract fun bindOfflineSessionSource(source: SessionManager): OfflineSessionSource

    @Binds
    @Singleton
    abstract fun bindDrawerCountsSource(source: EmptyDrawerCountsSource): DrawerCountsSource

    @Binds
    @Singleton
    abstract fun bindSyncStatusSource(source: ReplicaSyncStatusSource): SyncStatusSource
}

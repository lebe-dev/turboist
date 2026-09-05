package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.settings.AppRelease
import ru.tinyops.turboist.nativeapp.settings.DataStoreDeviceOptions
import ru.tinyops.turboist.nativeapp.settings.DeviceOptionActions
import ru.tinyops.turboist.nativeapp.settings.DeviceOptionsStore
import ru.tinyops.turboist.nativeapp.settings.PackageAppRelease
import ru.tinyops.turboist.nativeapp.settings.ServerConnection
import ru.tinyops.turboist.nativeapp.settings.SessionServerConnection
import ru.tinyops.turboist.nativeapp.settings.SettingsActions
import ru.tinyops.turboist.nativeapp.settings.StoredDeviceOptionActions
import ru.tinyops.turboist.nativeapp.settings.WriteRepoSettingsActions
import javax.inject.Singleton

/**
 * The seams the settings screen depends on.
 *
 * Four narrow ports rather than one, because what is behind them differs in
 * kind: two documents on the server, a file on this device, and the session
 * itself. A single "settings service" would let a screen that meant to change a
 * colour reach the sign-out path.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class SettingsModule {
    @Binds
    @Singleton
    abstract fun bindSettingsActions(actions: WriteRepoSettingsActions): SettingsActions

    @Binds
    @Singleton
    abstract fun bindDeviceOptions(store: DataStoreDeviceOptions): DeviceOptionsStore

    @Binds
    @Singleton
    abstract fun bindDeviceOptionActions(actions: StoredDeviceOptionActions): DeviceOptionActions

    @Binds
    @Singleton
    abstract fun bindServerConnection(connection: SessionServerConnection): ServerConnection

    @Binds
    @Singleton
    abstract fun bindAppRelease(release: PackageAppRelease): AppRelease
}

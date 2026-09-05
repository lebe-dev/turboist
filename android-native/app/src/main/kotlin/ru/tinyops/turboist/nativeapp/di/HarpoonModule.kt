package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.harpoon.DataStoreHarpoonStore
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonStore
import javax.inject.Singleton

/**
 * Where this device keeps the two things the user hops between.
 *
 * One store for the process: a DataStore file may be opened only once, and two
 * instances writing the same pair would race to decide which two things the app
 * wakes up offering.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class HarpoonModule {
    @Binds
    @Singleton
    abstract fun bindHarpoonStore(store: DataStoreHarpoonStore): HarpoonStore
}

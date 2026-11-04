package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.switcher.DataStoreRecentItems
import ru.tinyops.turboist.nativeapp.switcher.QuickSwitcherRepository
import ru.tinyops.turboist.nativeapp.switcher.RecentItems
import ru.tinyops.turboist.nativeapp.switcher.ReplicaQuickSwitcherRepository
import javax.inject.Singleton

/** The two seams the quick switcher depends on, so it can be checked without a database or a file on disk. */
@Module
@InstallIn(SingletonComponent::class)
abstract class QuickSwitcherModule {
    @Binds
    @Singleton
    abstract fun bindQuickSwitcherRepository(repository: ReplicaQuickSwitcherRepository): QuickSwitcherRepository

    @Binds
    @Singleton
    abstract fun bindRecentItems(recent: DataStoreRecentItems): RecentItems
}

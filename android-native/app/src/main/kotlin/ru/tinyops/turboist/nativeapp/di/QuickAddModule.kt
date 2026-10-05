package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.quickadd.DataStoreRecentProjects
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddActions
import ru.tinyops.turboist.nativeapp.quickadd.RecentProjects
import ru.tinyops.turboist.nativeapp.quickadd.WriteRepoQuickAddActions
import javax.inject.Singleton

/**
 * The two seams the capture surface depends on.
 *
 * It asks for interfaces so the sheet can be driven in a check without a
 * database behind it and without a file on disk — which is why its behaviour
 * lives in a plain object rather than in a view model.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class QuickAddModule {
    @Binds
    @Singleton
    abstract fun bindQuickAddActions(actions: WriteRepoQuickAddActions): QuickAddActions

    @Binds
    @Singleton
    abstract fun bindRecentProjects(recent: DataStoreRecentProjects): RecentProjects
}

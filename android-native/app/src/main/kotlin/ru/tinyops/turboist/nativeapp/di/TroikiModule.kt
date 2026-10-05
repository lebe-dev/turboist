package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.troiki.TroikiActions
import ru.tinyops.turboist.nativeapp.troiki.WriteRepoTroikiActions
import javax.inject.Singleton

/**
 * The seam the daily plan depends on.
 *
 * The screen asks for an interface so that what happens when a project is put
 * into a bucket, or a cycle is ended, can be replaced in a test without the
 * screen changing — and so a check about the plan needs no database.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class TroikiModule {
    @Binds
    @Singleton
    abstract fun bindTroikiActions(actions: WriteRepoTroikiActions): TroikiActions
}
